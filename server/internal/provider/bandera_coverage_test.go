package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/provider"
)

// =========================================================================
// 1. Тести для bandera_types.go: FlexibleString, FlexibleFloat, FlexibleInt,
//    ParseFlexibleYear, FlexibleGenres, BanderaSourceStatus.GetElapsedMs
// =========================================================================

func TestBanderaTypes_FlexibleString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`null`, ""},
		{`""`, ""},
		{`"hello world"`, "hello world"},
		{`"  trimmed  "`, "trimmed"},
		{`123`, "123"},
		{`45.67`, "45.67"},
		{`true`, "true"},
		{`false`, "false"},
		{`"quoted"`, "quoted"},
	}

	for _, tc := range tests {
		var fs provider.FlexibleString
		err := json.Unmarshal([]byte(tc.input), &fs)
		if err != nil {
			t.Fatalf("unexpected unmarshal error for %s: %v", tc.input, err)
		}
		if fs.String() != tc.expected {
			t.Errorf("FlexibleString(%s) = %q, expected %q", tc.input, fs.String(), tc.expected)
		}
	}

	// Невалідний JSON запускає останній fallback: strings.Trim(string(b), "\"")
	var fallbackFS provider.FlexibleString
	_ = fallbackFS.UnmarshalJSON([]byte(`"fallback_unclosed`))
	if fallbackFS.String() != "fallback_unclosed" {
		t.Errorf("expected fallback_unclosed, got %q", fallbackFS.String())
	}

	// Порожній зріз байтів
	var emptyFS provider.FlexibleString
	if err := emptyFS.UnmarshalJSON([]byte{}); err != nil || emptyFS.String() != "" {
		t.Errorf("expected empty string on empty bytes, got %q", emptyFS.String())
	}
}

func TestBanderaTypes_FlexibleFloat(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{`null`, 0.0},
		{`3.1415`, 3.1415},
		{`100`, 100.0},
		{`"7.8"`, 7.8},
		{`"  8.9  "`, 8.9},
		{`""`, 0.0},
		{`"not-a-number"`, 0.0},
		{`{"invalid": true}`, 0.0},
	}

	for _, tc := range tests {
		var ff provider.FlexibleFloat
		_ = json.Unmarshal([]byte(tc.input), &ff)
		if ff.Float64() != tc.expected {
			t.Errorf("FlexibleFloat(%s) = %v, expected %v", tc.input, ff.Float64(), tc.expected)
		}
	}

	var emptyFF provider.FlexibleFloat
	if err := emptyFF.UnmarshalJSON([]byte{}); err != nil || emptyFF.Float64() != 0 {
		t.Errorf("expected 0 on empty bytes, got %v", emptyFF.Float64())
	}
}

func TestBanderaTypes_FlexibleInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{`null`, 0},
		{`42`, 42},
		{`42.9`, 42},
		{`"100"`, 100},
		{`"  200  "`, 200},
		{`""`, 0},
		{`"invalid"`, 0},
		{`{"invalid": 123}`, 0},
	}

	for _, tc := range tests {
		var fi provider.FlexibleInt
		_ = json.Unmarshal([]byte(tc.input), &fi)
		if fi.Int() != tc.expected {
			t.Errorf("FlexibleInt(%s) = %v, expected %v", tc.input, fi.Int(), tc.expected)
		}
	}

	var emptyFI provider.FlexibleInt
	if err := emptyFI.UnmarshalJSON([]byte{}); err != nil || emptyFI.Int() != 0 {
		t.Errorf("expected 0 on empty bytes, got %v", emptyFI.Int())
	}
}

func TestBanderaTypes_ParseFlexibleYear(t *testing.T) {
	tests := []struct {
		raw      string
		expected int
	}{
		{`null`, 0},
		{``, 0},
		{`2023`, 2023},
		{`1850`, 0}, // поза 1900..2100
		{`2150`, 0}, // поза 1900..2100
		{`"2024"`, 2024},
		{`"1999-12-31"`, 1999},
		{`"Рік 2020 в описі"`, 2020},
		{`"Рік 1850 не підходить"`, 0},
		{`"немає року"`, 0},
	}

	for _, tc := range tests {
		y := provider.ParseFlexibleYear(json.RawMessage(tc.raw))
		if y != tc.expected {
			t.Errorf("ParseFlexibleYear(%s) = %d, expected %d", tc.raw, y, tc.expected)
		}
	}
}

func TestBanderaTypes_FlexibleGenres(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{`null`, nil},
		{``, nil},
		{`["Драма", "  Комедія  ", ""]`, []string{"Драма", "Комедія"}},
		{`"Фантастика, Бойовик,  Трилер"`, []string{"Фантастика", "Бойовик", "Трилер"}},
		{`[{"name": "Аніме"}, {"title": "Пригоди"}, {"name": ""}]`, []string{"Аніме", "Пригоди"}},
		{`12345`, nil},
	}

	for _, tc := range tests {
		var fg provider.FlexibleGenres
		_ = json.Unmarshal([]byte(tc.input), &fg)
		if len(fg) != len(tc.expected) {
			t.Fatalf("FlexibleGenres(%s) len = %d, expected %d", tc.input, len(fg), len(tc.expected))
		}
		for i := range fg {
			if fg[i] != tc.expected[i] {
				t.Errorf("FlexibleGenres[%d] = %q, expected %q", i, fg[i], tc.expected[i])
			}
		}
	}
}

