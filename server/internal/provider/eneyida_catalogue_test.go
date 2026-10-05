package provider

// Catalogue endpoint coverage for the DLE providers, and the regression test
// for a bug the production logs exposed.
//
// The logs showed Eneyida answering `GET /content/popular` with 200 and a
// 5-byte body. Five bytes is `null` followed by a newline: the parsers
// declared `var items []domain.MediaItem`, and json.Marshal renders a nil
// slice as `null` rather than `[]`. So a source with nothing to offer was
// indistinguishable on the wire from a source that returned a broken payload,
// and every consumer had to special-case null. These tests pin that an empty
// catalogue serialises as an empty JSON array.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
)

// realClient builds the same TLS client production uses. The parsers dereference
// it unconditionally, so a nil client would panic rather than fail.
func realClient(t *testing.T) *TLSClient {
	t.Helper()
	c, err := NewTLSClient()
	if err != nil {
		t.Skipf("TLS client unavailable: %v", err)
	}
	return c
}

// catalogServer serves a fixed body and records the paths it was asked for.
func catalogServer(t *testing.T, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestEneyidaEmptyCatalogSerialisesAsArray(t *testing.T) {
	// A page whose markup does not match the parser's selectors at all.
	srv, _ := catalogServer(t, `<html><body><p>Каталог порожній</p></body></html>`)

	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %d", len(items))
	}

	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("empty catalogue encoded as %q, want %q — a nil slice "+
			"marshals to null and clients cannot read that as a list", string(encoded), "[]")
	}
}

func TestUakinoEmptyCatalogSerialisesAsArray(t *testing.T) {
	srv, _ := catalogServer(t, `<html><body><p>Нічого</p></body></html>`)

	p := NewUakinoProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	encoded, _ := json.Marshal(items)
	if string(encoded) != "[]" {
		t.Errorf("empty catalogue encoded as %q, want %q", string(encoded), "[]")
	}
}

func TestLavakinoEmptyCatalogSerialisesAsArray(t *testing.T) {
	srv, _ := catalogServer(t, `<html><body><p>Нічого</p></body></html>`)

	p := NewLavakinoProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	encoded, _ := json.Marshal(items)
	if string(encoded) != "[]" {
		t.Errorf("empty catalogue encoded as %q, want %q", string(encoded), "[]")
	}
}

// ---- section routing --------------------------------------------------------

// Живі перевірки eneyida.tv (Chrome120UserAgent + Referer, 2026-10):
//
//	/films/          200
//	/series/         200
//	/cartoon/        200
//	/cartoon-series/ 200
//	/anime/          200
//	/serials/        404   <- старе значення для series
//	/multfilmy/      404   <- старе значення для cartoon
//
// Сталі значення «serials»/«multfilmy» робили розділи серіалів і
// мультфільмів порожніми — саме це й було відкритим дефектом.
func TestEneyidaGetSectionMatchesLiveSlugs(t *testing.T) {
	cases := []struct {
		contentType string
		want        string
	}{
		{"movie", "films"},
		{"series", "series"},
		{"cartoon", "cartoon"},
		{"anime", "anime"},
		{"", ""},
		{"unknown-type", ""},
		{"Movie", ""}, // matching is case-sensitive; an odd type must not route
		// Значення, які НЕ мають права з'явитися: обидва дають 404.
		{"series-never", ""},
	}
	p := NewEneyidaProvider(realClient(t))
	for _, tc := range cases {
		t.Run("type="+tc.contentType, func(t *testing.T) {
			got := p.getSection(tc.contentType)
			if got != tc.want {
				t.Errorf("getSection(%q) = %q, want %q", tc.contentType, got, tc.want)
			}
			if got == "serials" || got == "multfilmy" {
				t.Errorf("getSection(%q) = %q, але цей слаг на живому сайті дає 404", tc.contentType, got)
			}
		})
	}
}

