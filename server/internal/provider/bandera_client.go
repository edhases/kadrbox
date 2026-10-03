package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// BanderaDefaultBaseURL — офіційний API endpoint для Bandera Online
const BanderaDefaultBaseURL = "https://bbe.lme.isroot.in/api/v2"

// BanderaDefaultSources — stale fallback на випадок, коли /sources недоступний (перевірено 2026-09-30).
// В основному потоці джерела завантажуються динамічно через GET /sources.
const BanderaDefaultSources = "uaflix,makhno,filmix,bambooua,animeon,mikai,starlight,franko"

// sourcesFetchBudget bounds one coalesced GET /sources flight.
//
// The flight runs on a context detached from its callers (see
// sourcesFetchGroup usage), so it needs its own deadline: the shared key must
// not stay occupied forever because a scraper hung. It sits slightly above the
// default 20s http.Client timeout so the client timeout, not this budget, is
// what normally fires.
const sourcesFetchBudget = 25 * time.Second

// BanderaClient інкапсулює мережеву взаємодію з API bbe.lme.isroot.in
type BanderaClient struct {
	baseURL    string
	httpClient *http.Client

	// sourcesMu guards the four cached fields below. sourceMetaMap and
	// enabledSearchStr are REPLACED, never mutated in place: a published value
	// is immutable, so readers can use maps.Clone under RLock without racing a
	// writer. Every read of them goes through cachedSources/staleSources.
	sourcesMu        sync.RWMutex
	sourcesTTL       time.Duration
	lastSourcesFetch time.Time
	sourceMetaMap    map[string]SourceMeta
	enabledSearchStr string

	// sourcesFetchGroup collapses concurrent cache misses into ONE upstream
	// fetch. Without it, N simultaneous searches after a TTL expiry would each
	// issue their own GET /sources.
	sourcesFetchGroup singleflight.Group
}