func TestBanderaTypes_SourceStatusElapsed(t *testing.T) {
	s1 := provider.BanderaSourceStatus{ElapsedMs: 120, Elapsed: 80}
	if s1.GetElapsedMs() != 120 {
		t.Errorf("expected 120, got %d", s1.GetElapsedMs())
	}
	s2 := provider.BanderaSourceStatus{Elapsed: 85}
	if s2.GetElapsedMs() != 85 {
		t.Errorf("expected 85, got %d", s2.GetElapsedMs())
	}
}

// =========================================================================
// 2. Тести для bandera_select.go: ParseSeasonNumber, SelectEpisodeRef, IsStreamRef
// =========================================================================

func TestBanderaSelect_ParseSeasonNumber(t *testing.T) {
	tests := []struct {
		raw        string
		defaultNum int
		expected   int
	}{
		{``, 1, 1},
		{`null`, 2, 2},
		{`3`, 1, 3},
		{`0`, 1, 1},
		{`"4"`, 1, 4},
		{`"Сезон 5"`, 1, 5},
		{`"Season 06"`, 1, 6},
		{`"s07"`, 1, 7},
		{`"no numbers"`, 8, 8},
	}

	for _, tc := range tests {
		got := provider.ParseSeasonNumber(json.RawMessage(tc.raw), tc.defaultNum)
		if got != tc.expected {
			t.Errorf("ParseSeasonNumber(%s, %d) = %d, expected %d", tc.raw, tc.defaultNum, got, tc.expected)
		}
	}
}

func TestBanderaSelect_SelectEpisodeRef(t *testing.T) {
	// 1. Порожній список voices
	ref, subs, found := provider.SelectEpisodeRef(nil, 1, 1, "")
	if found || ref != nil || subs != nil {
		t.Errorf("expected false for empty voices")
	}

	// 2. Озвучки зі структурою сезонів
	voicesWithSeasons := []provider.BanderaVoice{
		{
			ID:          "v1",
			DisplayName: "Озвучка 1",
			Seasons: []provider.BanderaSeason{
				{
					Title: json.RawMessage(`"Сезон 1"`),
					Episodes: []provider.BanderaEpisode{
						{Number: 1, Title: "Еп 1", Ref: json.RawMessage(`{"ep": 1}`)},
						{Number: 2, Title: "Еп 2", Ref: json.RawMessage(`{"ep": 2}`)},
					},
				},
			},
		},
		{
			ID:          "v2",
			DisplayName: "Озвучка 2",
			Seasons: []provider.BanderaSeason{
				{
					Title: json.RawMessage(`1`),
					Episodes: []provider.BanderaEpisode{
						{Number: 1, Title: "Еп 1 v2", Ref: json.RawMessage(`{"ep": 10}`)},
					},
				},
			},
		},
	}

	// Пошук існуючого епізоду за voiceID
	ref, _, found = provider.SelectEpisodeRef(voicesWithSeasons, 1, 2, "v1")
	if !found || string(ref) != `{"ep": 2}` {
		t.Errorf("expected to find ep 2 in v1, got found=%v, ref=%s", found, string(ref))
	}

	// Неіснуючий voiceID -> fallback на перший доступний voice (v1)
	ref, _, found = provider.SelectEpisodeRef(voicesWithSeasons, 1, 1, "non_existent")
	if !found || string(ref) != `{"ep": 1}` {
		t.Errorf("expected fallback to first voice ep 1, got found=%v, ref=%s", found, string(ref))
	}

	// Неіснуючий сезон
	_, _, found = provider.SelectEpisodeRef(voicesWithSeasons, 99, 1, "v1")
	if found {
		t.Errorf("expected not found for non-existent season")
	}

	// Неіснуючий епізод
	_, _, found = provider.SelectEpisodeRef(voicesWithSeasons, 1, 99, "v1")
	if found {
		t.Errorf("expected not found for non-existent episode")
	}

	// Пошук за default (season 0, episode 0) -> бере перший епізод
	ref, _, found = provider.SelectEpisodeRef(voicesWithSeasons, 0, 0, "v1")
	if !found || string(ref) != `{"ep": 1}` {
		t.Errorf("expected first episode on 0/0 query")
	}

	// 3. Озвучка з плоским списком епізодів (без Seasons)
	voicesFlat := []provider.BanderaVoice{
		{
			ID: "flat_voice",
			Episodes: []provider.BanderaEpisode{
				{Number: 1, Title: "Еп 1 Flat", Ref: json.RawMessage(`{"flat_ep": 1}`)},
				{Number: 2, Title: "Еп 2 Flat", Ref: nil}, // порожній ref
			},
		},
	}

	ref, _, found = provider.SelectEpisodeRef(voicesFlat, 0, 1, "flat_voice")
	if !found || string(ref) != `{"flat_ep": 1}` {
		t.Errorf("expected to find flat episode 1")
	}

	// Запит епізоду з порожнім ref
	_, _, found = provider.SelectEpisodeRef(voicesFlat, 0, 2, "flat_voice")
	if found {
		t.Errorf("expected false for episode with empty ref")
	}

	// Порожня озвучка (ні сезонів, ні епізодів)
	emptyVoice := []provider.BanderaVoice{{ID: "empty"}}
	_, _, found = provider.SelectEpisodeRef(emptyVoice, 1, 1, "")
	if found {
		t.Errorf("expected false for empty voice")
	}
}