// Категорія має бути вкладена в правильний, а не в мертвий, розділ.
func TestEneyidaGetSectionNeverReturnsDeadSlugs(t *testing.T) {
	p := NewEneyidaProvider(realClient(t))
	dead := map[string]bool{"serials": true, "multfilmy": true}
	for _, ct := range []string{"movie", "series", "cartoon", "anime"} {
		if s := p.getSection(ct); dead[s] {
			t.Errorf("getSection(%q) = %q — 404 на живому eneyida.tv", ct, s)
		}
	}
}

// The first page and later pages must hit different paths, otherwise
// pagination silently repeats page one forever.
func TestEneyidaPopularRoutesToDistinctPaths(t *testing.T) {
	srv, seen := catalogServer(t, `<html><body></body></html>`)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	if _, err := p.GetPopular(context.Background(), "movie", 1); err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if _, err := p.GetPopular(context.Background(), "movie", 4); err != nil {
		t.Fatalf("page 4: %v", err)
	}
	if _, err := p.GetPopular(context.Background(), "", 3); err != nil {
		t.Fatalf("untyped page 3: %v", err)
	}
	if _, err := p.GetPopular(context.Background(), "movie", 0); err != nil {
		t.Fatalf("page 0 should clamp: %v", err)
	}

	want := []string{"/films/", "/films/page/4/", "/page/3/", "/films/"}
	if len(*seen) != len(want) {
		t.Fatalf("requested %v, want %v", *seen, want)
	}
	for i := range want {
		if (*seen)[i] != want[i] {
			t.Errorf("request %d: path = %q, want %q", i, (*seen)[i], want[i])
		}
	}
}

func TestEneyidaGetByCategoryFallsBackToPopular(t *testing.T) {
	srv, seen := catalogServer(t, `<html><body></body></html>`)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	if _, err := p.GetByCategory(context.Background(), "", "series", 1); err != nil {
		t.Fatalf("GetByCategory: %v", err)
	}
	if len(*seen) != 1 || !strings.Contains((*seen)[0], "/series/") {
		t.Errorf("an empty category should reuse the type section, requested %v", *seen)
	}

	if _, err := p.GetByCategory(context.Background(), "comedy", "movie", 1); err != nil {
		t.Fatalf("GetByCategory comedy: %v", err)
	}
	if (*seen)[1] != "/comedy/" {
		t.Errorf("category path = %q, want /comedy/", (*seen)[1])
	}
}

func TestEneyidaDescribe(t *testing.T) {
	desc := NewEneyidaProvider(realClient(t)).Describe()

	if desc.ID != "eneyida" {
		t.Errorf("ID = %q, want eneyida", desc.ID)
	}
	if desc.BaseURL != "https://eneyida.tv" {
		t.Errorf("BaseURL = %q, want https://eneyida.tv", desc.BaseURL)
	}
	if !desc.ShowOnHome {
		t.Error("eneyida should appear on the home screen")
	}
	if desc.HasFixedStreams {
		t.Error("eneyida has no fixed streams")
	}
	if !desc.SearchEnabledDefault {
		t.Error("eneyida search should be enabled by default")
	}
	for _, want := range []string{"movie", "series", "cartoon", "anime"} {
		found := false
		for _, got := range desc.ContentTypes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ContentTypes is missing %q, got %v", want, desc.ContentTypes)
		}
	}
}

// A transport failure must surface as an error, never as an empty catalogue:
// a broken scraper and a source with nothing to show are different states.
func TestEneyidaTransportFailureIsNotAnEmptyCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err == nil {
		t.Fatalf("expected an error, got items %v", items)
	}
}

func TestEneyidaSearchHitsSearchEndpoint(t *testing.T) {
	srv, seen := catalogServer(t, `<html><body></body></html>`)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	if _, err := p.Search(context.Background(), "Матриця & Ренесанс"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("expected one request, got %v", *seen)
	}
	if !strings.Contains((*seen)[0], "/index.php") {
		t.Errorf("search path = %q, want the DLE search endpoint", (*seen)[0])
	}
}

// ---- parsing ---------------------------------------------------------------

