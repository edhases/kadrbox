package provider

// Покриття парсерів провайдерів без зовнішньої мережі: in-package тест
// підміняє неекспортований baseURL на httptest-сервер з фіксованим HTML.
// Мережевий шар при цьому справжній (TLSClient), парсинг — реальний код.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func covTLS(t *testing.T) *TLSClient {
	t.Helper()
	c, err := NewTLSClient()
	if err != nil {
		t.Fatalf("failed to create TLS client: %v", err)
	}
	return c
}

func covFixtureServer(pages map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, ok := pages[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
}

// ---- UAKino ----

const covUakinoSearchHTML = `<html><body>
<div class="short-story">
  <h2 class="title"><a href="https://uakino.best/filmy/123-matrix.html">Матриця</a></h2>
  <div class="movie-poster"><img src="/posters/matrix.jpg"/></div>
  <span class="movie-date">1999 рік</span>
</div>
<div class="short-story">
  <h2 class="title"><a href="https://uakino.best/seriali/456-show.html">Тестовий серіал</a></h2>
  <div class="movie-poster"><img src="https://cdn.example/poster.jpg"/></div>
  <span class="movie-date">2021</span>
</div>
<div class="short-story">
  <h2 class="title"><a>Без посилання</a></h2>
</div>
</body></html>`

const covUakinoDetailsHTML = `<html><body>
<h1 class="movie-title">Матриця</h1>
<div class="full-poster"><img src="/posters/matrix_full.jpg"/></div>
<div class="full-text">Опис фільму про симуляцію.</div>
<a href="/genre/fantasy/">Фантастика</a>
<a href="/genre/action/">Бойовик</a>
<a href="/other/">Не жанр</a>
</body></html>`

const covPlayerFrameHTML = `<html><body><script>
var player = new Playerjs({file:"https://cdn.example/hls/movie-1080p/master.m3u8", width: 720});
</script></body></html>`

// Сторінка без розпізнаного медіа: резолвер обязан повернути ErrUnresolvablePlayer.
const covDeadPlayerFrameHTML = `<html><body><div id="player"></div><script>var t="no media here";</script></body></html>`

const covUakinoStreamsHTML = `<html><body>
<iframe src="https://disqus.com/embed/comments/1"></iframe>
<div class="player"><iframe src="/player/embed/123" allowfullscreen></iframe></div>
</body></html>`

func TestCovProviderUakinoStreamsFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{
		"/item/1":           covUakinoStreamsHTML,
		"/player/embed/123": covPlayerFrameHTML,
		"/item/2":           `<html><body>без плеєра</body></html>`,
		"/item/3":           `<html><body><iframe src="/player/dead"></iframe></body></html>`,
		"/player/dead":      covDeadPlayerFrameHTML,
	})
	defer srv.Close()
	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}

	resp, err := p.GetStreams(context.Background(), srv.URL+"/item/1", 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %+v", resp)
	}
	st := resp.Streams[0]

	// Регресія: раніше сюди повертався URL самого iframe (HTML), який libmpv не демодулює.
	if !rePlayableURL.MatchString(st.URL) {
		t.Fatalf("stream url must be a playable media url, got %q", st.URL)
	}
	if st.URL != "https://cdn.example/hls/movie-1080p/master.m3u8" {
		t.Errorf("unexpected stream URL: %q", st.URL)
	}
	if st.DirectURL != st.URL {
		t.Errorf("DirectURL must mirror URL, got %q", st.DirectURL)
	}
	if strings.Contains(st.URL, srv.URL) {
		t.Errorf("stream url must not be the local fixture/iframe page: %q", st.URL)
	}
	if st.Quality != "1080p" {
		t.Errorf("unexpected quality: %q", st.Quality)
	}
	if st.Headers["Referer"] != "https://cdn.example/" || st.Headers["Origin"] != "https://cdn.example" {
		t.Errorf("headers must be derived from the MEDIA origin, got %v", st.Headers)
	}
	if st.Headers["User-Agent"] != Chrome120UserAgent {
		t.Errorf("unexpected User-Agent: %v", st.Headers["User-Agent"])
	}

	// Без iframe: порожній список + ErrUnresolvablePlayer, а не HTML-сторінка.
	empty, err := p.GetStreams(context.Background(), srv.URL+"/item/2", 0, 0, "")
	if !errors.Is(err, ErrUnresolvablePlayer) {
		t.Fatalf("expected ErrUnresolvablePlayer without iframe, got %v", err)
	}
	if empty == nil || len(empty.Streams) != 0 {
		t.Fatalf("expected empty stream list, got %+v", empty)
	}

	// iframe є, але медіа не розпізнано — теж ErrUnresolvablePlayer.
	dead, err := p.GetStreams(context.Background(), srv.URL+"/item/3", 0, 0, "")
	if !errors.Is(err, ErrUnresolvablePlayer) {
		t.Fatalf("expected ErrUnresolvablePlayer for dead player, got %v", err)
	}
	if dead == nil || len(dead.Streams) != 0 {
		t.Fatalf("expected empty stream list, got %+v", dead)
	}
}

func TestCovProviderUakinoSearchFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{"/index.php": covUakinoSearchHTML})
	defer srv.Close()
	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}

	items, err := p.Search(context.Background(), "матриця")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items (third has no href), got %d", len(items))
	}

	movie := items[0]
	if movie.Title != "Матриця" || movie.Year != 1999 || movie.Type != "movie" {
		t.Errorf("unexpected movie: %+v", movie)
	}
	if movie.ProviderID != "uakino" || movie.ID != movie.URL {
		t.Errorf("unexpected ids: %+v", movie)
	}
	// Відносний постер доповнюється baseURL.
	if movie.PosterURL != srv.URL+"/posters/matrix.jpg" {
		t.Errorf("unexpected poster: %q", movie.PosterURL)
	}

	series := items[1]
	if series.Type != "series" {
		t.Errorf("expected series (serial in href + 'серіал' in title), got %+v", series)
	}
	if series.Year != 2021 {
		t.Errorf("unexpected year: %+v", series)
	}
	// Абсолютний постер не чіпається.
	if series.PosterURL != "https://cdn.example/poster.jpg" {
		t.Errorf("unexpected poster: %q", series.PosterURL)
	}
}

func TestCovProviderUakinoSearchEmpty(t *testing.T) {
	srv := covFixtureServer(map[string]string{"/index.php": `<html><body>нічого</body></html>`})
	defer srv.Close()
	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}

	items, err := p.Search(context.Background(), "xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected no items, got %+v", items)
	}
}

func TestCovProviderUakinoSearchError(t *testing.T) {
	srv := covFixtureServer(nil)
	defer srv.Close()
	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // скасований контекст роняє транспорт до парсингу
	if _, err := p.Search(ctx, "x"); err == nil ||
		!strings.Contains(err.Error(), "uakino search request") {
		t.Errorf("expected wrapped transport error, got %v", err)
	}
}

func TestCovProviderUakinoDetailsFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{"/item/1": covUakinoDetailsHTML})
	defer srv.Close()
	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}

	d, err := p.GetDetails(context.Background(), srv.URL+"/item/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Title != "Матриця" || d.ProviderID != "uakino" {
		t.Errorf("unexpected details: %+v", d.MediaItem)
	}
	if d.PosterURL != srv.URL+"/posters/matrix_full.jpg" {
		t.Errorf("unexpected poster: %q", d.PosterURL)
	}
	if !strings.Contains(d.Description, "симуляцію") {
		t.Errorf("unexpected description: %q", d.Description)
	}
	if len(d.Genres) != 2 || d.Genres[0] != "Фантастика" || d.Genres[1] != "Бойовик" {
		t.Errorf("unexpected genres: %v", d.Genres)
	}
}

// ---- Eneyida ----

const covEneyidaSearchHTML = `<html><body>
<article class="short">
  <h2 class="short_title"><a href="https://eneyida.tv/123">Дюна</a></h2>
  <div class="short_img"><img src="/img/duna.jpg"/></div>
</article>
</body></html>`

const covEneyidaDetailsHTML = `<html><body>
<h1 class="full-title">Дюна</h1>
<div class="full-poster"><img src="https://cdn.example/duna.jpg"/></div>
<div class="full-text">Опис епопеї.</div>
</body></html>`

const covEneyidaStreamsHTML = `<html><body>
<iframe src="about:blank"></iframe>
<iframe src="https://ad.doubleclick.net/ads/creative.html"></iframe>
<iframe src="/player/embed/7"></iframe>
</body></html>`

func TestCovProviderEneyidaSearchFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{"/index.php": covEneyidaSearchHTML})
	defer srv.Close()
	p := &EneyidaProvider{client: covTLS(t), baseURL: srv.URL}

	items, err := p.Search(context.Background(), "дюна")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %+v", items)
	}
	it := items[0]
	if it.Title != "Дюна" || it.Type != "movie" || it.ProviderID != "eneyida" {
		t.Errorf("unexpected item: %+v", it)
	}
	if it.PosterURL != srv.URL+"/img/duna.jpg" {
		t.Errorf("unexpected poster: %q", it.PosterURL)
	}
}

func TestCovProviderEneyidaDetailsFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{"/item/7": covEneyidaDetailsHTML})
	defer srv.Close()
	p := &EneyidaProvider{client: covTLS(t), baseURL: srv.URL}

	d, err := p.GetDetails(context.Background(), srv.URL+"/item/7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Title != "Дюна" || !strings.Contains(d.Description, "епопеї") {
		t.Errorf("unexpected details: %+v", d)
	}
	if d.PosterURL != "https://cdn.example/duna.jpg" {
		t.Errorf("unexpected poster: %q", d.PosterURL)
	}
}

func TestCovProviderEneyidaStreamsFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{
		"/item/7":         covEneyidaStreamsHTML,
		"/player/embed/7": covPlayerFrameHTML,
		"/item/8":         `<html><body>без плеєра</body></html>`,
		"/item/9":         `<html><body><iframe src="/player/dead"></iframe></body></html>`,
		"/player/dead":    covDeadPlayerFrameHTML,
	})
	defer srv.Close()
	p := &EneyidaProvider{client: covTLS(t), baseURL: srv.URL}

	resp, err := p.GetStreams(context.Background(), srv.URL+"/item/7", 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Streams) != 1 || resp.ProviderID != "eneyida" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	st := resp.Streams[0]
	// Регресія: раніше повертався URL iframe-сторінки (HTML), а не медіа.
	if !rePlayableURL.MatchString(st.URL) {
		t.Fatalf("stream url must be a playable media url, got %q", st.URL)
	}
	if st.URL != "https://cdn.example/hls/movie-1080p/master.m3u8" {
		t.Errorf("unexpected stream: %+v", st)
	}
	if strings.Contains(st.URL, srv.URL) {
		t.Errorf("stream url must not be the fixture/iframe page: %q", st.URL)
	}
	if st.Headers["Referer"] != "https://cdn.example/" || st.Headers["Origin"] != "https://cdn.example" {
		t.Errorf("headers must be derived from the MEDIA origin, got %v", st.Headers)
	}

	for _, path := range []string{"/item/8", "/item/9"} {
		dead, err := p.GetStreams(context.Background(), srv.URL+path, 0, 0, "")
		if !errors.Is(err, ErrUnresolvablePlayer) {
			t.Fatalf("%s: expected ErrUnresolvablePlayer, got %v", path, err)
		}
		if dead == nil || len(dead.Streams) != 0 {
			t.Fatalf("%s: expected empty stream list, got %+v", path, dead)
		}
	}
}

// ---- Lavakino ----

const covLavakinoStreamsHTML = `<html><body>
<iframe src="https://www.youtube.com/embed/trailer"></iframe>
<iframe src="/player/embed/9"></iframe>
<iframe src="/player/embed/10"></iframe>
</body></html>`

func TestCovProviderLavakinoStreamsFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{
		"/item/1":          covLavakinoStreamsHTML,
		"/player/embed/9":  covPlayerFrameHTML,
		"/player/embed/10": `<html><body>тут немає PlayerJS</body></html>`,
		"/item/2":          `<html><body><iframe src="/player/dead"></iframe></body></html>`,
		"/player/dead":     covDeadPlayerFrameHTML,
	})
	defer srv.Close()
	p := &LavakinoProvider{client: covTLS(t), baseURL: srv.URL}

	resp, err := p.GetStreams(context.Background(), srv.URL+"/item/1", 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Streams) != 1 {
		t.Fatalf("expected exactly 1 stream (early exit after first real stream), got %+v", resp.Streams)
	}
	st := resp.Streams[0]
	if !rePlayableURL.MatchString(st.URL) {
		t.Fatalf("stream url must be a playable media url, got %q", st.URL)
	}
	if st.URL != "https://cdn.example/hls/movie-1080p/master.m3u8" {
		t.Errorf("unexpected stream url: %q", st.URL)
	}
	if st.Headers["Referer"] != "https://cdn.example/" || st.Headers["Origin"] != "https://cdn.example" {
		t.Errorf("headers must be derived from the MEDIA origin, got %v", st.Headers)
	}

	dead, err := p.GetStreams(context.Background(), srv.URL+"/item/2", 0, 0, "")
	if !errors.Is(err, ErrUnresolvablePlayer) {
		t.Fatalf("expected ErrUnresolvablePlayer, got %v", err)
	}
	if dead == nil || len(dead.Streams) != 0 {
		t.Fatalf("expected empty stream list, got %+v", dead)
	}
}

// Фанаут обмежено MaxPlayerIframes: 10 iframe, але сервер приймає не більше ніж
// MaxPlayerIframes запитів до плеєрів (без цього один виклик робив 1 + N запитів).
func TestCovProviderLavakinoIframeFanOutIsCapped(t *testing.T) {
	var frames int32
	var item string
	mux := http.NewServeMux()
	mux.HandleFunc("/item/1", func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder
		for i := 0; i < 10; i++ {
			sb.WriteString(`<iframe src="/player/embed/` + strconv.Itoa(i) + `"></iframe>`)
		}
		item = "<html><body>" + sb.String() + "</body></html>"
		_, _ = w.Write([]byte(item))
	})
	mux.HandleFunc("/player/embed/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&frames, 1)
		_, _ = w.Write([]byte(covDeadPlayerFrameHTML))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := &LavakinoProvider{client: covTLS(t), baseURL: srv.URL}
	resp, err := p.GetStreams(context.Background(), srv.URL+"/item/1", 0, 0, "")
	if !errors.Is(err, ErrUnresolvablePlayer) {
		t.Fatalf("expected ErrUnresolvablePlayer, got %v", err)
	}
	if resp == nil || len(resp.Streams) != 0 {
		t.Fatalf("expected empty stream list, got %+v", resp)
	}
	if got := atomic.LoadInt32(&frames); got > int32(MaxPlayerIframes) {
		t.Fatalf("iframe fan-out not capped: %d player fetches, limit %d", got, MaxPlayerIframes)
	}
}