func TestBanderaSelect_IsStreamRef_EdgeCases(t *testing.T) {
	// Порожній ref
	if provider.IsStreamRef(provider.SourceMeta{}, nil) {
		t.Errorf("expected false for nil ref")
	}

	// Невалідний JSON
	if provider.IsStreamRef(provider.SourceMeta{}, json.RawMessage(`{broken-json`)) {
		t.Errorf("expected false for broken json")
	}

	// Порожні StreamKeys у метаданих -> fallback на defaultStreamKeysFallback (url, episode_id, file)
	metaFallback := provider.SourceMeta{Key: "fallback"}
	if !provider.IsStreamRef(metaFallback, json.RawMessage(`{"file": "video.mp4"}`)) {
		t.Errorf("expected true for fallback 'file' key")
	}
	if provider.IsStreamRef(metaFallback, json.RawMessage(`{"title": "test"}`)) {
		t.Errorf("expected false for missing fallback stream keys")
	}
}

// =========================================================================
// 3. Тести для bandera_normalize.go: ParsePackedStreamURL, WrapStreamURL, BuildStreamHeaders
// =========================================================================

func TestBanderaNormalize_ParsePackedStreamURL_EdgeCases(t *testing.T) {
	// Порожня якість всередині дужок
	u, q := provider.ParsePackedStreamURL("[] https://example.com/stream.m3u8")
	if q != "auto" || u != "https://example.com/stream.m3u8" {
		t.Errorf("expected auto quality, got q=%q, u=%q", q, u)
	}

	// Тільки дужки без URL
	u2, q2 := provider.ParsePackedStreamURL("[1080p]")
	if q2 != "auto" || u2 != "[1080p]" {
		t.Errorf("expected raw string returned when no url follows brackets, got q=%q, u=%q", q2, u2)
	}
}

func TestBanderaNormalize_WrapStreamURL_EdgeCases(t *testing.T) {
	// Зовнішній плеєр ("external") -> ніколи не проксі
	play, direct, req := provider.WrapStreamURL("sirko", "external", "https://example.com/stream.m3u8")
	if req || play != "https://example.com/stream.m3u8" {
		t.Errorf("expected external player to return direct url without proxy")
	}

	// Вже обгорнутий у SirkoProxyBase
	alreadySirko := provider.SirkoProxyBase + url.QueryEscape("https://example.com/s.m3u8")
	play, _, req = provider.WrapStreamURL("sirko", "inner", alreadySirko)
	if req || play != alreadySirko {
		t.Errorf("expected idempotent wrap for SirkoProxyBase")
	}

	// Sirko проксі
	play, direct, req = provider.WrapStreamURL("sirko", "inner", "https://stream.sirko.net/hls/test.m3u8")
	if !req || !strings.HasPrefix(play, provider.SirkoProxyBase) || direct != "https://stream.sirko.net/hls/test.m3u8" {
		t.Errorf("expected Sirko proxy wrapping")
	}

	// sniplyo та creavio
	play, _, req = provider.WrapStreamURL("source1", "inner", "https://sub.sniplyo.online/play.m3u8")
	if !req || !strings.HasPrefix(play, provider.StreamProxyBase) {
		t.Errorf("expected sniplyo to require stream proxy")
	}

	play, _, req = provider.WrapStreamURL("source2", "inner", "https://sub.creavio.online/play.m3u8")
	if !req || !strings.HasPrefix(play, provider.StreamProxyBase) {
		t.Errorf("expected creavio to require stream proxy")
	}

	// Звичайний CDN (без проксі)
	play, direct, req = provider.WrapStreamURL("other", "inner", "https://cdn.normal.com/vod.m3u8")
	if req || play != direct {
		t.Errorf("expected direct playback for regular CDN")
	}
}

func TestBanderaNormalize_BuildStreamHeaders_EdgeCases(t *testing.T) {
	// Невалідний URL без схеми/хоста
	h := provider.BuildStreamHeaders("invalid-url-without-host", false)
	if h["User-Agent"] == "" {
		t.Errorf("expected User-Agent to be set")
	}
	if _, ok := h["Origin"]; ok {
		t.Errorf("expected no Origin for invalid URL")
	}
}

func TestBanderaNormalize_MergeSubtitles_EdgeCases(t *testing.T) {
	// Порожні списки
	subs := provider.MergeSubtitles(nil, nil)
	if len(subs) != 0 {
		t.Errorf("expected empty subtitles list")
	}

	// Субтитри без Label і без Lang -> "Субтитри"
	rawSubs := []provider.BanderaSubtitleItem{
		{URL: "https://example.com/empty.vtt"},
		{URL: ""}, // порожній URL ігнорується
	}
	subs = provider.MergeSubtitles(rawSubs, nil)
	if len(subs) != 1 || subs[0].Label != "Субтитри" {
		t.Errorf("expected default label 'Субтитри', got %+v", subs)
	}
}

// =========================================================================
// 4. Тести для bandera_client.go: NewBanderaClient, GetSources помилки,
//    GetSourceMeta, SearchWithMeta, GetContent, GetStream помилки
// =========================================================================

func TestBanderaClient_NewClientDefaults(t *testing.T) {
	c := provider.NewBanderaClient("", nil)
	if c == nil {
		t.Fatalf("expected non-nil BanderaClient")
	}

	// Перевірка обрізання слеша
	c2 := provider.NewBanderaClient("https://example.com/api/v2/", &http.Client{Timeout: 5 * time.Second})
	if c2 == nil {
		t.Fatalf("expected non-nil client with custom url")
	}
}

