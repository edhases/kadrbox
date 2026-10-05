package provider_test

import (
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

// Ці тести закривають дефекти, знайдені аудитом Quality/Voiceover
// на ЖИВОМУ трафіку 2026-10-05.

// --- Voiceover не має права містити ім'я CDN ------------------------

// Канонічне правило: назва CDN не є студією озвучення.
//
// УВАЖА про uaflix / bambooua / mikai: вони Є в словнику, і це
// правильно. Ці сайти справді приходять міткою озвучки в плеєрі,
// тож показувати «Mikai» там правильно. Регресія B7 була не в
// словнику, а в тому, що Bandera підставляв ключ джерела API
// (targetSource) у Voiceover без урахування контексту.
//
// Список нижче — це хости й CDN, які studios-ніколи не бувають
// назвою озвучки.
func TestVoiceoverNeverEqualsBalancerName(t *testing.T) {
	cdnOnlyNames := []string{"HDVB", "Ashdi", "Zenith", "allarknow", "Tortuga", "filmix", "stloadi"}

	for _, cdn := range cdnOnlyNames {
		if _, ok := provider.ResolveStudio(cdn); ok {
			t.Errorf("%q must not resolve as a dubbing studio — it is a CDN host", cdn)
		}
	}
}

// Справжні студії мають розпізнаватися, інакше ми б просто підмінили
// одну проблему іншою.
func TestVoiceoverResolvesRealStudios(t *testing.T) {
	for _, name := range []string{"1+1", "Postmodern", "DniproFilm", "Студія Качур"} {
		info, ok := provider.ResolveStudio(name)
		if !ok {
			t.Errorf("%q must resolve to a studio", name)
			continue
		}
		if info.Name == "" || info.ID == "" {
			t.Errorf("%q resolved to an empty studio: %+v", name, info)
		}
	}
}

// --- qualityFromURL: підкреслення -------------------------------------

// Регресія B4. \b не вважає "_" роздільником слова, а _ входить у
// \w. Тому найпоширеніший формат назви релізу на DLE-сайтах не давав
// жодної якості.
func TestQualityFromURL_UnderscoreSeparators(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{
			"https://s30.hdvbua.pro/media1/hls/films/the_shawshank_redemption_1994_bdrip_1080p_h.265_3xukr_eng_1141/hls/index.m3u8",
			"1080p",
		},
		{".../release_720p_h264/index.m3u8", "720p"},
		{".../film_480p_bdrip/index.m3u8", "480p"},
		{".../movie_4k_hevc/index.m3u8", "4K"},
		// Роздільники інших типів не зламалися.
		{"https://cdn/hls/1080p/index.m3u8", "1080p"},
		{"https://cdn/hls/720/index.m3u8", "720p"},
	}
	for _, tc := range cases {
		if got := provider.QualityFromURL(tc.url); got != tc.want {
			t.Errorf("QualityFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// Розширення класу роздільників не має створити хибних збігів:
// рік у назві релізу — це не якість.
func TestQualityFromURL_NoFalsePositives(t *testing.T) {
	for _, url := range []string{
		"https://cdn/hls/films/series_1990/hls/index.m3u8",
		"https://cdn/hls/1984/index.m3u8",
		"https://cdn/hls/some_movie_name/index.m3u8",
		"https://cdn/hls/index.m3u8",
	} {
		if got := provider.QualityFromURL(url); got != "Auto" {
			t.Errorf("QualityFromURL(%q) = %q, want Auto", url, got)
		}
	}
}

// --- Мова субтитрів --------------------------------------------------

// Регресія B6: мітка і код мови були одним рядком, тож
// SubtitleSource.Language міг дорівнювати "Українські".
func TestCanonicalSubtitleLang(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Українські", "uk"},
		{"Українська", "uk"},
		{"укр", "uk"},
		{"ua", "uk"},
		{"uk", "uk"},
		{"English", "en"},
		{"en", "en"},
		{"Русский", "ru"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := provider.CanonicalSubtitleLang(tc.in); got != tc.want {
			t.Errorf("CanonicalSubtitleLang(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Мова, якої немає у словнику, лишається як була — краще показати
// невідому мову, ніж вигадати uk.
func TestCanonicalSubtitleLang_UnknownKeptVerbatim(t *testing.T) {
	if got := provider.CanonicalSubtitleLang("Угорська"); got != "Угорська" {
		t.Fatalf("unknown language must stay verbatim, got %q", got)
	}
}

// --- Регресія B5: список доріжок без ключа subtitle: -----------------

// Значення поля subtitle вузла плейлиста — це «[Мітка]url» БЕЗ ключа
// subtitle:. Раніше такий список не парсився взагалі, тобто на
// Ashdi (18 доріжок з 46 листків) субтитри зникали на шляху дерева.
func TestParseSubtitlesFromHTML_PlayerJSListWithoutKey(t *testing.T) {
	text := "[Українські]https://ashdi.vip/player/subtitle/77104_ua.vtt,[English]https://ashdi.vip/player/subtitle/77104_en.vtt"

	subs := provider.ParseSubtitlesFromHTML(text)
	if len(subs) != 2 {
		t.Fatalf("want 2 subtitle tracks, got %d: %+v", len(subs), subs)
	}

	if subs[0].Language != "uk" {
		t.Errorf("track 0 Language = %q, want uk", subs[0].Language)
	}
	if subs[0].Label != "Українські" {
		t.Errorf("track 0 Label = %q, want Українські", subs[0].Label)
	}
	if subs[0].URL != "https://ashdi.vip/player/subtitle/77104_ua.vtt" {
		t.Errorf("track 0 URL = %q", subs[0].URL)
	}
	if subs[1].Language != "en" {
		t.Errorf("track 1 Language = %q, want en", subs[1].Language)
	}
}

// Той самий шаблон «[X]url» використовується для якості. Без фільтра
// ми додали б у субтитри кожне джерело.
func TestParseSubtitlesFromHTML_DoesNotTreatQualityAsSubtitle(t *testing.T) {
	text := "[1080p]https://cdn.example/hls/1080/index.m3u8,[720p]https://cdn.example/hls/720/index.m3u8"

	subs := provider.ParseSubtitlesFromHTML(text)
	if len(subs) != 0 {
		t.Fatalf("quality labels must not become subtitles, got %+v", subs)
	}
}

// Дублікати залишаються дублями тільки один раз.
func TestParseSubtitlesFromHTML_Dedupes(t *testing.T) {
	text := "[Українські]https://a/x.vtt [Українські]https://a/x.vtt"

	subs := provider.ParseSubtitlesFromHTML(text)
	if len(subs) != 1 {
		t.Fatalf("want 1 unique track, got %d: %+v", len(subs), subs)
	}
}

// --- Канонічні назви якості ----------------------------------------

// Bandera віддає "auto" з малої літери. Клієнт чекає "Auto".
func TestNormalizeQualityLabel_AutoIsCanonical(t *testing.T) {
	for _, in := range []string{"auto", "Auto", "AUTO", "авто", ""} {
		if got := provider.NormalizeQualityLabel(in); got != "Auto" {
			t.Errorf("NormalizeQualityLabel(%q) = %q, want Auto", in, got)
		}
	}
}

// Рядок, який не є роздільністю, не повинен виглядати як якість.
func TestNormalizeQualityLabel_KeepsUnrecognisedVerbatim(t *testing.T) {
	// Ми не вигадуємо: невідома мітка лишається, але клієнт зведе її
	// до unknown. Важливо, що normalizeQualityLabel не вигадує
	// «720p» з порожнього рядка.
	if got := provider.NormalizeQualityLabel(""); got != "Auto" {
		t.Fatalf("empty must be Auto, got %q", got)
	}
	for _, in := range []string{"1080", "1080p", "720P", "4k", "2160p"} {
		got := provider.NormalizeQualityLabel(in)
		if got == "Auto" {
			t.Errorf("NormalizeQualityLabel(%q) must not fall back to Auto", in)
		}
	}
}