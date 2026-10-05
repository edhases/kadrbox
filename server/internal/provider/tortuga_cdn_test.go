package provider

// Тести екстрактора CDN Tortuga.
//
// Головне, що тут захищається: алгоритм розшифровки file:. Він
// належить сторонньому плеєру, тож «покращити» його (змінити крок 7 на
// 8, викинути зсув 13, розшифрувати як UTF-8 замість байтів) можна
// непомітно — а на живому трафіку це означає порожній плейлист.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
)

// --- фікстури -----------------------------------------------------------

func tortugaFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return strings.TrimSpace(string(data))
}

// encryptTortuga — зворотна операція до decryptTortugaBlob. Живе у
// тесті, бо на сервері шифрувати ніколи не доводиться: воно потрібне
// лише для round-trip перевірок і для генерації фікстури
// testdata/tortuga_encoded_sample.txt.
func encryptTortuga(plain []byte, key byte) string {
	raw := make([]byte, 0, len(plain)+1)
	raw = append(raw, key)
	for i := 0; i < len(plain); i++ {
		raw = append(raw, plain[i]^byte((int(key)+i*7+13)%256))
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	// Плеєр не віддає padding — і наш розшифрувач не повинен його вимагати.
	return strings.TrimRight(enc, "=")
}

// --- регресія: round-trip ------------------------------------------------

// Найпростіша перевірка, що алгоритм узагалі самостійний: зашифрували
// відоме — розшифрували — порівняли з оригіналом.
func TestDecodeTortugaPlaylist_RoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		plain string
		key   byte
	}{
		{"ascii", `{"title":"Сезон 1"}`, 0x37},
		{"cyrillic", `{"title":"Серія 1","file":"{1+1}https://cdn/x.m3u8(subtitle:)"}`, 0x05},
		{"zero key", `{"title":"Сезон 2"}`, 0x00},
		{"max key", `{"title":"Сезон 3"}`, 0xFF},
		{"escapes", `{"file":"https://cdn/a\"b/index.m3u8"}`, 0x11},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := encryptTortuga([]byte(tc.plain), tc.key)

			blob, err := decryptTortugaBlob(enc)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if string(blob) != tc.plain {
				t.Fatalf("round trip mismatch:\n got %q\nwant %q", blob, tc.plain)
			}
		})
	}
}

// Мутаційний якір: будь-яка зміна констант 7/13 або формули XOR ламає
// round-trip. Перевірено вручну — див. звіт.

// --- Latin-1 vs UTF-8 ----------------------------------------------------