func TestEneyidaParsesCatalogMarkup(t *testing.T) {
	body := `<html><body>
      <article class="short">
        <div class="short_img"><img src="/img/dune.jpg"></div>
        <h2 class="short_title"><a href="/films/dune-2021.html">Дюна</a></h2>
        <div class="short_info">2021</div>
      </article>
      <article class="short">
        <div class="short_img"><img data-src="/img/serial.jpg"></div>
        <h2 class="short_title"><a href="/serials/fargo.html">Фарґо</a></h2>
        <div class="short_info">2014</div>
      </article>
    </body></html>`
	srv, _ := catalogServer(t, body)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}

	first := items[0]
	if first.Title != "Дюна" {
		t.Errorf("title = %q, want Дюна", first.Title)
	}
	if first.Year != 2021 {
		t.Errorf("year = %d, want 2021", first.Year)
	}
	if first.PosterURL != srv.URL+"/img/dune.jpg" {
		t.Errorf("poster = %q, want the absolute URL", first.PosterURL)
	}
	if first.ProviderID != "eneyida" {
		t.Errorf("providerID = %q, want eneyida", first.ProviderID)
	}
	if first.ID != "/films/dune-2021.html" || first.URL != "/films/dune-2021.html" {
		t.Errorf("href should become both id and url, got id=%q url=%q", first.ID, first.URL)
	}

	// Fallback-тип береться з href, коли в картці немає бейджа
	// «сезон/серія» (тобто вона не серіальна).
	if items[1].Type != "series" {
		t.Errorf("serial href should yield type series, got %q", items[1].Type)
	}
	if items[1].PosterURL != srv.URL+"/img/serial.jpg" {
		t.Errorf("data-src poster not used: %q", items[1].PosterURL)
	}
}

// Головний регресійний тест дефекту №1: на живому eneyida.tv картка
// виглядає як <a class="short_title" id="short_title" href=…>, а не
// <h2 class="short_title"><a…>. Старий селектор шукав нащадка у самого
// себе і повертав 0 карток навіть з живою розміткою.
//
// Фікстура testdata/eneyida_catalog.html — реальний фрагмент сторінки
// /films/, знятий 2026-10.
func TestEneyidaParsesLiveCatalogFixture(t *testing.T) {
	items, srvURL := parseEneyidaFixture(t, "films")

	if len(items) != 5 {
		t.Fatalf("expected 5 usable cards from the live fixture, got %d: %+v", len(items), items)
	}

	type want struct {
		title, origTitle, mediaType string
		year                        int
	}
	wants := []want{
		{"Сальмокджі: Шепіт води", "Salmokji: Whispering Water", "movie", 2026},
		// Картка з бейджем «2 сезон 1 серія», розібрана в розділі
		// /series/ (те саме фікстура віддається для кожного розділу
		// нижче).
		{"Темна матерія", "Dark Matter", "series", 2024},
		{"Поза межами часу", "Guangyin Zhi Wai", "series", 2026},
		{"Звільнити ту відьму", "Release that Witch", "series", 2026},
		// Кореневий href без жодного маркера + .short_subtitle без лінка
		// року: назва лишається текстом підзаголовка.
		{"Матриця: Воскресіння", "The Matrix Resurrections", "movie", 0},
	}
	for i, w := range wants {
		got := items[i]
		if got.Title != w.title {
			t.Errorf("item %d: title = %q, want %q", i, got.Title, w.title)
		}
		if got.OriginalTitle != w.origTitle {
			t.Errorf("item %d (%s): original title = %q, want %q", i, got.Title, got.OriginalTitle, w.origTitle)
		}
		if got.Year != w.year {
			t.Errorf("item %d (%s): year = %d, want %d", i, got.Title, got.Year, w.year)
		}
		if got.Type != w.mediaType {
			t.Errorf("item %d (%s): type = %q, want %q", i, got.Title, got.Type, w.mediaType)
		}
		if got.ProviderID != "eneyida" {
			t.Errorf("item %d: providerID = %q, want eneyida", i, got.ProviderID)
		}
		if got.ID != got.URL || got.ID == "" {
			t.Errorf("item %d: id/url mismatch: id=%q url=%q", i, got.ID, got.URL)
		}
	}

	// Постер береться з data-src (на живому сайті src немає) і
	// склеюється з baseURL.
	if want := srvURL + "/uploads/posts/2026-09/mv5bmte5nzg1mzytmdg5ys00zjyyltg2.webp"; items[0].PosterURL != want {
		t.Errorf("poster = %q, want %q", items[0].PosterURL, want)
	}
}

