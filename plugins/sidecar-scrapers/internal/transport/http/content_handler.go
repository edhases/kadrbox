package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	"golang.org/x/sync/singleflight"
)

type ContentHandler struct {
	registry  *provider.Registry
	cacheRepo ContentCache

	// sf collapses concurrent identical cache-miss requests into a single
	// upstream call. Without it, N clients opening the app at the same moment
	// each fan out to every provider and each write the same cache row.
	sf singleflight.Group
}

func NewContentHandler(registry *provider.Registry, cacheRepo ContentCache) *ContentHandler {
	return &ContentHandler{
		registry:  registry,
		cacheRepo: cacheRepo,
	}
}

func (h *ContentHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeAPIError(w, "search query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	providerID := r.URL.Query().Get("provider")
	resp, fromCache, err := h.runSearch(r.Context(), query, providerID)
	if err != nil {
		switch {
		case providerID == "" && errors.Is(err, errMetaSearchUnsupported):
			// The aggregator is registered but cannot answer a meta search.
			// Falling back to the fan-out here would hand the client a
			// different contract for the same endpoint, so this is reported as
			// an explicit server-side gap rather than a silent shape change.
			// (A missing aggregator does fall back: that path returns the same
			// search.SearchResponse shape, so there is no contract change.)
			log.Printf("[Content] unified search unavailable: %v", err)
			writeAPIError(w, "unified search is not available on this server", http.StatusNotImplemented)
		case errors.Is(err, errMetaSearchUnsupported):
			writeAPIError(w, "provider does not support unified search", http.StatusNotImplemented)
		case errors.Is(err, provider.ErrProviderDisabled):
			writeAPIError(w, "provider disabled", http.StatusForbidden)
		case errors.Is(err, provider.ErrProviderNotFound):
			writeAPIError(w, "unknown provider", http.StatusNotFound)
		case errors.Is(err, provider.ErrProviderPanic):
			// A recovered provider panic is a server fault, not an upstream
			// fault. Falling through to writeUpstreamError would answer 503 +
			// Retry-After and tell the client to retry something that fails the
			// same way every time. Kept in step with writeProviderError, which
			// already maps this sentinel to 500 for the other five endpoints.
			log.Printf("[Content] provider panic during search: %v", err)
			writeAPIError(w, "internal server error", http.StatusInternalServerError)
		default:
			writeUpstreamError(w, err, "failed to execute search")
		}
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	}
	writeObject(w, resp)
}

func (h *ContentHandler) GetDetails(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	itemURL := r.URL.Query().Get("url")
	if itemURL == "" {
		itemURL = r.URL.Query().Get("id")
	}

	if providerID == "" || itemURL == "" {
		writeAPIError(w, "provider and url parameters are required", http.StatusBadRequest)
		return
	}

	if err := ValidateSafeURL(itemURL); err != nil {
		writeAPIError(w, "invalid or unsafe item url", http.StatusBadRequest)
		return
	}

	details, fromCache, err := cached(r.Context(), h, detailsCacheKey(providerID, itemURL), providerID, "details", negativeDetailsTTL,
		func(ctx context.Context) (*domain.MediaDetails, time.Duration, error) {
			loaded, loadErr := h.registry.Details(ctx, providerID, itemURL)
			return loaded, detailsCacheTTL, loadErr
		})
	if err != nil {
		writeProviderError(w, err, "failed to get media details")
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	}
	writeObject(w, details)
}

// unwrapSelectionRef replaces a series-episode ref envelope with the item URL it
// carries, filling in the selection the ref encodes.
//
// It exists because the SSRF gate only knows one envelope schema: the Bandera
// `{"source","ref"}` one, which validateJSONEnvelope enforces. A selection ref is
// a different envelope (`{"item_url","season","episode","voice"}`) with no
// `source` key, so it failed validation as ErrMissingSource and every episode
// click came back 400.
//
// Unwrapping here means ValidateSafeURL checks the URL that is actually fetched.
// A Bandera envelope is not a selection ref, so it passes through untouched and
// keeps its existing validation path.
func unwrapSelectionRef(itemURL string, season, episode int, voiceID string) (string, int, int, string) {
	realURL, refSeason, refEpisode, refVoice, ok := provider.DecodeSelectionRef(itemURL)
	if !ok {
		return itemURL, season, episode, voiceID
	}
	if season <= 0 {
		season = refSeason
	}
	if episode <= 0 {
		episode = refEpisode
	}
	if voiceID == "" {
		voiceID = refVoice
	}
	return realURL, season, episode, voiceID
}

func (h *ContentHandler) GetStreams(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	itemURL := r.URL.Query().Get("url")
	if itemURL == "" {
		itemURL = r.URL.Query().Get("id")
	}

	if providerID == "" || itemURL == "" {
		writeAPIError(w, "provider and url parameters are required", http.StatusBadRequest)
		return
	}

	season, _ := strconv.Atoi(r.URL.Query().Get("season"))
	episode, _ := strconv.Atoi(r.URL.Query().Get("episode"))
	voiceID := r.URL.Query().Get("voice")

	// A series episode arrives as an opaque ref envelope, not a URL. Validating
	// the envelope as if it were a URL rejected every episode click with a 400,
	// so unwrap first and validate the real target it carries — otherwise the
	// SSRF gate would be checking a string nobody fetches.
	itemURL, season, episode, voiceID = unwrapSelectionRef(itemURL, season, episode, voiceID)
	if err := ValidateSafeURL(itemURL); err != nil {
		writeAPIError(w, "invalid or unsafe item url", http.StatusBadRequest)
		return
	}

	// Deliberately NOT cached. Upstream stream URLs are signed and short-lived:
	// internal/provider/bandera_client.go:263-264 documents that a resolved
	// /stream URL carries an expiry and a signature ("?expires=...&sig=...").
	// Caching the response would hand the client a URL that has already
	// expired, which surfaces as a playback failure, not as a cache error.
	resp, err := h.registry.Streams(r.Context(), providerID, itemURL, season, episode, voiceID)
	if err != nil {
		if errors.Is(err, provider.ErrUnresolvablePlayer) {
			writeAPIError(w, "player page exposes no playable media", http.StatusUnprocessableEntity)
			return
		}
		writeProviderError(w, err, "failed to get streams")
		return
	}

	writeObject(w, resp)
}

// Providers — GET /api/v1/content/providers
// Публічний каталог провайдерів: джерело правди для застосунків.
func (h *ContentHandler) Providers(w http.ResponseWriter, r *http.Request) {
	writeObject(w, h.registry.Catalog())
}

// Popular — GET /api/v1/content/popular
func (h *ContentHandler) Popular(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	contentType := r.URL.Query().Get("type")
	page := pageParam(r, 1)

	cacheKey := fmt.Sprintf("catalogue:%s:popular:%s:%d", providerLabel(providerID), contentType, page)
	results, fromCache, err := cached(r.Context(), h, cacheKey, providerID, "popular", negativeCatalogueTTL,
		func(ctx context.Context) ([]domain.MediaItem, time.Duration, error) {
			items, loadErr := h.registry.Popular(ctx, providerID, contentType, page)
			return nonNilItems(items), catalogueCacheTTL, loadErr
		})
	if err != nil {
		writeProviderError(w, err, "failed to load popular content")
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	}
	writeList(w, r, results, &PageMeta{Page: intp(page), HasMore: len(results) > 0})
}

// Category — GET /api/v1/content/category
func (h *ContentHandler) Category(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	category := r.URL.Query().Get("category")
	contentType := r.URL.Query().Get("type")
	page := pageParam(r, 1)

	cacheKey := fmt.Sprintf("catalogue:%s:category:%s:%s:%d", providerLabel(providerID), category, contentType, page)
	results, fromCache, err := cached(r.Context(), h, cacheKey, providerID, "category", negativeCatalogueTTL,
		func(ctx context.Context) ([]domain.MediaItem, time.Duration, error) {
			items, loadErr := h.registry.Category(ctx, providerID, category, contentType, page)
			return nonNilItems(items), catalogueCacheTTL, loadErr
		})
	if err != nil {
		writeProviderError(w, err, "failed to load category content")
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	}
	writeList(w, r, results, &PageMeta{Page: intp(page), HasMore: len(results) > 0})
}

// cached is the single read-through cache path for every cacheable endpoint.
//
// Three defects it fixes at once:
//   - a cache read error is logged and treated as a miss, never silently
//     swallowed, and never mistaken for a hit;
//   - concurrent misses for the same key collapse into one upstream call and
//     one Set, so N clients opening the app together cost one fan-out;
//   - an upstream failure writes a short negative-cache record, so a refresh
//     loop during an outage costs one row read per client instead of one
//     full fan-out per client.
//
// load returns the value together with its TTL so a caller can shorten it for
// an empty result.
func cached[T any](
	ctx context.Context,
	h *ContentHandler,
	key, providerID, contentType string,
	negativeTTL time.Duration,
	load func(context.Context) (T, time.Duration, error),
) (T, bool, error) {
	var zero T

	if h.cacheRepo == nil {
		val, _, err := load(ctx)
		return val, false, err
	}

	var rec cacheRecord[T]
	hit, cacheErr := h.cacheRepo.Get(ctx, key, &rec)
	switch {
	case cacheErr != nil:
		// A corrupt row, a dead pool or pool exhaustion must not be
		// laundered into a miss: the miss path fans out to every provider, so
		// reporting a cache-layer outage as a miss amplifies a database blip
		// into a scraping-layer failure.
		log.Printf("[Cache] read %q failed, treating as miss: %v", key, cacheErr)
	case hit && rec.Failed:
		return zero, false, fmt.Errorf("%w: %s", errUpstreamDegraded, rec.Reason)
	case hit:
		return rec.Value, true, nil
	}

	val, err, _ := h.sf.Do(key, func() (interface{}, error) {
		// The shared context is created inside the flight, so the goroutine
		// that runs the work owns its cancellation. Creating it out here would
		// mean the first caller's `defer cancel()` tore down the shared fetch
		// for everyone else the moment that caller disconnected.
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), provider.SearchFanoutBudget)
		defer cancel()

		loaded, ttl, loadErr := load(shared)
		if loadErr != nil {
			if setErr := h.cacheRepo.Set(shared, key, providerID, contentType,
				cacheRecord[T]{Failed: true, Reason: loadErr.Error()}, negativeTTL); setErr != nil {
				log.Printf("[Cache] negative entry for %q failed: %v", key, setErr)
			}
			return nil, loadErr
		}
		if setErr := h.cacheRepo.Set(shared, key, providerID, contentType, cacheRecord[T]{Value: loaded}, ttl); setErr != nil {
			// A failed write costs a repeat fetch, not correctness.
			log.Printf("[Cache] write %q failed: %v", key, setErr)
		}
		return loaded, nil
	})
	if err != nil {
		return zero, false, err
	}
	loaded, ok := val.(T)
	if !ok {
		return zero, false, fmt.Errorf("cache key %q produced %T", key, val)
	}
	return loaded, false, nil
}

