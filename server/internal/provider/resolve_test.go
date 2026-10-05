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
	"encoding/json"
	"errors"
	"fmt"
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

// Третій аргумент — назва СТУДІЇ озвучення, а не ім'я CDN.
// Раніше тут передавалося "Ashdi", і тест цим закриплював баг:
// Voiceover не має права містити ім'я балансера.
func TestParseMultiQualityString(t *testing.T) {
	raw := "[1080p]https://cdn.example/video_1080.m3u8,[720p]https://cdn.example/video_720.m3u8,[480p]https://cdn.example/video_480.m3u8"
	streams := parseMultiQualityString(raw, "https://ashdi.vip/vod/123", "1+1")
	if len(streams) != 3 {
		t.Fatalf("expected 3 streams, got %d", len(streams))
	}
	if streams[0].Quality != "1080p" || streams[0].Player != "Ashdi" || streams[0].Voiceover != "1+1" {
		t.Errorf("stream 0 mismatch: %+v", streams[0])
	}
	if streams[1].Quality != "720p" {
		t.Errorf("stream 1 quality mismatch: %s", streams[1].Quality)
	}
	if streams[2].Quality != "480p" {
		t.Errorf("stream 2 quality mismatch: %s", streams[2].Quality)
	}

	// Test " or " separator
	rawOr := "[FullHD]https://cdn.example/1080.mp4 or [HD]https://cdn.example/720.mp4"
	streamsOr := parseMultiQualityString(rawOr, "https://hdvbua.pro/embed/1", "HDVB")
	if len(streamsOr) != 2 {
		t.Fatalf("expected 2 streams for 'or' format, got %d", len(streamsOr))
	}
	if streamsOr[0].Quality != "1080p" || streamsOr[1].Quality != "720p" {
		t.Errorf("unexpected qualities: %s, %s", streamsOr[0].Quality, streamsOr[1].Quality)
	}
}

func TestParseSubtitlesFromPlayerHTML(t *testing.T) {
	html := `<html><body>
	<script>
	var player = new Playerjs({
		file: "https://cdn.example/master.m3u8",
		subtitle: "[Українська]https://cdn.example/subs/uk.vtt,[English]https://cdn.example/subs/en.vtt"
	});
	</script>
	<video>
		<track kind="subtitles" src="https://cdn.example/subs/pl.vtt" label="Польська" srclang="pl">
	</video>
	</body></html>`

	subs := parseSubtitlesFromPlayerHTML(html)
	if len(subs) != 3 {
		t.Fatalf("expected 3 subtitles, got %d", len(subs))
	}
	if subs[0].Label != "Українська" || subs[0].URL != "https://cdn.example/subs/uk.vtt" {
		t.Errorf("sub 0 mismatch: %+v", subs[0])
	}
	// Мова має бути канонічним кодом, а не тією ж міткою. Раніше
	// Label і Language були одним рядком, тобто Language дорівнював
	// «Українська», і клієнт не міг її порівняти з «uk».
	if subs[0].Language != "uk" {
		t.Errorf("sub 0 Language = %q, want uk", subs[0].Language)
	}
	if subs[1].Label != "English" || subs[1].URL != "https://cdn.example/subs/en.vtt" {
		t.Errorf("sub 1 mismatch: %+v", subs[1])
	}
	if subs[2].Label != "Польська" || subs[2].URL != "https://cdn.example/subs/pl.vtt" {
		t.Errorf("sub 2 mismatch: %+v", subs[2])
	}
}

func TestExtractStreamsFromPlaylistTree(t *testing.T) {
	playlistJSON := `[
		{
			"title": "1+1 (Дубляж)",
			"folder": [
				{
					"title": "1 сезон",
					"folder": [
						{"title": "1 серія", "file": "[1080p]https://cdn.example/s1e1_1080.m3u8,[720p]https://cdn.example/s1e1_720.m3u8"},
						{"title": "2 серія", "file": "https://cdn.example/s1e2.m3u8"}
					]
				}
			]
		},
		{
			"title": "Цікава Ідея",
			"folder": [
				{
					"title": "1 сезон",
					"folder": [
						{"title": "1 серія", "file": "https://cdn.example/ci_s1e1.m3u8"}
					]
				}
			]
		}
	]`

	var items []playerJSPlaylistItem
	if err := json.Unmarshal([]byte(playlistJSON), &items); err != nil {
		t.Fatalf("unmarshal playlist failed: %v", err)
	}

	// 1. Season 1, Episode 1 (all voices if voiceID empty)
	streams, _ := extractStreamsFromPlaylistTree(items, "https://ashdi.vip/serial/1", "Ashdi", 1, 1, "")
	if len(streams) != 3 { // 2 qualities from 1+1, 1 from Цікава Ідея
		t.Fatalf("expected 3 streams for s1e1 across voices, got %d", len(streams))
	}
	if !strings.Contains(streams[0].Voiceover, "1+1") {
		t.Errorf("expected 1+1 voiceover, got %q", streams[0].Voiceover)
	}

	// 2. Specific voice: "Цікава Ідея"
	streamsVoice, _ := extractStreamsFromPlaylistTree(items, "https://ashdi.vip/serial/1", "Ashdi", 1, 1, "Цікава Ідея")
	if len(streamsVoice) != 1 {
		t.Fatalf("expected 1 stream for Цікава Ідея, got %d", len(streamsVoice))
	}
	if streamsVoice[0].URL != "https://cdn.example/ci_s1e1.m3u8" {
		t.Errorf("unexpected URL: %s", streamsVoice[0].URL)
	}
}