// Розділ розрізняє аніме/мультсеріал/серіал: бейдж «N сезон M серія» у
// них однаковий, тож без розділу все епізодичне читалося б як «series».
// Живі слади: /series/, /anime/, /cartoon-series/, /films/.
func TestEneyidaCatalogTypeFollowsSection(t *testing.T) {
	cases := []struct {
		section string
		want    map[string]string // title -> type
	}{
		{"films", map[string]string{
			"Сальмокджі: Шепіт води": "movie",
			"Матриця: Воскресіння":   "movie",
		}},
		{"series", map[string]string{
			"Темна матерія":       "series",
			"Поза межами часу":    "series",
			"Звільнити ту відьму": "series",
		}},
		{"anime", map[string]string{
			"Поза межами часу":    "anime",
			"Темна матерія":       "anime",
			"Звільнити ту відьму": "anime",
		}},
		{"cartoon", map[string]string{
			"Звільнити ту відьму": "cartoon",
			"Темна матерія":       "cartoon",
		}},
		{"cartoon-series", map[string]string{
			"Звільнити ту відьму": "cartoon",
			"Поза межами часу":    "cartoon",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.section, func(t *testing.T) {
			items, _ := parseEneyidaFixture(t, tc.section)
			byTitle := map[string]string{}
			for _, it := range items {
				byTitle[it.Title] = it.Type
			}
			for title, want := range tc.want {
				if got := byTitle[title]; got != want {
					t.Errorf("[%s] %q: type = %q, want %q", tc.section, title, got, want)
				}
			}
		})
	}
}

// Картки без .short_title або без href пропускаються, а не видаються
// порожніми.
func TestEneyidaLiveFixtureSkipsUnusableCards(t *testing.T) {
	items, _ := parseEneyidaFixture(t, "films")
	for _, it := range items {
		if it.Title == "Без назви" || it.Title == "Без посилання" {
			t.Errorf("unusable card leaked into the catalogue: %+v", it)
		}
		if it.ID == "" {
			t.Errorf("card without href leaked: %+v", it)
		}
	}
}

// classifyEneyidaType — окрема таблиця, бо саме тут була розбіжність
// між каталогом і деталями: кореневий href /6782-matrycia.html не
// містить жодного маркера, тож тип міг визначатися лише текстом і
// розділом.
func TestClassifyEneyidaType(t *testing.T) {
	const rootHref = "https://eneyida.tv/6782-matrycia.html"

	cases := []struct {
		name     string
		cardText string
		href     string
		section  string
		want     string
	}{
		{"фільм без маркерів", "FHD 1080p", rootHref, "films", "movie"},
		{"серіал за бейджем", "2 сезон 1 серія", rootHref, "series", "series"},
		{"бейдж без розділу (пошук)", "2 сезон 1 серія", rootHref, "", "series"},
		{"бейдж у розділі anime", "1 сезон 12 серія", rootHref, "anime", "anime"},
		{"бейдж у розділі cartoon", "1 сезон 8 серія", rootHref, "cartoon", "cartoon"},
		{"бейдж у розділі cartoon-series", "1 сезон 8 серія", rootHref, "cartoon-series", "cartoon"},
		{"слово «серіал» у тексті", "серіал", rootHref, "", "series"},
		{"фолбек: посилання на жанр-розділ", "фільм", "https://eneyida.tv/anime/", "", "anime"},
		{"фолбек: мультсеріал", "фільм", "https://eneyida.tv/cartoon-series/", "", "cartoon"},
		{"фолбек: мультфільм", "фільм", "https://eneyida.tv/cartoon/", "", "cartoon"},
		{"фолбек: фільм", "фільм", "https://eneyida.tv/films/", "", "movie"},
		// Звичайний жанр (/sci-fi/) не має розділу — лишаємо movie.
		{"фолбек: не розділ", "фільм", "https://eneyida.tv/sci-fi/", "", "movie"},
		{"фолбек: кореневий href", "фільм", rootHref, "", "movie"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyEneyidaType(tc.cardText, tc.href, tc.section); got != tc.want {
				t.Errorf("classifyEneyidaType(%q, %q, %q) = %q, want %q",
					tc.cardText, tc.href, tc.section, got, tc.want)
			}
		})
	}
}