// Ключова різниця між «правильною» та «наївною» реалізацією.
//
// Алгоритм оперує БАЙТАМИ. Наш код повертає []byte, і перетворення
// []byte -> string у Go не змінює байти. Наївна реалізація натомість
// «перекодує» результат у рядок і припускає, що це текст — тоді
// кирилиця «Сезон» (6 байтів у UTF-8, 0xD0/0xA1/0xB7/0xBE/0xBE/0xBD)
// перетворюється на 5 рунів, і наступний XOR уже по рунах дає сміття.
//
// Цей тест має ПАДАТИ, якщо хтось змінить decryptTortugaBlob так, щоб
// він розшифровував «як текст» замість байтів. Щоб перевірка була
// не самопідтверджуюою, поруч лежить еталонна НАИВНА реалізація, і тест
// вимагає, щоб її результат відрізнявся від нашого.
func TestDecodeTortugaPlaylist_Latin1BytesNotUTF8(t *testing.T) {
	const plain = `{"title":"Сезон 1","file":"{1+1}https://cdn/x.m3u8"}`
	enc := encryptTortuga([]byte(plain), 0x37)

	blob, err := decryptTortugaBlob(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(blob) != plain {
		t.Fatalf("byte-wise decrypt must reproduce the original bytes, got %q", blob)
	}

	// Еталонна помилка: та сама XOR-формула, але результат зводять до
	// рядка через руни, тобто неявно перекодують Latin-1 у UTF-8.
	// На кирилиці це ОБОВ'ЯЗКОВО дає інший результат — саме це й
	// робить тест корисним: якщо хтось «поправить» decryptTortugaBlob
	// на руни, наш код перестане збігатися з еталоном.
	naive := naiveUTF8Decode(enc)
	if naive == plain {
		t.Fatalf("test is broken: the naive UTF-8 decoder reproduced the plaintext, " +
			"so this fixture no longer distinguishes byte-wise from rune-wise decryption")
	}
	t.Logf("byte-wise: %d bytes; naive UTF-8: %d bytes (diverges as expected)", len(plain), len(naive))
}

// naiveUTF8Decode — як «розшифровка у тексті» виглядає на практиці:
// та сама XOR-формула, але результат зводять до рядка і назад через
// руни, тобто неявно перетворюють Latin-1 на UTF-8.
func naiveUTF8Decode(encoded string) string {
	enc := strings.TrimRight(strings.TrimSpace(encoded), "=")
	switch len(enc) % 4 {
	case 2:
		enc += "=="
	case 3:
		enc += "="
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(raw) < 2 {
		return ""
	}
	key := int(raw[0])
	// Ось де криється помилка: out екранізується в руни і назад, тож
	// байти >0x7F стають многобайтними послідовностями UTF-8.
	runes := make([]rune, 0, len(raw)-1)
	for i := 1; i < len(raw); i++ {
		runes = append(runes, rune(raw[i])^rune((key+(i-1)*7+13)%256))
	}
	return string(runes)
}

// --- padding ------------------------------------------------------------

// Реальний рядок від плеєра НЕ має '=' у кінці. Розшифрувач мусить
// сам дописати padding, інакше StdEncoding повертає помилку.
func TestDecodeTortugaPlaylist_PaddingIsReadded(t *testing.T) {
	plain := `{"title":"Серія 42"}`

	raw := make([]byte, 0, len(plain)+1)
	raw = append(raw, 0x37)
	for i := 0; i < len(plain); i++ {
		raw = append(raw, plain[i]^byte((0x37+i*7+13)%256))
	}
	full := base64.StdEncoding.EncodeToString(raw)

	// 1) без padding (те, що реально приходить від CDN)
	if strings.HasSuffix(full, "=") {
		// short: залежить від довжини, але перевіримо всі три варіанти нижче
		t.Logf("payload happened to need padding")
	}

	trimmed := strings.TrimRight(full, "=")
	got, err := decryptTortugaBlob(trimmed)
	if err != nil {
		t.Fatalf("decrypt without padding: %v", err)
	}
	if string(got) != plain {
		t.Fatalf("trimmed payload: got %q want %q", got, plain)
	}

	// 2) з повним padding — має бути те саме
	if got, err := decryptTortugaBlob(full); err != nil {
		t.Fatalf("decrypt with padding: %v", err)
	} else if string(got) != plain {
		t.Fatalf("padded payload: got %q want %q", got, plain)
	}

	// 3) з надлишковими '=' — TrimRight зрізає ВСІ хвостові,
	//    тож надлишок не ламає розшифровку.
	if got, err := decryptTortugaBlob(full + "==="); err != nil {
		t.Fatalf("decrypt with over-padding: %v", err)
	} else if string(got) != plain {
		t.Fatalf("over-padded payload: got %q want %q", got, plain)
	}
}

// --- реальна фікстура ----------------------------------------------------

func TestDecodeTortugaPlaylist_RealFixture(t *testing.T) {
	encoded := tortugaFixture(t, "tortuga_encoded_sample.txt")
	plain := tortugaFixture(t, "tortuga_vod_plain.txt")

	items, err := DecodeTortugaPlaylist(encoded)
	if err != nil {
		t.Fatalf("DecodeTortugaPlaylist: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("decoded playlist must not be empty")
	}

	// Порівнюємо не байт у байт (JSON від руки не канонізований), а
	// семантично: розпарсений еталон.
	var want []playerJSPlaylistItem
	if err := json.Unmarshal([]byte(plain), &want); err != nil {
		t.Fatalf("plain fixture is not valid JSON: %v", err)
	}

	gotJSON, _ := json.Marshal(items)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("decoded playlist differs from fixture:\n got %s\nwant %s", gotJSON, wantJSON)
	}

	if desc := tortugaDescribe(items); !strings.Contains(desc, "episodes=") {
		t.Fatalf("describe must report episode count, got %q", desc)
	}
}

// --- сміття та помилки --------------------------------------------------

// Порожній і битий вхід мають давати ПОМІЛКУ, а не паніку і не
// «порожній плейлист, який непомітно зробить сторінку порожньою».
func TestDecodeTortugaPlaylist_RejectsGarbage(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"spaces", "   \t\n "},
		{"only padding", "===="},
		{"not base64", "!!!не base64!!!"},
		{"bad length", "AAAAA"}, // len%4 == 1 — у base64 неможливий
		{"single byte", "gA=="}, // один байт = тільки ключ, тіла немає
		{"valid b64 but not json", encryptTortuga([]byte("just some text"), 0x11)},
		{"json object not array", encryptTortuga([]byte(`{"title":"Сезон 1"}`), 0x11)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, err := DecodeTortugaPlaylist(tc.input)
			if err == nil {
				t.Fatalf("expected an error, got %d items", len(items))
			}
			if !errors.Is(err, ErrTortugaUndecodable) {
				t.Fatalf("error must wrap ErrTortugaUndecodable, got %v", err)
			}
			if items != nil {
				t.Fatalf("on error must return nil items, got %d", len(items))
			}
		})
	}
}

