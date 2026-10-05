package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUakino_DescribeAndBasic(t *testing.T) {
	p := NewUakinoProvider(nil)
	desc := p.Describe()
	if desc.ID != "uakino" {
		t.Errorf("expected ID 'uakino', got %q", desc.ID)
	}
	if desc.Name != "UAKino" {
		t.Errorf("expected Name 'UAKino', got %q", desc.Name)
	}
	if desc.BaseURL != "https://uakino.biz" {
		t.Errorf("expected BaseURL 'https://uakino.biz', got %q", desc.BaseURL)
	}
	if !desc.ShowOnHome {
		t.Errorf("expected ShowOnHome true")
	}
	if desc.HasFixedStreams {
		t.Errorf("expected HasFixedStreams false")
	}
	if !desc.SearchEnabledDefault {
		t.Errorf("expected SearchEnabledDefault true")
	}
	if len(desc.ContentTypes) != 4 {
		t.Errorf("expected 4 content types, got %d", len(desc.ContentTypes))
	}
}

// Живі перевірки uakino.biz (Chrome120UserAgent + Referer, 2026-10):
//
//	/filmy/           200
//	/seriesss/        200   <- так, з трьома s
//	/cartoon/         200
//	/animeukr/        200
//	/multfilmy/       404   <- НЕ існує
//	/anime/           404   <- НЕ існує
//	/serials/         404   <- НЕ існує
//	/cartoonss/       404   <- НЕ існує
//	/animes/          404   <- НЕ існує
//
// Тест раніше просто кодуфікував припущення; тепер він пінить
// перевірені значення і окремо забороняє мертві слади.
func TestUakino_GetSection(t *testing.T) {
	p := NewUakinoProvider(nil)
	tests := []struct {
		contentType string
		expected    string
	}{
		{"movie", "filmy"},
		{"series", "seriesss"},
		{"cartoon", "cartoon"},
		{"anime", "animeukr"},
		{"other", ""},
		{"", ""},
	}
	for _, tc := range tests {
		got := p.getSection(tc.contentType)
		if got != tc.expected {
			t.Errorf("getSection(%q) = %q, expected %q", tc.contentType, got, tc.expected)
		}
	}
}

func TestUakino_GetSectionNeverReturnsDeadSlugs(t *testing.T) {
	p := NewUakinoProvider(nil)
	dead := map[string]bool{
		"multfilmy": true,
		"anime":     true,
		"serials":   true,
		"cartoonss": true,
		"animes":    true,
	}
	for _, ct := range []string{"movie", "series", "cartoon", "anime"} {
		got := p.getSection(ct)
		if dead[got] {
			t.Errorf("getSection(%q) = %q — 404 на живому uakino.biz", ct, got)
		}
	}
}

// classifyByPath — спільна функція для каталогу й деталей. Раніше дві
// різні сходи давали різні відповіді на одному шляху: fetchCatalog
// шукав «cartoon»/«мультфільм», GetDetails — «mult».
func TestUakino_ClassifyByPath(t *testing.T) {
	cases := []struct {
		name string
		href string
		want string
	}{
		// Реальні шляхи, зняті з живих сторінок каталогу.
		{"фільм", "https://uakino.biz/filmy/genre_comedy/36141-seredina-90.html", "movie"},
		{"серіал", "https://uakino.biz/seriesss/drama_series/35629-seredina-90.html", "series"},
		{"серіал із підрозділу", "https://uakino.biz/seriesss/subtitle-serials/123-x.html", "series"},
		{"мультсеріал", "https://uakino.biz/cartoon/cartoonseries/36136-seredina-90.html", "cartoon"},
		{"короткометражка", "https://uakino.biz/cartoon/short_cartoons/36132-seredina-90.html", "cartoon"},
		{"аніме-серіал", "https://uakino.biz/animeukr/anime-series/35681-seredina-90.html", "anime"},
		{"аніме-соло", "https://uakino.biz/animeukr/anime-solo/36126-seredina-90.html", "anime"},
		// Старі шляхи, які теж треба впізнавати.
		{"multfilmy (мертвий слаг)", "https://uakino.biz/multfilmy/123-x.html", "cartoon"},
		{"старий serials", "https://uakino.biz/serials/123-x.html", "series"},
		{"старий cartoon", "https://uakino.biz/cartoon/890-x.html", "cartoon"},
		{"корінь без розділу", "https://uakino.biz/123-dune.html", "movie"},
		{"кореневий мульт", "https://uakino.biz/456-ataka-titanov.html", "movie"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyByPath(tc.href); got != tc.want {
				t.Errorf("classifyByPath(%q) = %q, want %q", tc.href, got, tc.want)
			}
		})
	}
}

