package provider

import (
	"encoding/json"
	"fmt"
	"strconv"
)

var defaultStreamKeysFallback = []string{"url", "episode_id", "file"}

// IsStreamRef визначає, чи є наданий ref посиланням на стрім (готовим для /stream),
// чи це посилання на контент/серіал/фільм (вимагає виклику /content).
// Використовує динамічні StreamKeys із /sources для відповідного джерела.
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

// ParseSeasonNumber парсить номер сезону з json.RawMessage (може бути int або string).
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
