package provider

// Regression tests for the player-page resolver.
//
// The bug these lock down: three DLE providers returned the <iframe> player page
// as the stream URL. libmpv cannot demux text/html, so every playback failed with
// "Failed to recognize file format.". ResolverPlayerHTML must therefore NEVER
// hand back a non-media URL — it fails with ErrUnresolvablePlayer instead.

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// rePlayableURL mirrors what the client actually needs: a manifest/progressive
// file extension. Anything else (HTML, iframe, /embed pages) is unplayable.
var rePlayableURL = regexp.MustCompile(`\.(m3u8|mpd|mp4)$`)

func resolvePlayerServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// player config wrapping a real (absolute) media URL
func playerJSConfig(mediaURL string) string {
	return `<html><body><script>
var player = new Playerjs({
  file: "` + mediaURL + `",
  width: 720, height: 405, autoplay: 0
});
</script></body></html>`
}

func b64PlayerJSConfig(t *testing.T, mediaURL string, enc *base64.Encoding) string {
	t.Helper()
	encoded := enc.EncodeToString([]byte(playerJSConfig(mediaURL)))
	return `<html><body><script>var cfg = {"file":"` + encoded + `"};</script></body></html>`
}

func TestResolvedURLsAreAlwaysPlayable(t *testing.T) {
	const (
		playerJS = "https://cdn.example/hls/movie-1080p/master.m3u8"
		sources  = "https://cdn.example/hls/master.m3u8"
		hls      = "https://cdn.example/live/index.m3u8"
		bare     = "https://cdn.example/files/movie.mp4"
	)

	cases := []struct {
		name     string
		body     string
		wantURL  string
		strategy string
	}{
		{
			name:     "1 playerjs file",
			body:     playerJSConfig(playerJS),
			wantURL:  playerJS,
			strategy: "playerjs-file",
		},
		{
			name:     "2 sources block",
			body:     `<html><body><script>var p = {sources: [{src: "` + sources + `", label: "hls"}]};</script></body></html>`,
			wantURL:  sources,
			strategy: "sources-block",
		},
		{
			name:     "3 hls loadSource",
			body:     `<html><body><script>Hls.loadSource("` + hls + `"); Hls.attachMedia(vid);</script></body></html>`,
			wantURL:  hls,
			strategy: "hls-loadsource",
		},
		{
			name:     "5 bare media url",
			body:     `<html><body><a href="` + bare + `">download</a></body></html>`,
			wantURL:  bare,
			strategy: "bare-media-url",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := resolvePlayerServer(t, tc.body)
			const siteBase = "https://uakino.example/"

			src, err := ResolvePlayerHTML(context.Background(), covTLS(t), srv.URL+"/embed/123", siteBase)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !rePlayableURL.MatchString(src.URL) {
				t.Fatalf("resolver returned a NON-PLAYABLE url %q (iframe/html pages are unplayable)", src.URL)
			}
			if src.URL != tc.wantURL {
				t.Errorf("unexpected url: got %q want %q", src.URL, tc.wantURL)
			}
			if src.DirectURL != src.URL {
				t.Errorf("DirectURL must mirror URL, got %q", src.DirectURL)
			}
			if strings.Contains(src.URL, srv.URL) {
				t.Errorf("stream url must be the MEDIA url, not the player page: %q", src.URL)
			}
			if _, strategy, ok := mustExtract(t, tc.body); !ok || strategy != tc.strategy {
				t.Errorf("expected strategy %q, got %q (ok=%v)", tc.strategy, strategy, ok)
			}
		})
	}
}

func mustExtract(t *testing.T, body string) (string, string, bool) {
	t.Helper()
	raw, strategy, ok := extractPlayableURL(body)
	return raw, strategy, ok
}

func TestResolveStrategyOrderingPrefersHighestPriority(t *testing.T) {
	// The page satisfies strategies 1, 3 and 5 at once; strategy 1 must win.
	body := `<html><head><link rel="preload" href="https://cdn.example/bare.mp4"></head><body>
<script>Hls.loadSource("https://cdn.example/hls/low/index.m3u8");</script>
<script>var player = new Playerjs({file:"https://cdn.example/hls/best/master.m3u8"});</script>
</body></html>`

	raw, strategy, ok := mustExtract(t, body)
	if !ok {
		t.Fatal("expected a match")
	}
	if strategy != "playerjs-file" {
		t.Errorf("expected highest-priority strategy playerjs-file, got %q", strategy)
	}
	if raw != "https://cdn.example/hls/best/master.m3u8" {
		t.Errorf("unexpected url: %q", raw)
	}
}

