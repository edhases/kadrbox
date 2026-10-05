package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

// GetStreams parsed voiceID with strconv.Atoi in two separate branches. When
// the parse failed, the selection branch silently left streamIdx at 0, so a
// client that picked dub #2 received source #1 with no error anywhere. The
// direct-URL branch had the mirror-image bug, collapsing the whole list down to
// a single entry. Both now reject an unusable voiceID instead of guessing,
// which is the difference between an honest 4xx and silently wrong playback.

// movieWithTwoDirectSources answers a movie card carrying two ready-to-play
// direct sources. Neither has a `ref`, so GetStreams takes the direct-URL
// branch and never calls /stream.
func movieWithTwoDirectSources() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sources":
			_, _ = w.Write([]byte(`{
				"ok": true,
				"sources": [
					{"key": "uatut", "enabled": true, "capabilities": {"stream": true}, "inputs": {"stream": ["url"]}}
				]
			}`))
		case "/content":
			_, _ = w.Write([]byte(`{
				"ok": true,
				"source": "uatut",
				"type": "movie",
				"streams": [
					{"title": "Джерело 1", "url": "[ 1080p ] https://cdn.example.org/one.m3u8", "quality": "1080p"},
					{"title": "Джерело 2", "url": "[ 720p ] https://cdn.example.org/two.m3u8", "quality": "720p"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func movieItemURL(t *testing.T) string {
	t.Helper()
	payload, err := json.Marshal(provider.BanderaItemPayload{
		Source: "uatut",
		Ref:    json.RawMessage(`{"id":1}`),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(payload)
}

func TestBanderaProvider_GetStreams_NonNumericVoiceIDIsRejected(t *testing.T) {
	srv := movieWithTwoDirectSources()
	defer srv.Close()
	p := provider.NewBanderaProviderWithConfig(srv.URL, "", srv.Client())

	resp, err := p.GetStreams(context.Background(), movieItemURL(t), 0, 0, "uatut")
	if err == nil {
		first := ""
		if len(resp.Streams) > 0 {
			first = resp.Streams[0].Voiceover
		}
		t.Fatalf("expected an error for a non-numeric voiceID, got %d stream(s), first voiceover %q",
			len(resp.Streams), first)
	}
	if !strings.Contains(err.Error(), "voiceID") {
		t.Errorf("error should name the offending parameter, got %q", err)
	}
}

func TestBanderaProvider_GetStreams_OutOfRangeVoiceIDIsRejected(t *testing.T) {
	srv := movieWithTwoDirectSources()
	defer srv.Close()
	p := provider.NewBanderaProviderWithConfig(srv.URL, "", srv.Client())

	// Index 99 does not exist: the content has 2 sources. Falling back to index
	// 0 would hand the client a different dub than the one it asked for.
	if _, err := p.GetStreams(context.Background(), movieItemURL(t), 0, 0, "99"); err == nil {
		t.Fatal("expected an error for an out-of-range voiceID, got nil")
	}
}

func TestBanderaProvider_GetStreams_ValidVoiceIDSelectsThatSource(t *testing.T) {
	srv := movieWithTwoDirectSources()
	defer srv.Close()
	p := provider.NewBanderaProviderWithConfig(srv.URL, "", srv.Client())

	// Index 1 is the second source. Getting source 1 here is what proves the
	// fix: before it, every voiceID resolved to index 0.
	resp, err := p.GetStreams(context.Background(), movieItemURL(t), 0, 0, "1")
	if err != nil {
		t.Fatalf("GetStreams with voiceID=1 failed: %v", err)
	}
	if len(resp.Streams) != 1 {
		t.Fatalf("expected exactly the selected source, got %d", len(resp.Streams))
	}
	if got := resp.Streams[0].Voiceover; got != "Джерело 2" {
		t.Errorf("voiceover = %q, want %q — the requested source was not selected", got, "Джерело 2")
	}
	if got := resp.Streams[0].Quality; got != "720p" {
		t.Errorf("quality = %q, want 720p (the second source's quality)", got)
	}
}

func TestBanderaProvider_GetStreams_NoVoiceIDReturnsEveryDirectSource(t *testing.T) {
	srv := movieWithTwoDirectSources()
	defer srv.Close()
	p := provider.NewBanderaProviderWithConfig(srv.URL, "", srv.Client())

	// The empty case must keep working: no voiceID means "give me the list".
	resp, err := p.GetStreams(context.Background(), movieItemURL(t), 0, 0, "")
	if err != nil {
		t.Fatalf("GetStreams without voiceID failed: %v", err)
	}
	if len(resp.Streams) != 2 {
		t.Fatalf("expected both sources, got %d", len(resp.Streams))
	}
}
