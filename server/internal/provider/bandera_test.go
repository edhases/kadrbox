package provider_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

// 1. Тест Describe (ShowOnHome має бути false)
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
	if desc.ShowOnHome {
		t.Errorf("expected ShowOnHome false (disabled from home until real popularity endpoint exists)")
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

// 2. Чекліст #1: reJSWhitespace та ParsePackedStreamURL з NBSP (U+00A0)
func TestParsePackedStreamURL_WhitespaceAndNBSP(t *testing.T) {
	// Звичайні пробіли
	u1, q1 := provider.ParsePackedStreamURL("[  720p  ]  https://example.com/stream.m3u8")
	if q1 != "720p" {
		t.Errorf("expected quality '720p', got %q", q1)
	}
	if u1 != "https://example.com/stream.m3u8" {
		t.Errorf("expected clean URL, got %q", u1)
	}

	// NBSP (U+00A0) та юнікодні розділювачі
	rawNBSP := "[\u00a0 1080p \u00a0]\u00a0 https://cdn.example.org/video.m3u8"
	u2, q2 := provider.ParsePackedStreamURL(rawNBSP)
	if q2 != "1080p" {
		t.Errorf("expected quality '1080p' with NBSP, got %q", q2)
	}
	if u2 != "https://cdn.example.org/video.m3u8" {
		t.Errorf("expected clean URL with NBSP, got %q", u2)
	}
}

// 3. Чекліст #8: Year з release_date "2010-04-06" або цілого числа дає 2010
func TestParseFlexibleYear(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{`"2010-04-06"`, 2010},
		{`"2024"`, 2024},
		{`2015`, 2015},
		{`"1899"`, 0}, // поза діапазоном 1900..2100
		{`"null"`, 0},
		{`""`, 0},
	}

	for _, tc := range tests {
		y := provider.ParseFlexibleYear(json.RawMessage(tc.input))
		if y != tc.expected {
			t.Errorf("ParseFlexibleYear(%s) = %d, expected %d", tc.input, y, tc.expected)
		}
	}
}

// 4. Чекліст #2: Розрізнення ref через inputs з /sources (динамічно, без хардкоду)
func TestIsStreamRef_WithSourceMeta(t *testing.T) {
	meta := provider.SourceMeta{
		Key:         "animeon",
		ContentKeys: []string{"id"},
		StreamKeys:  []string{"episode_id", "id"},
	}

	// ref контенту (серіалу): тільки id -> в animeon це і content, але коли це episode_id -> це stream
	episodeRef := json.RawMessage(`{"episode_id": 60300, "season": 1, "episode": 1}`)
	if !provider.IsStreamRef(meta, episodeRef) {
		t.Errorf("expected episodeRef with 'episode_id' to be recognized as stream ref")
	}

	bambooMeta := provider.SourceMeta{
		Key:         "bambooua",
		ContentKeys: []string{"href"},
		StreamKeys:  []string{"url"},
	}
	contentRef := json.RawMessage(`{"href": "https://bambooua.com/cinema/123.html"}`)
	if provider.IsStreamRef(bambooMeta, contentRef) {
		t.Errorf("expected contentRef with 'href' to NOT be recognized as stream ref")
	}

	streamRef := json.RawMessage(`{"url": "https://zetvideo.net/stream.m3u8"}`)
	if !provider.IsStreamRef(bambooMeta, streamRef) {
		t.Errorf("expected streamRef with 'url' to be recognized as stream ref")
	}
}

// 5. Чекліст #3: ValidateStreamRef захищає від 400 MISSING_URL без надсилання HTTP
func TestValidateStreamRef_RejectsInvalidRef(t *testing.T) {
	meta := provider.SourceMeta{
		Key:        "bambooua",
		StreamKeys: []string{"url"},
	}

	invalidRef := json.RawMessage(`{"href": "https://bambooua.com/cinema/123.html"}`)
	err := provider.ValidateStreamRef(meta, invalidRef)
	if err == nil {
		t.Fatalf("expected ValidateStreamRef to return error for item ref missing stream keys")
	}
}

