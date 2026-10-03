package http_test

// Cache-behaviour tests for the content endpoints.
//
// The defects pinned here:
//   - a cache read error must never be reported as a plain miss. It used to be
//     swallowed into `return false, nil` and then `_`-discarded by the caller,
//     which turned a database blip into a full multi-provider fan-out;
//   - a corrupt cached row must surface an error rather than being served;
//   - a cache miss must be de-duplicated, so N concurrent identical cold
//     requests cost one upstream call;
//   - upstream failures must be negatively cached and answered 503, not 500;
//   - GetStreams must never be cached, because the URLs are signed and
//     short-lived.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

// --- fakes -----------------------------------------------------------------

// ccCache is a ContentCache that can be told to fail, and that records the
// traffic so a test can assert what was and was not cached.
type ccCache struct {
	mu      sync.Mutex
	entries map[string][]byte
	raw     map[string]string // key -> deliberately invalid JSON

	getErr error
	setErr error

	gets  int
	sets  int
	lastK []string
}

func newCCCache() *ccCache {
	return &ccCache{entries: map[string][]byte{}, raw: map[string]string{}}
}

func (c *ccCache) Get(_ context.Context, key string, target interface{}) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	c.lastK = append(c.lastK, key)
	if c.getErr != nil {
		return false, c.getErr
	}
	if _, ok := c.raw[key]; ok {
		// A corrupt row: present and unexpired, but not decodable into the
		// target. The repository surfaces this as an error, and so must this.
		return false, fmt.Errorf("unmarshal cache %q: invalid character", key)
	}
	blob, ok := c.entries[key]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(blob, target); err != nil {
		return false, fmt.Errorf("unmarshal cache %q: %w", key, err)
	}
	return true, nil
}

func (c *ccCache) Set(_ context.Context, key, _, _ string, data interface{}, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	blob, err := json.Marshal(data)
	if err != nil {
		return err
	}
	c.sets++
	c.entries[key] = blob
	return nil
}

func (c *ccCache) counts() (gets, sets int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gets, c.sets
}

func (c *ccCache) keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lastK...)
}

func (c *ccCache) hasPrefix(prefix string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// ccLogCapture redirects the standard logger for the duration of a test so the
// "log a warning with the key" requirement can be asserted.
func ccLogCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	flags := log.Flags()
	prevOut := log.Writer()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(flags)
	})
	return &buf
}

// ccProvider is a provider double that counts its calls and can be told to
// block, fail, or panic.
type ccProvider struct {
	id string

	items   []domain.MediaItem
	details *domain.MediaDetails
	streams *domain.ContentStreamsResponse

	err error

	popularCalls  int32
	categoryCalls int32
	detailsCalls  int32
	streamsCalls  int32

	// gate, when non-nil, blocks every call until it is closed. Used to make
	// two requests overlap on the same cache key.
	gate chan struct{}
	// onCall runs inside each call, before it returns.
	onCall func()
}

func (p *ccProvider) ID() string   { return p.id }
func (p *ccProvider) Name() string { return "cc-" + p.id }
func (p *ccProvider) BaseURL() string {
	return "https://" + p.id + ".example.com"
}
func (p *ccProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), ShowOnHome: true, SearchEnabledDefault: true}
}

func (p *ccProvider) enter() {
	if p.gate != nil {
		<-p.gate
	}
	if p.onCall != nil {
		p.onCall()
	}
}

func (p *ccProvider) Search(context.Context, string) ([]domain.MediaItem, error) {
	p.enter()
	return p.items, p.err
}

func (p *ccProvider) GetPopular(context.Context, string, int) ([]domain.MediaItem, error) {
	atomic.AddInt32(&p.popularCalls, 1)
	p.enter()
	return p.items, p.err
}

func (p *ccProvider) GetByCategory(context.Context, string, string, int) ([]domain.MediaItem, error) {
	atomic.AddInt32(&p.categoryCalls, 1)
	p.enter()
	return p.items, p.err
}

func (p *ccProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	atomic.AddInt32(&p.detailsCalls, 1)
	p.enter()
	return p.details, p.err
}

func (p *ccProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	atomic.AddInt32(&p.streamsCalls, 1)
	p.enter()
	return p.streams, p.err
}

func (p *ccProvider) popularCount() int { return int(atomic.LoadInt32(&p.popularCalls)) }
func (p *ccProvider) detailsCount() int { return int(atomic.LoadInt32(&p.detailsCalls)) }
func (p *ccProvider) streamsCount() int { return int(atomic.LoadInt32(&p.streamsCalls)) }