// Стеля на розмір: зловмисна сторінка не має змусити нас виділити
// гігабайт під розшифровку.
//
// Важлива деталь: payload має бути ВАЛІДНИМ плейлистом, просто
// гігантським. Якщо підсунути безглуздий потік байтів, тест зелений і
// з вимкненою стелею — він не перевіряє саме стелю, а те, що сміття
// не парситься.
func TestDecodeTortugaPlaylist_RejectsHugePayload(t *testing.T) {
	var b strings.Builder
	b.WriteString(`[`)
	for i := 0; b.Len() < tortugaMaxDecodedBytes+1024; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"title":"Серія `)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`","file":"https://calypso.tortuga.tw/hls/x/hls/index.m3u8"}`)
	}
	b.WriteString(`]`)

	huge := encryptTortuga([]byte(b.String()), 0x2B)
	if _, err := DecodeTortugaPlaylist(huge); err == nil {
		t.Fatalf("expected the %d byte limit to reject an oversized but valid playlist", tortugaMaxDecodedBytes)
	}

	// Еталон: такий самий плейлист, але в межах стелі — проходить.
	small := `[{"title":"Серія 1","file":"https://calypso.tortuga.tw/hls/x/hls/index.m3u8"}]`
	if _, err := DecodeTortugaPlaylist(encryptTortuga([]byte(small), 0x2B)); err != nil {
		t.Fatalf("a small valid playlist must decode: %v", err)
	}
}

// Стеля на глибину вкладеності.
//
// Payload навмисно ВАЛІДНИЙ для json.Unmarshal у playerJSPlaylistItem
// (вкладені folder), а не «[[[[…»:Go-парсер прийняв би такий без
// жодного ліміту з нашого боку, і тест був би зелений із вимкненою
// стелею.
func TestDecodeTortugaPlaylist_RejectsDeepNesting(t *testing.T) {
	const depth = 200

	var b strings.Builder
	b.WriteString(`[`)
	for i := 0; i < depth; i++ {
		b.WriteString(`{"title":"Сезон `)
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(`","folder":[`)
	}
	b.WriteString(`{"title":"Серія 1","file":"https://cdn/x.m3u8"}`)
	for i := 0; i < depth; i++ {
		b.WriteString(`]}`)
	}
	b.WriteString(`]`)

	if _, err := DecodeTortugaPlaylist(encryptTortuga([]byte(b.String()), 0x22)); err == nil {
		t.Fatalf("expected the nesting depth limit to reject a %d-level payload", depth)
	}

	// Еталон: реалістична глибина 2 — проходить.
	shallow := `[{"title":"Сезон 1","folder":[{"title":"Серія 1","file":"https://cdn/x.m3u8"}]}]`
	if _, err := DecodeTortugaPlaylist(encryptTortuga([]byte(shallow), 0x22)); err != nil {
		t.Fatalf("a realistic two-level playlist must decode: %v", err)
	}
}

// --- санітизація URL ----------------------------------------------------

// geoblock=1 робить відповідь 403, season/episode зрізають дерево до
// однієї серії. Ми знімаємо їх, а все інше (?id=) лишаємо.
func TestSanitizeTortugaURL_DropsDestructiveParams(t *testing.T) {
	raw := "https://calypso.tortuga.tw/hls/x/hls/index.m3u8?id=66961&geoblock=1&season=2&episode=3&quality=720&lang=uk&sub=on#frag"

	got, err := sanitizeTortugaURL(raw)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	for _, bad := range []string{"geoblock", "season=", "episode=", "quality=", "lang=", "sub=", "#frag"} {
		if strings.Contains(got, bad) {
			t.Fatalf("query param %q must be stripped, got %s", bad, got)
		}
	}
	if !strings.Contains(got, "id=66961") {
		t.Fatalf("harmless params must survive, got %s", got)
	}
}