func TestResolveUnresolvablePlayerReturnsNoStream(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "no media at all",
			body: `<html><body><div id="player"></div><script>var ads = "hello";</script></body></html>`,
		},
		{
			name: "player page url dressed up as a file",
			// Regression: this is exactly the shape that used to reach mpv.
			body: playerJSConfig("https://player.example.com/embed/12345"),
		},
		{
			name: "html fragment as file value",
			body: `<html><body><script>var player = new Playerjs({file:"<iframe src=\"//ads/x\"></iframe>"});</script></body></html>`,
		},
		{
			name: "protocol relative non media",
			body: `<html><body><script>var p = {sources: [{src: "/player/index.html"}]};</script></body></html>`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := resolvePlayerServer(t, tc.body)
			const playerURLSuffix = "/player/embed/123"

			src, err := ResolvePlayerHTML(context.Background(), covTLS(t), srv.URL+playerURLSuffix, "https://uakino.example/")
			if !errors.Is(err, ErrUnresolvablePlayer) {
				t.Fatalf("expected ErrUnresolvablePlayer, got %v", err)
			}
			if src.URL != "" || src.DirectURL != "" || len(src.Headers) != 0 {
				t.Fatalf("expected zero-value source, got %+v", src)
			}
			if strings.Contains(src.URL, srv.URL) {
				t.Fatalf("resolver MUST NOT return the iframe/player url: %q", src.URL)
			}
		})
	}
}

func TestResolvePlayerHTMLHeadersComeFromMediaOrigin(t *testing.T) {
	const mediaURL = "https://ukrtelcdn.example/hls/movie/master.m3u8"
	srv := resolvePlayerServer(t, playerJSConfig(mediaURL))
	const siteBase = "https://uakino.example/film/1-matrix.html"

	src, err := ResolvePlayerHTML(context.Background(), covTLS(t), srv.URL+"/embed/9", siteBase)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := src.Headers["Referer"]; got != "https://ukrtelcdn.example/" {
		t.Errorf("Referer must be the MEDIA origin with trailing slash, got %q", got)
	}
	if got := src.Headers["Origin"]; got != "https://ukrtelcdn.example" {
		t.Errorf("Origin must be the MEDIA origin without trailing slash, got %q", got)
	}
	if src.Headers["User-Agent"] != Chrome120UserAgent {
		t.Errorf("unexpected User-Agent: %q", src.Headers["User-Agent"])
	}
	if src.Headers["Referer"] == siteBase || src.Headers["Origin"] == siteBase {
		t.Error("headers must NOT reuse the aggregator site base url")
	}
	if strings.Count(src.Headers["Origin"], "/") != 2 {
		t.Errorf("Origin must be scheme://host only, got %q", src.Headers["Origin"])
	}
}

func TestResolveBase64WrappedPlayerJS(t *testing.T) {
	const mediaURL = "https://cdn.example/hls/series/480p/playlist.m3u8"

	cases := []struct {
		name string
		enc  *base64.Encoding
	}{
		{"std alphabet with padding", base64.StdEncoding},
		{"url-safe alphabet without padding", base64.RawURLEncoding},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body := b64PlayerJSConfig(t, mediaURL, tc.enc)
			srv := resolvePlayerServer(t, body)

			src, err := ResolvePlayerHTML(context.Background(), covTLS(t), srv.URL+"/embed/77", "https://eneyida.example/")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if src.URL != mediaURL {
				t.Fatalf("unexpected url: %q", src.URL)
			}
			if !rePlayableURL.MatchString(src.URL) {
				t.Fatalf("non-playable url: %q", src.URL)
			}
			if _, strategy, ok := mustExtract(t, body); !ok || strategy != "base64-playerjs" {
				t.Errorf("expected base64-playerjs strategy, got %q (ok=%v)", strategy, ok)
			}
		})
	}
}

func TestDecodeBase64Loose(t *testing.T) {
	raw := `{"file":"https://cdn.example/hls/master.m3u8"}`
	encoded := base64.RawURLEncoding.EncodeToString([]byte(raw)) // no '=', url-safe

	decoded, err := decodeBase64Loose(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded != raw {
		t.Errorf("unexpected decode result: %q", decoded)
	}
}

func TestRankPlayerIframesSkipsNonPlayerFrames(t *testing.T) {
	itemURL := "https://uakino.example/filmy/123-matrix.html"
	itemHTML := `<html><body>
<iframe src="about:blank" width="0" height="0"></iframe>
<iframe src="https://www.google.com/recaptcha/api2/anchor"></iframe>
<iframe src="https://ad.doubleclick.net/ads/creative.html"></iframe>
<iframe src="https://disqus.com/embed/comments/123"></iframe>
<iframe src="/some/promo/banner.html"></iframe>
<iframe src="//player.example.com/embed/123"></iframe>
</body></html>`

	got := rankPlayerIframes(itemHTML, itemURL)
	want := []string{"https://player.example.com/embed/123"}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("expected only the player iframe %v, got %v", want, got)
	}
}