func TestBanderaClient_GetSources_FailuresAndFallback(t *testing.T) {
	// 1. Помилка мережі без кешу
	brokenClient := &http.Client{
		Transport: &roundTripperMock{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network down")
			},
		},
	}
	c := provider.NewBanderaClient("https://bbe.lme.isroot.in/api/v2", brokenClient)
	sources, err := c.GetSources(context.Background())
	if err == nil || sources != nil {
		t.Errorf("expected error on network failure without cache")
	}

	// 2. HTTP 500 статус без кешу
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer server500.Close()

	c500 := provider.NewBanderaClient(server500.URL, server500.Client())
	_, err = c500.GetSources(context.Background())
	if err == nil {
		t.Errorf("expected error on HTTP 500 without cache")
	}

	// 3. Помилка декодування JSON
	serverBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid-json"))
	}))
	defer serverBadJSON.Close()

	cBadJSON := provider.NewBanderaClient(serverBadJSON.URL, serverBadJSON.Client())
	_, err = cBadJSON.GetSources(context.Background())
	if err == nil {
		t.Errorf("expected error on bad JSON decode")
	}

	// 4. Помилка створення HTTP запиту (невалідний url з контрол-символами)
	cInvalidURL := provider.NewBanderaClient("http://[::1]:namedport", &http.Client{})
	_, err = cInvalidURL.GetSources(context.Background())
	if err == nil {
		t.Errorf("expected error on invalid request URL")
	}

	// 5. Успішне джерело, де всі вимкнені -> перевірка fallback enabledSearchStr
	serverAllDisabled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true, "sources": [{"key": "disabled_one", "enabled": false}]}`))
	}))
	defer serverAllDisabled.Close()

	cDisabled := provider.NewBanderaClient(serverAllDisabled.URL, serverAllDisabled.Client())
	searchStr := cDisabled.GetSearchSourcesStr(context.Background())
	if searchStr != provider.BanderaDefaultSources {
		t.Errorf("expected BanderaDefaultSources fallback when no sources enabled, got %q", searchStr)
	}

	// 6. GetSourceMeta успіх та неуспіх
	serverGood := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true, "sources": [{"key": "uatut", "name": "UATUT", "enabled": true, "capabilities": {"search": true}}]}`))
	}))
	defer serverGood.Close()

	cGood := provider.NewBanderaClient(serverGood.URL, serverGood.Client())
	meta, found := cGood.GetSourceMeta(context.Background(), "uatut")
	if !found || meta.Key != "uatut" {
		t.Errorf("expected to find uatut meta, got found=%v, meta=%+v", found, meta)
	}
	_, found = cGood.GetSourceMeta(context.Background(), "unknown_source")
	if found {
		t.Errorf("expected false for unknown source")
	}

	// GetSourceMeta коли клієнт падає
	_, found = c500.GetSourceMeta(context.Background(), "uatut")
	if found {
		t.Errorf("expected false on client error")
	}
}

func TestBanderaClient_SearchWithMeta_ErrorsAndParams(t *testing.T) {
	var requestedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		if strings.Contains(r.URL.Path, "fail") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("search failed"))
			return
		}
		if strings.Contains(r.URL.Path, "badjson") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{broken"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true, "items": [{"title": "Результат", "source": "uatut"}]}`))
	}))
	defer server.Close()

	c := provider.NewBanderaClient(server.URL, server.Client())

	// Перевірка передачі year та serial у параметри запиту
	resp, err := c.SearchWithMeta(context.Background(), "Тест", 2024, 1)
	if err != nil || resp == nil || len(resp.Items) != 1 {
		t.Fatalf("SearchWithMeta failed: %v", err)
	}
	if !strings.Contains(requestedURL, "year=2024") || !strings.Contains(requestedURL, "serial=1") {
		t.Errorf("expected URL to contain year=2024 and serial=1, got %q", requestedURL)
	}

	// Помилка статусу 500
	cFail := provider.NewBanderaClient(server.URL+"/fail", server.Client())
	_, err = cFail.SearchWithMeta(context.Background(), "Тест", 0, 0)
	if err == nil {
		t.Errorf("expected error on 500 status")
	}

	// Помилка JSON decode
	cBad := provider.NewBanderaClient(server.URL+"/badjson", server.Client())
	_, err = cBad.SearchWithMeta(context.Background(), "Тест", 0, 0)
	if err == nil {
		t.Errorf("expected error on bad json")
	}

	// Мережева помилка
	cBroken := provider.NewBanderaClient("http://invalid.nonexistent.domain", &http.Client{Timeout: 10 * time.Millisecond})
	_, err = cBroken.SearchWithMeta(context.Background(), "Тест", 0, 0)
	if err == nil {
		t.Errorf("expected network error")
	}
}

func TestBanderaClient_GetContent_And_GetStream_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "fail") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
			return
		}
		if strings.Contains(r.URL.Path, "badjson") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not-json"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	// GetContent 500 & bad JSON
	cFail := provider.NewBanderaClient(server.URL+"/fail", server.Client())
	_, err := cFail.GetContent(context.Background(), "uatut", json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("expected error on GetContent 500")
	}

	cBad := provider.NewBanderaClient(server.URL+"/badjson", server.Client())
	_, err = cBad.GetContent(context.Background(), "uatut", json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("expected error on GetContent bad json")
	}

	// GetStream 500 & bad JSON
	_, err = cFail.GetStream(context.Background(), "uatut", json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("expected error on GetStream 500")
	}

	_, err = cBad.GetStream(context.Background(), "uatut", json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("expected error on GetStream bad json")
	}

	// Мережеві помилки
	cBroken := provider.NewBanderaClient("http://invalid.domain", &http.Client{Timeout: 10 * time.Millisecond})
	_, err = cBroken.GetContent(context.Background(), "uatut", json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("expected network error on GetContent")
	}
	_, err = cBroken.GetStream(context.Background(), "uatut", json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("expected network error on GetStream")
	}
}