var _ domain.Provider = (*ccProvider)(nil)

func ccHandler(ps ...domain.Provider) (*transporthttp.ContentHandler, *provider.Registry) {
	reg := provider.NewRegistry()
	for _, p := range ps {
		reg.Register(p)
	}
	return transporthttp.NewContentHandler(reg, nil), reg
}

func ccGet(h *transporthttp.ContentHandler, target string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	switch {
	case strings.Contains(target, "/popular"):
		h.Popular(rr, httptest.NewRequest(http.MethodGet, target, nil))
	case strings.Contains(target, "/category"):
		h.Category(rr, httptest.NewRequest(http.MethodGet, target, nil))
	case strings.Contains(target, "/details"):
		h.GetDetails(rr, httptest.NewRequest(http.MethodGet, target, nil))
	case strings.Contains(target, "/streams"):
		h.GetStreams(rr, httptest.NewRequest(http.MethodGet, target, nil))
	case strings.Contains(target, "/providers"):
		h.Providers(rr, httptest.NewRequest(http.MethodGet, target, nil))
	default:
		h.Search(rr, httptest.NewRequest(http.MethodGet, target, nil))
	}
	return rr
}

type ccList[T any] struct {
	Data []T `json:"data"`
	Meta *struct {
		Page  *int `json:"page"`
		Count int  `json:"count"`
	} `json:"meta"`
}

func ccJSONError(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json; body: %s", ct, rr.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not a JSON object (%v): %q", err, rr.Body.String())
	}
	msg, _ := body["error"].(string)
	if msg == "" {
		t.Fatalf(`error body has no "error" string: %q`, rr.Body.String())
	}
	return msg
}

func ccObject[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var env struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not the object envelope: %v (%s)", err, rr.Body.String())
	}
	return env.Data
}

// --- a cache read error is not a miss --------------------------------------

// A database failure during the cache read must not be silently converted into
// "just a miss". The request still succeeds — falling back to the upstream is
// the right recovery — but the failure is logged with the cache key, and it is
// distinguishable from a genuine miss by that log line.
func TestCacheReadErrorIsNotSilentlyAMiss(t *testing.T) {
	cache := newCCCache()
	cache.getErr = errors.New("conn refused: remaining connection slots reserved")

	p := &ccProvider{id: "p1", items: []domain.MediaItem{{ID: "1", ProviderID: "p1", Title: "Матриця", Type: "movie"}}}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	logs := ccLogCapture(t)
	rr := httptest.NewRecorder()
	h.Popular(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=p1", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a cache failure must fall back, not fail the request): %s",
			rr.Code, rr.Body.String())
	}
	items := ccObject[[]domain.MediaItem](t, rr)
	if len(items) != 1 {
		t.Fatalf("got %d items, want the upstream result", len(items))
	}
	// The upstream was consulted, so the failure really was treated as a miss
	// for control flow...
	if p.popularCount() != 1 {
		t.Errorf("upstream called %d times, want 1", p.popularCount())
	}
	// ...but it was not silent, and it names the key.
	out := logs.String()
	if !strings.Contains(out, "Cache") {
		t.Errorf("a cache read failure was not logged at all: %q", out)
	}
	if !strings.Contains(out, "catalogue:") {
		t.Errorf("the log line does not name the cache key: %q", out)
	}
	if !strings.Contains(out, "conn refused") {
		t.Errorf("the log line does not carry the underlying error: %q", out)
	}
}

// A corrupt cached row is not a miss either: it must be reported, and the
// corrupt bytes must never reach the client.
func TestCorruptCacheRowSurfacesAnErrorAndIsNotServed(t *testing.T) {
	cache := newCCCache()
	p := &ccProvider{id: "p1", items: []domain.MediaItem{{ID: "1", ProviderID: "p1", Title: "Матриця", Type: "movie"}}}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	// Warm the cache so the key is known.
	warm := ccGet(h, "/api/v1/content/popular?provider=p1")
	if warm.Code != http.StatusOK {
		t.Fatalf("warm-up status = %d", warm.Code)
	}
	var key string
	for _, k := range cache.keys() {
		if strings.HasPrefix(k, "catalogue:") {
			key = k
		}
	}
	if key == "" {
		t.Fatalf("no catalogue cache key was used; keys = %v", cache.keys())
	}

	// Now poison that key with a row that is present but undecodable.
	cache.mu.Lock()
	cache.raw[key] = "{not json at all"
	cache.mu.Unlock()

	logs := ccLogCapture(t)
	rr := ccGet(h, "/api/v1/content/popular?provider=p1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "not json at all") {
		t.Fatalf("the corrupt row reached the client: %s", rr.Body.String())
	}
	if p.popularCount() != 2 {
		t.Errorf("upstream called %d times, want 2 (a corrupt row must not be served)", p.popularCount())
	}
	out := logs.String()
	if !strings.Contains(out, key) || !strings.Contains(out, "unmarshal") {
		t.Errorf("the corrupt row was not reported with its key: %q", out)
	}
}

