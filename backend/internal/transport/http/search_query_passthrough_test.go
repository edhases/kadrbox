package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

// queryRecordingProvider captures the exact string its Search receives, so a
// test can assert what was actually sent upstream rather than inferring it from
// the response.
type queryRecordingProvider struct {
	id        string
	items     []domain.MediaItem
	searchErr error

	mu   sync.Mutex
	seen []string
}

func (p *queryRecordingProvider) record(q string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, q)
}

func (p *queryRecordingProvider) lastQuery() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.seen) == 0 {
		return ""
	}
	return p.seen[len(p.seen)-1]
}

func (p *queryRecordingProvider) ID() string      { return p.id }
func (p *queryRecordingProvider) Name() string    { return p.id }
func (p *queryRecordingProvider) BaseURL() string { return "https://" + p.id + ".example.com" }
func (p *queryRecordingProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), SearchEnabledDefault: true}
}
func (p *queryRecordingProvider) Search(_ context.Context, query string) ([]domain.MediaItem, error) {
	p.record(query)
	return p.items, p.searchErr
}
func (p *queryRecordingProvider) GetPopular(context.Context, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *queryRecordingProvider) GetNew(context.Context, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *queryRecordingProvider) GetByCategory(context.Context, string, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *queryRecordingProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	return nil, nil
}
func (p *queryRecordingProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

var _ domain.Provider = (*queryRecordingProvider)(nil)

// panickingProvider is a third-party HTML scraper that panics. The registry
// recovers it into provider.ErrProviderPanic, so the handler decides the status.
type panickingProvider struct{ id string }

func (p *panickingProvider) ID() string      { return p.id }
func (p *panickingProvider) Name() string    { return p.id }
func (p *panickingProvider) BaseURL() string { return "https://" + p.id + ".example.com" }
func (p *panickingProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), SearchEnabledDefault: true}
}
func (p *panickingProvider) Search(context.Context, string) ([]domain.MediaItem, error) {
	panic("scraper walked into a nil node")
}
func (p *panickingProvider) GetPopular(context.Context, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *panickingProvider) GetNew(context.Context, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *panickingProvider) GetByCategory(context.Context, string, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *panickingProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	return nil, nil
}
func (p *panickingProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

var _ domain.Provider = (*panickingProvider)(nil)

// The single-provider path used to forward the RAW query while the unified
// path and the fan-out both forwarded plan.Canonical. So the same search hit
// different upstreams depending only on whether ?provider= was present:
// "Дюна фільм 1080p" reached the aggregator as "дюна" and a DLE scraper in
// full. Both paths must send the same normalised string.
func TestSearchQuery_NormalisedForBothUnifiedAndSingleProvider(t *testing.T) {
	direct := &queryRecordingProvider{id: "uakino"}

	// A minimal aggregator so resolveMetaSearcher("bandera") succeeds and the
	// unified path is the one under test alongside the single-provider one.
	aggregator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sources":
			_ = json.NewEncoder(w).Encode(provider.BanderaSourcesResponse{OK: true})
		default:
			_ = json.NewEncoder(w).Encode(provider.BanderaSearchResponse{OK: true})
		}
	}))
	defer aggregator.Close()

	reg := provider.NewRegistry()
	reg.Register(provider.NewBanderaProviderWithConfig(aggregator.URL, "", aggregator.Client()))
	reg.Register(direct)
	h := transporthttp.NewContentHandler(reg, nil)

	// A query as a user actually types it in a search box: title plus a quality
	// tag plus a year. The plan strips the quality token as noise and lifts the
	// year out into its own field, leaving "дюна" as the canonical title to send
	// upstream. The raw string is what the single-provider path used to forward.
	raw := "Дюна 1080p 2021"

	rr := httptest.NewRecorder()
	h.Search(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/search?provider=uakino&q="+urlEncode(raw), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("single-provider search: status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}

	if got := direct.lastQuery(); got != "дюна" {
		t.Errorf("single-provider path sent %q upstream, want %q", got, "дюна")
	}

	// Now the unified path, with the same raw query and no ?provider=.
	rr2 := httptest.NewRecorder()
	h.Search(rr2, httptest.NewRequest(http.MethodGet, "/api/v1/content/search?q="+urlEncode(raw), nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("unified search: status = %d, want 200 (%s)", rr2.Code, rr2.Body.String())
	}

	if got := direct.lastQuery(); got != "дюна" {
		t.Errorf("unified path sent %q upstream, want %q (the two paths must agree)", got, "дюна")
	}
}

// BuildQueryPlan can strip a query down to nothing (q="??" becomes all
// separators, which become spaces, which are then trimmed). Dispatching that
// empty title upstream asks every scraper for its entire catalogue.
func TestSearchQuery_EmptyCanonicalIsNotDispatchedUpstream(t *testing.T) {
	direct := &queryRecordingProvider{id: "uakino"}

	reg := provider.NewRegistry()
	reg.Register(direct)
	h := transporthttp.NewContentHandler(reg, nil)

	rr := httptest.NewRecorder()
	h.Search(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/search?provider=uakino&q="+urlEncode("??"), nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}
	if direct.lastQuery() != "" {
		t.Errorf("provider was called with %q; a query that normalises to empty must not reach upstream", direct.lastQuery())
	}

	var env struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not the search envelope: %v (%s)", err, rr.Body.String())
	}
	if env.Data.Items == nil {
		t.Error("items must serialise as [] rather than null")
	}
	if len(env.Data.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(env.Data.Items))
	}
}

// A recovered provider panic is a server fault. Search had its own error switch
// without a case for the sentinel, so it fell through to the generic upstream
// branch and answered 503 + Retry-After: 5 — telling the client to retry a bug
// that fails identically every time. The other five endpoints already answered
// 500 via writeProviderError.
func TestSearchProviderPanic_IsServerErrorNotRetryableUpstreamFault(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&panickingProvider{id: "uakino"})
	h := transporthttp.NewContentHandler(reg, nil)

	rr := httptest.NewRecorder()
	h.Search(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/search?provider=uakino&q=%D0%B4%D1%8E%D0%BD%D0%B0", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500; a recovered provider panic is a server fault, not a retryable upstream fault (%s)",
			rr.Code, rr.Body.String())
	}
	if retry := rr.Header().Get("Retry-After"); retry != "" {
		t.Errorf("Retry-After = %q, want no Retry-After on a 500", retry)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want a JSON body", ct)
	}
}

func urlEncode(s string) string {
	return strings.NewReplacer(" ", "%20", "?", "%3F").Replace(s)
}