func NewBanderaClient(baseURL string, client *http.Client) *BanderaClient {
	if baseURL == "" {
		baseURL = BanderaDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if client == nil {
		client = &http.Client{
			Timeout: 20 * time.Second,
		}
	}
	return &BanderaClient{
		baseURL:       baseURL,
		httpClient:    client,
		sourcesTTL:    10 * time.Minute,
		sourceMetaMap: make(map[string]SourceMeta),
	}
}

// SetSourcesTTL встановлює TTL кешу для джерел
func (c *BanderaClient) SetSourcesTTL(d time.Duration) {
	c.sourcesMu.Lock()
	defer c.sourcesMu.Unlock()
	c.sourcesTTL = d
}

// decodeLimitedJSON decodes a Bandera JSON body under the same ceiling as every
// other upstream read (client.go readLimitedBody → MaxUpstreamBodyBytes).
//
// The decoder timeout bounds DURATION, not BYTES: a compromised or
// decompression-bombing endpoint can push tens of MiB in a second. Reading
// through readLimitedBody also rejects an oversized body EXPLICITLY, instead of
// silently truncating it into a confusing "unexpected EOF".
func decodeLimitedJSON(r io.Reader, dst any) error {
	body, err := readLimitedBody(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}

// sourcesFetchKey is the singleflight key for GET /sources.
const sourcesFetchKey = "bandera/sources"

// sourcesSnapshot couples the source metadata with the enabled-search string
// derived from the SAME upstream response, so a caller can never pair metadata
// from response N with a source list from response N+1.
type sourcesSnapshot struct {
	meta    map[string]SourceMeta
	enabled string
}

// cachedSources returns the cached snapshot when it is still fresh.
//
// The map is cloned because it escapes to callers; the published
// sourceMetaMap is never mutated in place, so cloning under RLock is safe.
func (c *BanderaClient) cachedSources() (map[string]SourceMeta, string, bool) {
	c.sourcesMu.RLock()
	defer c.sourcesMu.RUnlock()
	if len(c.sourceMetaMap) == 0 || time.Since(c.lastSourcesFetch) >= c.sourcesTTL {
		return nil, "", false
	}
	return maps.Clone(c.sourceMetaMap), c.enabledSearchStr, true
}

// staleSources returns the cached snapshot regardless of age, for the
// "upstream is down but we have something to serve" path.
func (c *BanderaClient) staleSources() (map[string]SourceMeta, string, bool) {
	c.sourcesMu.RLock()
	defer c.sourcesMu.RUnlock()
	if len(c.sourceMetaMap) == 0 {
		return nil, "", false
	}
	return maps.Clone(c.sourceMetaMap), c.enabledSearchStr, true
}

// publishSources swaps the cached snapshot. It is the only writer.
func (c *BanderaClient) publishSources(meta map[string]SourceMeta, enabled string) {
	c.sourcesMu.Lock()
	defer c.sourcesMu.Unlock()
	c.sourceMetaMap = meta
	c.enabledSearchStr = enabled
	c.lastSourcesFetch = time.Now()
}

// GetSources виконує GET /sources із кешуванням на 10 хвилин.
//
// No lock is held across the HTTP call: the fetch runs on the singleflight
// goroutine and the write lock is taken only to publish the result. Concurrent
// cache misses share one upstream request instead of serialising behind it.
func (c *BanderaClient) GetSources(ctx context.Context) (map[string]SourceMeta, error) {
	meta, _, err := c.loadSources(ctx)
	return meta, err
}

// GetSearchSourcesStr повертає кому-розділений список увімкнених джерел для пошуку.
//
// The string comes from loadSources rather than from a field read: enabledSearchStr
// is written by the fetch goroutine and was previously read here with no lock,
// which a two-word string header can tear into a truncated source list.
func (c *BanderaClient) GetSearchSourcesStr(ctx context.Context) string {
	if _, enabled, err := c.loadSources(ctx); err == nil && enabled != "" {
		return enabled
	}
	return BanderaDefaultSources
}

// loadSources returns a fresh snapshot, fetching it if the cache is cold.
func (c *BanderaClient) loadSources(ctx context.Context) (map[string]SourceMeta, string, error) {
	if meta, enabled, ok := c.cachedSources(); ok {
		return meta, enabled, nil
	}

	// DoChan, not Do: each waiter applies its own cancellation to its own wait and
	// gives up without tearing down the shared work for the others.
	ch := c.sourcesFetchGroup.DoChan(sourcesFetchKey, func() (interface{}, error) {
		// Re-check: the caller may have queued behind a flight that just
		// published, in which case its result is already obsolete.
		if meta, enabled, ok := c.cachedSources(); ok {
			return sourcesSnapshot{meta: meta, enabled: enabled}, nil
		}
		// The shared context is created INSIDE the flight, so the flight is not
		// cancelled by whichever caller happened to arrive first; the budget
		// bounds it so a hung upstream cannot hold the key forever.
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), sourcesFetchBudget)
		defer cancel()
		return c.fetchSources(shared)
	})

	select {
	case <-ctx.Done():
		// The caller gave up waiting, but the flight keeps running for the other
		// waiters. Serve it whatever is already cached, which is what the old
		// "request failed but we have a cache" path did.
		if meta, enabled, ok := c.staleSources(); ok {
			return meta, enabled, nil
		}
		return nil, "", ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, "", res.Err
		}
		snap, ok := res.Val.(sourcesSnapshot)
		if !ok {
			return nil, "", fmt.Errorf("unexpected sources snapshot type %T", res.Val)
		}
		return snap.meta, snap.enabled, nil
	}
}

