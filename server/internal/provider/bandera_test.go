package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestBanderaProviderDescribe(t *testing.T) {
	p := provider.NewBanderaProvider()
	if p.ID() != "bandera" {
		t.Errorf("expected ID 'bandera', got '%s'", p.ID())
	}
	if p.Name() != "Bandera Online" {
		t.Errorf("expected Name 'Bandera Online', got '%s'", p.Name())
	}
	if p.BaseURL() != provider.BanderaDefaultBaseURL {
		t.Errorf("expected BaseURL '%s', got '%s'", provider.BanderaDefaultBaseURL, p.BaseURL())
	}

	desc := p.Describe()
	if !desc.ShowOnHome {
		t.Errorf("expected ShowOnHome true")
	}
	if !desc.SearchEnabledDefault {
		t.Errorf("expected SearchEnabledDefault true")
	}
	if desc.HasFixedStreams {
		t.Errorf("expected HasFixedStreams false")
	}
	if len(desc.ContentTypes) != 3 {
		t.Fatalf("expected 3 content types, got %d", len(desc.ContentTypes))
	}
}

func TestBanderaProviderSearch(t *testing.T) {
	mockResponse := `{
		"ok": true,
		"items": [
			{
				"source": "bambooua",
				"title": "Бетмен проти Дракули",
				"title_en": "The Batman vs Dracula",
				"poster": "https://bambooua.com/uploads/posts/poster.jpg",
				"type": "movie",
				"year": "2005",
				"ref": {
					"href": "https://bambooua.com/cinema/702.html"
				},
				"group_key": "бетмен проти дракули"
			},
			{
				"source": "animeon",
				"title": "Нінджя Камуї",
				"title_en": "Ninja Kamui",
				"poster": "https://animeon.club/poster.jpg",
				"type": "series",
				"year": 2024,
				"ref": {
					"id": 1414,
					"season": 1
				},
				"group_key": "нінджя камуї"
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("title") != "batman" {
			t.Errorf("expected title query 'batman', got '%s'", r.URL.Query().Get("title"))
		}
		sources := r.URL.Query().Get("sources")
		if sources != "bambooua,makhno,animeon,franko,starlight" {
			t.Errorf("unexpected sources query: %s", sources)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())
	items, err := p.Search(context.Background(), "batman")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	item1 := items[0]
	if item1.Title != "Бетмен проти Дракули" {
		t.Errorf("unexpected title: %s", item1.Title)
	}
	if item1.Year != 2005 {
		t.Errorf("expected year 2005, got %d", item1.Year)
	}
	if item1.ProviderID != "bandera" {
		t.Errorf("expected provider_id 'bandera', got '%s'", item1.ProviderID)
	}

	item2 := items[1]
	if item2.Title != "Нінджя Камуї" {
		t.Errorf("unexpected title: %s", item2.Title)
	}
	if item2.Year != 2024 {
		t.Errorf("expected year 2024, got %d", item2.Year)
	}
	if item2.Type != "series" {
		t.Errorf("expected series, got %s", item2.Type)
	}
}

func TestBanderaProviderGetDetailsAndStreamsMovie(t *testing.T) {
	mockContentResp := `{
		"ok": true,
		"source": "bambooua",
		"type": "movie",
		"info": {
			"title": "Бетмен проти Дракули",
			"title_en": "The Batman vs Dracula",
			"description": "Анімаційний фільм про протистояння Бетмена і графа Дракули.",
			"release_date": "2005",
			"episode_time": "84 хв",
			"rating": "6.7",
			"genres": ["Бойовик", "Мультфільм", "Жахи"],
			"image": "https://bambooua.com/uploads/posts/poster.jpg"
		},
		"streams": [
			{
				"title": "Основне джерело (Дубляж)",
				"ref": {
					"url": "https://ongoing2.bambooua.com/films/BatmanVsDracula/dub/video/index.m3u8"
				}
			}
		]
	}`

	mockStreamResp := `{
		"ok": true,
		"source": "bambooua",
		"streams": [
			{
				"url": "https://ongoing2.bambooua.com/films/BatmanVsDracula/dub/video/index.m3u8",
				"quality": "auto"
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/content":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockContentResp))
		case "/stream":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockStreamResp))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	itemPayload := provider.BanderaItemPayload{
		Source: "bambooua",
		Ref:    json.RawMessage(`{"href":"https://bambooua.com/cinema/702.html"}`),
		Type:   "movie",
		Title:  "Бетмен проти Дракули",
		Poster: "https://bambooua.com/uploads/posts/poster.jpg",
		Year:   2005,
	}
	rawPayload, _ := json.Marshal(itemPayload)

	details, err := p.GetDetails(context.Background(), string(rawPayload))
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}

	if details.Title != "Бетмен проти Дракули" {
		t.Errorf("expected title 'Бетмен проти Дракули', got '%s'", details.Title)
	}
	if details.Rating != 6.7 {
		t.Errorf("expected rating 6.7, got %v", details.Rating)
	}
	if len(details.Genres) != 3 {
		t.Errorf("expected 3 genres, got %d", len(details.Genres))
	}
	if len(details.Voiceovers) != 1 {
		t.Errorf("expected 1 voiceover (stream source), got %d", len(details.Voiceovers))
	}

	// Перевірка GetStreams для фільму
	streamsResp, err := p.GetStreams(context.Background(), string(rawPayload), 0, 0, "0")
	if err != nil {
		t.Fatalf("GetStreams failed: %v", err)
	}

	if streamsResp.ProviderID != "bandera" {
		t.Errorf("expected provider 'bandera', got '%s'", streamsResp.ProviderID)
	}
	if len(streamsResp.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streamsResp.Streams))
	}
	if streamsResp.Streams[0].Quality != "auto" {
		t.Errorf("expected quality 'auto', got '%s'", streamsResp.Streams[0].Quality)
	}
}