// --- caching actually happens ---------------------------------------------

func TestPopularIsCachedAndNamespaced(t *testing.T) {
	cache := newCCCache()
	p := &ccProvider{id: "uakino", items: []domain.MediaItem{{ID: "1", ProviderID: "uakino", Title: "Дюна", Type: "movie"}}}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	first := ccGet(h, "/api/v1/content/popular?provider=uakino&type=movie&page=1")
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d", first.Code)
	}
	if first.Header().Get("X-Cache") == "HIT" {
		t.Error("the first request cannot be a cache hit")
	}

	second := ccGet(h, "/api/v1/content/popular?provider=uakino&type=movie&page=1")
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d", second.Code)
	}
	if got := second.Header().Get("X-Cache"); got != "HIT" {
		t.Errorf("X-Cache = %q, want HIT: the catalogue is not being cached", got)
	}
	if p.popularCount() != 1 {
		t.Errorf("upstream called %d times for two identical requests, want 1", p.popularCount())
	}

	// A different page must not collide with the first.
	ccGet(h, "/api/v1/content/popular?provider=uakino&type=movie&page=2")
	if p.popularCount() != 2 {
		t.Errorf("a different page reused the first page's cache entry")
	}
	if !cache.hasPrefix("catalogue:uakino:") {
		t.Errorf("cache keys are not namespaced per provider: %v", cache.keys())
	}
}

func TestGetDetailsIsCached(t *testing.T) {
	cache := newCCCache()
	p := &ccProvider{id: "p1", details: &domain.MediaDetails{
		MediaItem:   domain.MediaItem{ID: "7", ProviderID: "p1", Title: "Дюна", Type: "movie", URL: "https://93.184.216.34/7"},
		Description: "Стефенівський пісок у пустелі",
	}}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	target := "/api/v1/content/details?provider=p1&url=https://93.184.216.34/7"
	first := ccGet(h, target)
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", first.Code, first.Body.String())
	}
	details := ccObject[domain.MediaDetails](t, first)
	if details.Description == "" {
		t.Error("details payload is empty")
	}

	second := ccGet(h, target)
	if second.Header().Get("X-Cache") != "HIT" {
		t.Errorf("X-Cache = %q, want HIT: GetDetails is not being cached", second.Header().Get("X-Cache"))
	}
	if p.detailsCount() != 1 {
		t.Errorf("upstream called %d times for two identical requests, want 1", p.detailsCount())
	}
	if !cache.hasPrefix("details:p1:") {
		t.Errorf("details cache keys are not namespaced per provider: %v", cache.keys())
	}
}

// GetStreams must never be cached: the upstream URL is signed and expires.
// internal/provider/bandera_client.go documents the ?expires=…&sig=… signature.
func TestGetStreamsIsNeverCached(t *testing.T) {
	cache := newCCCache()
	p := &ccProvider{id: "p1", streams: &domain.ContentStreamsResponse{
		ProviderID: "p1",
		Streams: []domain.StreamSource{{
			Quality: "1080p",
			URL:     "https://cdn.example/stream.m3u8?expires=1700000000&sig=deadbeef",
		}},
	}}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	target := "/api/v1/content/streams?provider=p1&url=https://93.184.216.34/7"
	for i := 0; i < 3; i++ {
		rr := ccGet(h, target)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d", i, rr.Code)
		}
		if got := rr.Header().Get("X-Cache"); got != "" {
			t.Errorf("request %d: X-Cache = %q; GetStreams must not be cached", i, got)
		}
	}
	if p.streamsCount() != 3 {
		t.Errorf("upstream called %d times for 3 requests, want 3: a signed URL was cached", p.streamsCount())
	}
	gets, sets := cache.counts()
	if gets != 0 || sets != 0 {
		t.Errorf("the cache was touched by GetStreams: %d gets, %d sets", gets, sets)
	}
	if strings.Contains(strings.Join(cache.keys(), ","), "streams") {
		t.Errorf("a streams cache key was written: %v", cache.keys())
	}
}

