package provider_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
)

// Цей файл закриває розрив, який був міжSeason.Episode.URL і
// запитом на потік.
//
// Схема така:
//
//	1. GetDetails повертає Episode.URL = ref (JSON-конверт або v:…)
//	2. Клієнт клацає серію і повертає цей ref як ?url=
//	3. GetStreams має розшифрувати ref і зрозуміти, яку саме серію
//	   та яку озвучку відтворювати
//
// Крок 3 був відсутній: DecodeVoiceEpisodeRef викликався лише з
// тестів. Тобто будь-який клік на серію в дереві озвучок давав
// 503 — «invalid item url payload», бо провайдер намагався
// розпарсити «v:1:2:1+1» як JSON.

// --- Розшифровка конверта ---------------------------------------------

func TestDecodeSelectionRef_Envelope(t *testing.T) {
	ref := provider.EncodeSelectionRef("https://uakino.biz/6970-x.html", "1+1", 2, 15)

	itemURL, season, episode, voice, ok := provider.DecodeSelectionRef(ref)
	if !ok {
		t.Fatalf("our own ref must decode, got %q", ref)
	}
	if itemURL != "https://uakino.biz/6970-x.html" {
		t.Fatalf("itemURL = %q, want the page url", itemURL)
	}
	if season != 2 || episode != 15 {
		t.Fatalf("selection = %d/%d, want 2/15", season, episode)
	}
	if voice != "1+1" {
		t.Fatalf("voice = %q, want 1+1", voice)
	}
}

// Звичайний URL сторінки не є нашим ref і мусить пройти наскрізь
// недоторканим. Якщо ми почнемо трактувати будь-який JSON як
// конверт, зламаються DLE-URL з JSON у query.
func TestDecodeSelectionRef_PassesThroughPlainURL(t *testing.T) {
	for _, raw := range []string{
		"https://uakino.biz/seriesss/x/6970-rok.html",
		"https://lavakino.net/serialy/4183.html",
		"",
	} {
		itemURL, season, episode, voice, ok := provider.DecodeSelectionRef(raw)
		if ok {
			t.Fatalf("%q must not be treated as a selection ref", raw)
		}
		if itemURL != raw {
			t.Fatalf("itemURL = %q, want unchanged %q", itemURL, raw)
		}
		if season != 0 || episode != 0 || voice != "" {
			t.Fatalf("selection must stay empty for %q, got %d/%d/%q", raw, season, episode, voice)
		}
	}
}

// Компактний формат лишається підтриманим: його пишуть старі
// результати, зібрані напряму з дерева плейлиста.
func TestDecodeSelectionRef_CompactForm(t *testing.T) {
	ref := "v:3:7:Postmodern"

	itemURL, season, episode, voice, ok := provider.DecodeSelectionRef(ref)
	if !ok {
		t.Fatalf("compact ref must decode, got %q", ref)
	}
	if itemURL != "" {
		t.Fatalf("compact ref carries no page url, got %q", itemURL)
	}
	if season != 3 || episode != 7 || voice != "Postmodern" {
		t.Fatalf("selection = %d/%d/%q, want 3/7/Postmodern", season, episode, voice)
	}
}

// --- Пріоритет явних параметрів над ref -------------------------------

// Клієнт має право перевизначити серію, не змінюючи ref.
func TestApplySelectionRef_ExplicitParamsWin(t *testing.T) {
	ref := provider.EncodeSelectionRef("https://x/1.html", "1+1", 2, 15)

	itemURL, season, episode, voice := provider.ApplySelectionRef(ref, 0, 99, "")

	if itemURL != "https://x/1.html" {
		t.Fatalf("itemURL = %q", itemURL)
	}
	if season != 2 {
		t.Fatalf("season must come from the ref, got %d", season)
	}
	if episode != 99 {
		t.Fatalf("explicit episode must win, got %d", episode)
	}
	if voice != "1+1" {
		t.Fatalf("voice = %q", voice)
	}
}

// Без ref усе має лишатися як було.
func TestApplySelectionRef_LeavesPlainURLAlone(t *testing.T) {
	itemURL, season, episode, voice := provider.ApplySelectionRef("https://x/1.html", 1, 2, "1+1")

	if itemURL != "https://x/1.html" || season != 1 || episode != 2 || voice != "1+1" {
		t.Fatalf("plain url must pass through untouched, got %q/%d/%d/%q", itemURL, season, episode, voice)
	}
}

// --- Наскрізна перевірка: дерево озвучок → ref → розшифровка ----------

func tree() []provider.PlayerJSPlaylistItem {
	return []provider.PlayerJSPlaylistItem{{
		Title: "Сезон 1",
		Folder: []provider.PlayerJSPlaylistItem{{
			Title: "1+1",
			Folder: []provider.PlayerJSPlaylistItem{
				{Title: "Серія 1", File: json.RawMessage(`"https://ashdi.vip/s1e1/index.m3u8"`)},
				{Title: "Серія 2", File: json.RawMessage(`"https://ashdi.vip/s1e2/index.m3u8"`)},
			},
		}},
	}}
}