// 6. Чекліст #4 та #5: full:true в POST /content та інспекція тіла вихідного JSON у мок-сервері
func TestBanderaProviderGetContent_InspectOutgoingBody(t *testing.T) {
	var receivedBody provider.BanderaContentRequest
	var contentRequestsCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/content" {
			atomic.AddInt32(&contentRequestsCount, 1)
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &receivedBody)

			resp := `{
				"ok": true,
				"source": "bambooua",
				"type": "movie",
				"info": {
					"title": "Фільм Тест",
					"rating": 7.5,
					"release_date": "2023-11-01"
				},
				"streams": [
					{
						"title": "Оригінал",
						"url": "https://cdn.example.org/play.m3u8",
						"ref": {"url": "https://cdn.example.org/play.m3u8"}
					}
				]
			}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())
	itemPayload := provider.BanderaItemPayload{
		Source: "bambooua",
		Ref:    json.RawMessage(`{"href":"https://bambooua.com/test.html"}`),
		Type:   "movie",
		Title:  "Фільм Тест",
	}
	payloadBytes, _ := json.Marshal(itemPayload)

	details, err := p.GetDetails(context.Background(), string(payloadBytes))
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}

	if details.Title != "Фільм Тест" {
		t.Errorf("expected title 'Фільм Тест', got %q", details.Title)
	}
	if details.Year != 2023 {
		t.Errorf("expected year 2023 from release_date, got %d", details.Year)
	}
	if details.Rating != 7.5 {
		t.Errorf("expected rating 7.5 from float, got %v", details.Rating)
	}

	// Перевірка інваріанта: full: true реально надіслано в тілі запиту!
	if !receivedBody.Full {
		t.Errorf("CRITICAL INVARIANT FAILED: POST /content did not include 'full: true'!")
	}
	if receivedBody.Source != "bambooua" {
		t.Errorf("expected source 'bambooua', got %q", receivedBody.Source)
	}
}

// 7. Чекліст #6: animeon /stream НЕ мемоізується (кожен виклик робить реальний HTTP запит)
func TestBanderaProviderGetStream_NotMemoized(t *testing.T) {
	var streamRequestsCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "animeon", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url", "episode_id"]}}
				]
			}`))
			return
		}
		if r.URL.Path == "/stream" {
			atomic.AddInt32(&streamRequestsCount, 1)
			_, _ = w.Write([]byte(`{
				"ok": true,
				"source": "animeon",
				"streams": [{"url": "https://s.moonanime.art/manifest.m3u8?expires=12345", "quality": "auto"}]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())
	streamRefPayload, _ := json.Marshal(provider.BanderaStreamRef{
		Source:      "animeon",
		Ref:         json.RawMessage(`{"url": "https://s.moonanime.art/manifest.m3u8?expires=12345"}`),
		IsStreamRef: true,
	})

	// Виклик 1
	_, err := p.GetStreams(context.Background(), string(streamRefPayload), 0, 0, "")
	if err != nil {
		t.Fatalf("first GetStreams failed: %v", err)
	}

	// Виклик 2
	_, err = p.GetStreams(context.Background(), string(streamRefPayload), 0, 0, "")
	if err != nil {
		t.Fatalf("second GetStreams failed: %v", err)
	}

	if streamRequestsCount != 2 {
		t.Errorf("CRITICAL INVARIANT FAILED: /stream was memoized! expected 2 network calls, got %d", streamRequestsCount)
	}
}

// 8. Чекліст #7: Стабільність ID між Search та GetDetails
func TestBanderaProvider_StableID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/search") {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"items": [
					{
						"source": "bambooua",
						"title": "Дюна",
						"type": "movie",
						"year": "2021",
						"ref": {"href": "https://bambooua.com/dune.html"}
					}
				]
			}`))
			return
		}
		if r.URL.Path == "/content" {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"source": "bambooua",
				"type": "movie",
				"info": {
					"title": "Дюна",
					"year": "2021"
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	searchResults, err := p.Search(context.Background(), "Дюна")
	if err != nil || len(searchResults) == 0 {
		t.Fatalf("Search failed: %v", err)
	}
	searchItem := searchResults[0]

	details, err := p.GetDetails(context.Background(), searchItem.URL)
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}

	if searchItem.ID != details.ID {
		t.Errorf("CRITICAL INVARIANT FAILED: ID mismatch! Search ID=%q vs Details ID=%q", searchItem.ID, details.ID)
	}
}