// --- stampede protection ---------------------------------------------------

// Two concurrent identical cold requests must produce one upstream call and one
// cache write. Without singleflight each would fan out to every provider.
func TestCacheMissIsDeduplicated(t *testing.T) {
	cache := newCCCache()
	gate := make(chan struct{})
	p := &ccProvider{
		id:   "p1",
		gate: gate,
		items: []domain.MediaItem{
			{ID: "1", ProviderID: "p1", Title: "Дюна", Type: "movie"},
			{ID: "2", ProviderID: "p1", Title: "Матриця", Type: "movie"},
		},
	}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	const callers = 6
	var wg sync.WaitGroup
	codes := make([]int, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rr := httptest.NewRecorder()
			h.Popular(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=p1", nil))
			codes[idx] = rr.Code
		}(i)
	}

	// Give every goroutine time to reach the cache read, then release the one
	// that is holding the singleflight key.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	for i, c := range codes {
		if c != http.StatusOK {
			t.Errorf("caller %d: status = %d, want 200", i, c)
		}
	}
	if p.popularCount() != 1 {
		t.Errorf("upstream called %d times for %d concurrent identical requests, want 1",
			p.popularCount(), callers)
	}
	if _, sets := cache.counts(); sets != 1 {
		t.Errorf("%d cache writes for %d concurrent identical requests, want 1", sets, callers)
	}
}

// --- negative caching and 503 ---------------------------------------------

// An upstream failure must be answered 503 with Retry-After, not 500, and must
// be negatively cached so the next request inside the window does not fan out
// again.
func TestNegativeCachingKicksInOnUpstreamFailure(t *testing.T) {
	cache := newCCCache()
	p := &ccProvider{id: "p1", err: errors.New("upstream 502 Bad Gateway")}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	first := ccGet(h, "/api/v1/content/popular?provider=p1")
	if first.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 so the client backs off instead of retrying blindly", first.Code)
	}
	if first.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After header on a 503")
	}
	if msg := ccJSONError(t, first); msg == "" {
		t.Error("the 503 body has no error message")
	}
	if strings.Contains(first.Body.String(), "upstream 502") {
		t.Errorf("the upstream error leaked to the client: %s", first.Body.String())
	}
	if p.popularCount() != 1 {
		t.Fatalf("upstream called %d times, want 1", p.popularCount())
	}

	// Second request inside the negative window: no second upstream call.
	second := ccGet(h, "/api/v1/content/popular?provider=p1")
	if second.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 from the negative cache", second.Code)
	}
	if p.popularCount() != 1 {
		t.Errorf("upstream called %d times; the negative cache did not suppress the retry storm", p.popularCount())
	}
}

// A negative-cache entry must not be served as if it were a successful empty
// result: the client has to be told 503, not handed an empty catalogue.
func TestNegativeCacheEntryIsNotServedAsAnEmptyResult(t *testing.T) {
	cache := newCCCache()
	p := &ccProvider{id: "p1", err: errors.New("down")}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	ccGet(h, "/api/v1/content/details?provider=p1&url=https://93.184.216.34/7")
	rr := ccGet(h, "/api/v1/content/details?provider=p1&url=https://93.184.216.34/7")

	if rr.Code == http.StatusOK {
		t.Fatalf("a negative cache entry was served as a 200: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"data":null`) {
		t.Errorf("a negative entry decoded into a null payload: %s", rr.Body.String())
	}
}

// GetDetails follows the same 503 + Retry-After contract as the catalogues.
func TestGetDetailsUpstreamFailureIs503(t *testing.T) {
	cache := newCCCache()
	p := &ccProvider{id: "p1", err: errors.New("boom")}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	rr := ccGet(h, "/api/v1/content/details?provider=p1&url=https://93.184.216.34/7")
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After header on a 503")
	}
	ccJSONError(t, rr)
}

// A cache write failure must not fail the request: the payload is already in
// hand, and the only cost of not storing it is a repeat fetch later.
func TestCacheWriteFailureDoesNotFailTheRequest(t *testing.T) {
	cache := newCCCache()
	cache.setErr = errors.New("deadlock detected")
	p := &ccProvider{id: "p1", items: []domain.MediaItem{{ID: "1", ProviderID: "p1", Title: "Дюна", Type: "movie"}}}
	reg := provider.NewRegistry()
	reg.Register(p)
	h := transporthttp.NewContentHandler(reg, cache)

	ccLogCapture(t)
	rr := ccGet(h, "/api/v1/content/popular?provider=p1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: a failed cache write is not a request failure", rr.Code)
	}
	if items := ccObject[[]domain.MediaItem](t, rr); len(items) != 1 {
		t.Errorf("got %d items, want 1", len(items))
	}
}