// mergePlaylistSeasons зводить (озвучка, сезон) до одного списку.
// Без нього клієнт бачив би «Сезон 1» двічі для серіалу з двома
// озвучками — BuildSeasonsFromPlaylist групує саме так.
func TestMergePlaylistSeasons(t *testing.T) {
	in := []domain.Season{
		{Number: 1, Title: "Сезон 1", Episodes: []domain.Episode{{Number: 1}, {Number: 3}}},
		{Number: 1, Title: "Сезон 1", Episodes: []domain.Episode{{Number: 1}, {Number: 2}}},
		{Number: 2, Title: "Сезон 2", Episodes: []domain.Episode{{Number: 1}}},
	}
	got := mergePlaylistSeasons(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 merged seasons, got %+v", got)
	}
	if got[0].Number != 1 || got[1].Number != 2 {
		t.Errorf("season numbers = %d,%d want 1,2", got[0].Number, got[1].Number)
	}
	if len(got[0].Episodes) != 3 {
		t.Errorf("season 1 episodes = %d, want 3 (union without duplicates): %+v", len(got[0].Episodes), got[0].Episodes)
	}
	for i, ep := range got[0].Episodes {
		if ep.Number != i+1 {
			t.Errorf("season 1 episode[%d].Number = %d, want %d (sorted)", i, ep.Number, i+1)
		}
	}
	if len(got[1].Episodes) != 1 {
		t.Errorf("season 2 episodes = %d, want 1", len(got[1].Episodes))
	}
	if mergePlaylistSeasons(nil) != nil {
		t.Error("empty input must stay nil")
	}
}

// parseEneyidaFixture віддає фікстуру через тестовий сервер із
// зазначеним «розділом» (щоб перевірити вплив розділу на тип).
// Повертає also baseURL тестового сервера — за ним перевіряється
// абсолютизація постерів.
func parseEneyidaFixture(t *testing.T, section string) ([]domain.MediaItem, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "eneyida_catalog.html"))
	if err != nil {
		t.Fatalf("cannot read eneyida fixture: %v", err)
	}
	srv, _ := catalogServer(t, string(raw))
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.fetchCatalog(context.Background(), srv.URL+"/"+section+"/", section)
	if err != nil {
		t.Fatalf("fetchCatalog: %v", err)
	}
	return items, srv.URL
}

// ---- details: селектори за живою розміткою ------------------------------

// Сторінка опису + плеєр у iframe. Обидва знімки — з testdata/.
//
// Хост плеєра в знімку живий (hdvbua.pro), тому підміняємо його на
// тестовий сервер: інакше тест ходив би в інтернет і падав би разом
// з hdvbua. Плейлист усередині плеєра лишається живим текстом — саме
// його й розбирає код.
func eneyidaDetailsServer(t *testing.T) *httptest.Server {
	t.Helper()
	read := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("cannot read fixture %s: %v", name, err)
		}
		return string(raw)
	}
	item := read("eneyida_item_series.html")
	player := read("eneyida_player_hdvb.html")

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	item = strings.ReplaceAll(item, "https://hdvbua.pro", srv.URL)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/9516-temna-materiia-2024.html":
			_, _ = w.Write([]byte(item))
		case "/embed/8667/b0c42c552":
			_, _ = w.Write([]byte(player))
		default:
			http.NotFound(w, r)
		}
	})
	return srv
}

