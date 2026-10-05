package search

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// QueryPlan містить нормалізовану та розібрану структуру пошукового запиту
type QueryPlan struct {
	Original  string   `json:"original"`
	Canonical string   `json:"canonical"`
	Tokens    []string `json:"tokens"`
	Year      int      `json:"year,omitempty"`
	TypeHint  string   `json:"type_hint,omitempty"` // "movie", "series", або порожньо
	Hash      string   `json:"hash"`                // SHA1 ключ для кешування
}

var (
	reMultipleSpaces = regexp.MustCompile(`\s+`)
	reYear           = regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)

	// Шумові токени (релізи, якість, переклад тощо)
	noiseWords = map[string]struct{}{
		"1080p": {}, "720p": {}, "2160p": {}, "4k": {}, "hd": {}, "fhd": {}, "uhd": {},
		"web-dl": {}, "webdl": {}, "web": {}, "dl": {}, "bdrip": {}, "dvdrip": {}, "rip": {}, "bluray": {},
		"cam": {}, "camrip": {}, "ts": {}, "x264": {}, "x265": {}, "hevc": {},
		"ukr": {}, "ua": {}, "sub": {}, "dub": {}, "переклад": {}, "озвучка": {}, "дубляж": {},
		"субтитри": {}, "субтитры": {}, "трейлер": {}, "online": {}, "онлайн": {},
		"сезон": {}, "серія": {}, "серіал": {}, "серии": {}, "серия": {}, "сериал": {}, "ep": {}, "season": {}, "series": {},
	}

	stopWords = map[string]struct{}{
		"в": {}, "і": {}, "та": {}, "на": {}, "з": {}, "про": {}, "до": {}, "у": {}, "за": {}, "не": {},
		"the": {}, "a": {}, "an": {}, "of": {}, "in": {}, "on": {}, "to": {}, "for": {}, "and": {},
	}

	// Фолдинг омогліфів (Cyrillic confusables -> Latin).
	// Результат використовується лише як ключ порівняння (кластеризація,
	// скоринг, стабільний content ID) і ніколи не показується користувачу.
	confusablesMap = map[rune]rune{
		'а': 'a', 'е': 'e', 'о': 'o', 'р': 'p', 'с': 'c', 'х': 'x', 'у': 'y',
		'і': 'i', 'ї': 'i', 'й': 'y', 'к': 'k', 'м': 'm', 'н': 'h', 'т': 't', 'в': 'b',
		// ґ і г — різні літери, але один звук. Сайти пишуть обидві
		// («Супергьорл» / «Суперґьорл»), тому без злиття два написання
		// дають неперетинні набори результатів.
		'ґ': 'г',
	}
)

// FoldConfusables нормалізує подібні за написанням кириличні літери
func FoldConfusables(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if mapped, ok := confusablesMap[r]; ok {
			b.WriteRune(mapped)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// BuildQueryPlan будує нормалізований план запиту
func BuildQueryPlan(raw string) *QueryPlan {
	rawTrimmed := strings.TrimSpace(raw)
	if rawTrimmed == "" {
		return &QueryPlan{}
	}

	// 1. NFC нормалізація Unicode
	normalized := norm.NFC.String(rawTrimmed)
	normalized = strings.ToLower(normalized)

	// 2. Витягуємо рік (якщо є)
	year := 0
	if match := reYear.FindString(normalized); match != "" {
		if y, err := strconv.Atoi(match); err == nil && y >= 1900 && y <= 2100 {
			year = y
		}
	}

	// 3. Визначаємо type hint
	typeHint := ""
	if strings.Contains(normalized, "серіал") || strings.Contains(normalized, "сезон") ||
		strings.Contains(normalized, "series") || strings.Contains(normalized, "season") ||
		strings.Contains(normalized, "сериал") {
		typeHint = "series"
	} else if strings.Contains(normalized, "фільм") || strings.Contains(normalized, "movie") ||
		strings.Contains(normalized, "film") || strings.Contains(normalized, "фильм") {
		typeHint = "movie"
	}

	// 4. Очищення від розділових знаків (залишаємо букви, цифри та пробіли)
	var cleanBuf strings.Builder
	cleanBuf.Grow(len(normalized))
	for _, r := range normalized {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cleanBuf.WriteRune(r)
		} else {
			cleanBuf.WriteRune(' ')
		}
	}

	// 5. Вирізка шуму та виділення токенів
	rawTokens := strings.Fields(cleanBuf.String())
	var contentTokens []string

	for _, t := range rawTokens {
		// Якщо токен — це знайдений рік, пропускаємо його з назви
		if year > 0 && t == strconv.Itoa(year) {
			continue
		}
		// Пропускаємо шумові слова
		if _, isNoise := noiseWords[t]; isNoise {
			continue
		}
		// Пропускаємо стоп-слова
		if _, isStop := stopWords[t]; isStop {
			continue
		}

		// Додаємо токени len >= 3 або коротші, якщо містять цифри
		if len([]rune(t)) >= 3 || (len([]rune(t)) >= 2 && containsDigit(t)) {
			contentTokens = append(contentTokens, t)
		}
	}

	canonical := strings.Join(contentTokens, " ")
	if canonical == "" {
		// Якщо вирізка шуму стерла все, повертаємо базовий очищений текст
		canonical = reMultipleSpaces.ReplaceAllString(cleanBuf.String(), " ")
		canonical = strings.TrimSpace(canonical)
	}

	// 6. Обчислення стабільного хешу для кешу
	hashInput := fmt.Sprintf("%s|%d|%s", canonical, year, typeHint)
	h := sha1.Sum([]byte(hashInput))
	hashKey := hex.EncodeToString(h[:])

	return &QueryPlan{
		Original:  rawTrimmed,
		Canonical: canonical,
		Tokens:    contentTokens,
		Year:      year,
		TypeHint:  typeHint,
		Hash:      hashKey,
	}
}

func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