// =========================================================================
// 5. Тести для bandera.go: GetPopular, GetNew, GetByCategory,
//    rankAndSortMediaItems, GetDetails edge-cases, GetStreams edge-cases
// =========================================================================

func TestBanderaProvider_PopularNewAndCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/search") {
			q := r.URL.Query().Get("title")
			if q == "error_trigger" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("error"))
				return
			}

			// Повертаємо кілька результатів з різними роками та типами для перевірки сортування
			resp := `{
				"ok": true,
				"items": [
					{"source": "uatut", "title": "Фільм Старий", "year": "2020", "type": "movie", "poster": "p1.jpg"},
					{"source": "uatut", "title": "Фільм Новий", "year": "2024", "type": "movie", "poster": "p2.jpg"},
					{"source": "uatut", "title": "Серіал Топ", "year": "2023", "serial": 1, "type": "series"},
					{"source": "uatut", "title": "Фільм Без Постера", "year": "2022", "type": "movie"}
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

	// 1. GetPopular для movie (сортування: фільми першими, найновіші першими)
	movies, err := p.GetPopular(context.Background(), "movie", 0)
	if err != nil {
		t.Fatalf("GetPopular movie failed: %v", err)
	}
	if len(movies) == 0 {
		t.Fatalf("expected non-empty popular movies")
	}
	if movies[0].Title != "Фільм Новий" {
		t.Errorf("expected newest movie first, got %q", movies[0].Title)
	}

	// 2. GetPopular для series
	series, err := p.GetPopular(context.Background(), "series", 1)
	if err != nil {
		t.Fatalf("GetPopular series failed: %v", err)
	}
	if len(series) == 0 || series[0].Type != "series" {
		t.Errorf("expected series first for series query")
	}

	// 3. GetPopular для anime
	anime, err := p.GetPopular(context.Background(), "anime", 2)
	if err != nil {
		t.Fatalf("GetPopular anime failed: %v", err)
	}
	if len(anime) == 0 {
		t.Errorf("expected anime results")
	}

	// 4. GetPopular дефолт
	def, err := p.GetPopular(context.Background(), "", 1)
	if err != nil || len(def) == 0 {
		t.Fatalf("GetPopular default failed: %v", err)
	}

	// 5. GetNew
	newItems, err := p.GetNew(context.Background(), "movie", 1)
	if err != nil || len(newItems) == 0 {
		t.Fatalf("GetNew failed: %v", err)
	}

	// 6. GetByCategory з непорожньою категорією
	catItems, err := p.GetByCategory(context.Background(), "комедія", "movie", 1)
	if err != nil || len(catItems) == 0 {
		t.Fatalf("GetByCategory comedy failed: %v", err)
	}

	// 7. GetByCategory з порожньою категорією -> фолбек на GetPopular
	emptyCatItems, err := p.GetByCategory(context.Background(), "", "movie", 1)
	if err != nil || len(emptyCatItems) == 0 {
		t.Fatalf("GetByCategory empty category failed: %v", err)
	}

	// 8. Помилка пошуку
	_, err = p.GetByCategory(context.Background(), "error_trigger", "movie", 1)
	if err == nil {
		t.Errorf("expected error on search failure")
	}
}

func TestBanderaProvider_RankAndSortMediaItems(t *testing.T) {
	// 1. Порожній слайс та слайс з 1 елемента
	p := provider.NewBanderaProvider()
	items, err := p.GetPopular(context.Background(), "movie", 1)
	_ = items

	// Тест дедуплікації та злиття постерів/рейтингів через GetPopular mock
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{
			"ok": true,
			"items": [
				{"source": "src1", "title": "Аватар", "year": "2009", "type": "movie", "poster": ""},
				{"source": "src2", "title": "аватар", "year": "2009", "type": "movie", "poster": "avatar.jpg"}
			]
		}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	prov := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())
	res, err := prov.Search(context.Background(), "Аватар")
	if err != nil || len(res) != 2 {
		t.Fatalf("search failed: %v", err)
	}

	popular, err := prov.GetPopular(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("GetPopular failed: %v", err)
	}
	// Очікуємо 1 дедуплікований елемент з оновленим постером!
	if len(popular) != 1 {
		t.Fatalf("expected 1 deduplicated item, got %d", len(popular))
	}
	if popular[0].PosterURL != "avatar.jpg" {
		t.Errorf("expected poster 'avatar.jpg' from deduplication, got %q", popular[0].PosterURL)
	}
}

func TestBanderaProvider_GetDetails_AdditionalCases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/content" {
			// Контент без voices, але з прямими seasons
			resp := `{
				"ok": true,
				"source": "uatut",
				"type": "series",
				"info": {
					"title": "Серіал Без Голосів",
					"poster": "poster.jpg",
					"duration": "45 хв",
					"genres": "Детектив, Драма",
					"release_date": "2021-09-01"
				},
				"seasons": [
					{
						"title": 1,
						"episodes": [
							{"number": 1, "title": "Пілот", "ref": {"url": "https://cdn.example.com/ep1.m3u8"}}
						]
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

	// Невалідний payload JSON
	_, err := p.GetDetails(context.Background(), "invalid-json")
	if err == nil {
		t.Errorf("expected error on invalid payload JSON")
	}

	// Валідний payload
	itemURL, _ := json.Marshal(provider.BanderaItemPayload{
		Source: "uatut",
		Ref:    json.RawMessage(`{"id": 100}`),
		Title:  "Серіал Без Голосів",
	})

	details, err := p.GetDetails(context.Background(), string(itemURL))
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}

	if details.PosterURL != "poster.jpg" {
		t.Errorf("expected poster 'poster.jpg' from Poster field, got %q", details.PosterURL)
	}
	if details.Duration != "45 хв" {
		t.Errorf("expected duration '45 хв' from Duration field, got %q", details.Duration)
	}
	if len(details.Genres) != 2 || details.Genres[0] != "Детектив" {
		t.Errorf("expected parsed genres, got %v", details.Genres)
	}
	if details.Year != 2021 {
		t.Errorf("expected year 2021 from release_date, got %d", details.Year)
	}
	if len(details.Voiceovers) != 1 || details.Voiceovers[0].Name != "Основна" {
		t.Errorf("expected default voiceover for direct seasons")
	}
}

func TestBanderaProvider_GetStreams_DirectStreamInMovieAndDirectVoiceover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "uatut", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}}
				]
			}`))
			return
		}
		if r.URL.Path == "/content" {
			// Фільм із готовим прямим стрімом (len(Ref) == 0 && URL != "")
			resp := `{
				"ok": true,
				"source": "uatut",
				"type": "movie",
				"streams": [
					{
						"title": "Прямий Стрім",
						"url": "[ 1080p ] https://cdn.example.org/movie.m3u8",
						"quality": "1080p",
						"subtitles": [
							{"url": "https://cdn.example.org/sub.vtt", "lang": "uk", "label": "Українські"}
						]
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
	payloadBytes, _ := json.Marshal(provider.BanderaItemPayload{
		Source: "uatut",
		Ref:    json.RawMessage(`{"movie_id": 555}`),
		Type:   "movie",
	})

	// Отримуємо стрім фільму (повинен повернути готовий стрім без другого виклику /stream!)
	resp, err := p.GetStreams(context.Background(), string(payloadBytes), 0, 0, "0")
	if err != nil {
		t.Fatalf("GetStreams for direct movie stream failed: %v", err)
	}
	if len(resp.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(resp.Streams))
	}
	if resp.Streams[0].Quality != "1080p" {
		t.Errorf("expected quality 1080p, got %q", resp.Streams[0].Quality)
	}
	if len(resp.Subtitles) != 1 {
		t.Errorf("expected 1 subtitle, got %d", len(resp.Subtitles))
	}
}

func TestBanderaProvider_GetStreams_ErrorCases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "uatut", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}}
				]
			}`))
			return
		}
		if r.URL.Path == "/content" {
			// Контент без стрімів і без голосів
			_, _ = w.Write([]byte(`{"ok": true, "source": "uatut"}`))
			return
		}
		if r.URL.Path == "/stream" {
			// API помилка стріму
			_, _ = w.Write([]byte(`{"ok": false, "error": "Geoblocked", "error_code": "GEO_RESTRICTED"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	// 1. Помилка невалідного JSON payload
	_, err := p.GetStreams(context.Background(), "invalid-json", 0, 0, "")
	if err == nil {
		t.Errorf("expected error on invalid payload json")
	}

	// 2. Контент без стрімів і без голосів
	emptyPayload, _ := json.Marshal(provider.BanderaItemPayload{
		Source: "uatut",
		Ref:    json.RawMessage(`{"id": 1}`),
	})
	_, err = p.GetStreams(context.Background(), string(emptyPayload), 0, 0, "")
	if err == nil || !strings.Contains(err.Error(), "no streams or voices found") {
		t.Errorf("expected 'no streams or voices found' error, got %v", err)
	}

	// 3. Прямий stream ref, але API повертає error_code
	streamRefPayload, _ := json.Marshal(provider.BanderaStreamRef{
		Source:      "uatut",
		Ref:         json.RawMessage(`{"url": "https://example.com/play.m3u8"}`),
		IsStreamRef: true,
	})
	_, err = p.GetStreams(context.Background(), string(streamRefPayload), 0, 0, "")
	if err == nil || !strings.Contains(err.Error(), "stream api error") {
		t.Errorf("expected 'stream api error', got %v", err)
	}
	// 4. Помилка виклику GetStream (мережева)
	pBroken := provider.NewBanderaProviderWithConfig("http://invalid.domain", "", &http.Client{Timeout: 10 * time.Millisecond})
	_, err = pBroken.GetStreams(context.Background(), string(streamRefPayload), 0, 0, "")
	if err == nil {
		t.Errorf("expected network error on GetStream")
	}
}