func TestSanitizeTortugaURL_RejectsNonHTTP(t *testing.T) {
	for _, raw := range []string{"", "   ", "javascript:alert(1)", "file:///etc/passwd", "/relative/path.m3u8", "https://"} {
		if got, err := sanitizeTortugaURL(raw); err == nil {
			t.Fatalf("%q must be rejected, got %q", raw, got)
		}
	}
}

// --- дерево → потоки ----------------------------------------------------

// tortugaTree — сире розшифроване дерево фікстури, з трейлерами
// усередині сезону (як на живому трафіку).
func tortugaTree(t *testing.T) []playerJSPlaylistItem {
	t.Helper()
	items, err := DecodeTortugaPlaylist(tortugaFixture(t, "tortuga_encoded_sample.txt"))
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return items
}

// tortugaTreeClean — те саме, але без трейлерів. Саме таке дерево
// отримують FetchTortugaEmbed і FetchTortugaEpisode.
func tortugaTreeClean(t *testing.T) []playerJSPlaylistItem {
	t.Helper()
	return PruneTortugaTrailers(tortugaTree(t))
}

// Трейлери приходять тим самим розшифрованим деревом. Без фільтра
// користувач отримує «Серія 7» у вигляді рекламного ролика.
func TestTortugaStreamsFromPlaylist_FiltersTrailers(t *testing.T) {
	// Перевірка на СИРОМУ дереві, без PruneTortugaTrailers: фільтр має
	// бути в самому TortugaStreamsFromPlaylist, а не лише в кроці
	// попереднього вирізання. Інакше викликач, який передасть дерево
	// напряму, отримає трейлер у списку стримів.
	streams, _ := TortugaStreamsFromPlaylist(tortugaTree(t), "https://tortuga.tw/embed/66961", "Tortuga", 0, 0, "")
	if len(streams) == 0 {
		t.Fatal("expected streams from a real tree")
	}
	for _, s := range streams {
		if IsTrailerURL(s.URL) {
			t.Fatalf("trailer leaked into streams: %s", s.URL)
		}
		if strings.Contains(s.URL, "/trailers/") {
			t.Fatalf("trailer path leaked into streams: %s", s.URL)
		}
	}
	// Еталон: у фікстурі 7 листків, один з них — трейлер.
	if len(streams) != 6 {
		t.Fatalf("want 6 episode streams (7 leaves - 1 trailer), got %d", len(streams))
	}

	// І на вирізаному дереві результат має бути ідентичний.
	clean, _ := TortugaStreamsFromPlaylist(tortugaTreeClean(t), "https://tortuga.tw/embed/66961", "Tortuga", 0, 0, "")
	if len(clean) != len(streams) {
		t.Fatalf("pruned tree gave %d streams, unpruned gave %d — pruning must not lose episodes", len(clean), len(streams))
	}
}

// Навіщо окремий крок PruneTortugaTrailers: трейлер у нашій фікстурі
// стоїть на позиції «Серія 3» усередині сезону 1. Без вирізання
// загальний обхід (playlist_tree.go) дає йому номер 3, а справжню
// «Серію 3», що йде за ним, відкидає як дублікат (dub, season,
// episode) — season 1 має 3 епізоди замість 4, і клієнт не може
// вибрати третю серію взагалі.
func TestPruneTortugaTrailers_TrailerDoesNotShadowEpisode(t *testing.T) {
	// Еталон проблеми: без вирізання саме ТРЕЙЛЕР займає номер 3 у
	// сезоні 1, а справжня «Серія 3» відкидається як дублікат.
	// ref у обох випадках однаковий (v:1:3:1+1), тож різницю видно
	// лише за URL листка. Якщо фікстура перестане це відтворювати,
	// тест має впасти, а не мовчки пройти — інакше він перестане
	// щось захищати.
	if s01e03 := findLeafURL(tortugaTree(t), 1, 3, "1+1"); !IsTrailerURL(s01e03) {
		t.Fatalf("fixture no longer exercises the shadowing case: unpruned S01E03 = %s", s01e03)
	}

	clean := tortugaTreeClean(t)

	seasons := BuildSeasonsFromPlaylist(clean)
	if len(seasons) != 3 {
		t.Fatalf("want 3 seasons after pruning, got %d", len(seasons))
	}
	if seasons[0].Number != 1 || len(seasons[0].Episodes) != 3 {
		t.Fatalf("season 1 after pruning = %d episodes, want 3", len(seasons[0].Episodes))
	}
	// Номери серій лишаються «зміщеними» — вирізання трейлера не
	// перенумеровує решту.
	for i, ep := range seasons[0].Episodes {
		if ep.Number != i+1 {
			t.Fatalf("season 1 episode[%d].Number = %d, want %d", i, ep.Number, i+1)
		}
	}

	// А тепер S01E03 має вести на справжню серію, а не на трейлер.
	// Кількість епізодів в обох випадках однакова (3) — різницю видно
	// лише за тим, на який URL вказує номер 3.
	got := findLeafURL(clean, 1, 3, "1+1")
	if IsTrailerURL(got) || !strings.Contains(got, "s01e03") {
		t.Fatalf("S01E03 after pruning = %s, want the real episode", got)
	}
}

