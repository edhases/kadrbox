package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	transportHttp "github.com/edhases/oxide-server/internal/transport/http"
)

type mockDirectProvider struct {
	id    string
	items []domain.MediaItem
}

func (p *mockDirectProvider) ID() string      { return p.id }
func (p *mockDirectProvider) Name() string    { return p.id }
func (p *mockDirectProvider) BaseURL() string { return "https://" + p.id + ".example.com" }
func (p *mockDirectProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), SearchEnabledDefault: true}
}
func (p *mockDirectProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	return p.items, nil
}
func (p *mockDirectProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *mockDirectProvider) GetNew(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *mockDirectProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	return nil, nil
}
func (p *mockDirectProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	return nil, nil
}
func (p *mockDirectProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

func TestSearchUnified_AggregatesBanderaAndDirectProviders(t *testing.T) {
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
						Title:  "Величне століття. Роксолана",
						Year:   json.RawMessage(`2011`),
						Type:   provider.FlexibleString("series"),
						Ref:    json.RawMessage(`{"href":"https://uaflix.com/velychne"}`),
					},
				},
				Meta: &provider.BanderaSearchMetaResponse{
					Statuses: map[string]provider.BanderaSourceStatus{
						"uaflix": {Status: "ok", Count: 1, ElapsedMs: 150},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer banderaMock.Close()

	registry := provider.NewRegistry()
	banderaProv := provider.NewBanderaProviderWithConfig(banderaMock.URL, "", banderaMock.Client())
	registry.Register(banderaProv)

	uakinoProv := &mockDirectProvider{
		id: "uakino",
		items: []domain.MediaItem{
			{
				ID:         "https://uakino.biz/velychne-stolittya.html",
				ProviderID: "uakino",
				Title:      "Величне століття. Роксолана",
				Year:       2011,
				Type:       "series",
				URL:        "https://uakino.biz/velychne-stolittya.html",
				PosterURL:  "https://uakino.biz/posters/velychne.jpg",
			},
		},
	}
	registry.Register(uakinoProv)

	lavakinoProv := &mockDirectProvider{
		id: "lavakino",
		items: []domain.MediaItem{
			{
				ID:         "https://lavakino.net/serialy/4182-velychne-stolittia-kosem.html",
				ProviderID: "lavakino",
				Title:      "Величне століття. Нова володарка",
				Year:       2015,
				Type:       "series",
				URL:        "https://lavakino.net/serialy/4182-velychne-stolittia-kosem.html",
				PosterURL:  "https://lavakino.net/posters/kosem.jpg",
			},
		},
	}
	registry.Register(lavakinoProv)

	handler := transportHttp.NewContentHandler(registry, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/search?q=Величне+століття", nil)
	rr := httptest.NewRecorder()
	handler.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var envelope struct {
		Data struct {
			Segments []struct {
				ID    string `json:"id"`
				Count int    `json:"count"`
			} `json:"segments"`
			Items []struct {
				ID      string `json:"id"`
				Title   string `json:"title"`
				Sources []struct {
					ProviderID string `json:"provider_id"`
					SourceKey  string `json:"source_key"`
				} `json:"sources"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	resp := envelope.Data
	if len(resp.Segments) < 3 {
		t.Fatalf("expected at least 3 segments (bandera, uakino, lavakino), got %d", len(resp.Segments))
	}

	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 clustered items, got %d", len(resp.Items))
	}

	// Verify "Величне століття. Роксолана" was clustered with both bandera (uaflix) and uakino sources
	var roksolanaSources int
	for _, it := range resp.Items {
		if it.Title == "Величне століття. Роксолана" {
			roksolanaSources = len(it.Sources)
		}
	}
	if roksolanaSources != 2 {
		t.Fatalf("expected 2 sources for Roksolana (uaflix + uakino), got %d", roksolanaSources)
	}
}