// --- nil vs [] -------------------------------------------------------------

// Commit 0bd7d00 made empty catalogues serialise as [] everywhere except the
// registry aggregates, which still returned nil and serialised as null.
func TestEmptyCatalogueSerialisesAsEmptyArray(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []domain.MediaItem
	}{
		{"nil slice", nil},
		{"empty slice", []domain.MediaItem{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Single explicit provider: the handler returns the provider's
			// slice verbatim.
			cache := newCCCache()
			p := &ccProvider{id: "p1", items: tc.items}
			reg := provider.NewRegistry()
			reg.Register(p)
			h := transporthttp.NewContentHandler(reg, cache)

			rr := ccGet(h, "/api/v1/content/popular?provider=p1")
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if strings.Contains(rr.Body.String(), `"data":null`) {
				t.Errorf("explicit provider: serialised as null: %s", rr.Body.String())
			}
			var env ccList[domain.MediaItem]
			if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
				t.Fatalf("body is not a list envelope: %v (%s)", err, rr.Body.String())
			}
			if env.Data == nil {
				t.Errorf("explicit provider: data decoded to nil: %s", rr.Body.String())
			}
		})
	}

	t.Run("aggregate across providers", func(t *testing.T) {
		cache := newCCCache()
		// Every provider returns nil, so the registry's aggregate is empty.
		reg := provider.NewRegistry()
		reg.Register(&ccProvider{id: "p1"})
		reg.Register(&ccProvider{id: "p2"})
		h := transporthttp.NewContentHandler(reg, cache)

		rr := ccGet(h, "/api/v1/content/popular")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if strings.Contains(rr.Body.String(), `"data":null`) {
			t.Errorf("aggregate: serialised as null: %s", rr.Body.String())
		}
		var env ccList[domain.MediaItem]
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("body is not a list envelope: %v (%s)", err, rr.Body.String())
		}
		if env.Data == nil {
			t.Errorf("aggregate: data decoded to nil: %s", rr.Body.String())
		}
	})
}

// The Providers catalogue is a single object, so it uses the object envelope.
func TestProvidersUsesTheObjectEnvelope(t *testing.T) {
	h, _ := ccHandler(&ccProvider{id: "p1"})
	rr := ccGet(h, "/api/v1/content/providers")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	cat := ccObject[domain.ProviderCatalog](t, rr)
	if len(cat.Providers) != 1 || cat.Providers[0].ID != "p1" {
		t.Errorf("unexpected catalogue: %+v", cat)
	}
}

// --- error envelope on the content endpoints -------------------------------