func TestRankPlayerIframesPrefersCrossHostPlayer(t *testing.T) {
	itemURL := "https://uakino.example/serial/456.html"
	itemHTML := `<html><body>
<iframe src="/player/index.html"></iframe>
<iframe src="https://ashdi.example/embed/777"></iframe>
</body></html>`

	got := rankPlayerIframes(itemHTML, itemURL)
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %v", got)
	}
	if got[0] != "https://ashdi.example/embed/777" {
		t.Errorf("cross-host player iframe must rank first, got %v", got)
	}
	if got[1] != "https://uakino.example/player/index.html" {
		t.Errorf("relative iframe must resolve against the item url, got %v", got)
	}
}

func TestIsPlayableMediaURLRejectsPlayerPages(t *testing.T) {
	rejected := []string{
		"",
		"//cdn.example/hls/master.m3u8",        // protocol relative, not absolute
		"/hls/master.m3u8",                     // relative
		"https://player.example.com/embed/123", // no media extension
		"https://player.example.com/player/index/", // trailing slash
		"https://x.example/watch/12345",            // looks like a page
		"https://x.example/iframe?id=1",            // looks like a page
		"<iframe src=\"//ads/x\"></iframe>",        // html fragment
		"javascript:alert(1)",                      // non http scheme
		"data:text/html,<h1>hi</h1>",               // non http scheme
	}
	for _, raw := range rejected {
		if isPlayableMediaURL(raw) {
			t.Errorf("expected %q to be rejected as unplayable", raw)
		}
	}

	accepted := []string{
		"https://cdn.example/hls/master.m3u8",
		"https://cdn.example/hls/master.m3u8?token=abc",
		"https://cdn.example/dash/manifest.mpd",
		"https://cdn.example/files/movie.mp4",
		"https://cdn.example/player/hls/720p.m3u8", // media extension wins over the path heuristic
	}
	for _, raw := range accepted {
		if !isPlayableMediaURL(raw) {
			t.Errorf("expected %q to be accepted", raw)
		}
	}
}

func TestQualityFromURL(t *testing.T) {
	cases := map[string]string{
		"https://cdn.example/hls/movie-1080p/master.m3u8": "1080p",
		"https://cdn.example/hls/720/index.m3u8":          "720p",
		"https://cdn.example/hls/2160p/index.m3u8":        "2160p",
		"https://cdn.example/4k/master.m3u8":              "4K",
		"https://cdn.example/hls/master.m3u8":             "Auto",
		"https://cdn.example/hls/48000/index.m3u8":        "Auto",
	}
	for raw, want := range cases {
		if got := qualityFromURL(raw); got != want {
			t.Errorf("qualityFromURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

// windows-1251 decoding: golang.org/x/net/html does no charset detection, so the
// transport must transcode or every Cyrillic label match fails silently.
func TestDecodeBodyCharset(t *testing.T) {
	utf8Body := []byte(`<html><body><h1>Матриця</h1><span class="fi-label">Режисер:</span></body></html>`)
	if got, _ := decodeBody("", utf8Body); got != string(utf8Body) {
		t.Error("absent charset must stay byte identical")
	}
	if got, _ := decodeBody("text/html; charset=UTF-8", utf8Body); got != string(utf8Body) {
		t.Error("utf-8 charset must stay byte identical")
	}

	cp1251 := []byte{
		'<', 'h', 't', 'm', 'l', '>', '<', 'b', 'o', 'd', 'y', '>',
		0xCC, 0xE0, 0xF2, 0xF0, 0xE8, 0xF6, 0xFF, // "Матриця" in windows-1251
		'<', '/', 'b', 'o', 'd', 'y', '>', '<', '/', 'h', 't', 'm', 'l', '>',
	}
	decoded, err := decodeBody("text/html; charset=windows-1251", cp1251)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(decoded, "Матриця") {
		t.Errorf("windows-1251 body was not transcoded: %q", decoded)
	}

	// Unknown label must not break the request.
	unknown := []byte{0xCC, 0xE5}
	if got, err := decodeBody("text/html; charset=definitely-not-a-charset", unknown); err != nil || string(got) != string(unknown) {
		t.Errorf("unknown charset must fall back to raw bytes, got %q err=%v", got, err)
	}
}

func TestCharsetOf(t *testing.T) {
	cases := map[string]string{
		"":                                       "",
		"text/html":                              "",
		"text/html; charset=utf-8":               "utf-8",
		"text/html;charset=WINDOWS-1251":         "WINDOWS-1251",
		"text/html; charset=\"koi8-r\"":          "koi8-r",
		"text/html; bad; charset=cp1251":         "cp1251",
		"application/json; charset=windows-1252": "windows-1252",
	}
	for header, want := range cases {
		if got := charsetOf(header); got != want {
			t.Errorf("charsetOf(%q) = %q, want %q", header, got, want)
		}
	}
}
