package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
)

var defaultStreamKeysFallback = []string{"url", "episode_id", "file"}

// IsStreamRef визначає, чи є наданий ref посиланням на стрім (готовим для /stream),
// чи це посилання на контент/серіал/фільм (вимагає виклику /content).
// Використовує динамічні StreamKeys та ContentKeys із /sources для відповідного джерела.
// Для джерел, де ключі перетинаються (наприклад, animeon має ContentKeys: [id] і StreamKeys: [episode_id, id]),
// пріоритет мають ключі з різниці множин (streamKeys \ contentKeys).
func IsStreamRef(meta SourceMeta, refRaw json.RawMessage) bool {
	if len(refRaw) == 0 {
		return false
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(refRaw, &rawMap); err != nil {
		return false
	}

	streamKeys := meta.StreamKeys
	if len(streamKeys) == 0 {
		streamKeys = defaultStreamKeysFallback
	}

	// 1. Обчислюємо ключі, унікальні для stream: streamKeys \ contentKeys
	contentSet := make(map[string]struct{}, len(meta.ContentKeys))
	for _, ck := range meta.ContentKeys {
		contentSet[ck] = struct{}{}
	}

	var uniqueStreamKeys []string
	for _, sk := range streamKeys {
		if _, inContent := contentSet[sk]; !inContent {
			uniqueStreamKeys = append(uniqueStreamKeys, sk)
		}
	}

	// 2. Якщо є унікальні стрімові ключі: наявність хоча б одного свідчить, що це stream ref
	if len(uniqueStreamKeys) > 0 {
		for _, k := range uniqueStreamKeys {
			if _, ok := rawMap[k]; ok {
				return true
			}
		}
		// Якщо жодного унікального ключа стріму немає — це НЕ стрім ref (навіть якщо є спільний ключ типу 'id')
		return false
	}

	// 3. Дегенерований випадок (якщо streamKeys повністю перекривається contentKeys або contentKeys порожній)
	for _, k := range streamKeys {
		if _, ok := rawMap[k]; ok {
			return true
		}
	}
	return false
}

// ValidateStreamRef перевіряє, чи придатний ref для передачі в /stream.
// Захищає від помилки 400 MISSING_URL: якщо ref не містить жодного ключа з StreamKeys,
// ми взагалі не робимо мережевий запит, а повертаємо зрозумілу помилку.
func ValidateStreamRef(meta SourceMeta, refRaw json.RawMessage) error {
	if !IsStreamRef(meta, refRaw) {
		return fmt.Errorf("invalid stream ref: missing required stream key for source %q (expected one of %v)",
			meta.Key, meta.StreamKeys)
	}
	return nil
}

var reSeasonDigits = regexp.MustCompile(`\d+`)

// ParseSeasonNumber парсить номер сезону з json.RawMessage (може бути int або string, наприклад "1", "Сезон 2", "s03").
func ParseSeasonNumber(raw json.RawMessage, defaultNum int) int {
	if len(raw) == 0 {
		return defaultNum
	}
	var num int
	if err := json.Unmarshal(raw, &num); err == nil && num > 0 {
		return num
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		if n, err := strconv.Atoi(str); err == nil && n > 0 {
			return n
		}
		if match := reSeasonDigits.FindString(str); match != "" {
			if n, err := strconv.Atoi(match); err == nil && n > 0 {
				return n
			}
		}
	}
	return defaultNum
}

// SelectEpisodeRef шукає ref епізоду для заданого сезону, номеру серії та voiceID.
func SelectEpisodeRef(voices []BanderaVoice, seasonNum, episodeNum int, voiceID string) (json.RawMessage, []BanderaSubtitleItem, bool) {
	if len(voices) == 0 {
		return nil, nil, false
	}

	// 1. Обираємо озвучку: якщо voiceID вказано — шукаємо її, інакше беремо першу доступну
	chosenVoice := voices[0]
	if voiceID != "" {
		for _, v := range voices {
			if string(v.ID) == voiceID {
				chosenVoice = v
				break
			}
		}
	}

	// 2. Якщо в озвучці є сезони
	if len(chosenVoice.Seasons) > 0 {
		for sIdx, s := range chosenVoice.Seasons {
			sNum := ParseSeasonNumber(s.Title, sIdx+1)
			if seasonNum > 0 && sNum != seasonNum {
				continue
			}

			for _, ep := range s.Episodes {
				epNum := ep.Number.Int()
				if episodeNum > 0 && epNum != episodeNum {
					continue
				}
				if len(ep.Ref) > 0 {
					return ep.Ref, ep.Subtitles, true
				}
			}
		}
	}

	// 3. Якщо в озвучці плоский список епізодів (без явної структури сезонів)
	if len(chosenVoice.Episodes) > 0 {
		for _, ep := range chosenVoice.Episodes {
			epNum := ep.Number.Int()
			if episodeNum > 0 && epNum != episodeNum {
				continue
			}
			if len(ep.Ref) > 0 {
				return ep.Ref, ep.Subtitles, true
			}
		}
	}

	return nil, nil, false
}