// fetchSources performs one GET /sources and publishes the result. It holds no
// lock: the only write happens in publishSources, after the response is parsed.
func (c *BanderaClient) fetchSources(ctx context.Context) (sourcesSnapshot, error) {
	reqURL := fmt.Sprintf("%s/sources", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return sourcesSnapshot{}, fmt.Errorf("create sources request: %w", err)
	}
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Якщо мережевий запит впав, але є кеш — повертаємо старий кеш
		if meta, enabled, ok := c.staleSources(); ok {
			return sourcesSnapshot{meta: meta, enabled: enabled}, nil
		}
		return sourcesSnapshot{}, fmt.Errorf("fetch sources: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if meta, enabled, ok := c.staleSources(); ok {
			return sourcesSnapshot{meta: meta, enabled: enabled}, nil
		}
		return sourcesSnapshot{}, fmt.Errorf("sources returned status %d: %s", resp.StatusCode, string(body))
	}

	var sourcesResp BanderaSourcesResponse
	if err := decodeLimitedJSON(resp.Body, &sourcesResp); err != nil {
		return sourcesSnapshot{}, fmt.Errorf("decode sources response: %w", err)
	}

	newMetaMap := make(map[string]SourceMeta, len(sourcesResp.Sources))
	var enabledList []string

	for _, s := range sourcesResp.Sources {
		meta := SourceMeta{
			Key:         s.Key,
			Name:        s.Name,
			Enabled:     s.Enabled,
			ContentKeys: s.Inputs.Content,
			StreamKeys:  s.Inputs.Stream,
			CanSearch:   s.Capabilities.Search,
			CanContent:  s.Capabilities.Content,
			CanStream:   s.Capabilities.Stream,
		}
		newMetaMap[s.Key] = meta
		if s.Enabled && s.Capabilities.Search {
			enabledList = append(enabledList, s.Key)
		}
	}

	enabledStr := BanderaDefaultSources
	if len(enabledList) > 0 {
		enabledStr = strings.Join(enabledList, ",")
	}

	c.publishSources(newMetaMap, enabledStr)
	return sourcesSnapshot{meta: maps.Clone(newMetaMap), enabled: enabledStr}, nil
}

// GetSourceMeta повертає метадані для конкретного ключа джерела
func (c *BanderaClient) GetSourceMeta(ctx context.Context, sourceKey string) (SourceMeta, bool) {
	sources, err := c.GetSources(ctx)
	if err != nil {
		return SourceMeta{}, false
	}
	m, ok := sources[sourceKey]
	return m, ok
}

// SearchWithMeta виконує пошук та повертає повну відповідь разом із meta.statuses
func (c *BanderaClient) SearchWithMeta(ctx context.Context, query string, year int, serial int) (*BanderaSearchResponse, error) {
	sourcesStr := c.GetSearchSourcesStr(ctx)
	reqURL := fmt.Sprintf("%s/search?sources=%s&title=%s",
		c.baseURL, url.QueryEscape(sourcesStr), url.QueryEscape(query))
	if year > 0 {
		reqURL += fmt.Sprintf("&year=%d", year)
	}
	if serial > 0 {
		reqURL += fmt.Sprintf("&serial=%d", serial)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp BanderaSearchResponse
	if err := decodeLimitedJSON(resp.Body, &searchResp); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	return &searchResp, nil
}

// Search виконує базовий пошук за запитом query
func (c *BanderaClient) Search(ctx context.Context, query string) ([]BanderaSearchItem, error) {
	resp, err := c.SearchWithMeta(ctx, query, 0, 0)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// GetContent запитує /content з обов'язковим прапорцем full: true
func (c *BanderaClient) GetContent(ctx context.Context, source string, ref json.RawMessage) (*BanderaContentResponse, error) {
	reqBody, err := json.Marshal(BanderaContentRequest{
		Source: source,
		Ref:    ref,
		Full:   true, // bo.js завжди надсилає full: true
	})
	if err != nil {
		return nil, fmt.Errorf("marshal content request: %w", err)
	}

	contentURL := fmt.Sprintf("%s/content", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create content request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("content request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("content returned status %d: %s", resp.StatusCode, string(body))
	}

	var contentResp BanderaContentResponse
	if err := decodeLimitedJSON(resp.Body, &contentResp); err != nil {
		return nil, fmt.Errorf("decode content response: %w", err)
	}

	return &contentResp, nil
}

// GetStream надсилає запит до /stream. Ця операція НІКОЛИ не мемоізується, оскільки URL джерел (як-от animeon)
// є підписаними і мають короткий час життя (?expires=...&sig=...).
func (c *BanderaClient) GetStream(ctx context.Context, source string, ref json.RawMessage) (*BanderaStreamResponse, error) {
	reqBody, err := json.Marshal(BanderaStreamRequest{
		Source: source,
		Ref:    ref,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	streamURL := fmt.Sprintf("%s/stream", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, streamURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("stream returned status %d: %s", resp.StatusCode, string(body))
	}

	var streamResp BanderaStreamResponse
	if err := decodeLimitedJSON(resp.Body, &streamResp); err != nil {
		return nil, fmt.Errorf("decode stream response: %w", err)
	}

	return &streamResp, nil
}
