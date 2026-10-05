package http

// The search pipeline, extracted from ContentHandler.Search.
//
// It was 148 lines with six nesting levels, and it answered the same endpoint
// with two different shapes depending on a runtime downcast to a concrete
// provider type: `search.SearchResponse` when the assertion held and
// `[]domain.MediaItem` when it did not, with no error either way. Here the
// handler is marshal-and-write, the capability is an interface rather than a
// concrete type, and a missing capability is a typed error instead of a silent
// change of contract.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	"github.com/edhases/oxide-server/internal/search"
)

const (
	searchCacheTTL       = 15 * time.Minute
	searchEmptyTTL       = 60 * time.Second
	detailsCacheTTL      = 10 * time.Minute
	catalogueCacheTTL    = 10 * time.Minute
	negativeSearchTTL    = 10 * time.Second
	negativeDetailsTTL   = 10 * time.Second
	negativeCatalogueTTL = 10 * time.Second
)

// metaSearcher is the capability ContentHandler needs from a provider to answer
// a unified search. It is deliberately not `*provider.BanderaProvider`: a
// downcast to a concrete type either succeeds or silently falls through, and a
// silent fall-through here changed the response shape.
type metaSearcher interface {
	ID() string
	SearchWithMeta(ctx context.Context, query string, year, serial int) (*provider.BanderaSearchResponse, error)
}

// errMetaSearchUnsupported is returned when the requested provider exists but
// does not implement metaSearcher. The handler maps it to 501.
var errMetaSearchUnsupported = errors.New("provider does not support unified search")

// errUpstreamDegraded is returned when a key is inside its negative-cache
// window, i.e. the upstream failed very recently. The handler maps it to 503,
// so a refresh loop during an aggregator outage costs one row read per client
// instead of one full fan-out per client.
var errUpstreamDegraded = errors.New("upstream is in a negative-cache window")