// findLeafURL повертає URL листка за (сезон, серія, озвучка).
func findLeafURL(items []playerJSPlaylistItem, season, episode int, dub string) string {
	var out string
	ForEachPlaylistLeaf(items, func(l PlaylistLeaf) {
		if out != "" || l.Ctx.Season != season || l.Ctx.Episode != episode || l.Ctx.Dub != dub {
			return
		}
		_, mediaURL, _ := ParseTortugaFileField(l.File)
		out = mediaURL
	})
	return out
}

// Обрізання трейлера не ламає озвучки: дерево не стає порожнім.
func TestPruneTortugaTrailers_KeepsDubs(t *testing.T) {
	dubs := BuildVoiceoversFromPlaylist(tortugaTreeClean(t))
	if len(dubs) != 2 {
		t.Fatalf("want 2 dubs after pruning, got %d: %+v", len(dubs), dubs)
	}
	for _, d := range dubs {
		if len(d.Seasons) == 0 {
			t.Fatalf("dub %q lost all seasons", d.Name)
		}
	}
}

// Медіатрафік НЕ йде через наш сервер: прямий CDN, усі три заголовки
// заповнені, URL == DirectURL.
func TestTortugaStreamsFromPlaylist_DirectCDNNoProxy(t *testing.T) {
	streams, _ := TortugaStreamsFromPlaylist(tortugaTree(t), "https://tortuga.tw/embed/66961", "Tortuga", 0, 0, "")
	if len(streams) == 0 {
		t.Fatal("expected streams")
	}

	for _, s := range streams {
		if s.RequiresProxy {
			t.Fatalf("RequiresProxy must be false: %+v", s)
		}
		if s.URL != s.DirectURL {
			t.Fatalf("URL must equal DirectURL, got %q vs %q", s.URL, s.DirectURL)
		}
		if !strings.Contains(s.DirectURL, "calypso.tortuga.tw") {
			t.Fatalf("stream must point at the CDN, got %s", s.DirectURL)
		}
		if !strings.HasSuffix(s.DirectURL, "/hls/index.m3u8") {
			t.Fatalf("must hand over the master playlist untouched, got %s", s.DirectURL)
		}
		if s.Headers["User-Agent"] == "" || s.Headers["Referer"] == "" || s.Headers["Origin"] == "" {
			t.Fatalf("UA/Referer/Origin must be set, got %+v", s.Headers)
		}
		if s.Headers["Referer"] != "https://calypso.tortuga.tw/" {
			t.Fatalf("Referer must be the CDN origin, got %q", s.Headers["Referer"])
		}
	}
}

// Сезон/серія в URL не додаємо: Tortuga віддасть одну серію замість
// дерева.
func TestTortugaStreamsFromPlaylist_NoSeasonEpisodeInURL(t *testing.T) {
	streams, _ := TortugaStreamsFromPlaylist(tortugaTree(t), "https://tortuga.tw/embed/66961", "Tortuga", 2, 1, "2+2")
	if len(streams) == 0 {
		t.Fatal("expected a stream for S02E01")
	}
	for _, s := range streams {
		lower := strings.ToLower(s.URL)
		for _, bad := range []string{"season=", "episode=", "s02e01.m3u8&", "&s="} {
			if strings.Contains(lower, bad) {
				t.Fatalf("url must not carry selection params (%s): %s", bad, s.URL)
			}
		}
		if !strings.Contains(s.URL, "s02e01") {
			t.Fatalf("wrong episode selected: %s", s.URL)
		}
	}
}