// writeProviderError maps registry-level dispatch failures onto status codes.
func writeProviderError(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, provider.ErrProviderDisabled):
		writeAPIError(w, "provider disabled", http.StatusForbidden)
	case errors.Is(err, provider.ErrProviderNotFound):
		writeAPIError(w, "unknown provider", http.StatusNotFound)
	case errors.Is(err, provider.ErrProviderPanic):
		// A recovered provider panic is a server fault, not an upstream fault:
		// 503 would tell the client to retry something that will fail the same
		// way. The 500 still carries a JSON body, which the four deleted
		// per-handler recover() blocks used to be there to guarantee.
		log.Printf("[Content] provider panic: %v", err)
		writeAPIError(w, "internal server error", http.StatusInternalServerError)
	default:
		writeUpstreamError(w, err, fallbackMsg)
	}
}

// writeUpstreamError answers a scraper failure with 503 + Retry-After rather
// than 500. A 500 tells the client the request was malformed, and the Flutter
// client retries 5xx three times with no method check; 503 plus a positive
// Retry-After gives it something to wait on. The underlying error is logged,
// never returned: upstream error strings can contain internal hostnames.
func writeUpstreamError(w http.ResponseWriter, err error, fallbackMsg string) {
	log.Printf("[Content] upstream failure (%s): %v", fallbackMsg, err)
	w.Header().Set("Retry-After", "5")
	writeAPIError(w, fallbackMsg, http.StatusServiceUnavailable)
}

func nonNilItems(items []domain.MediaItem) []domain.MediaItem {
	if items == nil {
		return []domain.MediaItem{}
	}
	return items
}

func providerLabel(providerID string) string {
	if providerID == "" {
		return "all"
	}
	return providerID
}

func pageParam(r *http.Request, def int) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return def
	}
	return page
}

func detailsCacheKey(providerID, itemURL string) string {
	sum := sha256.Sum256([]byte(itemURL))
	return "details:" + providerID + ":" + hex.EncodeToString(sum[:12])
}
