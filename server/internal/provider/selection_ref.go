package provider

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// Ref — це посилання, з якого сторінка деталей складає запит
// на потік. Проблема в тому, що клієнт повертає серверу саме те, що
// ми поклали в Episode.URL, а сам Episode.URL мусить уміти сказати
// «який саме сезон, яка серія, яка озвучка».
//
// Існуючі два формати:
//
//  1. Конверт (основний). JSON з item_url, season, episode, voice.
//     Саме він несе адресу сторінки, бо без неї провайдер не знає,
//     яку сторінку завантажувати.
//
//  2. Компактний «v:сезон:серія:озвучка». Використовується, коли
//     адреси сторінки немає — наприклад, у старих тестах і в
//     результатах, зібраних напряму з дерева плейлиста.
//
// Розшифровка відбувається в одному місці — Registry.Streams — щоб
// не дублювати її в кожному провайдері.

type selectionEnvelope struct {
	ItemURL string `json:"item_url"`
	Season  int    `json:"season,omitempty"`
	Episode int    `json:"episode,omitempty"`
	Voice   string `json:"voice,omitempty"`
}

// EncodeSelectionRef збирає ref, який клієнт поверне як itemURL.
//
// itemURL обов'язковий: без нього провайдер не знає, що
// завантажувати. Якщо він порожній, ref все одно буде розшифровано,
// але провайдер отримає порожній URL і поверне помилку — це краще,
// ніж мовчки віддати потоки не того епізоду.
func EncodeSelectionRef(itemURL, voice string, season, episode int) string {
	b, err := json.Marshal(selectionEnvelope{
		ItemURL: itemURL,
		Season:  season,
		Episode: episode,
		Voice:   voice,
	})
	if err != nil {
		// Структура з трьома int і двома string не може дати помилку
		// маршалінгу. Повертаємо компактний формат як запобіжник.
		return voiceEpisodeRefPrefix + strconv.Itoa(season) + ":" + strconv.Itoa(episode) + ":" + voice
	}
	return string(b)
}

// DecodeSelectionRef розбирає ref і повертає справжню адресу
// сторінки разом із вибором (сезон, серія, озвучка).
//
// ok=false означає, що це не наш ref, а звичайний URL сторінки —
// тоді вибір лишається тим, що прийшов окремими параметрами запиту.
func DecodeSelectionRef(raw string) (itemURL string, season, episode int, voice string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw, 0, 0, "", false
	}

	// Компактний формат «v:…» перевіряємо першим: він коротший за
	// JSON і не потребує розбору.
	if strings.HasPrefix(trimmed, voiceEpisodeRefPrefix) {
		v, s, e, decoded := DecodeVoiceEpisodeRef(trimmed)
		return "", s, e, v, decoded
	}

	if !strings.HasPrefix(trimmed, "{") {
		return raw, 0, 0, "", false
	}

	var env selectionEnvelope
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		return raw, 0, 0, "", false
	}
	if env.ItemURL == "" {
		return "", env.Season, env.Episode, env.Voice, true
	}
	return env.ItemURL, env.Season, env.Episode, env.Voice, true
}

// ApplySelectionRef — зручна обгортка для провайдерів: повертає
// адресу сторінки та вибір, уже підставлені з ref.
//
// Значення season>0, episode>0, voice з явних аргументів мають
// пріоритет над ref: якщо запит містить і ref, і явний параметр,
// ми віримо параметру. Так клієнт може перевизначити серію,
// не змінюючи ref.
func ApplySelectionRef(itemURL string, season, episode int, voiceID string) (string, int, int, string) {
	realURL, refSeason, refEpisode, refVoice, ok := DecodeSelectionRef(itemURL)
	if !ok {
		return itemURL, season, episode, voiceID
	}
	if season <= 0 {
		season = refSeason
	}
	if episode <= 0 {
		episode = refEpisode
	}
	if voiceID == "" {
		voiceID = refVoice
	}
	return realURL, season, episode, voiceID
}

// BuildVoiceoversForItem — це BuildVoiceoversFromPlaylist, але ref
// кожної серії несе ще й адресу сторінки.
//
// Без адреси ref непридатний: провайдер не знатиме, що
// завантажувати. Тому провайдери, які показують сезони, мусять
// передавати itemURL сюди.
func BuildVoiceoversForItem(items []playerJSPlaylistItem, itemURL string) []domain.Voiceover {
	var order []string
	seen := map[string]bool{}

	ForEachPlaylistLeaf(items, func(l PlaylistLeaf) {
		dub := l.Ctx.Dub
		if dub == "" || seen[dub] {
			return
		}
		seen[dub] = true
		order = append(order, dub)
	})

	if len(order) == 0 {
		return nil
	}

	out := make([]domain.Voiceover, 0, len(order))
	for _, dub := range order {
		sub := filterByDub(items, dub)
		id, name, _ := ResolveVoiceoverNameNormalised(dub)

		seasons := BuildSeasonsFromPlaylist(sub)
		if itemURL != "" {
			stampSelectionRefs(seasons, itemURL, name)
		}

		out = append(out, domain.Voiceover{ID: id, Name: name, Seasons: seasons})
	}
	SortVoiceoversByWeight(out)
	return out
}

// BuildSeasonsForItem — сезони з ref, що несе адресу сторінки.
func BuildSeasonsForItem(items []playerJSPlaylistItem, itemURL string) []domain.Season {
	seasons := BuildSeasonsFromPlaylist(items)
	if itemURL != "" {
		stampSelectionRefs(seasons, itemURL, "")
	}
	return seasons
}

// stampSelectionRefs проставляє в кожну серію ref із адресою
// сторінки та озвучкою.
//
// voice передається порожнім для BuildSeasonsForItem: тоді озвучку
// бере клієнт із Voiceover, а якщо озвучка не задана — сервер сам
// віддасть першу доступну.
func stampSelectionRefs(seasons []domain.Season, itemURL, voice string) {
	for si := range seasons {
		for ei := range seasons[si].Episodes {
			v := voice
			if v == "" {
				// Витягуємо озвучку з уже проставленого ref, щоб не
				// втратити її під час перезапису.
				if existing, _, _, ok := DecodeVoiceEpisodeRef(seasons[si].Episodes[ei].URL); ok {
					v = existing
				}
			}
			seasons[si].Episodes[ei].URL = EncodeSelectionRef(itemURL, v, seasons[si].Number, seasons[si].Episodes[ei].Number)
		}
	}
}