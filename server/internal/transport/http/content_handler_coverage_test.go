package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

// covStubProvider — фейковий domain.Provider для тестів content-хендлера.
// Керується полями: що повертати і яку помилку імітувати.
type covStubProvider struct {
	id         string
	items      []domain.MediaItem
	details    *domain.MediaDetails
	streams    *domain.ContentStreamsResponse
	err        error
	gotURL     string
	gotSeason  int
	gotEpisode int
	gotVoice   string
}

func (f *covStubProvider) ID() string   { return f.id }
func (f *covStubProvider) Name() string { return "cov-stub-" + f.id }
func (f *covStubProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: f.id, Name: "cov-stub-" + f.id, ShowOnHome: true, SearchEnabledDefault: true}
}
func (f *covStubProvider) BaseURL() string { return "https://cov.example/" + f.id }

func (f *covStubProvider) Search(_ context.Context, _ string) ([]domain.MediaItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func (f *covStubProvider) GetDetails(_ context.Context, itemURL string) (*domain.MediaDetails, error) {
	f.gotURL = itemURL
	if f.err != nil {
		return nil, f.err
	}
	return f.details, nil
}

func (f *covStubProvider) GetStreams(_ context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	f.gotURL = itemURL
	f.gotSeason = season
	f.gotEpisode = episode
	f.gotVoice = voiceID
	if f.err != nil {
		return nil, f.err
	}
	return f.streams, nil
}

// covContentHandler будує хендлер з реєстром, що містить подані фейки.
func covContentHandler(fakes ...*covStubProvider) (*transporthttp.ContentHandler, []*covStubProvider) {
	reg := provider.NewRegistry()
	for _, f := range fakes {
		reg.Register(f)
	}
	// cacheRepo зберігається але не використовується — передаємо nil.
	return transporthttp.NewContentHandler(reg, nil), fakes
}

// TestCovHttpSearchMissingQuery — без параметра q має бути 400.
func TestCovHttpSearchMissingQuery(t *testing.T) {
	h, _ := covContentHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/search", nil)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("очікувався 400, отримано %d", rr.Code)
	}
}

// TestCovHttpSearchSuccess — два фейки, тіло містить дані обох, Content-Type JSON.
func TestCovHttpSearchSuccess(t *testing.T) {
	f1 := &covStubProvider{id: "p1", items: []domain.MediaItem{
		{ID: "1", ProviderID: "p1", Title: "cov-film-alpha", Type: "movie", URL: "https://x/1"},
	}}
	f2 := &covStubProvider{id: "p2", items: []domain.MediaItem{
		{ID: "2", ProviderID: "p2", Title: "cov-film-beta", Type: "series", URL: "https://x/2"},
	}}
	h, _ := covContentHandler(f1, f2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/search?q=test", nil)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("очікувався Content-Type application/json, отримано %q", ct)
	}
	body := rr.Body.String()
	// Порядок агрегації недетермінований (map + горутини), тому перевіряємо вміст.
	if !strings.Contains(body, "cov-film-alpha") {
		t.Errorf("тіло не містить даних першого фейка: %s", body)
	}
	if !strings.Contains(body, "cov-film-beta") {
		t.Errorf("тіло не містить даних другого фейка: %s", body)
	}
}

// TestCovHttpSearchPropagatesProviderError — ХАРАКТЕРИЗАЦІЯ:
// Registry.SearchAll мовчки ковтає помилки провайдерів (див. registry.go:58-63)
// і SingleFlightSearch завжди повертає nil error, тому гілка 500 у Search
// недосяжна через помилку провайдера: навіть з фейком, що повертає err,
// хендлер відповідає 200 з "null". Тест фіксує фактичну поведінку.
func TestCovHttpSearchPropagatesProviderError(t *testing.T) {
	h, _ := covContentHandler(&covStubProvider{id: "p1", err: errors.New("cov-boom")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/search?q=test", nil)
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("характеризація: помилка провайдера ковтається, очікувався 200, отримано %d", rr.Code)
	}
}

// TestCovHttpDetailsMissingParams — таблиця: порожні provider/url дають 400.
func TestCovHttpDetailsMissingParams(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"без обох параметрів", "/api/v1/content/details"},
		{"без url", "/api/v1/content/details?provider=p1"},
		{"без provider", "/api/v1/content/details?url=https://x/1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := covContentHandler(&covStubProvider{id: "p1"})
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()

			h.GetDetails(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("очікувався 400, отримано %d", rr.Code)
			}
		})
	}
}