// cacheRecord is the stored form of every cached payload. Wrapping the value
// keeps a negative entry distinguishable from a real one: a bare `null` would
// unmarshal into a zero-valued payload and be served as a successful empty
// result.
type cacheRecord[T any] struct {
	Value  T      `json:"value"`
	Failed bool   `json:"failed,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// runSearch resolves the provider, checks the cache, and on a miss runs the
// upstream call under a singleflight key. It returns the response and whether
// it was served from the cache.
func (h *ContentHandler) runSearch(ctx context.Context, query, providerID string) (search.SearchResponse, bool, error) {
	start := time.Now()

	// An explicit ?provider= is a single-provider search. It bypasses the
	// aggregator and the cache: the client named a source, and the cached
	// response is an aggregation over all of them.
	if providerID != "" {
		return h.searchSingleProvider(ctx, providerID, query, start)
	}

	searcher, err := h.resolveMetaSearcher("bandera")
	if err != nil {
		if errors.Is(err, provider.ErrProviderNotFound) {
			return h.searchFanout(ctx, query, start), false, nil
		}
		return search.SearchResponse{}, false, err
	}

	plan := search.BuildQueryPlan(query)

	resp, fromCache, err := cached(ctx, h, "search:"+plan.Hash, "unified", "search", negativeSearchTTL,
		func(ctx context.Context) (search.SearchResponse, time.Duration, error) {
			built, buildErr := h.buildUnifiedSearchResponse(ctx, searcher, plan, query, start)
			if buildErr != nil {
				return search.SearchResponse{}, 0, buildErr
			}
			if len(built.Items) == 0 {
				return built, searchEmptyTTL, nil
			}
			return built, searchCacheTTL, nil
		})
	if err != nil {
		return search.SearchResponse{}, false, err
	}
	return resp, fromCache, nil
}

// searchFanout is the degraded path used when the aggregator is not
// registered. It is not a different contract: the result is assembled into the
// same search.SearchResponse the unified path returns.
func (h *ContentHandler) searchFanout(ctx context.Context, query string, start time.Time) search.SearchResponse {
	plan := search.BuildQueryPlan(query)
	items, err := h.registry.SingleFlightSearch(ctx, plan.Canonical)
	if err != nil {
		log.Printf("[Content] search fan-out failed: %v", err)
		items = nil
	}

	scored := make([]search.ScoredSearchItem, 0, len(items))
	for _, item := range items {
		scored = append(scored, search.ScoredSearchItem{
			MediaItem:  item,
			ClusterKey: search.GenerateClusterKey(item.Title, item.Year, item.Type),
			Sources:    []search.SearchSourceRef{{ProviderID: item.ProviderID, ItemID: item.ID}},
		})
	}

	status := "ok"
	if len(scored) == 0 {
		status = "empty"
	}

	return search.SearchResponse{
		Query:     query,
		Canonical: plan.Canonical,
		TookMs:    time.Since(start).Milliseconds(),
		Segments: []search.SearchSegment{{
			ID:     "fanout",
			Status: status,
			Count:  len(scored),
		}},
		Items: scored,
	}
}

// buildUnifiedSearchResponse fans out concurrently to Bandera (meta-searcher)
// and all other registered & enabled providers (e.g. UAKino, Lavakino, Eneyida),
// scoring, clustering and deduplicating all candidates into a single response.
func (h *ContentHandler) buildUnifiedSearchResponse(ctx context.Context, searcher metaSearcher, plan *search.QueryPlan, query string, start time.Time) (search.SearchResponse, error) {
	serial := 0
	if plan.TypeHint == "series" {
		serial = 1
	}

	var (
		mu               sync.Mutex
		wg               sync.WaitGroup
		allCandidates    []search.ScoredSearchItem
		totalFilteredOut int
		banderaSegment   *search.SearchSegment
		otherSegments    []search.SearchSegment
	)

	// 1. Meta-searcher (Bandera), if registered and enabled
	if searcher != nil && h.registry.IsEnabled(searcher.ID()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rawResp, bErr := searcher.SearchWithMeta(ctx, plan.Canonical, plan.Year, serial)
			mu.Lock()
			defer mu.Unlock()
			if bErr != nil {
				log.Printf("[Search] bandera search failed: %v", bErr)
				banderaSegment = &search.SearchSegment{
					ID:     searcher.ID(),
					Status: "error",
					Count:  0,
				}
				return
			}
			candidates, filteredOut := scoreCandidates(searcher.ID(), rawResp.Items, plan)
			allCandidates = append(allCandidates, candidates...)
			totalFilteredOut += filteredOut
			segmentStatus, sourceStatuses := summariseSources(rawResp)
			banderaSegment = &search.SearchSegment{
				ID:      searcher.ID(),
				Status:  segmentStatus,
				Count:   len(rawResp.Items),
				Sources: sourceStatuses,
			}
		}()
	}

	// 2. Direct providers (Uakino, Lavakino, Eneyida, etc.)
	for _, prov := range h.registry.List() {
		if prov.ID() == "bandera" || !h.registry.IsEnabled(prov.ID()) {
			continue
		}
		wg.Add(1)
		go func(p domain.Provider) {
			defer wg.Done()
			items, pErr := h.registry.SearchProvider(ctx, p.ID(), plan.Canonical)
			if pErr != nil {
				mu.Lock()
				otherSegments = append(otherSegments, search.SearchSegment{
					ID:     p.ID(),
					Status: "error",
					Count:  0,
					Sources: map[string]search.SourceStatusInfo{
						p.ID(): {Status: "error", Count: 0},
					},
				})
				mu.Unlock()
				return
			}

			var pCandidates []search.ScoredSearchItem
			pFilteredOut := 0

			for _, it := range items {
				year := it.Year
				scoreRes := search.CalculateRelevance(plan, it.Title, year, it.Type)
				if scoreRes.Dropped {
					pFilteredOut++
					continue
				}

				poster := it.PosterURL
				if strings.Contains(poster, "uakino.best") || strings.Contains(poster, "uakino.me") {
					poster = strings.ReplaceAll(strings.ReplaceAll(poster, "uakino.best", "uakino.biz"), "uakino.me", "uakino.biz")
					it.PosterURL = poster
				}
				if !strings.HasPrefix(poster, "http://") && !strings.HasPrefix(poster, "https://") {
					it.PosterURL = ""
				}

				pCandidates = append(pCandidates, search.ScoredSearchItem{
					MediaItem: domain.MediaItem{
						ID:            it.ID,
						ProviderID:    p.ID(),
						Title:         it.Title,
						OriginalTitle: it.OriginalTitle,
						PosterURL:     it.PosterURL,
						Year:          it.Year,
						Type:          it.Type,
						Rating:        it.Rating,
						URL:           it.URL,
					},
					Score:      scoreRes.Score,
					MatchedBy:  scoreRes.MatchedBy,
					ClusterKey: search.GenerateClusterKey(it.Title, year, it.Type),
					Sources: []search.SearchSourceRef{{
						ProviderID: p.ID(),
						SourceKey:  p.ID(),
						ItemID:     it.ID,
						URL:        it.URL,
					}},
				})
			}

			status := "ok"
			if len(items) == 0 {
				status = "empty"
			}

			mu.Lock()
			allCandidates = append(allCandidates, pCandidates...)
			totalFilteredOut += pFilteredOut
			otherSegments = append(otherSegments, search.SearchSegment{
				ID:     p.ID(),
				Status: status,
				Count:  len(items),
				Sources: map[string]search.SourceStatusInfo{
					p.ID(): {Status: status, Count: len(items)},
				},
			})
			mu.Unlock()
		}(prov)
	}

	wg.Wait()

	sort.Slice(otherSegments, func(i, j int) bool {
		return otherSegments[i].ID < otherSegments[j].ID
	})

	var finalSegments []search.SearchSegment
	if banderaSegment != nil {
		finalSegments = append(finalSegments, *banderaSegment)
	}
	finalSegments = append(finalSegments, otherSegments...)

	clustered := search.ClusterAndDeduplicate(allCandidates)
	if clustered == nil {
		clustered = []search.ScoredSearchItem{}
	}

	return search.SearchResponse{
		Query:       query,
		Canonical:   plan.Canonical,
		TookMs:      time.Since(start).Milliseconds(),
		Segments:    finalSegments,
		Items:       clustered,
		FilteredOut: totalFilteredOut,
		HasMore:     false,
	}, nil
}

func (h *ContentHandler) resolveMetaSearcher(id string) (metaSearcher, error) {
	p, ok := h.registry.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w %q", provider.ErrProviderNotFound, id)
	}
	ms, ok := p.(metaSearcher)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errMetaSearchUnsupported, id)
	}
	return ms, nil
}

// searchSingleProvider serves ?provider=<id>. It returns the same
// search.SearchResponse shape as the unified path, so the endpoint has one
// contract rather than two depending on a query parameter.
func (h *ContentHandler) searchSingleProvider(ctx context.Context, providerID, query string, start time.Time) (search.SearchResponse, bool, error) {
	items, err := h.registry.SearchProvider(ctx, providerID, query)
	if err != nil {
		return search.SearchResponse{}, false, err
	}

	scored := make([]search.ScoredSearchItem, 0, len(items))
	for _, item := range items {
		scored = append(scored, search.ScoredSearchItem{
			MediaItem:  item,
			ClusterKey: search.GenerateClusterKey(item.Title, item.Year, item.Type),
			Sources: []search.SearchSourceRef{{
				ProviderID: item.ProviderID,
				ItemID:     item.ID,
			}},
		})
	}

	status := "ok"
	if len(scored) == 0 {
		status = "empty"
	}
	plan := search.BuildQueryPlan(query)

	return search.SearchResponse{
		Query:     query,
		Canonical: plan.Canonical,
		TookMs:    time.Since(start).Milliseconds(),
		Segments: []search.SearchSegment{{
			ID:     providerID,
			Status: status,
			Count:  len(scored),
			Sources: map[string]search.SourceStatusInfo{
				providerID: {Status: status, Count: len(scored)},
			},
		}},
		Items:   scored,
		HasMore: false,
	}, false, nil
}

// buildSearchResponse performs the upstream call and assembles the scored,
// clustered response. Every failure returns an error, so there is no path that
// produces a half-built response.
func buildSearchResponse(ctx context.Context, searcher metaSearcher, plan *search.QueryPlan, query string, start time.Time) (search.SearchResponse, error) {
	serial := 0
	if plan.TypeHint == "series" {
		serial = 1
	}

	rawResp, err := searcher.SearchWithMeta(ctx, plan.Canonical, plan.Year, serial)
	if err != nil {
		return search.SearchResponse{}, err
	}

	candidates, filteredOut := scoreCandidates(searcher.ID(), rawResp.Items, plan)
	clustered := search.ClusterAndDeduplicate(candidates)
	if clustered == nil {
		clustered = []search.ScoredSearchItem{}
	}

	segmentStatus, sourceStatuses := summariseSources(rawResp)

	return search.SearchResponse{
		Query:       query,
		Canonical:   plan.Canonical,
		TookMs:      time.Since(start).Milliseconds(),
		Segments:    []search.SearchSegment{{ID: searcher.ID(), Status: segmentStatus, Count: len(rawResp.Items), Sources: sourceStatuses}},
		Items:       clustered,
		FilteredOut: filteredOut,
		HasMore:     false,
	}, nil
}

func scoreCandidates(providerID string, items []provider.BanderaSearchItem, plan *search.QueryPlan) ([]search.ScoredSearchItem, int) {
	candidates := make([]search.ScoredSearchItem, 0, len(items))
	filteredOut := 0

	for _, item := range items {
		year := provider.ParseFlexibleYear(item.Year)
		mediaType := item.Type.String()
		if mediaType == "" {
			mediaType = "movie"
		}

		scoreRes := search.CalculateRelevance(plan, item.Title, year, mediaType)
		if scoreRes.Dropped {
			filteredOut++
			continue
		}

		stableID := provider.GenerateStableContentID(item.Source, item.Title, year, item.Ref)
		poster := item.Poster.String()
		if !strings.Contains(poster, ".") && !strings.HasPrefix(poster, "http") {
			poster = ""
		}
		payload := provider.BanderaItemPayload{
			ID:        stableID,
			Source:    item.Source,
			Ref:       item.Ref,
			Type:      mediaType,
			Title:     item.Title,
			Poster:    poster,
			Year:      year,
			IsItemRef: true,
		}
		payloadBytes, _ := json.Marshal(payload)

		candidates = append(candidates, search.ScoredSearchItem{
			MediaItem: domain.MediaItem{
				ID:            stableID,
				ProviderID:    providerID,
				Title:         item.Title,
				OriginalTitle: item.TitleEn.String(),
				PosterURL:     poster,
				Year:          year,
				Type:          mediaType,
				URL:           string(payloadBytes),
			},
			Score:      scoreRes.Score,
			MatchedBy:  scoreRes.MatchedBy,
			ClusterKey: search.GenerateClusterKey(item.Title, year, mediaType),
			Sources: []search.SearchSourceRef{{
				ProviderID: providerID,
				SourceKey:  item.Source,
				ItemID:     stableID,
			}},
		})
	}
	return candidates, filteredOut
}

// summariseSources folds per-source statuses into one segment status: error
// when every source failed, partial when some did, empty when nothing came
// back, ok otherwise.
func summariseSources(rawResp *provider.BanderaSearchResponse) (string, map[string]search.SourceStatusInfo) {
	statuses := make(map[string]search.SourceStatusInfo)
	hasSuccess, hasError := false, false

	if rawResp.Meta != nil {
		for srcKey, st := range rawResp.Meta.Statuses {
			norm := normalizeSourceStatus(st.Status, st.Count, st.Error)
			switch norm {
			case "ok":
				hasSuccess = true
			case "error", "timeout":
				hasError = true
			}
			statuses[srcKey] = search.SourceStatusInfo{
				Status:    norm,
				Count:     st.Count,
				ElapsedMs: st.GetElapsedMs(),
			}
		}
	}

	segmentStatus := "ok"
	switch {
	case len(rawResp.Items) == 0 && hasError && !hasSuccess:
		segmentStatus = "error"
	case len(rawResp.Items) == 0:
		segmentStatus = "empty"
	case hasError:
		segmentStatus = "partial"
	}
	return segmentStatus, statuses
}

func normalizeSourceStatus(rawStatus string, count int, errStr string) string {
	s := strings.ToLower(strings.TrimSpace(rawStatus))
	switch {
	case errStr != "" || s == "error" || s == "failed":
		return "error"
	case s == "timeout":
		return "timeout"
	case s == "empty" || count == 0:
		return "empty"
	case s == "ok" || s == "success" || count > 0:
		return "ok"
	default:
		return "unknown"
	}
}