// Каталог і деталі мають давати однаковий тип на ОДНОМУ й тому самому
// шляху. Раніше /multfilmy/123.html у каталозі давав movie, а на
// сторінці опису — cartoon.
func TestUakino_CatalogAndDetailsAgreeOnType(t *testing.T) {
	html := `<html><body>
	  <div class="movie-item"><div class="movie-title"><a href="https://uakino.biz/multfilmy/123-x.html">Мульт</a></div></div>
	  <div class="movie-item"><div class="movie-title"><a href="https://uakino.biz/seriesss/456-x.html">Серіал</a></div></div>
	  <div class="movie-item"><div class="movie-title"><a href="https://uakino.biz/animeukr/789-x.html">Аніме</a></div></div>
	  <div class="movie-item"><div class="movie-title"><a href="https://uakino.biz/cartoon/cartoonseries/101-x.html">Мультсеріал</a></div></div>
	</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}
	items, err := p.GetPopular(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 cards, got %d: %+v", len(items), items)
	}

	for _, it := range items {
		d, err := p.GetDetails(context.Background(), srv.URL+it.URL[len("https://uakino.biz"):])
		if err != nil {
			t.Fatalf("GetDetails(%s): %v", it.ID, err)
		}
		if d.Type != it.Type {
			t.Errorf("%s: catalog type %q != details type %q", it.ID, it.Type, d.Type)
		}
	}
	if items[0].Type != "cartoon" {
		t.Errorf("/multfilmy/ card type = %q, want cartoon (the open defect)", items[0].Type)
	}
	// /cartoon/cartoonseries/ — єдиний шлях, де стара сходи деталей
	// («serial» / «mult» / «anime») і classifyByPath давали різні
	// відповіді: жодного з трьох маркерів у ньому немає, але є
	// «cartoon».
	if items[3].Type != "cartoon" {
		t.Errorf("/cartoon/cartoonseries/ card type = %q, want cartoon", items[3].Type)
	}
}

func TestUakino_GetPopular_And_GetByCategory_Routing(t *testing.T) {
	var requestedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<html><body>
			<div class="movie-item">
				<div class="movie-title"><a href="/filmy/1-test.html">Тестовий Фільм</a></div>
				<div class="movie-img"><img src="/img/test.jpg" /></div>
				<div class="movie-date">2023</div>
			</div>
			<div class="movie-item">
				<div class="movie-title"><a href="/serials/2-show.html">Тестовий Серіал</a></div>
				<div class="movie-img"><img data-src="/img/show.jpg" /></div>
				<div class="movie-date">2022</div>
			</div>
			<div class="movie-item">
				<div class="movie-title"><a href="/anime/3-anime.html">Тестове Аніме</a></div>
				<div class="movie-img"><img src="https://example.com/anime.jpg" /></div>
				<div class="movie-date">2021</div>
			</div>
			<div class="movie-item">
				<div class="movie-title"><a href="/cartoon/4-cart.html">Тестовий Мультфільм</a></div>
				<div class="movie-img"><img src="/img/cart.jpg" /></div>
				<div class="movie-date">2020</div>
			</div>
		</body></html>`
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	tls := covTLS(t)
	p := &UakinoProvider{
		client:  tls,
		baseURL: server.URL,
	}

	// 1. GetPopular: page < 1 перетворюється на 1, section == ""
	items, err := p.GetPopular(context.Background(), "", 0)
	if err != nil || len(items) != 4 {
		t.Fatalf("GetPopular default page 1 failed: %v (items: %d)", err, len(items))
	}
	if requestedPaths[len(requestedPaths)-1] != "/" {
		t.Errorf("expected path '/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 2. GetPopular: section == "", page 2
	_, err = p.GetPopular(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("GetPopular default page 2 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/page/2/" {
		t.Errorf("expected path '/page/2/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 3. GetPopular: movie, page 1
	_, err = p.GetPopular(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("GetPopular movie page 1 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/filmy/" {
		t.Errorf("expected path '/filmy/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 4. GetPopular: movie, page 2
	_, err = p.GetPopular(context.Background(), "movie", 2)
	if err != nil {
		t.Fatalf("GetPopular movie page 2 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/filmy/page/2/" {
		t.Errorf("expected path '/filmy/page/2/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 5. GetByCategory: category == "" -> викликає GetPopular
	_, err = p.GetByCategory(context.Background(), "", "series", 1)
	if err != nil {
		t.Fatalf("GetByCategory empty category failed: %v", err)
	}

	// 6. GetByCategory: category != "", section == "", page 1
	_, err = p.GetByCategory(context.Background(), "comedy", "", 1)
	if err != nil {
		t.Fatalf("GetByCategory comedy page 1 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/xfsearch/genre/comedy/" {
		t.Errorf("expected path '/xfsearch/genre/comedy/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 7. GetByCategory: category != "", section == "", page 2
	_, err = p.GetByCategory(context.Background(), "comedy", "", 2)
	if err != nil {
		t.Fatalf("GetByCategory comedy page 2 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/xfsearch/genre/comedy/page/2/" {
		t.Errorf("expected path '/xfsearch/genre/comedy/page/2/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 8. GetByCategory: category != "", section == "filmy", page 1
	_, err = p.GetByCategory(context.Background(), "drama", "movie", 1)
	if err != nil {
		t.Fatalf("GetByCategory drama movie page 1 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/filmy/xfsearch/genre/drama/" {
		t.Errorf("expected path '/filmy/xfsearch/genre/drama/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 9. GetByCategory: category != "", section == "filmy", page 2
	_, err = p.GetByCategory(context.Background(), "drama", "movie", 2)
	if err != nil {
		t.Fatalf("GetByCategory drama movie page 2 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/filmy/xfsearch/genre/drama/page/2/" {
		t.Errorf("expected path '/filmy/xfsearch/genre/drama/page/2/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 10. GetByCategory: page < 1 (page = 0)
	_, err = p.GetByCategory(context.Background(), "drama", "movie", 0)
	if err != nil {
		t.Fatalf("GetByCategory page 0 failed: %v", err)
	}
}

func TestUakino_GetDetails_Comprehensive(t *testing.T) {
	detailsHTML := `<html><body>
		<h1 class="movie-title">Головна Назва</h1>
		<meta itemprop="alternateName" content="Alt Original Title" />
		<div class="movie-img"><img data-src="/posters/full.jpg" /></div>
		<div class="full-text">Детальний сюжетний опис фільму.</div>

		<!-- Schema metas -->
		<meta itemprop="genre" content="Фантастика, Пригоди" />
		<meta itemprop="contentLocation" content="США, Великобританія" />
		<meta itemprop="actors" content="Актор Один, Актор Два" />
		<meta itemprop="director" content="Крістофер Нолан" />
		<meta itemprop="dateCreated" content="2020-07-15" />

		<!-- Info block fallback -->
		<div class="film-info">
			<div class="fi-item-s">
				<span class="fi-label">Тривалість:</span>
				<span class="fi-desc">150 хв</span>
			</div>
		</div>

		<span class="imdb-rate">8.7</span>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(detailsHTML))
	}))
	defer server.Close()

	tls := covTLS(t)
	p := &UakinoProvider{
		client:  tls,
		baseURL: server.URL,
	}

	// 1. Повний розбір details для серіалу за URL
	d, err := p.GetDetails(context.Background(), server.URL+"/serial/1-show.html")
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}

	if d.Title != "Головна Назва" {
		t.Errorf("expected title 'Головна Назва', got %q", d.Title)
	}
	if d.OriginalTitle != "Alt Original Title" {
		t.Errorf("expected original title 'Alt Original Title', got %q", d.OriginalTitle)
	}
	if d.PosterURL != server.URL+"/posters/full.jpg" {
		t.Errorf("expected poster URL %q, got %q", server.URL+"/posters/full.jpg", d.PosterURL)
	}
	if d.Year != 2020 {
		t.Errorf("expected year 2020 from dateCreated, got %d", d.Year)
	}
	if d.Director != "Крістофер Нолан" {
		t.Errorf("expected director 'Крістофер Нолан', got %q", d.Director)
	}
	if len(d.Genres) != 2 || d.Genres[0] != "Фантастика" {
		t.Errorf("expected 2 genres, got %v", d.Genres)
	}
	if len(d.Countries) != 2 || d.Countries[0] != "США" {
		t.Errorf("expected 2 countries, got %v", d.Countries)
	}
	if len(d.Actors) != 2 || d.Actors[0] != "Актор Один" {
		t.Errorf("expected 2 actors, got %v", d.Actors)
	}
	if d.Duration != "150 хв" {
		t.Errorf("expected duration '150 хв', got %q", d.Duration)
	}
	if d.Rating != 8.7 {
		t.Errorf("expected rating 8.7, got %v", d.Rating)
	}
	if d.Type != "series" {
		t.Errorf("expected type 'series' from URL with serial, got %q", d.Type)
	}

	// 2. /mult/ (старий слаг) -> cartoon, /anime/ -> anime
	dCart, _ := p.GetDetails(context.Background(), server.URL+"/mult/2-cart.html")
	if dCart != nil && dCart.Type != "cartoon" {
		t.Errorf("expected type cartoon, got %q", dCart.Type)
	}
	dCartReal, _ := p.GetDetails(context.Background(), server.URL+"/multfilmy/3-cart.html")
	if dCartReal != nil && dCartReal.Type != "cartoon" {
		t.Errorf("expected type cartoon for /multfilmy/, got %q", dCartReal.Type)
	}

	dAnime, _ := p.GetDetails(context.Background(), server.URL+"/anime/3-anime.html")
	if dAnime != nil && dAnime.Type != "anime" {
		t.Errorf("expected type anime, got %q", dAnime.Type)
	}

	// 3. Fallback розбір через .fi-item коли meta немає
	fallbackDetailsHTML := `<html><body>
		<h1 class="solotitle">Фільм Фолбек</h1>
		<div class="flist">
			<li>Режисер: <a>Джеймс Кемерон</a></li>
			<li>Актори: <a>Сем Вортінгтон</a>, <a>Зої Салдана</a></li>
			<li>Жанр: <a>Бойовик</a>, <a>Пригоди</a></li>
			<li>Країна: <a>США</a></li>
			<li>Рік: 2009</li>
			<li>Тривалість: 162 хв</li>
		</div>
	</body></html>`

	serverFallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(fallbackDetailsHTML))
	}))
	defer serverFallback.Close()

	pFallback := &UakinoProvider{
		client:  tls,
		baseURL: serverFallback.URL,
	}

	dFB, err := pFallback.GetDetails(context.Background(), serverFallback.URL+"/movie/10-avatar.html")
	if err != nil {
		t.Fatalf("GetDetails fallback failed: %v", err)
	}
	if dFB.Director != "Джеймс Кемерон" {
		t.Errorf("expected director 'Джеймс Кемерон', got %q", dFB.Director)
	}
	if len(dFB.Actors) != 2 {
		t.Errorf("expected 2 actors, got %v", dFB.Actors)
	}
	if len(dFB.Genres) != 2 {
		t.Errorf("expected 2 genres, got %v", dFB.Genres)
	}
	if len(dFB.Countries) != 1 || dFB.Countries[0] != "США" {
		t.Errorf("expected 1 country 'США', got %v", dFB.Countries)
	}
	if dFB.Year != 2009 {
		t.Errorf("expected year 2009, got %d", dFB.Year)
	}
}

func TestUakino_ErrorPaths(t *testing.T) {
	tls := covTLS(t)
	// Сервер, який завжди повертає 500
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server500.Close()

	p := &UakinoProvider{
		client:  tls,
		baseURL: server500.URL,
	}

	// 1. GetPopular помилка
	_, err := p.GetPopular(context.Background(), "movie", 1)
	if err == nil {
		t.Errorf("expected error on GetPopular 500")
	}

	// 2. GetByCategory помилка
	_, err = p.GetByCategory(context.Background(), "action", "movie", 1)
	if err == nil {
		t.Errorf("expected error on GetByCategory 500")
	}

	// 3. GetDetails помилка
	_, err = p.GetDetails(context.Background(), server500.URL+"/item/1")
	if err == nil {
		t.Errorf("expected error on GetDetails 500")
	}

	// 4. GetStreams помилка завантаження сторінки
	_, err = p.GetStreams(context.Background(), server500.URL+"/item/1", 0, 0, "")
	if err == nil {
		t.Errorf("expected error on GetStreams 500")
	}
}

func TestUakino_MultipleAnchorsInCardTitle(t *testing.T) {
	html := `<html><body>
		<div class="movie-item">
			<div class="movie-title">
				<a href="https://uakino.biz/123-dune.html">Дюна: Частина друга</a>
				<a href="/tags/sci-fi">Фантастика</a>
			</div>
			<div class="movie-img"><img src="/img/dune.jpg" /></div>
			<div class="movie-date">2024</div>
		</div>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	p := &UakinoProvider{
		client:  covTLS(t),
		baseURL: server.URL,
	}

	items, err := p.Search(context.Background(), "дюна")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Title != "Дюна: Частина друга" {
		t.Errorf("expected title 'Дюна: Частина друга', got %q", items[0].Title)
	}
	if items[0].ID != "https://uakino.biz/123-dune.html" {
		t.Errorf("expected href 'https://uakino.biz/123-dune.html', got %q", items[0].ID)
	}
}

// ---- сезони й озвучки (дефект №4) ----------------------------------------

// Плеєр із деревом PlayerJS, вбудованим прямо в HTML сторінки опису:
// season -> dub -> episode.
const uakinoPlayerTree = `<!DOCTYPE html><html><body><script>
var player = new Playerjs({file: '[{"title":"1 сезон","folder":[{"title":"Студия Моги","folder":[{"title":"1 серія","file":"https://cdn.example/hls/snowfall.s01e01.mogi/hls/index.m3u8","subtitle":"[Українські]https://cdn.example/sub/s01e01.vtt"},{"title":"2 серія","file":"https://cdn.example/hls/snowfall.s01e02.mogi/hls/index.m3u8"}]},{"title":"Студия Тортуга","folder":[{"title":"1 серія","file":"https://calypso.tortuga.tw/hls/snowfall/s01e01/index.m3u8"}]}]},{"title":"2 сезон","folder":[{"title":"Студия Моги","folder":[{"title":"1 серія","file":"https://cdn.example/hls/snowfall.s02e01.mogi/hls/index.m3u8"}]}]}]'});
</script></body></html>`

func TestUakino_GetDetails_FillsSeasonsAndVoiceovers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(uakinoPlayerTree))
	}))
	defer srv.Close()

	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}
	d, err := p.GetDetails(context.Background(), srv.URL+"/seriesss/drama_series/35629-x.html")
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if len(d.Seasons) != 2 {
		t.Fatalf("expected 2 seasons, got %d: %+v", len(d.Seasons), d.Seasons)
	}
	if d.Seasons[0].Number != 1 || d.Seasons[1].Number != 2 {
		t.Errorf("season numbers = %d,%d want 1,2", d.Seasons[0].Number, d.Seasons[1].Number)
	}
	// Сезон 1: 2 серії «Моги» + 1 «Тортуга» -> після злиття 2.
	if len(d.Seasons[0].Episodes) != 2 {
		t.Errorf("season 1 episodes = %d, want 2 after merging dubs: %+v", len(d.Seasons[0].Episodes), d.Seasons[0].Episodes)
	}
	if len(d.Seasons[1].Episodes) != 1 {
		t.Errorf("season 2 episodes = %d, want 1", len(d.Seasons[1].Episodes))
	}

	if len(d.Voiceovers) != 2 {
		t.Fatalf("expected 2 voiceovers, got %d: %+v", len(d.Voiceovers), d.Voiceovers)
	}
	names := map[string]int{}
	for _, v := range d.Voiceovers {
		if v.ID == "" || v.Name == "" {
			t.Errorf("voiceover without id/name: %+v", v)
		}
		names[v.Name] = len(v.Seasons)
	}
	if _, ok := names["Студия Моги"]; !ok {
		t.Errorf("voiceovers = %v, want «Студия Моги»", names)
	}
	if names["Студия Моги"] != 2 {
		t.Errorf("«Студия Моги» carries %d seasons, want 2", names["Студия Моги"])
	}
	if names["Студия Тортуга"] != 1 {
		t.Errorf("«Студия Тортуга» carries %d seasons, want 1", names["Студия Тортуга"])
	}
}

// GetStreams має віддати потрібну озвучку, а не ім'я балансера.
// Розмітка відповідає живій: плеєр живе в iframe, дерево — всередині нього.
func TestUakino_GetStreams_SelectsRealVoiceover(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/seriesss/x.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><div class="player-box">
		  <iframe src="/player/embed/35629"></iframe>
		</div></body></html>`))
	})
	mux.HandleFunc("/player/embed/35629", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(uakinoPlayerTree))
	})

	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}
	resp, err := p.GetStreams(context.Background(), srv.URL+"/seriesss/x.html", 2, 1, "Студия Моги")
	if err != nil {
		t.Fatalf("GetStreams: %v", err)
	}
	if len(resp.Streams) == 0 {
		t.Fatal("expected at least one stream")
	}
	for _, s := range resp.Streams {
		if !strings.Contains(s.URL, "snowfall.s02e01.mogi") {
			t.Errorf("season 2 / episode 1 / dub Моги must pick s02e01, got %s", s.URL)
		}
		if s.Voiceover != "Студия Моги" {
			t.Errorf("voiceover = %q, want «Студия Моги», not a balancer name", s.Voiceover)
		}
	}
}

// Без плейлиста поля лишаються порожніми — вигадані сезони гірші за
// відсутність сезонів.
func TestUakino_GetDetails_NoPlaylistMeansNoSeasons(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>Фільм без плеєра</h1></body></html>`))
	}))
	defer srv.Close()

	p := &UakinoProvider{client: covTLS(t), baseURL: srv.URL}
	d, err := p.GetDetails(context.Background(), srv.URL+"/filmy/1-x.html")
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}
	if len(d.Seasons) != 0 || len(d.Voiceovers) != 0 {
		t.Errorf("expected empty seasons/voiceovers, got %d/%d", len(d.Seasons), len(d.Voiceovers))
	}
}
