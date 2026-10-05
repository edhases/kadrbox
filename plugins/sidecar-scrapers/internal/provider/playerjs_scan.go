package provider

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Сканер значення file: у конфігурації PlayerJS.
//
// Навіщо сканер, а не регулярка. Раніше ми шукали JSON-плейлист
// регуляркою
//
//	file\s*:\s*(\[[^"'].*?\])           // rePlayerJSJSONFile
//
// Вона вимагає «[» одразу після двокрапки та забороняє подвійні
// лапки всередині. Реальний HDVB пише
//
//	file: '[{"title":"1 сезон","folder":[{"title":"1+1",…'
//
// тобто одинарні лапки зовні та подвійні всередині — регулярка не
// збігалася взагалі. Наслідок був неочевидним: extractPlayableURL
// переходила до наступної стратегії (bare-media-url), яскрала
// перший m3u8 зі сторінки, і клієнт отримував рівно ОДИН стрім
// замість 155 серій чотирьох сезонів. Аргументи season та episode
// при цьому ігнорувалися, бо до дерева код просто не доходив.
//
// Сканер рахує глибину дужок і при цьому пам'ятає, чи ми всередині
// рядкового літерала, тож екрановані лапки всередині значення не
// збивають підрахунок.

// reFileKey знаходить ключ file: разом з роздільником. Далі ми НЕ
// йдемо регуляркою, а скануємо значення вручну: regexp Go (RE2)
// не підтримує ані зворотних посилань (\1), ані лічильників
// повтору більше за 1000, а плейлист на 155 серій — це десятки
// кілобайт, і регулярка просто не могла його витягнути.
var reFileKey = regexp.MustCompile(`(?s)["']?\bfile["']?\s*:\s*`)
// maxPlaylistScanBytes обмежує сканування. Значення file: у
// нормальному випадку — це десятки кілобайт JSON; 8 MiB — стеля
// на випадок зловмисної сторінки, яка підсуне мегабайт дурних
// дужок. maxPlaylistDepth обмежує вкладеність, щоб «{{{{{{…» не
// змусив нас читати весь файл у пам'яті.
const (
	maxPlaylistScanBytes = 8 << 20
	maxPlaylistDepth    = 64
)

// ExtractPlaylistFromPlayerHTML дістає сире значення file: з HTML
// або конфігурації плеєра.
//
// found=true означає «значення file: знайдено». Порожній raw при
// found=true можливий, якщо file: присутній, але має форму,
// яку ми не розпізнали — це корисніше за found=false, бо
// клієнт тоді не спробує інші стратегії бездумно.
func ExtractPlaylistFromPlayerHTML(text string) (raw string, found bool) {
	if len(text) > maxPlaylistScanBytes {
		text = text[:maxPlaylistScanBytes]
	}

	loc := reFileKey.FindStringIndex(text)
	if loc == nil {
		return "", false
	}

	rest := text[loc[1]:]
	trimmed := strings.TrimLeft(rest, " \t\r\n")
	if trimmed == "" {
		return "", false
	}

	switch trimmed[0] {
	case '[':
		// Значення — одразу масив об'єктів, без рядкової обгортки.
		if end := scanJSONArray(trimmed); end > 0 {
			return trimmed[:end], true
		}
		return "", true

	case '"', '\'':
		quote := trimmed[0]
		if end := scanQuoted(trimmed, quote); end > 0 {
			value := unescapeJSONString(trimmed[1 : end-1])
			if isBalancedJSON(value) {
				return value, true
			}
			// Не JSON — можливо, це прямий URL. Повертаємо як є:
			// ParsePlayerJSPlaylist перевірить і зробить штучне
			// дерево з одного файлу.
			if strings.HasPrefix(value, "http") {
				return value, true
			}
			return value, true
		}
		return "", true
	}

	return "", true
}

// scanQuoted повертає індекс одразу після закриваючої лапки,
// враховуючи екранування. 0 — лапка не закрилася.
func scanQuoted(s string, quote byte) int {
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == quote {
			return i + 1
		}
	}
	return 0
}

// scanJSONArray повертає індекс одразу після закриваючої дужки
// масиву. Рахує глибину, ігноруючи дужки всередині рядків, і
// обмежує глибину maxPlaylistDepth. 0 — масив не закрився.
func scanJSONArray(s string) int {
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			escaped = false
			continue
		}
		switch {
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// інші символи всередині рядка ігноруємо
		case c == '[':
			depth++
			if depth > maxPlaylistDepth {
				return 0
			}
		case c == ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}
// unescapeJSONString розекрановує значення file:, зняте сканером
// (тобто без обрамляючих лапок).
//
// Символ \ перед будь-яким символом означає, що наступний символ
// літеральний: \/ — це просто /, \" — це ", \n — перевід строки.
func unescapeJSONString(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			// висячий зворотний слэш: віддаємо як є, без паніки
			b.WriteByte('\\')
			break
		}
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// isBalancedJSON швидко перевіряє, що значення виглядає як
// завершений JSON-масив.
//
// Не повний парсер: він лише рахує глибину дужок, ігноруючи ті,
// що всередині рядків. Повну коректність перевірить json.Unmarshal
// далі — ця функція існує лише для того, щоб не віддавати клієнту
// обрізаний або змішаний текст замість плейлиста.
func isBalancedJSON(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' {
		return false
	}
	return scanJSONArray(s) == len(strings.TrimSpace(s))
}

// ParsePlayerJSPlaylist дістає й розбирає дерево плейлиста з HTML
// плеєра за один виклик.
//
// Це та точка, де з'єдналися дві проблеми: регулярка не бачила
// значення file:, а extractStreamsFromPlaylistTree не вміла
// вкладеність «сезон → озвучка → серія».
func ParsePlayerJSPlaylist(html string) ([]playerJSPlaylistItem, bool) {
	raw, found := ExtractPlaylistFromPlayerHTML(html)
	if !found {
		return nil, false
	}

	// Спершу пробуємо як масив об'єктів плейлиста.
	if items, ok := ParsePlaylistJSON(raw); ok {
		return items, true
	}

	// Інакше це може бути один файл-рядок. Тоді будуємо штучне
	// дерево з одного елемента, щоб решта пайплайну працювала
	// без окремого коду для фільмів.
	var single string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &single); err == nil && single != "" {
		return []playerJSPlaylistItem{{File: json.RawMessage(strconv.Quote(single))}}, true
	}
	return nil, false
}