func TestRankPlayerCandidates_TabsAndSelect(t *testing.T) {
	html := `<html><body>
		<ul class="player-tabs">
			<li data-src="https://ashdi.vip/vod/101">Плеєр 1 (Ashdi)</li>
			<li data-player="https://hdvbua.pro/embed/202">Плеєр 2 (HDVB - Дубляж 1+1)</li>
		</ul>
		<select id="player_select">
			<option value="https://zenith.media/embed/303">Zenith</option>
		</select>
		<iframe id="main_frame" src="https://videohost.org/video/404" title="Videohost"></iframe>
		<iframe src="https://google.com/recaptcha/api"></iframe>
	</body></html>`

	candidates := rankPlayerCandidates(html, "https://lavakino.net/filmys/1-test.html")
	if len(candidates) != 4 {
		t.Fatalf("expected 4 player candidates, got %d", len(candidates))
	}

	urls := make([]string, len(candidates))
	for i, c := range candidates {
		urls[i] = c.URL
	}

	// Verify all 4 players are discovered and recaptcha skipped
	expectedHosts := []string{"ashdi.vip", "hdvbua.pro", "zenith.media", "videohost.org"}
	for _, expected := range expectedHosts {
		found := false
		for _, u := range urls {
			if strings.Contains(u, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected host %q in candidates, but got %v", expected, urls)
		}
	}
}

func TestResolveStreamsFromItemPage_MultiplePlayers(t *testing.T) {
	// Setup 4 mock player servers (e.g. Lavakino "Величне століття: Роксолана" with 4 players)
	srvAshdi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><script>
		var player = new Playerjs({
			file: "[1080p]https://cdn.ashdi.vip/1080.m3u8,[720p]https://cdn.ashdi.vip/720.m3u8",
			subtitle: "[Українська]https://cdn.ashdi.vip/sub_uk.vtt"
		});
		</script></body></html>`))
	}))
	defer srvAshdi.Close()

	srvZenith := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><script>
		Hls.loadSource("https://cdn.zenith.media/master.m3u8");
		</script></body></html>`))
	}))
	defer srvZenith.Close()

	srvHDVB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><script>
		var p = {sources: [
			{src: "https://cdn.hdvb.pro/1080p.m3u8", label: "1080p"},
			{src: "https://cdn.hdvb.pro/720p.m3u8", label: "720p"}
		]};
		</script></body></html>`))
	}))
	defer srvHDVB.Close()

	srvVideohost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><script>
		var player = new Playerjs({file: "https://cdn.videohost.org/video.mp4"});
		</script></body></html>`))
	}))
	defer srvVideohost.Close()

	// Item page containing 4 player iframes with tabs
	itemPageHTML := fmt.Sprintf(`<html><body>
		<div class="tabs-box">
			<iframe src="%s/embed/ashdi" title="Ashdi"></iframe>
			<iframe src="%s/embed/zenith" title="Zenith"></iframe>
			<iframe src="%s/embed/hdvb" title="HDVB"></iframe>
			<iframe src="%s/embed/videohost" title="Videohost"></iframe>
		</div>
	</body></html>`, srvAshdi.URL, srvZenith.URL, srvHDVB.URL, srvVideohost.URL)

	tls := covTLS(t)
	resp, err := resolveStreamsFromItemPage(context.Background(), tls, "lavakino", "https://lavakino.net/filmys/100-velychne-stolittya.html", itemPageHTML, 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error resolving 4 players: %v", err)
	}

	// Ashdi gave 2, Zenith gave 1, HDVB gave 2, Videohost gave 1 -> total 6 streams!
	if len(resp.Streams) != 6 {
		t.Fatalf("expected 6 streams from 4 players, got %d", len(resp.Streams))
	}

	// Verify subtitle merged from Ashdi
	if len(resp.Subtitles) != 1 || resp.Subtitles[0].Label != "Українська" {
		t.Errorf("unexpected subtitles: %+v", resp.Subtitles)
	}

	// Verify all 4 player balancers are represented in streams
	foundAshdi := false
	foundZenith := false
	foundHDVB := false
	foundVideohost := false

	for _, s := range resp.Streams {
		if strings.Contains(s.Player, "Ashdi") {
			foundAshdi = true
		}
		if strings.Contains(s.Player, "Zenith") {
			foundZenith = true
		}
		if strings.Contains(s.Player, "HDVB") {
			foundHDVB = true
		}
		if strings.Contains(s.Player, "Videohost") {
			foundVideohost = true
		}
	}

	if !foundAshdi || !foundZenith || !foundHDVB || !foundVideohost {
		t.Errorf("all 4 players must be present in resolved streams! Ashdi=%v, Zenith=%v, HDVB=%v, Videohost=%v",
			foundAshdi, foundZenith, foundHDVB, foundVideohost)
	}
}

