package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	transportHttp "github.com/edhases/oxide-server/internal/transport/http"
)

// dleTrapProvider stands in for the DLE scrapers (uakino, eneyida, lavakino).
// Its Search panics, and it records the call before doing so.
//
// The recording is the point. The registry recovers a provider panic into
// ErrProviderPanic and the pipeline turns that into an "error" segment, so a
// test that only registers this provider and then checks the response shape
// passes whether or not the DLE providers were ever reached. Counting the calls
// is what makes the assertion mean something.
type dleTrapProvider struct {
	id string
	t  *testing.T

	mu    sync.Mutex
	calls int
}

func (p *dleTrapProvider) searchCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *dleTrapProvider) ID() string      { return p.id }
func (p *dleTrapProvider) Name() string    { return "DLE Trap " + p.id }
func (p *dleTrapProvider) BaseURL() string { return "https://" + p.id + ".example.com" }
func (p *dleTrapProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), SearchEnabledDefault: true}
}
func (p *dleTrapProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	panic("DLE provider " + p.id + " was called during search")
}
func (p *dleTrapProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *dleTrapProvider) GetNew(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *dleTrapProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *dleTrapProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	return nil, nil
}
func (p *dleTrapProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

func TestSearchInvariants_CutoffSegmentSumAndZeroDLE(t *testing.T) {
	// 1. Створюємо mock-сервер для бекенду Bandera API
	banderaMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_ = json.NewEncoder(w).Encode(provider.BanderaSourcesResponse{
				OK: true,
				Sources: []provider.BanderaSourceEntry{
					{
						Key:     "uaflix",
						Name:    "UAFlix",
						Enabled: true,
						Capabilities: provider.BanderaSourceCapabilities{
							Search:  true,
							Content: true,
							Stream:  true,
						},
						Inputs: provider.BanderaSourceInputs{
							Search:  []string{"title"},
							Content: []string{"href"},
							Stream:  []string{"url"},
						},
					},
					{
						Key:     "mikai",
						Name:    "Mikai",
						Enabled: true,
						Capabilities: provider.BanderaSourceCapabilities{
							Search:  true,
							Content: true,
							Stream:  true,
						},
						Inputs: provider.BanderaSourceInputs{
							Search:  []string{"title"},
							Content: []string{"id"},
							Stream:  []string{"url"},
						},
					},
				},
			})
			return
		}

		if r.URL.Path == "/search" {
			// Повертаємо 4 елементи: 2 дублікати Дюни (uaflix + mikai), 1 нерелевантний тайтл, 1 чистий шум
			_ = json.NewEncoder(w).Encode(provider.BanderaSearchResponse{
				OK: true,
				Items: []provider.BanderaSearchItem{
					{
						Source: "uaflix",
						Title:  "Дюна",
						Year:   json.RawMessage(`2021`),
						Type:   provider.FlexibleString("movie"),
						Ref:    json.RawMessage(`{"href":"https://uaflix.com/dune"}`),
					},
					{
						Source: "mikai",
						Title:  "Дюна",
						Year:   json.RawMessage(`2021`),
						Type:   provider.FlexibleString("movie"),
						Ref:    json.RawMessage(`{"id":12345}`),
					},
					{
						Source: "uaflix",
						Title:  "Матриця: Воскресіння 1080p BDRip",
						Year:   json.RawMessage(`2021`),
						Type:   provider.FlexibleString("movie"),
						Ref:    json.RawMessage(`{"href":"https://uaflix.com/matrix"}`),
					},
					{
						Source: "mikai",
						Title:  "Абсолютно Інший Фільм",
						Year:   json.RawMessage(`2015`),
						Type:   provider.FlexibleString("movie"),
						Ref:    json.RawMessage(`{"id":99999}`),
					},
				},
				Meta: &provider.BanderaSearchMetaResponse{
					Statuses: map[string]provider.BanderaSourceStatus{
						"uaflix": {Status: "ok", Count: 2, ElapsedMs: 120},
						"mikai":  {Status: "ok", Count: 2, ElapsedMs: 150},
					},
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer banderaMock.Close()

	// 2. Ініціалізуємо Registry з BanderaProvider та DLE-пастками
	registry := provider.NewRegistry()
	banderaProv := provider.NewBanderaProviderWithConfig(banderaMock.URL, "", banderaMock.Client())
	registry.Register(banderaProv)

	// Реєструємо DLE-пастки: кожна панікує на Search і рахує виклики, щоб
	// інваріант 3 міряв факт звернення, а не лише відсутність падіння.
	dleTraps := []*dleTrapProvider{
		{id: "dle-trap", t: t},
		{id: "uakino", t: t},
		{id: "eneyida", t: t},
		{id: "lavakino", t: t},
	}
	for _, trap := range dleTraps {
		registry.Register(trap)
	}

	handler := transportHttp.NewContentHandler(registry, nil)

	// 3. Виконуємо пошуковий запит через HTTP хендлер
	searchURL := "/api/v1/content/search?q=" + url.QueryEscape("Дюна 2021")
	req := httptest.NewRequest(http.MethodGet, searchURL, nil)
	rr := httptest.NewRecorder()

	handler.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// The search response is wrapped in the single-object envelope:
	// {"data": {query, canonical, filtered_out, segments, items}}.
	type searchPayload struct {
		Query       string `json:"query"`
		Canonical   string `json:"canonical"`
		FilteredOut int    `json:"filtered_out"`
		Segments    []struct {
			ID     string `json:"id"`
			Count  int    `json:"count"`
			Status string `json:"status"`
		} `json:"segments"`
		Items []struct {
			ID      string  `json:"id"`
			Title   string  `json:"title"`
			Score   float64 `json:"score"`
			Sources []struct {
				SourceKey string `json:"source_key"`
			} `json:"sources"`
		} `json:"items"`
	}
	var envelope struct {
		Data searchPayload `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode search response: %v", err)
	}
	resp := envelope.Data

	// Інваріант 1: Cutoff працює (відсіяно шумні та нерелевантні елементи)
	if resp.FilteredOut <= 0 {
		t.Fatalf("INVARIANT 1 FAILED: expected filtered_out > 0 (got %d), hard cutoff is not filtering noise!", resp.FilteredOut)
	}
	if resp.FilteredOut != 2 {
		t.Fatalf("expected filtered_out == 2, got %d", resp.FilteredOut)
	}

	// Інваріант 2: Нетавтологічна перевірка кластеризації.
	// segments[0].Count містить СИРУ кількість результатів від джерела (4),
	// а len(items) — результат ПІСЛЯ cutoff та кластеризації дублів (1).
	if len(resp.Segments) == 0 {
		t.Fatalf("expected at least 1 segment in response")
	}
	rawCandidateCount := resp.Segments[0].Count
	if rawCandidateCount != 4 {
		t.Fatalf("INVARIANT 2 FAILED: expected raw candidate count 4, got %d", rawCandidateCount)
	}
	if rawCandidateCount <= len(resp.Items) {
		t.Fatalf("INVARIANT 2 FAILED: rawCandidateCount (%d) must be strictly greater than len(items) (%d) on duplicates", rawCandidateCount, len(resp.Items))
	}

	// Перевірка кластеризації: 2 дублікати Дюни з uaflix та mikai мають об'єднатися в 1 елемент з 2 sources
	if len(resp.Items) != 1 {
		t.Fatalf("expected exactly 1 clustered item for 'Дюна', got %d", len(resp.Items))
	}
	if len(resp.Items[0].Sources) != 2 {
		t.Fatalf("expected 2 aggregated sources in clustered item, got %d", len(resp.Items[0].Sources))
	}

	// Інваріант 3: DLE-пастки реально викликаються, і їхня паніка не руйнує
	// відповідь. Раніше тут стояв лише recover-тест самої пастки, який нічого не
	// доводив про конвеєр: safeSearch ковтав паніку в error-сегмент, тож тест
	// проходив однаково — викликали DLE-провайдери чи ні.
	for _, trap := range dleTraps {
		if n := trap.searchCalls(); n != 1 {
			t.Errorf("INVARIANT 3 FAILED: expected the search to fan out to DLE provider %q exactly once, got %d calls", trap.ID(), n)
		}
	}
	for _, trap := range dleTraps {
		if n := trap.searchCalls(); n != 1 {
			t.Errorf("INVARIANT 3 FAILED: expected the search to fan out to DLE provider %q exactly once, got %d calls", trap.ID(), n)
		}
	}

	// Кожен викликаний DLE-провайдер має з'явитися окремим error-сегментом: саме
	// це і є «паніка ізольована, решта пошуку жива».
	gotErrorSegments := map[string]bool{}
	for _, seg := range resp.Segments {
		if seg.Status == "error" {
			gotErrorSegments[seg.ID] = true
		}
	}
	for _, trap := range dleTraps {
		if !gotErrorSegments[trap.ID()] {
			t.Errorf("INVARIANT 3 FAILED: expected an error segment for panicking provider %q, got segments %+v", trap.ID(), resp.Segments)
		}
	}

	// І сам виклик Search у пастки справді панікує — інакше лічильник викликів
	// вимірював би не те.
	trap := &dleTrapProvider{id: "dle-trap", t: t}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected dleTrapProvider.Search to panic, but it did not")
			}
		}()
		_, _ = trap.Search(context.Background(), "test")
	}()
}

func TestSearchSourceStatuses_NormalizationAndNonEmptyKeys(t *testing.T) {
	// Перевіряємо, що у відповіді пошуку всі SourceKey непорожні, а статуси джерел нормалізуються в ok/empty/error/timeout
	banderaMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_ = json.NewEncoder(w).Encode(provider.BanderaSourcesResponse{OK: true})
			return
		}
		if r.URL.Path == "/search" {
			_ = json.NewEncoder(w).Encode(provider.BanderaSearchResponse{
				OK: true,
				Items: []provider.BanderaSearchItem{
					{
						Source: "uaflix",
						Title:  "Бетмен",
						Year:   json.RawMessage(`2022`),
						Type:   provider.FlexibleString("movie"),
						Ref:    json.RawMessage(`{"href":"https://uaflix.com/batman"}`),
					},
				},
				Meta: &provider.BanderaSearchMetaResponse{
					Statuses: map[string]provider.BanderaSourceStatus{
						"uaflix":   {Status: "ok", Count: 1, ElapsedMs: 120},
						"makhno":   {Status: "empty", Count: 0, ElapsedMs: 10},
						"bambooua": {Status: "timeout", ElapsedMs: 5000},
						"animeon":  {Status: "error", Error: "network down"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer banderaMock.Close()

	registry := provider.NewRegistry()
	registry.Register(provider.NewBanderaProviderWithConfig(banderaMock.URL, "", banderaMock.Client()))
	handler := transportHttp.NewContentHandler(registry, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/search?q=Бетмен", nil)
	rr := httptest.NewRecorder()
	handler.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var envelope struct {
		Data struct {
			Segments []struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				Sources map[string]struct {
					Status string `json:"status"`
				} `json:"sources"`
			} `json:"segments"`
			Items []struct {
				Sources []struct {
					SourceKey string `json:"source_key"`
				} `json:"sources"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	resp := envelope.Data

	if len(resp.Segments) == 0 {
		t.Fatalf("missing segments")
	}
	// Оскільки є і успіх (uaflix), і помилки (animeon/bambooua), статус сегмента повинен бути "partial"
	if resp.Segments[0].Status != "partial" {
		t.Fatalf("expected segment status 'partial', got %q", resp.Segments[0].Status)
	}

	sources := resp.Segments[0].Sources
	if sources["uaflix"].Status != "ok" {
		t.Fatalf("expected uaflix status 'ok', got %q", sources["uaflix"].Status)
	}
	if sources["makhno"].Status != "empty" {
		t.Fatalf("expected makhno status 'empty', got %q", sources["makhno"].Status)
	}
	if sources["bambooua"].Status != "timeout" {
		t.Fatalf("expected bambooua status 'timeout', got %q", sources["bambooua"].Status)
	}
	if sources["animeon"].Status != "error" {
		t.Fatalf("expected animeon status 'error', got %q", sources["animeon"].Status)
	}

	// Перевірка, що кожен SourceKey у знайдених items непорожній
	for _, item := range resp.Items {
		for _, s := range item.Sources {
			if s.SourceKey == "" {
				t.Fatalf("expected non-empty SourceKey in item source ref, got empty")
			}
		}
	}
}
