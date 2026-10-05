package provider

// Тести провайдера uaserials.com.
//
// Усі фікстури зняті з живого сайту 2026-10-05 і лежать у testdata/.
// Тести не ходять у мережу: перевіряються чисті функції розбору,
// розшифровки та відбору вкладок — тобто саме ті місця, де в першій
// версії провайдера були реальні дефекти (нуль карток у пошуку,
// трейлер у списку потоків, паніка на битому шифротексті).

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

func loadUaserialsFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}

// ---- парс карток ----

// TestUaserialsSearchCardsNotEmpty — регресія на «0 карток у пошуку».
//
// Перша версія провайдера шукала «.th-title a». На сторінці пошуку
// (uaserials_search.html) картки мають іншу розмітку — a.uas-card, і
// .th-title там узагалі немає. Перевірка падала б, якби ми знову
// розбирали лише .short-item.
func TestUaserialsSearchCardsNotEmpty(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	items := p.parseUaserialsCards(loadUaserialsFixture(t, "uaserials_search.html"), "")

	if len(items) == 0 {
		t.Fatal("search fixture produced 0 cards — the .th-title a regression is back")
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 search cards, got %d", len(items))
	}

	first := items[0]
	if first.Title != "Ніглі" {
		t.Errorf("first title = %q, want %q", first.Title, "Ніглі")
	}
	if first.OriginalTitle != "Neagley" {
		t.Errorf("first original title = %q, want %q", first.OriginalTitle, "Neagley")
	}
	if first.URL != "https://uaserials.com/12722-nigli.html" {
		t.Errorf("first url = %q", first.URL)
	}
	if first.PosterURL != "https://uaserials.com/posters/12722.jpg" {
		t.Errorf("poster must be absolutised, got %q", first.PosterURL)
	}
	if first.ProviderID != "uaserials" {
		t.Errorf("provider id = %q", first.ProviderID)
	}
	if first.Year != 2026 {
		t.Errorf("year = %d, want 2026", first.Year)
	}
	if first.Rating != 7.7 {
		t.Errorf("rating = %v, want 7.7", first.Rating)
	}
	if first.ID != first.URL {
		t.Errorf("id %q must equal url %q", first.ID, first.URL)
	}

	// Картки людей (/person/...) — не медіа. Якщо вони протікають у
	// результат, клієнт отримає «пост» без гравця.
	for _, it := range items {
		if strings.Contains(it.URL, "/person/") {
			t.Fatalf("person link leaked into results: %q", it.URL)
		}
	}
}

// TestUaserialsSearchCollectPostIDs — збирання id карток для вкладок
// «Серіали»/«Мультсеріали». Люди мають теж data-uas-id, тому перевіряємо
// саме вибірку a.uas-card.
func TestUaserialsSearchCollectPostIDs(t *testing.T) {
	ids := uaserialsCollectPostIDs(loadUaserialsFixture(t, "uaserials_search.html"))
	if len(ids) != 5 {
		t.Fatalf("expected 5 post ids, got %d: %v", len(ids), ids)
	}
	if !ids["12753"] {
		t.Error("post 12753 (Вбити Джекі) must be in the id set")
	}
	if ids["7"] {
		t.Error("person id 7 must not be treated as a post")
	}
}

