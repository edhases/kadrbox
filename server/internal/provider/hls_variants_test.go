package provider_test

import (
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
)

// Фістури нижче — справжні відповіді CDN, зняті 2026-10-05.

// tortugaMaster — master «Величне століття. Роксолана» S01E01.
// ВАРІАНТИ РЕАЛЬНО РОЗБИТІ: сегмент 1080 = 567196 байт,
// 720 = 325616, 480 = 251732.
const tortugaMaster = `#EXTM3U
#EXT-X-STREAM-INF:RESOLUTION=1920x1080,BANDWIDTH=2128000
https://calypso.tortuga.tw/content/stream/serials/muhtesem.yuzyil/s01/x.mvo_66961/hls/1080/index.m3u8
#EXT-X-STREAM-INF:RESOLUTION=1280x720,BANDWIDTH=1096000
https://calypso.tortuga.tw/content/stream/serials/muhtesem.yuzyil/s01/x.mvo_66961/hls/720/index.m3u8
#EXT-X-STREAM-INF:RESOLUTION=854x480,BANDWIDTH=714000
https://calypso.tortuga.tw/content/stream/serials/muhtesem.yuzyil/s01/x.mvo_66961/hls/480/index.m3u8
`

// ashdiMaster — master того самого тайтлу на Ashdi. Оголошує
// ЛИШЕ 480p. Шлях /hls/1080/ віддає 200 на плейлисті, але
// сегменти за ним 404 — саме тому якість не можна вигадувати.
const ashdiMaster = `#EXTM3U
#EXT-X-STREAM-INF:RESOLUTION=854x480,BANDWIDTH=714000
https://ashdi.vip/video5/3/serials/x/hls/480/DK6XiHWKjuZahA39AQ==/index.m3u8
`