// Це і є та петля, якої не існувало: GetDetails віддає дерево,
// клієнт повертає Episode.URL, GetStreams мусить зрозуміти вибір.
func TestSelectionRefRoundTrip_TreeToStreamRequest(t *testing.T) {
	const pageURL = "https://uakino.biz/seriesss/x/6970-rok.html"

	voices := provider.BuildVoiceoversForItem(tree(), pageURL)
	if len(voices) != 1 {
		t.Fatalf("want 1 voiceover, got %d", len(voices))
	}
	if len(voices[0].Seasons) != 1 || len(voices[0].Seasons[0].Episodes) != 2 {
		t.Fatalf("want 1 season x 2 episodes, got %+v", voices[0].Seasons)
	}

	// Клієнт клацає другу серію.
	clicked := voices[0].Seasons[0].Episodes[1].URL
	if clicked == "" {
		t.Fatal("episode must carry a ref")
	}

	itemURL, season, episode, voice, ok := provider.DecodeSelectionRef(clicked)
	if !ok {
		t.Fatalf("episode ref %q must decode", clicked)
	}
	if itemURL != pageURL {
		t.Fatalf("itemURL = %q, want %q — інакше провайдер вантажитиме не ту сторінку", itemURL, pageURL)
	}
	if season != 1 || episode != 2 {
		t.Fatalf("selection = %d/%d, want 1/2", season, episode)
	}
	if voice != "1+1" {
		t.Fatalf("voice = %q, want 1+1", voice)
	}

	// І тепер провайдер має віддати саме другу серію.
	streams, _ := provider.SelectPlaylistStream(tree(), pageURL, "Ashdi", season, episode, voice)
	if len(streams) != 1 {
		t.Fatalf("want exactly 1 stream, got %d", len(streams))
	}
	if !strings.Contains(streams[0].URL, "s1e2") {
		t.Fatalf("must return S01E02, got %s", streams[0].URL)
	}
}

// Регресія: ref мусить нести адресу сторінки. Без неї провайдер
// завантажуватиме порожній URL і поверне 503.
func TestBuildVoiceoversForItem_EpisodeRefCarriesPageURL(t *testing.T) {
	voices := provider.BuildVoiceoversForItem(tree(), "https://site/page.html")
	ref := voices[0].Seasons[0].Episodes[0].URL

	var envelope struct {
		ItemURL string `json:"item_url"`
	}
	if err := json.Unmarshal([]byte(ref), &envelope); err != nil {
		t.Fatalf("ref must be the JSON envelope, got %q", ref)
	}
	if envelope.ItemURL != "https://site/page.html" {
		t.Fatalf("item_url = %q, want the page url", envelope.ItemURL)
	}
}

// Сезони мають нести ту ж інформацію — інакше клієнт, який бере
// сезони з MediaDetails.Seasons (а не з voiceovers), не зможе
// відтворити серію.
func TestBuildSeasonsForItem_EpisodeRefCarriesPageURL(t *testing.T) {
	seasons := provider.BuildSeasonsForItem(tree(), "https://site/page.html")
	ref := seasons[0].Episodes[0].URL

	itemURL, season, episode, _, ok := provider.DecodeSelectionRef(ref)
	if !ok {
		t.Fatalf("season episode ref %q must decode", ref)
	}
	if itemURL != "https://site/page.html" || season != 1 || episode != 1 {
		t.Fatalf("got %q/%d/%d", itemURL, season, episode)
	}
}

// Старий виклик без адреси сторінки лишається робочим: він дає
// компактний ref, який розбирається, хоча й без item_url.
func TestBuildVoiceoversFromPlaylist_StillUsableWithoutPageURL(t *testing.T) {
	voices := provider.BuildVoiceoversFromPlaylist(tree())
	if len(voices) != 1 {
		t.Fatalf("want 1 voiceover, got %d", len(voices))
	}
	ref := voices[0].Seasons[0].Episodes[0].URL
	if _, season, episode, _, ok := provider.DecodeSelectionRef(ref); !ok || season != 1 || episode != 1 {
		t.Fatalf("compact ref must still decode, got %q (ok=%v)", ref, ok)
	}
}

// --- Стіртість ------------------------------------------------------

// Конверт не повинен роздуватися на 10k серій у пам'яті.
func TestEncodeSelectionRef_StaysCompact(t *testing.T) {
	ref := provider.EncodeSelectionRef("https://a-very-long-page-url-that-goes-on.example.com/path/to/item.html", "DniproFilm Studio", 12, 345)
	if len(ref) > 200 {
		t.Fatalf("ref is %d bytes — too fat for a URL parameter", len(ref))
	}
}

var _ = domain.StreamSource{} // лише щоб імпорт домену не здавався зайвим у цьому файлі