func TestEneyidaGetDetailsParsesLiveMarkup(t *testing.T) {
	srv := eneyidaDetailsServer(t)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	d, err := p.GetDetails(context.Background(), srv.URL+"/9516-temna-materiia-2024.html")
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Title != "Темна матерія" {
		t.Errorf("title = %q, want Темна матерія", d.Title)
	}
	// .full_orig-title на живому сайті немає; оригінальна назва лежить у
	// .full_header-subtitle.
	if d.OriginalTitle != "Dark Matter" {
		t.Errorf("original title = %q, want Dark Matter", d.OriginalTitle)
	}
	if d.PosterURL != srv.URL+"/uploads/posts/2024-05/1715241899_p.jpg" {
		t.Errorf("poster = %q, want the joined uploads URL", d.PosterURL)
	}
	// .full-text у шаблоні eneyida — це блок коментарів. Опис має
	// приходити з .full_content-desc.
	if !strings.Contains(d.Description, "Викрадений Джейсон Дессен") {
		t.Errorf("description = %q, want the .full_content-desc text (not the comment block)", d.Description)
	}
	if strings.Contains(d.Description, "Коментар глядача") {
		t.Errorf("description leaked the comment block: %q", d.Description)
	}

	// ul.full_info li — реальна розмітка, а не .full_info-item.
	if d.Year != 2024 {
		t.Errorf("year = %d, want 2024", d.Year)
	}
	if d.Rating != 7.3 {
		t.Errorf("rating = %v, want 7.3 from .r_imdb", d.Rating)
	}
	if d.Duration != "00:50:00" {
		t.Errorf("duration = %q, want 00:50:00", d.Duration)
	}
	if len(d.Genres) != 3 || d.Genres[0] != "серіал" {
		t.Errorf("genres = %v, want [серіал фантастика драма]", d.Genres)
	}
	if len(d.Countries) != 1 || d.Countries[0] != "United States of America" {
		t.Errorf("countries = %v", d.Countries)
	}
	if len(d.Actors) != 2 || d.Actors[0] != "Joel Edgerton" {
		t.Errorf("actors = %v", d.Actors)
	}
	if d.Director != "Jakob Verbruggen, Roxann Dawson" {
		t.Errorf("director = %q, want both directors joined", d.Director)
	}
	// Тип — з першого посилання в рядку «Жанр:», бо URL сторінки
	// кореневий (/9516-...html) і жодного маркера не містить.
	if d.Type != "series" {
		t.Errorf("type = %q, want series", d.Type)
	}
}

// Сезони й озвучки з дерева PlayerJS-плейлиста. Плейлист на eneyida живе
// в iframe hdvbua.pro, а не в HTML сторінки опису — тож GetDetails має
// дозавантажити кандидата плеєра.
func TestEneyidaGetDetailsFillsSeasonsAndVoiceovers(t *testing.T) {
	srv := eneyidaDetailsServer(t)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	d, err := p.GetDetails(context.Background(), srv.URL+"/9516-temna-materiia-2024.html")
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if len(d.Seasons) != 2 {
		t.Fatalf("expected 2 seasons from the playlist tree, got %d: %+v", len(d.Seasons), d.Seasons)
	}
	if d.Seasons[0].Number != 1 || d.Seasons[1].Number != 2 {
		t.Errorf("season numbers = %d,%d want 1,2", d.Seasons[0].Number, d.Seasons[1].Number)
	}
	// Сезон 1 об’єднує дві озвучки: 2 серії «Цікава Ідея» + 2 «HDrezka».
	// Без mergePlaylistSeasons клієнт бачив би «Сезон 1» двічі.
	if len(d.Seasons[0].Episodes) != 2 {
		t.Errorf("season 1 episodes = %d, want 2 after merging dubs: %+v", len(d.Seasons[0].Episodes), d.Seasons[0].Episodes)
	}
	if len(d.Seasons[1].Episodes) != 3 {
		t.Errorf("season 2 episodes = %d, want 3", len(d.Seasons[1].Episodes))
	}

	if len(d.Voiceovers) != 2 {
		t.Fatalf("expected 2 voiceovers, got %d: %+v", len(d.Voiceovers), d.Voiceovers)
	}
	ids := map[string]bool{}
	seasonsPer := map[string]int{}
	for _, v := range d.Voiceovers {
		if v.ID == "" || v.Name == "" {
			t.Errorf("voiceover without id/name: %+v", v)
		}
		ids[v.Name] = true
		seasonsPer[v.Name] = len(v.Seasons)
	}
	if !ids["Цікава Ідея"] || !ids["HDrezka Studio"] {
		t.Fatalf("voiceovers = %v, want the two real dub names", ids)
	}
	// Кожна озвучка несе ПОВНИЙ набір своїх сезонів (не об'єднаний):
	// саме з нього GetStreams через SelectPlaylistStream фільтрує
	// потрібну студію. «HDrezka Studio» на живому сайті є лише в
	// першому сезоні.
	if seasonsPer["Цікава Ідея"] != 2 {
		t.Errorf("«Цікава Ідея» carries %d seasons, want 2", seasonsPer["Цікава Ідея"])
	}
	if seasonsPer["HDrezka Studio"] != 1 {
		t.Errorf("«HDrezka Studio» carries %d seasons, want 1", seasonsPer["HDrezka Studio"])
	}

	// Перевірка ref: кожна серія мусить нести адресу сторінки в конверті,
	// щоб клієнт міг передати її в GetStreams без 400 invalid URL.
	epURL := d.Seasons[0].Episodes[0].URL
	decodedURL, s, ep, _, ok := DecodeSelectionRef(epURL)
	wantURL := srv.URL + "/9516-temna-materiia-2024.html"
	if !ok || decodedURL != wantURL || s != 1 || ep != 1 {
		t.Errorf("episode URL = %q, want selection ref carrying %q (got %q, s=%d, ep=%d, ok=%v)",
			epURL, wantURL, decodedURL, s, ep, ok)
	}
}