// 9. Тест proxy-правил (msg-002): uaflix+zetvideo БЕЗ проксі, ashdi З проксі, headers перевірено
func TestWrapStreamURL_RulesAndHeaders(t *testing.T) {
	// uaflix + zetvideo -> DIRECT (БЕЗ proxy)
	pURL, _, reqProxy := provider.WrapStreamURL("uaflix", "inner", "https://zetvideo.net/vod/12345/index.m3u8")
	if reqProxy {
		t.Errorf("expected uaflix + zetvideo to be DIRECT (requiresProxy=false)")
	}
	if strings.HasPrefix(pURL, provider.StreamProxyBase) {
		t.Errorf("expected uaflix + zetvideo to NOT be wrapped in StreamProxyBase")
	}

	// ashdi -> WITH proxy
	ashdiPlayable, _, ashdiProxy := provider.WrapStreamURL("bambooua", "inner", "https://ashdi.vip/vod/54321.m3u8")
	if !ashdiProxy {
		t.Errorf("expected ashdi to require proxy")
	}
	if !strings.HasPrefix(ashdiPlayable, provider.StreamProxyBase) {
		t.Errorf("expected ashdi to be wrapped with StreamProxyBase")
	}

	// Заголовки при requiresProxy=true: Referer та Origin НЕ повинні передаватися
	ashdiHeaders := provider.BuildStreamHeaders(ashdiPlayable, true)
	if _, ok := ashdiHeaders["Origin"]; ok {
		t.Errorf("INVARIANT VIOLATION: Origin should NOT be set when requiresProxy=true")
	}
	if _, ok := ashdiHeaders["Referer"]; ok {
		t.Errorf("INVARIANT VIOLATION: Referer should NOT be set when requiresProxy=true")
	}
	if ashdiHeaders["User-Agent"] == "" {
		t.Errorf("expected User-Agent to be set")
	}

	// Заголовки при requiresProxy=false: Referer та Origin виставляються
	directHeaders := provider.BuildStreamHeaders("https://zetvideo.net/vod/12345/index.m3u8", false)
	if directHeaders["Origin"] != "https://zetvideo.net" {
		t.Errorf("expected Origin 'https://zetvideo.net', got %q", directHeaders["Origin"])
	}
	if directHeaders["Referer"] != "https://zetvideo.net/" {
		t.Errorf("expected Referer 'https://zetvideo.net/', got %q", directHeaders["Referer"])
	}
}

// 10. Перевірка Ogham Space Mark (\u1680)
func TestParsePackedStreamURL_OghamSpace(t *testing.T) {
	rawOgham := "[\u1680 4K \u1680]\u1680 https://cdn.example.org/uhd.m3u8"
	u, q := provider.ParsePackedStreamURL(rawOgham)
	if q != "4K" {
		t.Errorf("expected quality '4K' with Ogham space, got %q", q)
	}
	if u != "https://cdn.example.org/uhd.m3u8" {
		t.Errorf("expected clean URL, got %q", u)
	}
}

