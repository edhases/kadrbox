package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

// Ці тести закривають регресію B7, знайдену аудитом на ЖИВОМУ
// трафіку 2026-10-05:
//
//	{"ok":true,"streams":[{"url":"https://ashdi.vip/…","quality":"auto"}]}
//
// давало q="auto" (з малої літери, замість канонічного "Auto") та
// voice="mikai" — тобто Voiceover містив КЛЮЧ ДЖЕРЕЛА агрегатора, а
// не студію озвучення.

// banderaStub піднімає мінімальний сервер агрегатора: /sources і
// /content. /stream не потрібен — для фільму Bandera бере потік
// прямо з /content.
func banderaStub(t *testing.T, quality string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/sources":
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "mikai", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}},
					{"key": "uaflix", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}}
				]
			}`))
		case "/content":
			_, _ = w.Write([]byte(`{
				"ok": true,
				"source": "mikai",
				"type": "movie",
				"streams": [
					{"title": "Рідний Голос", "url": "https://ashdi.vip/vod/1/hls/index.m3u8", "quality": "` + quality + `"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func banderaMoviePayload(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(provider.BanderaItemPayload{
		Source: "mikai",
		Ref:    json.RawMessage(`{"movie_id": 555}`),
		Type:   "movie",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(b)
}

// Voiceover не має права містити ключ джерела агрегатора.
// «mikai» — це джерело, не студія озвучення.
func TestBanderaGetStreams_VoiceoverIsNotTheSourceKey(t *testing.T) {
	server := banderaStub(t, "auto")
	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	// voiceID порожній: студію ми не знаємо, тож і не вигадуємо.
	resp, err := p.GetStreams(context.Background(), banderaMoviePayload(t), 0, 0, "")
	if err != nil {
		t.Fatalf("GetStreams: %v", err)
	}
	if len(resp.Streams) == 0 {
		t.Fatal("expected at least one stream")
	}

	for i, s := range resp.Streams {
		switch s.Voiceover {
		case "mikai", "uaflix", "filmix", "bambooua":
			t.Errorf("stream[%d].Voiceover = %q — this is an aggregator source key, not a dub", i, s.Voiceover)
		}
	}
}

// На шляху /content назва озвучки приходить із поля title, тож
// Voiceover законно дорівнює «Рідний Голос».
func TestBanderaGetStreams_ContentTitleBecomesVoiceover(t *testing.T) {
	server := banderaStub(t, "1080p")
	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	resp, err := p.GetStreams(context.Background(), banderaMoviePayload(t), 0, 0, "0")
	if err != nil {
		t.Fatalf("GetStreams: %v", err)
	}
	if len(resp.Streams) == 0 {
		t.Fatal("expected at least one stream")
	}
	for i, s := range resp.Streams {
		if s.Voiceover != "Рідний Голос" {
			t.Errorf("stream[%d].Voiceover = %q, want Рідний Голос from the content title", i, s.Voiceover)
		}
	}
}

// Нечисловий voiceID Bandera відкидає — і має відкидати, бо це
// не індекс джерела.
func TestBanderaGetStreams_RejectsNonNumericVoiceID(t *testing.T) {
	server := banderaStub(t, "1080p")
	p := provider.NewBanderaProviderWithConfig(server.URL, "", server.Client())

	if _, err := p.GetStreams(context.Background(), banderaMoviePayload(t), 0, 0, "1+1"); err == nil {
		t.Fatal("a non-numeric voiceID must be rejected, not silently accepted")
	}
}