func TestUaserialsPostID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://uaserials.com/12753-vbyty-dzheki.html", "12753"},
		{"https://uaserials.com/422-velychne-stolittya-roksolana-2011s.html", "422"},
		{"https://uaserials.com/series/", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := uaserialsPostID(c.in); got != c.want {
			t.Errorf("uaserialsPostID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestUaserialsCatalogCards — картки розділу з міткою «1 сезон».
func TestUaserialsCatalogCards(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	items := p.parseUaserialsCards(loadUaserialsFixture(t, "uaserials_catalog.html"), "series")

	if len(items) != 2 {
		t.Fatalf("expected 2 catalog cards, got %d", len(items))
	}
	if items[0].Title != "Вбити Джекі" {
		t.Errorf("title = %q", items[0].Title)
	}
	if items[0].Type != "series" {
		t.Errorf("card with «1 сезон» must be a series, got %q", items[0].Type)
	}
	if items[0].PosterURL != "https://uaserials.com/posters/12753.jpg" {
		t.Errorf("poster = %q (lazyload placeholder must not win)", items[0].PosterURL)
	}
	if items[1].Title != "Квантовий стрибок" {
		t.Errorf("second title = %q", items[1].Title)
	}
	if items[1].Type != "series" {
		t.Errorf("second card type = %q", items[1].Type)
	}
}

// ---- розшифровка data-tag ----

type uaserialsVector struct {
	Passphrase string       `json:"passphrase"`
	Tag        uaserialsTag `json:"tag"`
	Plain      string       `json:"plain"`
}

// TestUaserialsDecryptTagVector — еталонна розшифровка.
//
// Очікуваний результат зафіксовано у фікстурі: без нього тест був би
// самопідтвердженням («повернулось те, що ми самі й поклали»).
func TestUaserialsDecryptTagVector(t *testing.T) {
	var v uaserialsVector
	if err := json.Unmarshal([]byte(loadUaserialsFixture(t, "uaserials_tag_vector.json")), &v); err != nil {
		t.Fatalf("decode vector: %v", err)
	}

	if v.Passphrase != uaserialsDefaultKey {
		t.Fatalf("vector passphrase %q differs from the compiled default %q",
			v.Passphrase, uaserialsDefaultKey)
	}

	raw, err := json.Marshal(v.Tag)
	if err != nil {
		t.Fatalf("re-encode tag: %v", err)
	}

	got, err := decryptUaserialsTag(v.Passphrase, string(raw))
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != v.Plain {
		t.Fatalf("plaintext mismatch\n got: %q\nwant: %q", got, v.Plain)
	}

	tabs, err := parseUaserialsTabs(got)
	if err != nil {
		t.Fatalf("parse tabs: %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d: %+v", len(tabs), tabs)
	}
	if tabs[0].TabName != "Плеєр" || tabs[0].URL != "https://tortuga.tw/embed/98" {
		t.Errorf("player tab = %+v", tabs[0])
	}
	if tabs[1].TabName != "Трейлер" || tabs[1].URL != "https://tortuga.tw/vod/5008" {
		t.Errorf("trailer tab = %+v", tabs[1])
	}
}

// TestUaserialsDecryptRejectsGarbage — битий вхід дає помилку, а не паніку.
//
// GetStreams викликає цю функцію на даних із чужої сторінки, тому
// «errors.New замість panic» тут не педантизм, а вимога.
func TestUaserialsDecryptRejectsGarbage(t *testing.T) {
	cases := []struct {
		name       string
		passphrase string
		raw        string
	}{
		{"empty", uaserialsDefaultKey, ""},
		{"blank", uaserialsDefaultKey, "   "},
		{"not json", uaserialsDefaultKey, "<b>404</b>"},
		{"missing fields", uaserialsDefaultKey, `{"ciphertext":"","iv":"","salt":""}`},
		{"ciphertext not base64", uaserialsDefaultKey, `{"ciphertext":"!!!","iv":"e454641dbf7a315191dd7b6547771a42","salt":"aa"}`},
		{"salt not hex", uaserialsDefaultKey, `{"ciphertext":"QUJD","iv":"e454641dbf7a315191dd7b6547771a42","salt":"zz"}`},
		{"iv not hex", uaserialsDefaultKey, `{"ciphertext":"QUJD","iv":"zz","salt":"aabb"}`},
		{"short iv", uaserialsDefaultKey, `{"ciphertext":"QUJD","iv":"aabb","salt":"aabb"}`},
		{"ciphertext not block aligned", uaserialsDefaultKey, `{"ciphertext":"QUJDR","iv":"e454641dbf7a315191dd7b6547771a42","salt":"aabb"}`},
		{"empty passphrase", "", `{"ciphertext":"QUJD","iv":"e454641dbf7a315191dd7b6547771a42","salt":"aabb"}`},
		{"wrong key", "0000000000000000000", `{"ciphertext":"4f28gsXfsdAoS5ApG3bZFnKy8Esfh0EI8WTyOqBgGqDS61Jv0k7C1KBkWVJryc1sTPmhZJZ4r3IB82TpQWCwvi2Ksh45CiaDvNQEF5TvQXRVAdqaCqx7q/gkDql3cYK4flf343PFv4qe/iWvNtYtSRVzoHb856v2jJcv07sijEX1mnNmnp9NpK4S+L/6mWIR","iv":"e454641dbf7a315191dd7b6547771a42","salt":"556e6fce84fec3e81764b787d888a6565278f67635ba056555d0af9f8680bb9088117341bab6896cc59ff1894001bd6da14be30e1c3844d9df05930d6141dad2e374024b0b14caf0be414bd797d2a35b712e2f89c96f491d19be1a75ee7fb267e951888e34d7bb08bdeb4b0a38e3dfa3590982b899682d8689521f43a7fff6ac459fe004048129c86b6c7e86ad5d74ee6850e576d25ff358b59de55e4072da59fcb8e5adea03f135610b0008cbefea78a93cade4c9a16a87ce7e2209f2758060ea10a19a630dd979e3a43a388fb4dc5b3c4c25a63713f46ebe3608d2b02758a59366f26ccf6d1f51a4ab6dfa075a9c565c3d4f3c517c351ecee867a092c64c63"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := decryptUaserialsTag(c.passphrase, c.raw)
			if err == nil {
				t.Fatalf("expected an error, got plaintext %q", out)
			}
			if !errors.Is(err, ErrUaserialsDecrypt) {
				t.Fatalf("error must wrap ErrUaserialsDecrypt, got %v", err)
			}
			if out != "" {
				t.Fatalf("failed decrypt must not return a partial plaintext: %q", out)
			}
		})
	}
}

func TestUaserialsStripPKCS7(t *testing.T) {
	got, err := stripPKCS7([]byte("abc" + string([]byte{3, 3, 3})))
	if err != nil {
		t.Fatalf("valid padding rejected: %v", err)
	}
	if string(got) != "abc" {
		t.Fatalf("got %q, want %q", got, "abc")
	}

	for _, bad := range [][]byte{
		{'a', 0},                  // нульова довжина вирівнювання
		{'a', 'b', 'c', 'd', 'e'}, // заявлено більше, ніж є в буфері
		{'a', 3, 'b', 'c'},        // байти хвоста не збігаються
	} {
		if _, err := stripPKCS7(bad); err == nil {
			t.Errorf("invalid padding %v must be rejected", bad)
		}
	}
}

func TestUaserialsParseTabsRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "   ", "not json", "[]", `[{"tabName":"x"}]`, `[{"tabName":"x","url":"  "}]`} {
		if _, err := parseUaserialsTabs(bad); err == nil {
			t.Errorf("parseUaserialsTabs(%q) must fail", bad)
		}
	}
}

// ---- теги на сторінці ----

func TestUaserialsExtractTags(t *testing.T) {
	tags := extractUaserialsTags(loadUaserialsFixture(t, "uaserials_roksolana.html"))
	if len(tags) != 1 {
		t.Fatalf("expected exactly one data-tag, got %d", len(tags))
	}
	if !strings.Contains(tags[0], `"ciphertext"`) || !strings.Contains(tags[0], `"salt"`) {
		t.Fatalf("tag does not look like the encrypted payload: %.120s", tags[0])
	}

	if got := extractUaserialsTags(`<div>no player here</div>`); len(got) != 0 {
		t.Fatalf("page without player-control must yield no tags, got %d", len(got))
	}
}

// TestUaserialsResolveTabsNoTags — сторінка без плеєра: зрозуміла помилка.
func TestUaserialsResolveTabsNoTags(t *testing.T) {
	p := &UaserialsProvider{baseURL: uaserialsDefaultBaseURL, key: uaserialsDefaultKey}
	_, err := p.resolveUaserialsTabs(context.Background(), "<html><body>404</body></html>")
	if !errors.Is(err, ErrUaserialsNoTags) {
		t.Fatalf("expected ErrUaserialsNoTags, got %v", err)
	}
}

// TestUaserialsResolveTabsGarbageCiphertext — GetStreams-рівний шлях на
// битому data-tag: помилка, а не паніка і не порожній список «успіху».
func TestUaserialsResolveTabsGarbageCiphertext(t *testing.T) {
	// client == nil: refreshKeyFromBundle мусить перевірити це ДО
	// звернення до мережі, інакше тест пішов би в інтернет.
	p := &UaserialsProvider{baseURL: uaserialsDefaultBaseURL, key: uaserialsDefaultKey}

	garbage := []string{
		`<player-control data-tag1='not-json'></player-control>`,
		`<player-control data-tag1='{"ciphertext":"","iv":"","salt":""}'></player-control>`,
		`<player-control data-tag1='{"ciphertext":"QUJD","iv":"zz","salt":"zz"}'></player-control>`,
		`<player-control data-tag2='{"ciphertext":"QUJD","iv":"e454641dbf7a315191dd7b6547771a42","salt":"aabb"}'></player-control>`,
	}
	for _, html := range garbage {
		tabs, err := p.resolveUaserialsTabs(context.Background(), html)
		if err == nil {
			t.Fatalf("expected an error for %s, got %+v", html, tabs)
		}
		if tabs != nil {
			t.Fatalf("failed resolve must not return tabs: %+v", tabs)
		}
		if !errors.Is(err, ErrUaserialsDecrypt) {
			t.Fatalf("expected ErrUaserialsDecrypt, got %v", err)
		}
	}
}

// TestUaserialsResolveTabsAcceptsRealFixture — щасливий шлях на реальному
// тегу, без мережі (підфатсований провайдер з nil-клієнтом).
func TestUaserialsResolveTabsAcceptsRealFixture(t *testing.T) {
	p := &UaserialsProvider{baseURL: uaserialsDefaultBaseURL, key: uaserialsDefaultKey}
	tabs, err := p.resolveUaserialsTabs(context.Background(), loadUaserialsFixture(t, "uaserials_roksolana.html"))
	if err != nil {
		t.Fatalf("resolve tabs: %v", err)
	}
	players, trailer := selectUaserialsPlayers(tabs)
	if len(players) != 1 || players[0].URL != "https://tortuga.tw/embed/98" {
		t.Fatalf("players = %+v, want the single tortuga embed", players)
	}
	if trailer != "https://tortuga.tw/vod/5008" {
		t.Fatalf("trailer = %q", trailer)
	}
}

// ---- відбір вкладок: трейлери та чужі домени ----

func TestUaserialsSelectPlayers(t *testing.T) {
	tabs := []uaserialsTab{
		{TabName: "Плеєр", URL: "https://tortuga.tw/embed/98"},
		// На живому сайті трейлер НЕ містить слова «trailer» у URL.
		{TabName: "Трейлер", URL: "https://tortuga.tw/vod/5008"},
		// Англомовний варіант назви вкладки.
		{TabName: "Trailer", URL: "https://tortuga.tw/vod/777"},
		// Трейлер у самому URL.
		{TabName: "Ролик", URL: "https://cdn.example.com/hls/trailers/x/index.m3u8"},
		// Посилання на сам сайт — це не плеєр.
		{TabName: "Всі серії", URL: "https://uaserials.com/series/"},
		{TabName: "Реєстрація", URL: "https://uaserials.com/index.php?do=register"},
		// Сміття.
		{TabName: "", URL: "javascript:alert(1)"},
		{TabName: "", URL: "   "},
	}

	players, trailer := selectUaserialsPlayers(tabs)
	if len(players) != 1 {
		t.Fatalf("expected exactly one player, got %+v", players)
	}
	if players[0].URL != "https://tortuga.tw/embed/98" {
		t.Fatalf("player = %q", players[0].URL)
	}
	if trailer != "https://tortuga.tw/vod/5008" {
		t.Fatalf("first trailer = %q", trailer)
	}

	// Жоден URL не повинен бути на нашому власному домені: такий плеєр
	// ми б віддали резолверу, а він повернув би ErrUnresolvablePlayer.
	for _, pl := range players {
		if isSelfHostedURL(pl.URL) {
			t.Fatalf("self-hosted URL survived the filter: %q", pl.URL)
		}
	}
}

// TestUaserialsSelectPlayersRejectsAllTabs — коли вкладки є, але всі
// відкинуті (лише трейлери або внутрішні посилання), результат має бути
// порожнім. Саме цей стан GetStreams перетворює на ErrUnresolvablePlayer,
// а не на «успішну» відповідь без потоків.
func TestUaserialsSelectPlayersRejectsAllTabs(t *testing.T) {
	tabs := []uaserialsTab{
		{TabName: "Трейлер", URL: "https://tortuga.tw/vod/5008"},
		{TabName: "Всі серії", URL: "https://uaserials.com/series/"},
	}
	players, trailer := selectUaserialsPlayers(tabs)
	if len(players) != 0 {
		t.Fatalf("expected no players, got %+v", players)
	}
	if trailer != "https://tortuga.tw/vod/5008" {
		t.Fatalf("trailer = %q", trailer)
	}
}

// TestUaserialsExtractTagsSecondaryAttribute — друга вкладка приходить у
// data-tag2 (саме так її читає і клієнтський JS наживо: dataset.tag2).
// Якщо читати лише data-tag1, друга вкладка зникне без жодної помилки.
func TestUaserialsExtractTagsSecondaryAttribute(t *testing.T) {
	tags := extractUaserialsTags(loadUaserialsFixture(t, "uaserials_roksolana.html"))
	if len(tags) != 1 {
		t.Fatalf("fixture must carry one tag, got %d", len(tags))
	}

	html := `<player-control data-tag1="AAA" data-tag2="BBB" data-tag3="CCC"></player-control>`
	got := extractUaserialsTags(html)
	if len(got) != 3 {
		t.Fatalf("all data-tag* attributes must be read, got %v", got)
	}
	if got[0] != "AAA" || got[1] != "BBB" || got[2] != "CCC" {
		t.Fatalf("unexpected tag order/content: %v", got)
	}

	// Дедуплікація: два однакові значення — одне data-tag.
	dup := `<player-control data-tag1="AAA" data-tag2="AAA"></player-control>`
	if got := extractUaserialsTags(dup); len(got) != 1 {
		t.Fatalf("duplicate tags must collapse, got %v", got)
	}
}

// TestUaserialsShortListDedupAndSectionLink — синтетичний short-list із
// дубльованою країною та жанром-посиланням на корінь розділу.
func TestUaserialsShortListDedupAndSectionLink(t *testing.T) {
	doc := mustParseHTML(t, `<ul class="short-list">
		<li><span>Жанр:</span>
			<a href="https://uaserials.com/films/">Фільм</a>,
			<a href="https://uaserials.com/films/f-action/">Бойовик</a>
		</li>
		<li><span>Країна:</span>
			<a href="https://uaserials.com/country/ukraine/">Україна</a>,
			<a href="https://uaserials.com/country/ukraine-2/">Україна</a>,
			<a href="https://uaserials.com/country/turkey/">Туреччина</a>
		</li>
		<li><span>Тривалість:</span> 1 год. 38 хв.</li>
	</ul>`)

	meta := parseUaserialsShortList(doc)
	if len(meta.genres) != 1 || meta.genres[0] != "Бойовик" {
		t.Errorf("genres = %v (the /films/ root link is the material type, not a genre)", meta.genres)
	}
	if len(meta.countries) != 2 {
		t.Fatalf("countries must be deduplicated, got %v", meta.countries)
	}
	if meta.countries[0] != "Україна" || meta.countries[1] != "Туреччина" {
		t.Errorf("countries = %v", meta.countries)
	}
	if meta.duration != "1 год. 38 хв." {
		t.Errorf("duration = %q", meta.duration)
	}
}

func TestUaserialsIsPlayerURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://tortuga.tw/embed/98", true},
		{"http://cdn.example.com/embed/1", true},
		{"https://uaserials.com/series/", false},
		{"https://www.uaserials.com/series/", false},
		{"https://sub.uaserials.com/x", false},
		{"ftp://tortuga.tw/embed/98", false},
		{"javascript:alert(1)", false},
		{"/relative/path", false},
		{"", false},
		{"https://uaserials.com.evil.tld/embed/1", true}, // не наш домен
	}
	for _, c := range cases {
		if got := isUaserialsPlayerURL(c.in); got != c.want {
			t.Errorf("isUaserialsPlayerURL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestUaserialsSelfHostedURL(t *testing.T) {
	for _, in := range []string{"https://uaserials.com/x", "https://UASerials.com/x", "https://a.uaserials.com/x"} {
		if !isSelfHostedURL(in) {
			t.Errorf("%q must be recognised as self-hosted", in)
		}
	}
	for _, in := range []string{"https://tortuga.tw/embed/98", "https://notuaserials.com/x", ""} {
		if isSelfHostedURL(in) {
			t.Errorf("%q must not be treated as self-hosted", in)
		}
	}
}

// ---- деталі ----

func TestUaserialsDetailsParse(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	itemURL := "https://uaserials.com/422-velychne-stolittya-roksolana-2011s.html"
	d := p.parseUaserialsDetails(loadUaserialsFixture(t, "uaserials_roksolana.html"), itemURL)

	if d.Title != "Величне століття. Роксолана" {
		t.Errorf("title = %q", d.Title)
	}
	if d.OriginalTitle != "Muhtesem Yuzyil" {
		t.Errorf("original title = %q", d.OriginalTitle)
	}
	if d.PosterURL != "https://uaserials.com/posters/422.jpg" {
		t.Errorf("poster = %q", d.PosterURL)
	}
	if d.Year != 2011 {
		t.Errorf("year = %d, want 2011", d.Year)
	}
	if d.Rating != 7.0 {
		t.Errorf("rating = %v, want 7.0", d.Rating)
	}
	if d.Type != "series" {
		t.Errorf("type = %q, want series", d.Type)
	}
	if d.ProviderID != "uaserials" {
		t.Errorf("provider id = %q", d.ProviderID)
	}
	if d.URL != itemURL || d.ID != itemURL {
		t.Errorf("url/id mismatch: %q / %q", d.URL, d.ID)
	}
	if !strings.HasPrefix(d.Description, "Сюжет оповідає") {
		t.Errorf("description = %.60q", d.Description)
	}

	// Перше жанр-посилання — «Серіал» (/series/), це тип матеріалу,
	// а не жанр. Воно не має потрапити в Genres.
	wantGenres := []string{"Драма", "Біографічний", "Мелодрама", "Історичний"}
	if len(d.Genres) != len(wantGenres) {
		t.Fatalf("genres = %v, want %v", d.Genres, wantGenres)
	}
	for i := range wantGenres {
		if d.Genres[i] != wantGenres[i] {
			t.Fatalf("genres = %v, want %v", d.Genres, wantGenres)
		}
	}

	if len(d.Countries) != 1 || d.Countries[0] != "Туреччина" {
		t.Errorf("countries = %v", d.Countries)
	}
	if d.Director != "Дурул Тайлан, Ягмур Тайлан, Мерт Байкал" {
		t.Errorf("director = %q", d.Director)
	}
	if len(d.Actors) == 0 || d.Actors[0] != "Халіт Ергенч" {
		t.Errorf("actors = %v", d.Actors)
	}
	if d.Actors[len(d.Actors)-1] != "Пелін Карахан" {
		t.Errorf("last actor = %q (plain text actors must be split by comma too)",
			d.Actors[len(d.Actors)-1])
	}
}

// ---- потоки: контракт StreamSource ----

func TestUaserialsFinalizeStreams(t *testing.T) {
	itemURL := "https://uaserials.com/422-velychne-stolittya-roksolana-2011s.html"
	raw := []domain.StreamSource{
		{
			Quality:       "1080p",
			URL:           "https://proxy.internal/manifest",
			DirectURL:     "https://calypso.tortuga.tw/hls/serials/x/s01/index.m3u8",
			RequiresProxy: true,
			Headers:       map[string]string{"X-Stale": "1"},
		},
		{ // без DirectURL і без Headers — теж має бути приведено до контракту
			URL: "https://calypso.tortuga.tw/hls/serials/y/s01/index.m3u8",
		},
	}

	got := finalizeUaserialsStreams(raw, itemURL, uaserialsDefaultBaseURL)
	if len(got) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(got))
	}
	for _, s := range got {
		if s.URL != s.DirectURL {
			t.Errorf("URL %q must equal DirectURL %q (media is never proxied)", s.URL, s.DirectURL)
		}
		if s.RequiresProxy {
			t.Errorf("%s: RequiresProxy must be false", s.URL)
		}
		if s.Headers["User-Agent"] != Chrome120UserAgent {
			t.Errorf("%s: User-Agent = %q", s.URL, s.Headers["User-Agent"])
		}
		if s.Headers["Referer"] != itemURL {
			t.Errorf("%s: Referer = %q, want the item page", s.URL, s.Headers["Referer"])
		}
		if s.Headers["Origin"] != "https://uaserials.com" {
			t.Errorf("%s: Origin = %q", s.URL, s.Headers["Origin"])
		}
		if _, stale := s.Headers["X-Stale"]; stale {
			t.Error("stale headers must be dropped, not merged")
		}
	}
}

func TestUaserialsOrigin(t *testing.T) {
	if got := uaserialsOrigin("https://uaserials.com/422-x.html", "https://fallback.tld"); got != "https://uaserials.com" {
		t.Errorf("origin from item url = %q", got)
	}
	if got := uaserialsOrigin("", "https://fallback.tld/"); got != "https://fallback.tld" {
		t.Errorf("fallback origin = %q", got)
	}
}

// ---- дрібні довідники ----

func TestUaserialsVoiceName(t *testing.T) {
	// Voiceover.ID «1plus1» мусить перетворитися на назву «1+1», бо саме
	// нею підписані доріжки в плеєрі Tortuga.
	if got := uaserialsVoiceName("1plus1"); got != "1+1" {
		t.Errorf("uaserialsVoiceName(1plus1) = %q, want 1+1", got)
	}
	if got := uaserialsVoiceName(""); got != "" {
		t.Errorf("empty voice id must stay empty, got %q", got)
	}
	if got := uaserialsVoiceName("какая-то студия"); got != "какая-то студия" {
		t.Errorf("unknown studio must pass through, got %q", got)
	}
}

func TestUaserialsSections(t *testing.T) {
	cases := map[string]string{
		"movie":   "films",
		"series":  "series",
		"cartoon": "cartoons",
		"anime":   "anime",
		"":        "",
		"unknown": "",
	}
	for in, want := range cases {
		if got := uaserialsSection(in); got != want {
			t.Errorf("uaserialsSection(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUaserialsGenreSlug(t *testing.T) {
	cases := map[string]string{
		"drama":   "drama",
		"Драма":   "drama", // запасний варіант — назва, а не slug
		"sci-fi":  "fantastic",
		"romance": "melodrama",
		"mystery": "detective",
		// Власний slug сайту приходить як є.
		"melodrama": "melodrama",
		"detective": "detective",
		"fantastic": "fantastic",
	}
	for in, want := range cases {
		got, ok := uaserialsGenreSlug(in)
		if !ok || got != want {
			t.Errorf("uaserialsGenreSlug(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}

	for _, bad := range []string{"", "   ", "musical", "щось-невідоме"} {
		if got, ok := uaserialsGenreSlug(bad); ok {
			t.Errorf("uaserialsGenreSlug(%q) must fail, got %q", bad, got)
		}
	}
}

func TestUaserialsCardType(t *testing.T) {
	cases := []struct {
		section, href, label, want string
	}{
		{"", "https://uaserials.com/1-a.html", "1 сезон", "series"},
		{"", "https://uaserials.com/series/1-a.html", "", "series"},
		{"", "https://uaserials.com/1-a.html", "", "movie"},
		{"anime", "https://uaserials.com/1-a.html", "1 сезон", "anime"},
		{"cartoons", "https://uaserials.com/1-a.html", "1 сезон", "cartoon"},
		{"films", "https://uaserials.com/1-a.html", "", "movie"},
	}
	for _, c := range cases {
		if got := uaserialsCardType(c.section, c.href, c.label); got != c.want {
			t.Errorf("uaserialsCardType(%q,%q,%q) = %q, want %q",
				c.section, c.href, c.label, got, c.want)
		}
	}
}

// TestUaserialsKeyFromBundle — видобуток пароля з бандлу.
//
// Дві частини перевірки навмисні різні:
//  1. Необфусцований бандл (як його описували в ТЗ) — ключ дістається.
//  2. Реальний бандл uaserials.com станом на 2026-10-05 — обфускований,
//     регулярка не збігається. Це зафіксовано, щоб наступна людина не
//     «полагодила» тест, видаливши другий випадок: тоді ключ при
//     зміні бандла знову не підхопиться мовчки.
func TestUaserialsKeyCache(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	if p.keyValue() != uaserialsDefaultKey {
		t.Fatalf("initial key = %q", p.keyValue())
	}

	p.rememberKey("NEWKEY")
	if p.keyValue() != "NEWKEY" {
		t.Fatalf("key after remember = %q", p.keyValue())
	}
	// Порожній ключ не має затирати робочий: саме це станеться, якщо
	// бандл віддасть junk і ми вирішимо «оновити» кеш порожнечиною.
	p.rememberKey("")
	if p.keyValue() != "NEWKEY" {
		t.Fatalf("empty remember must be ignored, got %q", p.keyValue())
	}
}

func TestUaserialsKeyFromBundle(t *testing.T) {
	plain := `var dd="297796CCB81D255125";var x=1;`
	if got := uaserialsKeyFromBundle(plain); got != "297796CCB81D255125" {
		t.Errorf("plain bundle key = %q", got)
	}
	if got := uaserialsKeyFromBundle(`var dd='AAA111BBB222';`); got != "AAA111BBB222" {
		t.Errorf("single-quoted bundle key = %q", got)
	}

	// Реальний фрагмент обфускованого бандлу.
	obfuscated := `case'2':var dd=_0x4bfc33(0x13f)+_0x4bfc33(0x185)+'25';continue;`
	if got := uaserialsKeyFromBundle(obfuscated); got != "" {
		t.Fatalf("obfuscated bundle yields no key via regex, got %q", got)
	}
	if got := uaserialsKeyFromBundle(""); got != "" {
		t.Fatalf("empty bundle yields no key, got %q", got)
	}
}

// ---- контракт провайдера ----

func TestUaserialsProviderContract(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	if p.ID() != "uaserials" {
		t.Errorf("ID = %q", p.ID())
	}
	if p.Name() != "UA Serials" {
		t.Errorf("Name = %q", p.Name())
	}
	if p.BaseURL() != uaserialsDefaultBaseURL {
		t.Errorf("BaseURL = %q", p.BaseURL())
	}

	info := p.Describe()
	if info.ID != p.ID() || info.BaseURL != p.BaseURL() {
		t.Errorf("Describe does not mirror the provider: %+v", info)
	}
	if !info.ShowOnHome || !info.SearchEnabledDefault || info.HasFixedStreams {
		t.Errorf("Describe flags: %+v", info)
	}
	want := map[string]bool{"movie": true, "series": true, "cartoon": true, "anime": true}
	if len(info.ContentTypes) != len(want) {
		t.Fatalf("ContentTypes = %v", info.ContentTypes)
	}
	for _, ct := range info.ContentTypes {
		if !want[ct] {
			t.Errorf("unexpected content type %q", ct)
		}
	}

	// Кастомний домен має перекривати дефолтний.
	custom := NewUaserialsProviderWithConfig("https://mirror.example", nil)
	if custom.BaseURL() != "https://mirror.example" {
		t.Errorf("custom base url = %q", custom.BaseURL())
	}
	// Кеш ключа ініціалізується дефолтним.
	if custom.keyValue() != uaserialsDefaultKey {
		t.Errorf("default key not installed: %q", custom.keyValue())
	}
}

func TestUaserialsSearchEmptyQuery(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	items, err := p.Search(context.Background(), "   ")
	if err != nil {
		t.Fatalf("blank query must not fail: %v", err)
	}
	if items == nil {
		t.Fatal("blank query must return a non-nil empty slice (serialises as [])")
	}
	if len(items) != 0 {
		t.Fatalf("blank query returned %d items", len(items))
	}
}

func TestUaserialsGetByCategoryUnknownSlug(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	items, err := p.GetByCategory(context.Background(), "musical", "series", 1)
	if err == nil {
		t.Fatalf("unknown genre slug must produce an error, got %v", items)
	}
	if items != nil {
		t.Fatalf("failed category request must return nil items, got %v", items)
	}
}

// TestUaserialsGetPopularAndCategoryFailures — маршрути каталогу на
// недоступному upstream: помилка з назвою запиту, а не (nil, nil) і не
// порожній «успіх».
func TestUaserialsGetPopularAndCategoryFailures(t *testing.T) {
	client, err := NewTLSClient()
	if err != nil {
		t.Skipf("TLS client unavailable: %v", err)
	}
	p := NewUaserialsProvider(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := map[string]func() ([]domain.MediaItem, error){
		"popular/series": func() ([]domain.MediaItem, error) { return p.GetPopular(ctx, "series", 1) },
		"popular/all":    func() ([]domain.MediaItem, error) { return p.GetPopular(ctx, "", 2) },
		"popular/anime":  func() ([]domain.MediaItem, error) { return p.GetPopular(ctx, "anime", 0) },
		"category/drama": func() ([]domain.MediaItem, error) { return p.GetByCategory(ctx, "drama", "series", 1) },
		"category/empty": func() ([]domain.MediaItem, error) { return p.GetByCategory(ctx, "  ", "movie", 1) },
		"category/pages": func() ([]domain.MediaItem, error) { return p.GetByCategory(ctx, "romance", "series", 3) },
	}
	for name, call := range calls {
		items, err := call()
		if err == nil {
			t.Errorf("%s: cancelled upstream must fail, got %d items", name, len(items))
		}
		if items != nil {
			t.Errorf("%s: failed request must return nil items, got %v", name, items)
		}
	}
}

// TestUaserialsGetStreamsDeadUpstream — GetStreams на недоступній сторінці.
//
// Сеть тут не потрібна: контекст скасовано, тож запит не вийде.
// Перевіряємо саме контракт помилки: ніколи (nil, nil).
func TestUaserialsCatalogURL(t *testing.T) {
	const base = "https://uaserials.com"
	cases := []struct {
		section, genre string
		page           int
		want           string
	}{
		{"", "", 1, "https://uaserials.com/"},
		{"", "", 3, "https://uaserials.com/page/3/"},
		{"", "", 0, "https://uaserials.com/"}, // сторінка < 1 — це перша
		{"series", "", 1, "https://uaserials.com/series/"},
		{"series", "", 2, "https://uaserials.com/series/page/2/"},
		{"films", "drama", 1, "https://uaserials.com/films/drama/"},
		{"films", "drama", 5, "https://uaserials.com/films/drama/page/5/"},
		{"anime", "fantastic", -3, "https://uaserials.com/anime/fantastic/"},
	}
	for _, c := range cases {
		got := uaserialsCatalogURL(base, c.section, c.genre, c.page)
		if got != c.want {
			t.Errorf("uaserialsCatalogURL(%q,%q,%d) = %q, want %q",
				c.section, c.genre, c.page, got, c.want)
		}
	}
}

func TestUaserialsSearchPath(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	// Живий знімок: /search/%D0%94%D0%B6%D0%B5%D0%BA%D0%B8/
	if got, want := p.searchPath("Джеки"), "https://uaserials.com/search/%D0%94%D0%B6%D0%B5%D0%BA%D0%B8/"; got != want {
		t.Errorf("searchPath = %q, want %q", got, want)
	}
	// Пробіл має ставати %20, а не «+» (QueryEscape дав би «+»).
	if got := p.searchPath("a b"); strings.Contains(got, "+") {
		t.Errorf("searchPath must percent-encode spaces, got %q", got)
	}
}

func TestUaserialsAbsolutize(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	cases := map[string]string{
		"/posters/1.jpg":            "https://uaserials.com/posters/1.jpg",
		"posters/1.jpg":             "https://uaserials.com/posters/1.jpg",
		"https://cdn.example/1.jpg": "https://cdn.example/1.jpg",
		"//cdn.example/1.jpg":       "https://cdn.example/1.jpg",
		"":                          "",
		"   ":                       "",
	}
	for in, want := range cases {
		if got := p.absolutize(in); got != want {
			t.Errorf("absolutize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUaserialsIsTortugaPlayerURL(t *testing.T) {
	for _, in := range []string{"https://tortuga.tw/embed/98", "https://TORTUGA.tw/vod/1", "https://sub.tortuga.tw/x"} {
		if !isTortugaPlayerURL(in) {
			t.Errorf("%q must be recognised as a tortuga player", in)
		}
	}
	for _, in := range []string{"https://cdn.example/embed/1", "https://uaserials.com/x", ""} {
		if isTortugaPlayerURL(in) {
			t.Errorf("%q must not be treated as a tortuga player", in)
		}
	}
}

// TestUaserialsOfflineBranchesWithoutClient — шляхи, які не мають ходити
// в мережу, але мусять повертати помилку, а не панікуватись.
func TestUaserialsOfflineBranchesWithoutClient(t *testing.T) {
	p := &UaserialsProvider{baseURL: uaserialsDefaultBaseURL, key: uaserialsDefaultKey}
	ctx := context.Background()

	if got := p.refreshKeyFromBundle(ctx); got != "" {
		t.Errorf("refreshKeyFromBundle without a client must return \"\", got %q", got)
	}
	if _, _, err := p.resolvePlayerStreams(ctx, "https://tortuga.tw/embed/98", "https://uaserials.com/1-x.html", "Плеєр", 1, 1, ""); err == nil {
		t.Error("resolvePlayerStreams without a client must fail")
	}
	if _, err := p.fetchPlayerTree(ctx, "https://tortuga.tw/embed/98", ""); err == nil {
		t.Error("fetchPlayerTree without a client must fail")
	}
	if _, err := p.fetchPlayerTree(ctx, "https://cdn.example/embed/1", ""); err == nil {
		t.Error("fetchPlayerTree must refuse a non-tortuga host explicitly")
	}
}

// TestUaserialsParseCardsOnGarbage — розбір сміття не має панікувати
// і не має повертати nil (nil серіалізується як JSON null).
func TestUaserialsParseCardsOnGarbage(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	for _, html := range []string{
		"", "<html>", "<<<>>>", "<div class=\"short-item\"></div>",
		"<a class=\"uas-card\"></a>",
		`<a class="uas-card" href="https://uaserials.com/1-x.html"></a>`,
		"<div class=\"short-item\"><a class=\"short-img\" href=\"\"></a></div>",
	} {
		items := p.parseUaserialsCards(html, "")
		if items == nil {
			t.Errorf("parseUaserialsCards(%q) returned nil — it must stay non-nil", html)
		}
	}
	// Картка без посилання або без назви — не медіа.
	if got := p.parseUaserialsCards(`<a class="uas-card" href="https://uaserials.com/1-x.html"></a>`, ""); len(got) != 0 {
		t.Errorf("card without a title must be skipped, got %+v", got)
	}
}

func TestUaserialsPosterFromImage(t *testing.T) {
	p := NewUaserialsProviderWithConfig("", nil)
	doc := mustParseHTML(t, `<div>
		<img class="lazyload" data-src="/posters/9.jpg" src="/media/default.png">
		<img src="https://cdn.example/plain.jpg">
		<img src="/media/default.png">
	</div>`)

	imgs := doc.Find("img")
	if got := p.posterFromImage(imgs.First()); got != "https://uaserials.com/posters/9.jpg" {
		t.Errorf("data-src must win over the lazyload placeholder, got %q", got)
	}
	if got := p.posterFromImage(imgs.Eq(1)); got != "https://cdn.example/plain.jpg" {
		t.Errorf("src fallback = %q", got)
	}
	if got := p.posterFromImage(imgs.Eq(2)); got != "" {
		t.Errorf("placeholder-only image must yield no poster, got %q", got)
	}
	if got := p.posterFromImage(nil); got != "" {
		t.Errorf("nil selection = %q", got)
	}
}

func mustParseHTML(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return doc
}

func TestUaserialsGetStreamsDeadUpstream(t *testing.T) {
	client, err := NewTLSClient()
	if err != nil {
		t.Skipf("TLS client unavailable: %v", err)
	}
	p := NewUaserialsProvider(client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := p.GetStreams(ctx, "https://uaserials.com/422-velychne-stolittya-roksolana-2011s.html", 1, 1, "")
	if err == nil {
		t.Fatalf("cancelled upstream must fail, got %+v", resp)
	}
	if resp == nil {
		t.Fatal("GetStreams must return a response skeleton even on failure")
	}
	if resp.ProviderID != "uaserials" {
		t.Errorf("provider id in response = %q", resp.ProviderID)
	}
	if len(resp.Streams) != 0 {
		t.Errorf("failed resolve must expose no streams, got %d", len(resp.Streams))
	}
}