// Озвучка живе у Voiceover, якість ніколи в Quality.
//
// Раніше Quality містив «1+1 (Tortuga)», тобто клієнт показував
// назву студії в колонці «Якість». Це плутало дві різні речі:
// озвучка — це Voiceover, якість — це окреме поле.
//
// Контракт піниться тут, бо саме на ньому тримається UI: сортування
// за DubWeight дивиться на Voiceover, а список якостей — на Quality.
func TestTortugaStreamsFromPlaylist_DubStaysOutOfQuality(t *testing.T) {
	streams, _ := TortugaStreamsFromPlaylist(tortugaTreeClean(t), "https://tortuga.tw/embed/66961", "Tortuga", 0, 0, "")

	dubs := map[string]bool{}
	for _, s := range streams {
		dubs[s.Voiceover] = true

		if s.Voiceover != "" && strings.Contains(s.Quality, s.Voiceover) {
			t.Fatalf("Quality %q must not carry the dub %q — that is Voiceover's job", s.Quality, s.Voiceover)
		}
		if s.Quality == "" {
			t.Fatalf("Quality must never be empty; client renders it directly")
		}
		if s.Player != "Tortuga" {
			t.Fatalf("player label = %q, want Tortuga", s.Player)
		}
	}
	if !dubs["1+1"] || !dubs["2+2"] {
		t.Fatalf("both dubs must be present, got %v", dubs)
	}
}

// Фільтр за озвучкою: 2+2 не повинна тягнути за собою 1+1.
func TestTortugaStreamsFromPlaylist_FiltersByDub(t *testing.T) {
	streams, _ := TortugaStreamsFromPlaylist(tortugaTree(t), "https://tortuga.tw/embed/66961", "Tortuga", 0, 0, "2+2")
	if len(streams) != 3 {
		t.Fatalf("want 3 streams for dub 2+2, got %d", len(streams))
	}
	for _, s := range streams {
		if s.Voiceover != "2+2" {
			t.Fatalf("foreign dub in result: %+v", s)
		}
	}
}

// Субтитри з хвоста file: і з окремого поля вузла.
func TestTortugaStreamsFromPlaylist_Subtitles(t *testing.T) {
	_, subs := TortugaStreamsFromPlaylist(tortugaTreeClean(t), "https://tortuga.tw/embed/66961", "Tortuga", 1, 3, "")
	if len(subs) == 0 {
		t.Fatal("expected subtitles for S01E03")
	}
	var found bool
	for _, s := range subs {
		if strings.Contains(s.URL, "s01e03.uk.vtt") {
			found = true
			if s.Label == "" || s.Language == "" {
				t.Fatalf("subtitle must carry label and language: %+v", s)
			}
		}
	}
	if !found {
		t.Fatalf("subtitle url missing, got %+v", subs)
	}
}

// geoblock=1 у фікстурі — реальна поведінка: Tortuga починає
// віддавати 403 «регіон заблоковано». Потік клієнту має йти без нього.
func TestTortugaStreamsFromPlaylist_StripsGeoblock(t *testing.T) {
	streams, _ := TortugaStreamsFromPlaylist(tortugaTreeClean(t), "https://tortuga.tw/embed/66961", "Tortuga", 1, 3, "")
	if len(streams) != 1 {
		t.Fatalf("want 1 stream for S01E03, got %d", len(streams))
	}
	if strings.Contains(strings.ToLower(streams[0].URL), "geoblock") {
		t.Fatalf("geoblock must be stripped before the client sees the url: %s", streams[0].URL)
	}
	if streams[0].DirectURL != streams[0].URL {
		t.Fatalf("sanitised url must be used for both URL and DirectURL: %q vs %q", streams[0].URL, streams[0].DirectURL)
	}
}

// --- сезони та озвучки ---------------------------------------------------