func TestBanderaProviderGetDetailsAndStreamsSeries(t *testing.T) {
	mockContentResp := `{
		"ok": true,
		"source": "animeon",
		"type": "series",
		"info": {
			"title": "Бакуман",
			"title_en": "Bakuman",
			"description": "Історія про двох підлітків манґак.",
			"release_date": "2010",
			"genres": ["Комедія", "Драма"]
		},
		"voices": [
			{
				"id": "fanvoxua",
				"display_name": "FanVoxUA",
				"seasons": [
					{
						"title": 1,
						"episodes": [
							{
								"number": 1,
								"title": "Мрія і Реальність",
								"ref": {
									"episode_id": 60300,
									"season": 1,
									"episode": 1
								}
							},
							{
								"number": 2,
								"title": "Дурні й розумні",
								"ref": {
									"episode_id": 60301,
									"season": 1,
									"episode": 2
								}
							}
						]
					}
				]
			}
		]
	}`

	mockStreamResp := `{
		"ok": true,
		"source": "animeon",
		"streams": [
			{
				"url": "https://s.moonanime.art/manifest.m3u8",
				"quality": "auto"
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/content":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockContentResp))
		case "/stream":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockStreamResp))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	itemPayload := provider.BanderaItemPayload{
		Source: "animeon",
		Ref:    json.RawMessage(`{"id": 587, "season": 1}`),
		Type:   "series",
		Title:  "Бакуман",
	}
	rawPayload, _ := json.Marshal(itemPayload)

	details, err := p.GetDetails(context.Background(), string(rawPayload))
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}

	if len(details.Seasons) != 1 {
		t.Fatalf("expected 1 season, got %d", len(details.Seasons))
	}
	if len(details.Seasons[0].Episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(details.Seasons[0].Episodes))
	}
	if details.Seasons[0].Episodes[0].Title != "Мрія і Реальність" {
		t.Errorf("unexpected episode title: %s", details.Seasons[0].Episodes[0].Title)
	}

	// 1. Отримання потоку через itemURL епізоду
	episodeURL := details.Seasons[0].Episodes[0].URL
	streamResp1, err := p.GetStreams(context.Background(), episodeURL, 1, 1, "fanvoxua")
	if err != nil {
		t.Fatalf("GetStreams by episode URL failed: %v", err)
	}
	if len(streamResp1.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streamResp1.Streams))
	}

	// 2. Отримання потоку через загальний itemURL серіалу з параметрами сезону і серії
	streamResp2, err := p.GetStreams(context.Background(), string(rawPayload), 1, 2, "fanvoxua")
	if err != nil {
		t.Fatalf("GetStreams by series payload failed: %v", err)
	}
	if len(streamResp2.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streamResp2.Streams))
	}
}