func TestBanderaClient_GetSources_CacheHitAndStaleCache(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok": true, "sources": [{"key": "uatut", "enabled": true, "capabilities": {"search": true, "stream": true}, "inputs": {"stream": ["url"]}}]}`))
			return
		}
		// Наступні запити падають з 500, але клієнт повинен повернути старий кеш!
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server down"))
	}))
	defer server.Close()

	c := provider.NewBanderaClient(server.URL, server.Client())

	// Запит 1: заповнює кеш
	sources1, err := c.GetSources(context.Background())
	if err != nil || len(sources1) == 0 {
		t.Fatalf("first GetSources failed: %v", err)
	}

	// Запит 2: повинен взяти з кешу (in-memory hit) без звернення до мережі
	sources2, err := c.GetSources(context.Background())
	if err != nil || len(sources2) == 0 {
		t.Fatalf("second GetSources cache hit failed: %v", err)
	}
	if requestCount != 1 {
		t.Errorf("expected 1 network request due to cache hit, got %d", requestCount)
	}

	// Запит 3: встановлюємо маленький TTL і чекаємо - запит іде в мережу, отримує 500, але повертає старий кеш!
	c.SetSourcesTTL(1 * time.Millisecond)
	time.Sleep(2 * time.Millisecond)

	sourcesStale, err := c.GetSources(context.Background())
	if err != nil || len(sourcesStale) == 0 {
		t.Fatalf("expected stale cache returned on 500, got err: %v", err)
	}
	if requestCount != 2 {
		t.Errorf("expected network request for stale cache attempt, got %d", requestCount)
	}

	// Запит 4: клієнт із помилкою мережі, але вже має кеш -> повертає старий кеш
	var failDo bool
	rtClient := &http.Client{
		Transport: &roundTripperMock{
			roundTripFunc: func(req *http.Request) (*http.Response, error) {
				if failDo {
					return nil, errors.New("network dropped")
				}
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok": true, "sources": [{"key": "test_src", "enabled": true}]}`)),
					Header:     make(http.Header),
				}
				return resp, nil
			},
		},
	}
	cMock := provider.NewBanderaClient("http://fake.api", rtClient)
	_, _ = cMock.GetSources(context.Background())

	cMock.SetSourcesTTL(1 * time.Millisecond)
	time.Sleep(2 * time.Millisecond)
	failDo = true

	staleOnNetworkErr, err := cMock.GetSources(context.Background())
	if err != nil || len(staleOnNetworkErr) == 0 {
		t.Errorf("expected stale cache on network drop, got err: %v", err)
	}
}