func TestTortugaResult_SeasonsAndVoiceovers(t *testing.T) {
	tree := tortugaTreeClean(t)

	seasons := BuildSeasonsFromPlaylist(tree)
	if len(seasons) != 3 {
		t.Fatalf("want 3 seasons, got %d", len(seasons))
	}
	// 6 серій після вирізання трейлера: 3 + 2 + 1.
	if desc := tortugaDescribe(tree); desc != "seasons=3 [S1(3) S2(2) S3(1)] episodes=6" {
		t.Fatalf("describe = %q", desc)
	}

	dubs := BuildVoiceoversFromPlaylist(tree)
	if len(dubs) != 2 {
		t.Fatalf("want 2 dubs, got %d: %+v", len(dubs), dubs)
	}
	for _, d := range dubs {
		if len(d.Seasons) == 0 {
			t.Fatalf("dub %q has no seasons — the client builds its selector from this", d.Name)
		}
		ep := d.Seasons[0].Episodes[0]
		dubName, season, episode, ok := DecodeVoiceEpisodeRef(ep.URL)
		if !ok {
			t.Fatalf("episode URL must be a decodable ref, got %q", ep.URL)
		}
		if dubName != d.Name || season != d.Seasons[0].Number || episode != ep.Number {
			t.Fatalf("ref round trip broken: %q/%d/%d vs dub %q season %d ep %d",
				dubName, season, episode, d.Name, d.Seasons[0].Number, ep.Number)
		}
	}
}

// --- embed-сторінка -----------------------------------------------------

func tortugaEmbedServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Енд-ту-енд: embed-сторінка з зашифрованим file: → готові потоки.
func TestFetchTortugaEmbed_EndToEnd(t *testing.T) {
	encoded := tortugaFixture(t, "tortuga_encoded_sample.txt")
	page := `<html><body><script>var p = new Playerjs({file: "` + encoded + `"});</script></body></html>`
	srv := tortugaEmbedServer(t, page)

	client, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	res, err := NewTortugaExtractor(client).FetchTortugaEmbed(context.Background(), srv.URL+"/embed/66961?geoblock=1", "")
	if err != nil {
		t.Fatalf("FetchTortugaEmbed: %v", err)
	}

	if len(res.Seasons) != 3 {
		t.Fatalf("want 3 seasons, got %d", len(res.Seasons))
	}
	if len(res.Voiceovers) != 2 {
		t.Fatalf("want 2 voiceovers, got %d", len(res.Voiceovers))
	}
	if len(res.Streams) != 6 {
		t.Fatalf("want 6 streams (trailer filtered), got %d", len(res.Streams))
	}
	for _, s := range res.Streams {
		if s.RequiresProxy || s.URL != s.DirectURL {
			t.Fatalf("media must not be proxied: %+v", s)
		}
	}
}

// /vod/{id} віддає той самий плеєр — окремий обробник не потрібен,
// але шлях має працювати.
func TestFetchTortugaEmbed_VodPath(t *testing.T) {
	encoded := tortugaFixture(t, "tortuga_encoded_sample.txt")
	srv := tortugaEmbedServer(t, `{"file":"`+encoded+`"}`)

	client, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}
	res, err := NewTortugaExtractor(client).FetchTortugaEmbed(context.Background(), srv.URL+"/vod/66961", "")
	if err != nil {
		t.Fatalf("FetchTortugaEmbed: %v", err)
	}
	if len(res.Streams) == 0 {
		t.Fatal("expected streams from the vod page")
	}
}

// Точечний запит — те, що потрібно GetStreams.
func TestFetchTortugaEpisode_SelectsOneEpisode(t *testing.T) {
	encoded := tortugaFixture(t, "tortuga_encoded_sample.txt")
	srv := tortugaEmbedServer(t, `<script>new Playerjs({file:"`+encoded+`"})</script>`)

	client, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}
	streams, _, err := NewTortugaExtractor(client).FetchTortugaEpisode(context.Background(), srv.URL+"/embed/66961", 1, 1, "1+1")
	if err != nil {
		t.Fatalf("FetchTortugaEpisode: %v", err)
	}

	// Фікстура віддає master з трьома варіантами, тож після
	// розкриття має бути три джерела — по одному на якість.
	// Раніше їх було одне з Quality=Auto, і клієнт показував «Auto».
	if len(streams) != 3 {
		t.Fatalf("want 3 streams after master expansion, got %d: %+v", len(streams), streams)
	}

	wantQuality := []string{"1080p", "720p", "480p"}
	for i, want := range wantQuality {
		if streams[i].Quality != want {
			t.Errorf("stream[%d].Quality = %q, want %q", i, streams[i].Quality, want)
		}
		if !strings.Contains(streams[i].URL, "s01e01") {
			t.Errorf("stream[%d] points at the wrong episode: %s", i, streams[i].URL)
		}
		if streams[i].Voiceover != "1+1" {
			t.Errorf("stream[%d].Voiceover = %q, want 1+1", i, streams[i].Voiceover)
		}
		if streams[i].URL == streams[i].DirectURL && !strings.Contains(streams[i].URL, "m3u8") {
			t.Errorf("stream[%d] lost its manifest URL: %s", i, streams[i].URL)
		}
		if streams[i].RequiresProxy {
			t.Errorf("stream[%d] must not require a proxy", i)
		}
	}
}