// TestCovHttpDetailsUnknownProvider — невідомий provider дає 404.
func TestCovHttpDetailsUnknownProvider(t *testing.T) {
	h, _ := covContentHandler(&covStubProvider{id: "p1"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/details?provider=nope&url=https://x/1", nil)
	rr := httptest.NewRecorder()

	h.GetDetails(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("очікувався 404, отримано %d", rr.Code)
	}
}

// TestCovHttpDetailsSuccess — тіло відповіді дорівнює details фейка.
func TestCovHttpDetailsSuccess(t *testing.T) {
	want := &domain.MediaDetails{
		MediaItem:   domain.MediaItem{ID: "7", ProviderID: "p1", Title: "cov-details", Type: "movie", URL: "https://x/7"},
		Description: "cov-description",
	}
	h, _ := covContentHandler(&covStubProvider{id: "p1", details: want})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/details?provider=p1&url=https://x/7", nil)
	rr := httptest.NewRecorder()

	h.GetDetails(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "cov-details") || !strings.Contains(body, "cov-description") {
		t.Errorf("тіло не збігається з details фейка: %s", body)
	}
}

// TestCovHttpDetailsProviderError — помилка провайдера дає 500.
func TestCovHttpDetailsProviderError(t *testing.T) {
	h, _ := covContentHandler(&covStubProvider{id: "p1", err: errors.New("cov-boom")})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/details?provider=p1&url=https://x/1", nil)
	rr := httptest.NewRecorder()

	h.GetDetails(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("очікувався 500, отримано %d", rr.Code)
	}
}

// TestCovHttpStreamsParams — фейк записує отримані season/episode/voiceID.
func TestCovHttpStreamsParams(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		wantSeason  int
		wantEpisode int
		wantVoice   string
	}{
		{"усі параметри", "?provider=p1&url=https://x/1&season=2&episode=5&voice=v1", 2, 5, "v1"},
		{"без параметрів", "?provider=p1&url=https://x/1", 0, 0, ""},
		// ХАРАКТЕРИЗАЦІЯ: помилки strconv.Atoi мовчки ігноруються (content_handler.go:78-79),
		// тому нечисловий season перетворюється на 0 без помилки.
		{"нечисловий season", "?provider=p1&url=https://x/1&season=abc", 0, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &covStubProvider{
				id:      "p1",
				streams: &domain.ContentStreamsResponse{ProviderID: "p1"},
			}
			h, _ := covContentHandler(fake)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/content/streams"+tc.query, nil)
			rr := httptest.NewRecorder()

			h.GetStreams(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("очікувався 200, отримано %d", rr.Code)
			}
			if fake.gotSeason != tc.wantSeason || fake.gotEpisode != tc.wantEpisode || fake.gotVoice != tc.wantVoice {
				t.Errorf("отримано season=%d episode=%d voice=%q; очікувалось %d/%d/%q",
					fake.gotSeason, fake.gotEpisode, fake.gotVoice, tc.wantSeason, tc.wantEpisode, tc.wantVoice)
			}
		})
	}
}

// TestCovHttpStreamsUnknownProvider — невідомий provider дає 404.
func TestCovHttpStreamsUnknownProvider(t *testing.T) {
	h, _ := covContentHandler(&covStubProvider{id: "p1"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/streams?provider=nope&url=https://x/1", nil)
	rr := httptest.NewRecorder()

	h.GetStreams(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("очікувався 404, отримано %d", rr.Code)
	}
}

// TestCovHttpStreamsProviderError — помилка провайдера дає 500.
func TestCovHttpStreamsProviderError(t *testing.T) {
	h, _ := covContentHandler(&covStubProvider{id: "p1", err: errors.New("cov-boom")})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/streams?provider=p1&url=https://x/1", nil)
	rr := httptest.NewRecorder()

	h.GetStreams(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("очікувався 500, отримано %d", rr.Code)
	}
}

// TestCovHttpProvidersCatalog — каталог містить зареєстрованих провайдерів.
func TestCovHttpProvidersCatalog(t *testing.T) {
	h, _ := covContentHandler(&covStubProvider{id: "p1"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/providers", nil)
	rr := httptest.NewRecorder()

	h.Providers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("очікувався application/json, отримано %q", ct)
	}
	var cat struct {
		Version   int64 `json:"version"`
		Providers []struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
			Healthy bool   `json:"healthy"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &cat); err != nil {
		t.Fatalf("невалідний JSON каталогу: %v", err)
	}
	if len(cat.Providers) != 1 || cat.Providers[0].ID != "p1" {
		t.Errorf("неочікуваний каталог: %+v", cat)
	}
	if !cat.Providers[0].Enabled || !cat.Providers[0].Healthy {
		t.Errorf("очікувався enabled+healthy провайдер: %+v", cat.Providers[0])
	}
}

// TestCovHttpDetailsDisabledProvider — вимкнений провайдер дає 403.
func TestCovHttpDetailsDisabledProvider(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&covStubProvider{id: "p1"})
	reg.SetEnabled("p1", false)
	h := transporthttp.NewContentHandler(reg, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/details?provider=p1&url=https://x/1", nil)
	rr := httptest.NewRecorder()

	h.GetDetails(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("очікувався 403, отримано %d", rr.Code)
	}
}