// 11. СТРОГИЙ ТЕСТ: GetStreams для серіалу.
// Mock-сервер інспектує тіла запитів:
// - спочатку мусить бути запит на /content з full:true та ref серіалу;
// - потім мусить бути запит на /stream саме з ref обраного епізоду {"episode_id": 60301}, а не серіалу!
// - якщо /stream отримає search-ref, mock відповідає 400 MISSING_URL!
func TestBanderaProviderGetStreams_SeriesFlow_StrictInspection(t *testing.T) {
	var contentCalled bool
	var streamCalled bool
	var streamRefReceived map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sources":
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{
						"key": "animeon",
						"enabled": true,
						"capabilities": {"content": true, "stream": true},
						"inputs": {"content": ["id"], "stream": ["episode_id"]}
					}
				]
			}`))
		case "/content":
			contentCalled = true
			var req provider.BanderaContentRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if !req.Full {
				t.Errorf("INVARIANT VIOLATION: /content request must have full: true")
			}
			resp := `{
				"ok": true,
				"source": "animeon",
				"type": "series",
				"voices": [
					{
						"id": "fanvoxua",
						"display_name": "FanVoxUA",
						"seasons": [
							{
								"title": 1,
								"episodes": [
									{"number": 1, "title": "Серія 1", "ref": {"episode_id": 60300}},
									{"number": 2, "title": "Серія 2", "ref": {"episode_id": 60301}}
								]
							}
						]
					}
				]
			}`
			_, _ = w.Write([]byte(resp))
		case "/stream":
			streamCalled = true
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if refMap, ok := req["ref"].(map[string]interface{}); ok {
				streamRefReceived = refMap
				// Якби прийшов search-ref без episode_id — повертаємо 400 MISSING_URL!
				if _, hasEpID := refMap["episode_id"]; !hasEpID {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"ok": false, "error": "Missing url", "error_code": "MISSING_URL"}`))
					return
				}
			}
			_, _ = w.Write([]byte(`{
				"ok": true,
				"source": "animeon",
				"streams": [{"url": "https://s.moonanime.art/stream2.m3u8", "quality": "1080p"}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	// Серіальний payload з каталогу (НЕ має episode_id)
	seriesPayload := provider.BanderaItemPayload{
		Source:    "animeon",
		Ref:       json.RawMessage(`{"id": 587, "season": 1}`),
		Type:      "series",
		Title:     "Бакуман",
		IsItemRef: true,
	}
	payloadBytes, _ := json.Marshal(seriesPayload)

	// Запитуємо серію 2 сезону 1 озвучки fanvoxua
	streamsResp, err := p.GetStreams(context.Background(), string(payloadBytes), 1, 2, "fanvoxua")
	if err != nil {
		t.Fatalf("GetStreams failed: %v", err)
	}

	if !contentCalled {
		t.Errorf("CRITICAL INVARIANT FAILED: /content was NOT called for series item payload!")
	}
	if !streamCalled {
		t.Errorf("CRITICAL INVARIANT FAILED: /stream was NOT called!")
	}
	if streamRefReceived == nil || streamRefReceived["episode_id"] == nil {
		t.Fatalf("CRITICAL INVARIANT FAILED: /stream did not receive episode_id ref! Got: %v", streamRefReceived)
	}
	if epID, ok := streamRefReceived["episode_id"].(float64); !ok || int(epID) != 60301 {
		t.Errorf("expected episode_id 60301 for episode 2, got %v", streamRefReceived["episode_id"])
	}
	if len(streamsResp.Streams) != 1 {
		t.Errorf("expected 1 stream, got %d", len(streamsResp.Streams))
	}
}

// 12. СТРОГИЙ ТЕСТ: ref без streamKeys -> нуль мережевих викликів до /stream
func TestBanderaProviderGetStreams_ZeroHTTPCalls_OnInvalidRef(t *testing.T) {
	var httpCallsCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "bambooua", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}}
				]
			}`))
			return
		}
		// Будь-який інший запит — порушення інваріанта
		atomic.AddInt32(&httpCallsCount, 1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	// Прямий streamRef, але з невалідними ключами (відсутній "url")
	invalidStreamRef, _ := json.Marshal(provider.BanderaStreamRef{
		Source:      "bambooua",
		Ref:         json.RawMessage(`{"invalid_key": "some_value"}`),
		IsStreamRef: true,
	})

	_, err := p.GetStreams(context.Background(), string(invalidStreamRef), 0, 0, "")
	if err == nil {
		t.Fatalf("expected error for invalid stream ref, got nil")
	}

	if httpCallsCount != 0 {
		t.Errorf("CRITICAL INVARIANT FAILED: expected 0 network calls to /content or /stream for invalid ref, got %d", httpCallsCount)
	}
}