// Значення без посилань (суцільний текст) теж мають розбиратися:
// «Тривалість: 00:50:00» — це не anchor, а «Мова озвучення: Цікава ідея,
// HDrezka Studio» розбивається по комі.
func TestEneyidaGetDetailsParsesPlainTextRows(t *testing.T) {
	body := `<html><body>
	  <h1>Матриця: Воскресіння</h1>
	  <ul class="full_info" id="full_info">
	    <li><span>Рік:</span> 2021</li>
	    <li class="vis"><span>Жанр:</span> фільм • бойовик • фантастика</li>
	    <li class="vis"><span>Тривалість:</span> 02:49:00</li>
	    <li class="vis"><span>Мова озвучення:</span> Цікава ідея, HDrezka Studio</li>
	  </ul>
	</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	d, err := p.GetDetails(context.Background(), srv.URL+"/6782-matrycia.html")
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}
	if d.Year != 2021 {
		t.Errorf("year = %d, want 2021", d.Year)
	}
	if d.Duration != "02:49:00" {
		t.Errorf("duration = %q, want 02:49:00 (без мітки)", d.Duration)
	}
	if len(d.Genres) != 3 || d.Genres[0] != "фільм" || d.Genres[2] != "фантастика" {
		t.Errorf("genres = %v, want the plain-text row split on •", d.Genres)
	}
}

// ---- дефект №3: трейлер не має потрапляти у стрими -----------------------

// Реальна сторінка eneyida.tv має ДВА iframe з однаковим score:
//
//	https://hdvbua.pro/embed/8667/b0c42c552  <- справжній плеєр
//	https://hdvbua.pro/vid/97206?tr=1        <- трейлер
//
// Слова «trailer» в URL трейлера немає, тож старий iframeSkips його не
// ловив, а m3u8 з /hls/trailers/ потрапляв у список стримів.
const eneyidaTrailerFrame = `<!DOCTYPE html><html><body><script>
var p = new Playerjs({file: "https://s30.hdvbua.pro/media1/hls/trailers/dark.matter.2024_97206/hls/index.m3u8"});
</script></body></html>`

func TestEneyidaTrailerIframeIsNotAStreamCandidate(t *testing.T) {
	itemPage := `<html><body><div class="tabs_box" id="tabs_box">
	  <div class="video_box tabs_b visible">
	    <iframe src="https://hdvbua.pro/embed/8667/b0c42c552"></iframe>
	  </div>
	  <div class="video_box tabs_b" id="trailer_place">
	    <iframe src="https://hdvbua.pro/vid/97206?tr=1"></iframe>
	  </div>
	</div></body></html>`

	got := rankPlayerCandidates(itemPage, "https://eneyida.tv/9516-temna-materiia-2024.html")
	if len(got) != 1 {
		t.Fatalf("expected only the real player candidate, got %d: %+v", len(got), got)
	}
	if got[0].URL != "https://hdvbua.pro/embed/8667/b0c42c552" {
		t.Errorf("candidate = %q, want the /embed/ player", got[0].URL)
	}
	if score := scorePlayerIframe("https://hdvbua.pro/vid/97206?tr=1", "eneyida.tv"); score != 0 {
		t.Errorf("trailer iframe score = %d, want 0 (must be skipped)", score)
	}
	if isPlausiblePlayerOrMedia("https://hdvbua.pro/vid/97206?tr=1") {
		t.Error("?tr=1 iframe must not be plausible player-or-media")
	}
}

// Другий рубеж: навіть якщо iframe трейлера не відкинули, його m3u8 не
// має пройти валідацію медіа.
func TestTrailerMediaURLIsNotPlayable(t *testing.T) {
	bad := []string{
		"https://s30.hdvbua.pro/media1/hls/trailers/dark.matter.2024_97206/hls/index.m3u8",
		"https://calypso.tortuga.tw/hls/trailers/b/hls/index.m3u8",
	}
	for _, raw := range bad {
		if isPlayableMediaURL(raw) {
			t.Errorf("trailer media URL %q must not be playable", raw)
		}
	}
	good := []string{
		"https://s30.hdvbua.pro/media1/hls/serials/dark.matter.s01e01.ci.mvo_97207/hls/index.m3u8",
		"https://cdn.example/hls/master.m3u8",
	}
	for _, raw := range good {
		if !isPlayableMediaURL(raw) {
			t.Errorf("episode media URL %q must stay playable", raw)
		}
	}
}

// E2E: сторінка з двома iframe, де трейлер віддає справжній m3u8.
func TestEneyidaGetStreamsSkipsTrailerFrame(t *testing.T) {
	player := `<!DOCTYPE html><html><body><script>
var p = new Playerjs({file: '[{"title":"1 сезон","folder":[{"title":"Цікава Ідея","folder":[{"title":"1 серія","file":"https://s30.hdvbua.pro/media1/hls/serials/dark.matter.s01e01.ci.mvo_97207/hls/index.m3u8"}]}]}]'});
</script></body></html>`

	item := `<html><body><div class="tabs_box" id="tabs_box">
	  <div class="video_box tabs_b visible">
	    <iframe src="/embed/8667/b0c42c552"></iframe>
	  </div>
	  <div class="video_box tabs_b" id="trailer_place">
	    <iframe src="/vid/97206?tr=1"></iframe>
	  </div>
	</div></body></html>`

	mux := http.NewServeMux()
	mux.HandleFunc("/item", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(item))
	})
	mux.HandleFunc("/embed/8667/b0c42c552", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(player))
	})
	mux.HandleFunc("/vid/97206", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(eneyidaTrailerFrame))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	resp, err := p.GetStreams(context.Background(), srv.URL+"/item", 1, 1, "")
	if err != nil {
		t.Fatalf("GetStreams: %v", err)
	}
	if len(resp.Streams) != 1 {
		t.Fatalf("expected exactly 1 stream (the episode), got %d: %+v", len(resp.Streams), resp.Streams)
	}
	if strings.Contains(resp.Streams[0].URL, "/trailers/") {
		t.Errorf("trailer leaked into streams: %s", resp.Streams[0].URL)
	}
	if !strings.Contains(resp.Streams[0].URL, "dark.matter.s01e01") {
		t.Errorf("wrong stream picked: %s", resp.Streams[0].URL)
	}
	// Voiceover має бути студією з дерева, а не іменем CDN.
	if resp.Streams[0].Voiceover != "Цікава Ідея" {
		t.Errorf("voiceover = %q, want «Цікава Ідея» (not the balancer name)", resp.Streams[0].Voiceover)
	}
}

var _ = domain.MediaItem{}