// mediaPlaylist — не master, а перелік сегментів.
const mediaPlaylist = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:10.0,
segment1.ts
#EXTINF:10.0,
segment2.ts
#EXT-X-ENDLIST
`

func TestParseMasterPlaylist_TortugaThreeRealVariants(t *testing.T) {
	base := "https://calypso.tortuga.tw/hls/serials/muhtesem.yuzyil/s01/x.mvo_66961/hls/index.m3u8"

	variants, ok := provider.ParseMasterPlaylist(tortugaMaster, base)
	if !ok {
		t.Fatal("tortuga master must parse")
	}
	if len(variants) != 3 {
		t.Fatalf("want 3 variants, got %d", len(variants))
	}

	// Найкраща якість першою.
	want := []struct {
		quality string
		height  int
	}{
		{"1080p", 1080},
		{"720p", 720},
		{"480p", 480},
	}
	for i, w := range want {
		if variants[i].Quality() != w.quality {
			t.Errorf("variant[%d].Quality = %q, want %q", i, variants[i].Quality(), w.quality)
		}
		if variants[i].Height != w.height {
			t.Errorf("variant[%d].Height = %d, want %d", i, variants[i].Height, w.height)
		}
		if variants[i].Bandwidth == 0 {
			t.Errorf("variant[%d] must carry BANDWIDTH", i)
		}
	}
}

// Ashdi оголошує один варіант. Саме це і є причиною не вигадувати
// якість: якби ми підставили /hls/1080/, користувач побачив би
// фантом, за яким нічого не відтворюється.
func TestParseMasterPlaylist_AshdiDeclaresOnly480(t *testing.T) {
	base := "https://ashdi.vip/video5/3/serials/x/hls/index.m3u8"

	variants, ok := provider.ParseMasterPlaylist(ashdiMaster, base)
	if !ok {
		t.Fatal("ashdi master must parse")
	}
	if len(variants) != 1 {
		t.Fatalf("ashdi declares 1 variant, got %d", len(variants))
	}
	if variants[0].Quality() != "480p" {
		t.Fatalf("quality = %q, want 480p", variants[0].Quality())
	}
	if strings.Contains(variants[0].URL, "/1080/") {
		t.Fatalf("must never invent a 1080p path: %s", variants[0].URL)
	}
}

// Медіа-плейлист — це не master. Розкриття має повернути false,
// щоб джерело лишилося одним з Auto, а не розпалося на порожні.
func TestParseMasterPlaylist_MediaPlaylistIsNotMaster(t *testing.T) {
	variants, ok := provider.ParseMasterPlaylist(mediaPlaylist, "https://calypso.tortuga.tw/hls/x/index.m3u8")
	if ok {
		t.Fatalf("media playlist must not be treated as master, got %+v", variants)
	}
	if variants != nil {
		t.Fatalf("variants must be nil, got %+v", variants)
	}
}

// Варіант може бути відносним. На реальних CDN трапляється, і без
// резолву ми віддали б плеєру URL, який він не розуміє.
//
// Відносний URI резолвиться щодо КАТАЛОГУ базового URL, а не щодо
// самого файлу — це правило RFC 3986, і саме так працюють CDN:
// база .../hls/index.m3u8, варіант 1080/index.m3u8 дає .../hls/1080/.
func TestParseMasterPlaylist_ResolvesRelativeVariant(t *testing.T) {
	master := "#EXTM3U\n#EXT-X-STREAM-INF:RESOLUTION=1920x1080\n1080/index.m3u8\n"

	variants, ok := provider.ParseMasterPlaylist(master, "https://cdn.example/media/hls/index.m3u8")
	if !ok || len(variants) != 1 {
		t.Fatalf("want 1 variant, ok=%v got=%d", ok, len(variants))
	}
	if variants[0].URL != "https://cdn.example/media/hls/1080/index.m3u8" {
		t.Fatalf("relative variant not resolved: %s", variants[0].URL)
	}
}

// Порядок у master не гарантований. Клієнт показує список у
// порядку, тож сортування має бути наше, а не випадкове.
func TestParseMasterPlaylist_SortsBestFirst(t *testing.T) {
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:RESOLUTION=854x480\nc/480.m3u8\n" +
		"#EXT-X-STREAM-INF:RESOLUTION=1920x1080\nc/1080.m3u8\n" +
		"#EXT-X-STREAM-INF:RESOLUTION=1280x720\nc/720.m3u8\n"

	variants, ok := provider.ParseMasterPlaylist(master, "https://cdn.example/m.m3u8")
	if !ok || len(variants) != 3 {
		t.Fatalf("want 3 variants, got %d (ok=%v)", len(variants), ok)
	}
	if variants[0].Quality() != "1080p" {
		t.Fatalf("best quality must be first, got %q", variants[0].Quality())
	}
}

// --- Розкриття в джерела --------------------------------------------

func masterSource() domain.StreamSource {
	return domain.StreamSource{
		URL:           "https://cdn.example/hls/index.m3u8",
		DirectURL:     "https://cdn.example/hls/index.m3u8",
		RequiresProxy: false,
		Headers:       map[string]string{"Referer": "https://site/", "User-Agent": "UA"},
		Voiceover:     "1+1",
		Player:        "CDN",
		Quality:       "Auto",
	}
}

func TestExpandMasterStream_OneSourcePerQuality(t *testing.T) {
	variants, ok := provider.ParseMasterPlaylist(tortugaMaster, "https://calypso.tortuga.tw/hls/index.m3u8")
	if !ok {
		t.Fatal("fixture must parse")
	}

	out := provider.ExpandMasterStream(masterSource(), variants)
	if len(out) != 3 {
		t.Fatalf("want 3 sources, got %d", len(out))
	}

	for i, want := range []string{"1080p", "720p", "480p"} {
		if out[i].Quality != want {
			t.Errorf("source[%d].Quality = %q, want %q", i, out[i].Quality, want)
		}
		if out[i].Voiceover != "1+1" {
			t.Errorf("source[%d] must keep the dub, got %q", i, out[i].Voiceover)
		}
		if out[i].Player != "CDN" {
			t.Errorf("source[%d] must keep the player, got %q", i, out[i].Player)
		}
		if out[i].URL == "https://cdn.example/hls/index.m3u8" {
			t.Errorf("source[%d] still points at the master", i)
		}
		if out[i].URL != out[i].DirectURL {
			t.Errorf("source[%d]: URL must equal DirectURL — media must not go through our server", i)
		}
		if out[i].RequiresProxy {
			t.Errorf("source[%d] must not require a proxy", i)
		}
		if out[i].Headers["Referer"] == "" {
			t.Errorf("source[%d] must keep headers: CDNs validate Referer on segments too", i)
		}
	}
}

// Без варіантів не розкриваємо нічого: краще один master з Auto,
// ніж вигадані джерела.
func TestExpandMasterStream_EmptyVariantsKeepsMaster(t *testing.T) {
	if out := provider.ExpandMasterStream(masterSource(), nil); out != nil {
		t.Fatalf("must not expand without variants, got %+v", out)
	}
}

// Два однакові варіанти — один URL двічі не потрібен.
func TestExpandMasterStream_DedupesSameURL(t *testing.T) {
	variants, _ := provider.ParseMasterPlaylist(
		"#EXTM3U\n#EXT-X-STREAM-INF:RESOLUTION=1920x1080\na.m3u8\n#EXT-X-STREAM-INF:RESOLUTION=1280x720\na.m3u8\n",
		"https://cdn.example/m.m3u8")

	out := provider.ExpandMasterStream(masterSource(), variants)
	if len(out) != 1 {
		t.Fatalf("duplicate variant URLs must collapse, got %d", len(out))
	}
}

// --- Назва якості ----------------------------------------------------

func TestHLSCVariantQuality(t *testing.T) {
	cases := []struct {
		height int
		want   string
	}{
		{2160, "4K"},
		{1080, "1080p"},
		{720, "720p"},
		{480, "480p"},
		{0, "Auto"}, // не оголочено — не вигадуємо
	}
	for _, tc := range cases {
		v := provider.HLSCVariant{Height: tc.height}
		if got := v.Quality(); got != tc.want {
			t.Errorf("height %d -> %q, want %q", tc.height, got, tc.want)
		}
	}
}

func TestMasterQualityOf(t *testing.T) {
	if got := provider.MasterQualityOf([]domain.StreamSource{
		{Quality: "480p"}, {Quality: "1080p"}, {Quality: "Auto"},
	}); got != "1080p" {
		t.Fatalf("best = %q, want 1080p", got)
	}
	if got := provider.MasterQualityOf([]domain.StreamSource{{Quality: "Auto"}}); got != "Auto" {
		t.Fatalf("all-auto must stay Auto, got %q", got)
	}
	if got := provider.MasterQualityOf(nil); got != "Auto" {
		t.Fatalf("empty must be Auto, got %q", got)
	}
}