// Фолбек: невірно розпізнаний номер не робить відповідь порожньою.
func TestFetchTortugaEpisode_FallsBackOnMiss(t *testing.T) {
	encoded := tortugaFixture(t, "tortuga_encoded_sample.txt")
	srv := tortugaEmbedServer(t, `<script>new Playerjs({file:"`+encoded+`"})</script>`)

	client, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}
	streams, _, err := NewTortugaExtractor(client).FetchTortugaEpisode(context.Background(), srv.URL+"/embed/66961", 99, 99, "")
	if err != nil {
		t.Fatalf("FetchTortugaEpisode: %v", err)
	}
	if len(streams) == 0 {
		t.Fatal("a miss must fall back to the first available episode, not return nothing")
	}
}

// Сторінка без file: — помилка, а не порожня відповідь.
func TestFetchTortugaEmbed_ReportsMissingPlaylist(t *testing.T) {
	srv := tortugaEmbedServer(t, `<html><body>nothing here</body></html>`)
	client, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}

	_, err = NewTortugaExtractor(client).FetchTortugaEmbed(context.Background(), srv.URL+"/embed/1", "")
	if !errors.Is(err, ErrTortugaUndecodable) {
		t.Fatalf("want ErrTortugaUndecodable, got %v", err)
	}
}

// Фолбек на відкритий JSON: частина сторінок Tortua не шифрує file:.
func TestFetchTortugaEmbed_FallsBackToPlainJSON(t *testing.T) {
	plain := `[{"title":"Сезон 1","folder":[{"title":"Серія 1","file":"https://calypso.tortuga.tw/hls/x/hls/index.m3u8"}]}]`
	srv := tortugaEmbedServer(t, `<script>new Playerjs({file: `+plain+`})</script>`)

	client, err := NewTLSClient()
	if err != nil {
		t.Fatalf("NewTLSClient: %v", err)
	}
	res, err := NewTortugaExtractor(client).FetchTortugaEmbed(context.Background(), srv.URL+"/embed/1", "")
	if err != nil {
		t.Fatalf("plain JSON must be accepted: %v", err)
	}
	if len(res.Streams) != 1 {
		t.Fatalf("want 1 stream, got %d", len(res.Streams))
	}
}

// Екстрактор без клієнта не повинен паникувати.
func TestFetchTortugaEmbed_RequiresClient(t *testing.T) {
	var e *TortugaExtractor
	if _, err := e.FetchTortugaEmbed(context.Background(), "https://tortuga.tw/embed/1", ""); err == nil {
		t.Fatal("expected an error from a nil extractor")
	}
	if _, err := NewTortugaExtractor(nil).FetchTortugaEmbed(context.Background(), "https://tortuga.tw/embed/1", ""); err == nil {
		t.Fatal("expected an error from an extractor without a client")
	}
	if _, err := NewTortugaExtractor(&TLSClient{}).FetchTortugaEmbed(context.Background(), "javascript:alert(1)", ""); err == nil {
		t.Fatal("expected an error for a non-http player url")
	}
}

// Контракт: жоден потік не йде через проксі, навіть коли дерево
// містить підозрілі URL.
func TestTortugaStreams_neverProxies(t *testing.T) {
	tree := []playerJSPlaylistItem{{
		Title: "Серія 1",
		File:  json.RawMessage(`"https://calypso.tortuga.tw/hls/a/hls/index.m3u8"`),
	}, {
		Title: "Серія 2",
		File:  json.RawMessage(`"https://calypso.tortuga.tw/hls/trailers/b/hls/index.m3u8"`),
	}, {
		Title: "Серія 3",
		File:  json.RawMessage(`"javascript:alert(1)"`),
	}}

	streams, _ := TortugaStreamsFromPlaylist(tree, "https://tortuga.tw/embed/1", "Tortuga", 0, 0, "")
	if len(streams) != 1 {
		t.Fatalf("only the playable non-trailer episode may pass, got %d", len(streams))
	}
	if streams[0].RequiresProxy {
		t.Fatal("must not be proxied")
	}
	if streams[0].Headers["Origin"] != "https://calypso.tortuga.tw" {
		t.Fatalf("origin = %q", streams[0].Headers["Origin"])
	}
}

var _ = domain.StreamSource{}
