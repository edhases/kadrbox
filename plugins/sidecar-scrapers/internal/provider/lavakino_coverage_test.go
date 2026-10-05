package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLavakino_DescribeAndBasic(t *testing.T) {
	p := NewLavakinoProvider(nil)
	desc := p.Describe()
	if desc.ID != "lavakino" {
		t.Errorf("expected ID 'lavakino', got %q", desc.ID)
	}
	if desc.Name != "Lavakino" {
		t.Errorf("expected Name 'Lavakino', got %q", desc.Name)
	}
	if desc.BaseURL != "https://lavakino.net" {
		t.Errorf("expected BaseURL 'https://lavakino.net', got %q", desc.BaseURL)
	}
	if desc.IconURL != "https://lavakino.net/favicon.ico" {
		t.Errorf("expected IconURL 'https://lavakino.net/favicon.ico', got %q", desc.IconURL)
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

func TestLavakino_Search(t *testing.T) {
	var requestedURL string
	var postedForm string

	catalogHTML := `<html><body>
		<div class="short">
			<a class="short-title" href="/film/1-inception.html">Початок</a>
			<div class="short-img"><img src="/posters/inception.jpg" /></div>
			<div class="sd-line">Рік: 2010</div>
			<div class="m-imdb">8,8</div>
		</div>
		<div class="short">
			<a class="short-title" href="/serials/2-breaking.html">Пуститися берега</a>
			<div class="short-img"><img data-src="https://example.com/breaking.jpg" /></div>
			<div class="sd-line">2008</div>
			<div class="m-imdb">9.5</div>
		</div>
		<div class="short">
			<a class="short-title" href="/cartoon/3-toy.html">Історія іграшок</a>
			<div class="short-img"><img src="/posters/toy.jpg" /></div>
			<div class="sd-line">1995</div>
		</div>
		<div class="short">
			<a class="short-title" href="/anime/4-naruto.html">Наруто</a>
			<div class="short-img"><img src="/posters/naruto.jpg" /></div>
			<div class="sd-line">2002</div>
		</div>
		<div class="short">
			<a class="short-title">Без посилання</a>
		</div>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			postedForm = string(b)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(catalogHTML))
	}))
	defer server.Close()

	tls := covTLS(t)
	p := &LavakinoProvider{
		client:  tls,
		baseURL: server.URL,
	}

	items, err := p.Search(context.Background(), "початок")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(items) != 4 {
		t.Fatalf("expected 4 items parsed, got %d", len(items))
	}
	if !strings.Contains(requestedURL, "/index.php?do=search") {
		t.Errorf("expected search URL, got %q", requestedURL)
	}
	if !strings.Contains(postedForm, "story=") {
		t.Errorf("expected posted form to contain story, got %q", postedForm)
	}

	// 1. Фільм з відносним постером
	m := items[0]
	if m.Title != "Початок" || m.Year != 2010 || m.Rating != 8.8 || m.Type != "movie" {
		t.Errorf("unexpected movie: %+v", m)
	}
	if m.PosterURL != server.URL+"/posters/inception.jpg" {
		t.Errorf("expected relative poster to be prefixed with baseURL, got %q", m.PosterURL)
	}

	// 2. Серіал з абсолютним постером (data-src)
	s := items[1]
	if s.Title != "Пуститися берега" || s.Year != 2008 || s.Rating != 9.5 || s.Type != "series" {
		t.Errorf("unexpected series: %+v", s)
	}
	if s.PosterURL != "https://example.com/breaking.jpg" {
		t.Errorf("expected absolute poster url, got %q", s.PosterURL)
	}

	// 3. Мультфільм
	c := items[2]
	if c.Type != "cartoon" {
		t.Errorf("expected cartoon type, got %q", c.Type)
	}

	// 4. Аніме
	a := items[3]
	if a.Type != "anime" {
		t.Errorf("expected anime type, got %q", a.Type)
	}
}

func TestLavakino_GetPopular_And_GetByCategory_Routing(t *testing.T) {
	var requestedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<html><body>
			<div class="short">
				<a class="short-title" href="/film/1.html">Фільм</a>
			</div>
		</body></html>`
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	tls := covTLS(t)
	p := &LavakinoProvider{
		client:  tls,
		baseURL: server.URL,
	}

	// 1. GetPopular: page < 1 перетворюється на 1, section == ""
	_, err := p.GetPopular(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("GetPopular default failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/" {
		t.Errorf("expected path '/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 2. GetPopular: section == "", page 2
	_, err = p.GetPopular(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("GetPopular page 2 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/page/2/" {
		t.Errorf("expected path '/page/2/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 3. GetPopular: movie (filmys), page 1
	_, err = p.GetPopular(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("GetPopular movie page 1 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/filmys/" {
		t.Errorf("expected path '/filmys/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 4. GetPopular: movie (filmys), page 2
	_, err = p.GetPopular(context.Background(), "movie", 2)
	if err != nil {
		t.Fatalf("GetPopular movie page 2 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/filmys/page/2/" {
		t.Errorf("expected path '/filmys/page/2/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 5. GetByCategory: category == "" -> викликає GetPopular
	_, err = p.GetByCategory(context.Background(), "", "movie", 1)
	if err != nil {
		t.Fatalf("GetByCategory empty category failed: %v", err)
	}

	// 6. GetByCategory: category != "", page < 1 (page = 0)
	_, err = p.GetByCategory(context.Background(), "comedy", "movie", 0)
	if err != nil {
		t.Fatalf("GetByCategory page 0 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/f/comedy/" {
		t.Errorf("expected path '/f/comedy/', got %q", requestedPaths[len(requestedPaths)-1])
	}

	// 7. GetByCategory: category != "", page 2
	_, err = p.GetByCategory(context.Background(), "comedy", "movie", 2)
	if err != nil {
		t.Fatalf("GetByCategory comedy page 2 failed: %v", err)
	}
	if requestedPaths[len(requestedPaths)-1] != "/f/comedy/page/2/" {
		t.Errorf("expected path '/f/comedy/page/2/', got %q", requestedPaths[len(requestedPaths)-1])
	}
}

func TestLavakino_GetDetails_Comprehensive(t *testing.T) {
	detailsHTML := `<html><body>
		<h1>Дюна 2</h1>
		<div class="fposter"><img src="/posters/dune2.jpg" /></div>
		<div class="fdesc">Продовження культової саги про Арракіс.</div>
		<div itemprop="alternateName">Dune: Part Two</div>

		<div itemprop="genre">Фантастика, Пригоди, Драма</div>
		<div itemprop="countryOfOrigin">США, Канада</div>
		<div itemprop="actor">Тімоті Шаламе, Зендея, Ребекка Фергюсон</div>
		<div itemprop="director">Дені Вільньов</div>
		<div itemprop="copyrightYear">2024</div>
		<div class="m-imdb">8,6</div>
	</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(detailsHTML))
	}))
	defer server.Close()

	tls := covTLS(t)
	p := &LavakinoProvider{
		client:  tls,
		baseURL: server.URL,
	}

	// 1. Повний розбір details для серіалу
	d, err := p.GetDetails(context.Background(), server.URL+"/serial/1-dune2.html")
	if err != nil {
		t.Fatalf("GetDetails failed: %v", err)
	}

	if d.Title != "Дюна 2" {
		t.Errorf("expected title 'Дюна 2', got %q", d.Title)
	}
	if d.OriginalTitle != "Dune: Part Two" {
		t.Errorf("expected original title 'Dune: Part Two', got %q", d.OriginalTitle)
	}
	if d.PosterURL != server.URL+"/posters/dune2.jpg" {
		t.Errorf("expected poster %q, got %q", server.URL+"/posters/dune2.jpg", d.PosterURL)
	}
	if d.Year != 2024 {
		t.Errorf("expected year 2024, got %d", d.Year)
	}
	if d.Rating != 8.6 {
		t.Errorf("expected rating 8.6, got %v", d.Rating)
	}
	if d.Director != "Дені Вільньов" {
		t.Errorf("expected director 'Дені Вільньов', got %q", d.Director)
	}
	if len(d.Genres) != 3 || d.Genres[0] != "Фантастика" {
		t.Errorf("expected 3 genres, got %v", d.Genres)
	}
	if len(d.Countries) != 2 || d.Countries[0] != "США" {
		t.Errorf("expected 2 countries, got %v", d.Countries)
	}
	if len(d.Actors) != 3 || d.Actors[0] != "Тімоті Шаламе" {
		t.Errorf("expected 3 actors, got %v", d.Actors)
	}
	if d.Type != "series" {
		t.Errorf("expected series type, got %q", d.Type)
	}

	// 2. URL з mult -> cartoon, anime -> anime
	dCart, _ := p.GetDetails(context.Background(), server.URL+"/mult/2-cart.html")
	if dCart != nil && dCart.Type != "cartoon" {
		t.Errorf("expected cartoon type, got %q", dCart.Type)
	}
	dAnime, _ := p.GetDetails(context.Background(), server.URL+"/anime/3-anime.html")
	if dAnime != nil && dAnime.Type != "anime" {
		t.Errorf("expected anime type, got %q", dAnime.Type)
	}

	// 3. Fallback розбір через .sd-line коли schema itemprop відсутні
	fallbackHTML := `<html><body>
		<h1>Інтерстеллар</h1>
		<div class="fposter"><img data-src="https://example.com/interstellar.jpg" /></div>
		<div class="full-text">Опис космічної подорожі.</div>

		<div class="sd-line">Жанр: Наукова фантастика, Драма</div>
		<div class="sd-line">Країна: США, Велика Британія</div>
		<div class="sd-line">Режисер: Крістофер Нолан</div>
		<div class="sd-line">Актори: Меттью Макконахі, Енн Гетевей</div>
		<div class="sd-line">Тривалість: 169 хв</div>
		<div class="sd-line">Рік: 2014</div>
	</body></html>`

	serverFB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(fallbackHTML))
	}))
	defer serverFB.Close()

	pFB := &LavakinoProvider{
		client:  tls,
		baseURL: serverFB.URL,
	}

	dFB, err := pFB.GetDetails(context.Background(), serverFB.URL+"/film/10-interstellar.html")
	if err != nil {
		t.Fatalf("GetDetails fallback failed: %v", err)
	}

	if dFB.Director != "Крістофер Нолан" {
		t.Errorf("expected director 'Крістофер Нолан', got %q", dFB.Director)
	}
	if len(dFB.Genres) != 2 || dFB.Genres[0] != "Наукова фантастика" {
		t.Errorf("expected 2 genres, got %v", dFB.Genres)
	}
	if len(dFB.Countries) != 2 || dFB.Countries[0] != "США" {
		t.Errorf("expected 2 countries, got %v", dFB.Countries)
	}
	if len(dFB.Actors) != 2 || dFB.Actors[0] != "Меттью Макконахі" {
		t.Errorf("expected 2 actors, got %v", dFB.Actors)
	}
	if dFB.Duration != "169 хв" {
		t.Errorf("expected duration '169 хв', got %q", dFB.Duration)
	}
	if dFB.Year != 2014 {
		t.Errorf("expected year 2014, got %d", dFB.Year)
	}
}

func TestLavakino_ErrorPaths(t *testing.T) {
	tls := covTLS(t)
	server500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server500.Close()

	p := &LavakinoProvider{
		client:  tls,
		baseURL: server500.URL,
	}

	// 1. Search помилка
	_, err := p.Search(context.Background(), "test")
	if err == nil {
		t.Errorf("expected error on Search 500")
	}

	// 2. GetPopular помилка (не 404)
	_, err = p.GetPopular(context.Background(), "movie", 1)
	if err == nil {
		t.Errorf("expected error on GetPopular 500")
	}

	// 3. GetByCategory помилка (не 404)
	_, err = p.GetByCategory(context.Background(), "action", "movie", 1)
	if err == nil {
		t.Errorf("expected error on GetByCategory 500")
	}

	// 4. GetDetails помилка
	_, err = p.GetDetails(context.Background(), server500.URL+"/item/1")
	if err == nil {
		t.Errorf("expected error on GetDetails 500")
	}

	// 5. GetStreams помилка завантаження HTML
	_, err = p.GetStreams(context.Background(), server500.URL+"/item/1", 0, 0, "")
	if err == nil {
		t.Errorf("expected error on GetStreams 500")
	}
}
