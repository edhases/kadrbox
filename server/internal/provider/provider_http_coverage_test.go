package provider

// Покриття парсерів провайдерів без зовнішньої мережі: in-package тест
// підміняє неекспортований baseURL на httptest-сервер з фіксованим HTML.
// Мережевий шар при цьому справжній (TLSClient), парсинг — реальний код.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

const covUakinoStreamsHTML = `<html><body>
<div class="player"><iframe src="//player.example.com/embed/123" allowfullscreen></iframe></div>
</body></html>`

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

func TestCovProviderUakinoStreamsFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{
		"/item/1": covUakinoStreamsHTML,
		"/item/2": `<html><body>без плеєра</body></html>`,
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
	// Протокол-відносний // доповнюється до https:.
	if st.URL != "https://player.example.com/embed/123" || st.DirectURL != st.URL {
		t.Errorf("unexpected stream URLs: %+v", st)
	}
	if st.Headers["Referer"] != srv.URL+"/" {
		t.Errorf("unexpected Referer: %v", st.Headers)
	}

	empty, err := p.GetStreams(context.Background(), srv.URL+"/item/2", 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(empty.Streams) != 0 {
		t.Errorf("expected no streams without iframe, got %+v", empty)
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
<iframe src="https://player2.example/x"></iframe>
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
	srv := covFixtureServer(map[string]string{"/item/7": covEneyidaStreamsHTML})
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
	if st.URL != "https://player2.example/x" || st.Quality != "Auto" {
		t.Errorf("unexpected stream: %+v", st)
	}
}

// ---- HDRezka ----

const covHdrezkaSearchHTML = `<html><body>
<div class="b-content__inline_item">
  <div class="b-content__inline_item-link"><a href="https://hdrezka.me/f/1">Інтерстеллар</a></div>
  <div class="b-content__inline_item-cover"><img src="https://cdn.example/i.jpg"/></div>
</div>
<div class="b-content__inline_item">
  <div class="b-content__inline_item-link"><span>Без посилання</span></div>
</div>
</body></html>`

const covHdrezkaDetailsHTML = `<html><body>
<div class="b-post__title"><h1>Інтерстеллар</h1></div>
<div class="b-sidecover"><img src="https://cdn.example/cover.jpg"/></div>
<div class="b-post__description_text">Опис про космос.</div>
</body></html>`

func TestCovProviderHdrezkaSearchFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{"/search/": covHdrezkaSearchHTML})
	defer srv.Close()
	p := &HdrezkaProvider{client: covTLS(t), baseURL: srv.URL}

	items, err := p.Search(context.Background(), "інтерстеллар")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (second has no link), got %+v", items)
	}
	it := items[0]
	if it.Title != "Інтерстеллар" || it.Type != "movie" || it.ProviderID != "hdrezka" {
		t.Errorf("unexpected item: %+v", it)
	}
	if it.PosterURL != "https://cdn.example/i.jpg" || it.URL != "https://hdrezka.me/f/1" {
		t.Errorf("unexpected urls: %+v", it)
	}
}

func TestCovProviderHdrezkaDetailsFixture(t *testing.T) {
	srv := covFixtureServer(map[string]string{"/f/1": covHdrezkaDetailsHTML})
	defer srv.Close()
	p := &HdrezkaProvider{client: covTLS(t), baseURL: srv.URL}

	d, err := p.GetDetails(context.Background(), srv.URL+"/f/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Title != "Інтерстеллар" || d.PosterURL != "https://cdn.example/cover.jpg" {
		t.Errorf("unexpected details: %+v", d.MediaItem)
	}
	if !strings.Contains(d.Description, "космос") {
		t.Errorf("unexpected description: %q", d.Description)
	}
}