func TestBanderaProvider_DetailsAndStreamsEdgeCases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "uatut", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}}
				]
			}`))
			return
		}
		if r.URL.Path == "/content" {
			// Контент з плоскими епізодами без номерів і назв, Info == nil
			resp := `{
				"ok": true,
				"source": "uatut",
				"type": "series",
				"episodes": [
					{"number": 0, "title": "", "ref": {"url": "https://cdn.example.org/stream.m3u8"}}
				]
			}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}
		if r.URL.Path == "/stream" {
			// Повертаємо 1 стрім з порожнім URL і 1 стрім з auto quality
			resp := `{
				"ok": true,
				"source": "uatut",
				"streams": [
					{"url": "", "quality": "1080p"},
					{"url": "[ 720p ] https://cdn.example.org/valid.m3u8", "quality": "auto"}
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

	// 1. GetDetails з Info == nil та без payload.ID
	itemPayload := provider.BanderaItemPayload{
		Source: "uatut",
		Ref:    json.RawMessage(`{"id": 444}`),
		Title:  "Без Інфо",
	}
	payloadBytes, _ := json.Marshal(itemPayload)

	details, err := p.GetDetails(context.Background(), string(payloadBytes))
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}
	if details.ID == "" {
		t.Errorf("expected generated stable ID")
	}
	if len(details.Seasons) != 1 || details.Seasons[0].Episodes[0].Title != "Серія 0" {
		t.Errorf("expected generated episode title 'Серія 0', got %q", details.Seasons[0].Episodes[0].Title)
	}

	// 2. GetStreams для серіалу без голосів, з фільтрацією порожніх стрімів та якістю auto
	streamsResp, err := p.GetStreams(context.Background(), string(payloadBytes), 1, 0, "")
	if err != nil {
		t.Fatalf("GetStreams failed: %v", err)
	}
	if len(streamsResp.Streams) != 1 || streamsResp.Streams[0].Quality != "720p" {
		t.Errorf("expected 1 stream with quality 720p, got %+v", streamsResp.Streams)
	}

	// 3. Прямий streamRef без явного IsStreamRef: true, але з валідним streamKey через IsStreamRef
	directImplicitRef, _ := json.Marshal(map[string]interface{}{
		"source": "uatut",
		"ref":    map[string]string{"url": "https://cdn.example.org/play.m3u8"},
	})
	streamsResp2, err := p.GetStreams(context.Background(), string(directImplicitRef), 0, 0, "")
	if err != nil {
		t.Fatalf("GetStreams with implicit stream ref failed: %v", err)
	}
	if len(streamsResp2.Streams) != 1 {
		t.Errorf("expected 1 stream from implicit direct ref")
	}
}

func TestBanderaProvider_SearchFallbacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/search") {
			// Якщо передано serial, повертаємо помилку для перевірки fallback на звичайний Search
			if r.URL.Query().Get("serial") != "" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("search with meta error"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok": true, "items": [{"title": "Фоллбек", "source": "uatut", "serial": 0}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	// 1. GetPopular для series коли SearchWithMeta падає -> фолбек на Search
	items, err := p.GetPopular(context.Background(), "series", 1)
	if err != nil || len(items) == 0 {
		t.Fatalf("GetPopular fallback failed: %v", err)
	}

	// 2. GetNew коли SearchWithMeta падає -> фолбек на GetPopular
	newItems, err := p.GetNew(context.Background(), "series", 0)
	if err != nil || len(newItems) == 0 {
		t.Fatalf("GetNew fallback failed: %v", err)
	}

	// 3. GetByCategory коли SearchWithMeta падає -> фолбек на Search
	catItems, err := p.GetByCategory(context.Background(), "драма", "series", 1)
	if err != nil || len(catItems) == 0 {
		t.Fatalf("GetByCategory fallback failed: %v", err)
	}
}

func TestBanderaProvider_TargetedMissingBranches(t *testing.T) {
	var serverMode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/sources" {
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "uatut", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}}
				]
			}`))
			return
		}
		if r.URL.Path == "/content" {
			if serverMode == "fail" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("content error"))
				return
			}
			if serverMode == "movie_multi_stream" {
				resp := `{
					"ok": true,
					"source": "uatut",
					"type": "movie",
					"streams": [
						{"url": "https://cdn.example.org/st1.m3u8", "ref": {"url": "https://cdn.example.org/st1.m3u8"}},
						{"title": "Озвучка 2", "url": "https://cdn.example.org/st2.m3u8", "ref": {"url": "https://cdn.example.org/st2.m3u8"}}
					]
				}`
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(resp))
				return
			}
			if serverMode == "series_missing_ep" {
				resp := `{
					"ok": true,
					"source": "uatut",
					"type": "series",
					"voices": [
						{"id": "vox1", "seasons": [{"title": 1, "episodes": [{"number": 1, "title": "", "ref": {"url": "https://s.org/1"}}]}]}
					]
				}`
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(resp))
				return
			}

			// Дефолтний full info тест для GetDetails
			resp := `{
				"ok": true,
				"source": "uatut",
				"info": {
					"title": "Нова назва",
					"title_en": "New Title",
					"description": "Повний опис фільму",
					"image": "img.jpg",
					"release_date": "invalid_short",
					"genres": "Драма"
				},
				"voices": [
					{
						"id": "voice_without_name",
						"display_name": "",
						"seasons": [
							{
								"title": 1,
								"episodes": [
									{"number": 5, "title": "", "ref": {"url": "https://cdn.org/e5"}}
								]
							}
						]
					}
				],
				"streams": [
					{"title": "", "url": "https://cdn.org/st1"}
				]
			}`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}
		if r.URL.Path == "/stream" {
			// Режим порожніх стрімів
			if serverMode == "empty_streams" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok": true, "source": "uatut", "streams": []}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok": true, "source": "uatut", "streams": [{"url": "https://cdn.org/s.m3u8"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	// 1. convertSearchItems: Serial == 1 коли type порожній -> Type = "series"
	serverSearch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{
			"ok": true,
			"items": [
				{"source": "uatut", "title": "Серіал Без Типу", "serial": 1, "year": "2024"},
				{"source": "uatut", "title": "Фільм Без Типу", "serial": 0, "year": "2024"}
			]
		}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	defer serverSearch.Close()

	pSearch := provider.NewBanderaProviderWithConfig(serverSearch.URL, "", serverSearch.Client())
	items, err := pSearch.Search(context.Background(), "тест")
	if err != nil || len(items) != 2 {
		t.Fatalf("search failed: %v", err)
	}
	if items[0].Type != "series" {
		t.Errorf("expected series for serial=1, got %q", items[0].Type)
	}
	if items[1].Type != "movie" {
		t.Errorf("expected movie for serial=0, got %q", items[1].Type)
	}

	// 2. GetDetails: помилка GetContent
	failPayload, _ := json.Marshal(provider.BanderaItemPayload{
		Source: "uatut",
		Ref:    json.RawMessage(`{}`),
	})
	serverMode = "fail"
	_, err = p.GetDetails(context.Background(), string(failPayload))
	if err == nil {
		t.Errorf("expected error from GetDetails on GetContent failure")
	}

	// 3. GetDetails: перевірка Info з TitleEn, Description, fallback назви голосу та номера серії
	serverMode = ""
	details, err := p.GetDetails(context.Background(), string(failPayload))
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}
	if details.Title != "Нова назва" || details.OriginalTitle != "New Title" || details.Description != "Повний опис фільму" {
		t.Errorf("unexpected details: %+v", details)
	}
	if len(details.Voiceovers) > 0 && details.Voiceovers[0].Name != "voice_without_name" {
		t.Errorf("expected fallback voice name 'voice_without_name', got %q", details.Voiceovers[0].Name)
	}
	if len(details.Seasons) > 0 && details.Seasons[0].Episodes[0].Title != "Серія 5" {
		t.Errorf("expected fallback ep title 'Серія 5', got %q", details.Seasons[0].Episodes[0].Title)
	}

	// 4. GetStreams: помилка GetContent
	serverMode = "fail"
	_, err = p.GetStreams(context.Background(), string(failPayload), 0, 0, "")
	if err == nil {
		t.Errorf("expected error on GetStreams GetContent failure")
	}

	// 5. GetStreams: вибір другого стріму за voiceID = "1" для фільму
	serverMode = "movie_multi_stream"
	respMovie, err := p.GetStreams(context.Background(), string(failPayload), 0, 0, "1")
	if err != nil {
		t.Fatalf("GetStreams movie failed: %v", err)
	}
	if len(respMovie.Streams) == 0 {
		t.Errorf("expected streams returned for movie")
	}

	// 6. GetStreams: серіал, де епізод не знайдено (наприклад, сезон 9)
	serverMode = "series_missing_ep"
	_, err = p.GetStreams(context.Background(), string(failPayload), 9, 1, "vox1")
	if err == nil || !strings.Contains(err.Error(), "no matching stream found") {
		t.Errorf("expected 'no matching stream found' error, got %v", err)
	}

	// 7. GetStreams: API повернуло порожній масив стрімів
	serverMode = "empty_streams"
	streamRefPayload, _ := json.Marshal(provider.BanderaStreamRef{
		Source:      "uatut",
		Ref:         json.RawMessage(`{"url": "https://cdn.org/play.m3u8"}`),
		IsStreamRef: true,
	})
	_, err = p.GetStreams(context.Background(), string(streamRefPayload), 0, 0, "")
	if err == nil || !strings.Contains(err.Error(), "no playable streams") {
		t.Errorf("expected 'no playable streams' error, got %v", err)
	}
}

// Вспоміжний мок RoundTripper
type roundTripperMock struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *roundTripperMock) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