func TestContentErrorPathsAreJSON(t *testing.T) {
	t.Run("400 without q", func(t *testing.T) {
		h, _ := ccHandler()
		rr := ccGet(h, "/api/v1/content/search")
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		ccJSONError(t, rr)
	})

	t.Run("400 without provider or url", func(t *testing.T) {
		h, _ := ccHandler()
		for _, target := range []string{
			"/api/v1/content/details",
			"/api/v1/content/details?provider=p1",
			"/api/v1/content/details?url=https://93.184.216.34/1",
			"/api/v1/content/streams",
		} {
			rr := ccGet(h, target)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", target, rr.Code)
			}
			ccJSONError(t, rr)
		}
	})

	t.Run("400 on an unsafe url", func(t *testing.T) {
		h, _ := ccHandler(&ccProvider{id: "p1"})
		for _, target := range []string{
			"/api/v1/content/details?provider=p1&url=http://169.254.169.254/x",
			"/api/v1/content/streams?provider=p1&url=http://169.254.169.254/x",
		} {
			rr := ccGet(h, target)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", target, rr.Code)
			}
			ccJSONError(t, rr)
		}
	})

	t.Run("404 for an unknown provider", func(t *testing.T) {
		h, _ := ccHandler(&ccProvider{id: "p1"})
		for _, target := range []string{
			"/api/v1/content/popular?provider=ghost",
			"/api/v1/content/category?provider=ghost&category=x",
			"/api/v1/content/details?provider=ghost&url=https://93.184.216.34/1",
			"/api/v1/content/streams?provider=ghost&url=https://93.184.216.34/1",
			"/api/v1/content/search?q=test&provider=ghost",
		} {
			rr := ccGet(h, target)
			if rr.Code != http.StatusNotFound {
				t.Errorf("%s: status = %d, want 404", target, rr.Code)
			}
			if msg := ccJSONError(t, rr); msg != "unknown provider" {
				t.Errorf("%s: error = %q, want %q", target, msg, "unknown provider")
			}
		}
	})

	t.Run("403 for a disabled provider", func(t *testing.T) {
		reg := provider.NewRegistry()
		reg.Register(&ccProvider{id: "p1"})
		reg.SetEnabled("p1", false)
		h := transporthttp.NewContentHandler(reg, nil)

		for _, target := range []string{
			"/api/v1/content/popular?provider=p1",
			"/api/v1/content/category?provider=p1&category=x",
			"/api/v1/content/details?provider=p1&url=https://93.184.216.34/1",
			"/api/v1/content/streams?provider=p1&url=https://93.184.216.34/1",
		} {
			rr := ccGet(h, target)
			if rr.Code != http.StatusForbidden {
				t.Errorf("%s: status = %d, want 403", target, rr.Code)
			}
			if msg := ccJSONError(t, rr); msg != "provider disabled" {
				t.Errorf("%s: error = %q, want %q", target, msg, "provider disabled")
			}
		}
	})

	t.Run("422 for an unresolvable player", func(t *testing.T) {
		h, _ := ccHandler(&ccProvider{id: "p1", err: provider.ErrUnresolvablePlayer})
		rr := ccGet(h, "/api/v1/content/streams?provider=p1&url=https://93.184.216.34/1")
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rr.Code)
		}
		ccJSONError(t, rr)
	})

	t.Run("501 when the provider cannot do a meta search", func(t *testing.T) {
		// A provider registered under the aggregator's id but without the
		// capability. The old code downcast to a concrete type and fell through
		// to a fan-out that returned a different response shape with no error.
		reg := provider.NewRegistry()
		reg.Register(&ccProvider{id: "bandera"})
		h := transporthttp.NewContentHandler(reg, nil)

		rr := ccGet(h, "/api/v1/content/search?q=Дюна")
		if rr.Code != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501 rather than a silent change of response shape", rr.Code)
		}
		ccJSONError(t, rr)
	})

	// A missing aggregator is a degraded-server condition, not a client error:
	// the fan-out still answers, in the same response shape. The one thing it
	// must never do is change the contract.
	t.Run("falls back to the fan-out when the aggregator is absent", func(t *testing.T) {
		p := &ccProvider{id: "p1", items: []domain.MediaItem{{ID: "1", ProviderID: "p1", Title: "Дюна", Type: "movie"}}}
		reg := provider.NewRegistry()
		reg.Register(p)
		h := transporthttp.NewContentHandler(reg, nil)

		rr := ccGet(h, "/api/v1/content/search?q=Дюна")
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
		}
		var env struct {
			Data struct {
				Query string `json:"query"`
				Items []struct {
					Title string `json:"title"`
				} `json:"items"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("the fallback changed the response shape: %v (%s)", err, rr.Body.String())
		}
		if len(env.Data.Items) != 1 || env.Data.Items[0].Title != "Дюна" {
			t.Errorf("unexpected items: %+v", env.Data.Items)
		}
	})
}

// Both ?provider= and the unified path must return the same shape.
func TestSearchHasOneShapeRegardlessOfProviderParam(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&ccProvider{id: "p1", items: []domain.MediaItem{{ID: "1", ProviderID: "p1", Title: "Дюна", Type: "movie"}}})
	h := transporthttp.NewContentHandler(reg, nil)

	rr := ccGet(h, "/api/v1/content/search?q=Дюна&provider=p1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rr.Code, rr.Body.String())
	}
	var env struct {
		Data struct {
			Query    string `json:"query"`
			Segments []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Count  int    `json:"count"`
			} `json:"segments"`
			Items []struct {
				Title string `json:"title"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not the object envelope: %v (%s)", err, rr.Body.String())
	}
	if len(env.Data.Items) != 1 || env.Data.Items[0].Title != "Дюна" {
		t.Errorf("unexpected items: %+v", env.Data.Items)
	}
	if len(env.Data.Segments) != 1 || env.Data.Segments[0].ID != "p1" {
		t.Errorf("unexpected segments: %+v", env.Data.Segments)
	}
	if env.Data.Query != "Дюна" {
		t.Errorf("data.query = %q, want the raw query", env.Data.Query)
	}
}
