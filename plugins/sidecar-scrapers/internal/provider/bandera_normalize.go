package provider

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// reJSWhitespace відповідає пробілам у JavaScript, включаючи NBSP (U+00A0) та інші юнікодні розділювачі.
// Go `\s` розпізнає лише ASCII [\t\n\f\r ], тоді як JS `\s` включає всі ці символи.
var reJSWhitespace = regexp.MustCompile(`[\t\n\f\r \x{00a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{205f}\x{3000}\x{feff}]+`)

const (
	SirkoProxyBase  = "https://stream.ernax.pro/proxy-hls?url="
	StreamProxyBase = "https://api.framextv.tech/api/proxy?url="

	sourceKeySirko  = "sirko"
	sourceKeyUAFlix = "uaflix"
)

var (
	reAshdi          = regexp.MustCompile(`(?i)(^|//)([^/]*\.)?ashdi\.vip(/|$)`)
	reZetvideo       = regexp.MustCompile(`(?i)(^|//)([^/]*\.)?zetvideo\.[^/]+(/|$)`)
	reSniplyo        = regexp.MustCompile(`(?i)(^|//)([^/]*\.)?sniplyo\.online(/|$)`)
	reCreavio        = regexp.MustCompile(`(?i)(^|//)([^/]*\.)?creavio\.online(/|$)`)
	reQualityBracket = regexp.MustCompile(`^\[(.*?)\](.*)$`)
)

// CleanJSWhitespace замінює будь-які юнікодні та ASCII пробіли на звичайний одинарний пробіл або обрізає їх.
func TrimJSWhitespace(s string) string {
	s = reJSWhitespace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ParsePackedStreamURL розбирає стрім формату "[  720p  ]  https://..." або повертає rawURL і якість
func ParsePackedStreamURL(raw string) (cleanURL, quality string) {
	raw = TrimJSWhitespace(raw)
	if m := reQualityBracket.FindStringSubmatch(raw); len(m) == 3 {
		q := TrimJSWhitespace(m[1])
		u := TrimJSWhitespace(m[2])
		if u != "" {
			if q == "" {
				q = "auto"
			}
			return u, q
		}
	}
	return raw, "auto"
}

// WrapStreamURL застосовує правила проксі відповідно до офіційної логіки bo.js.
// 1. Ідемпотентність: якщо вже обгорнуто — повертаємо як є.
// 2. Зовнішній плеєр — ніколи не проксі (лише inner).
// 3. Sirko — окремий проксі, значення екранується (url.QueryEscape).
// 4. uaflix + zetvideo — ВИНЯТОК, БЕЗ проксі (працює напряму).
// 5. ashdi / zetvideo / sniplyo / creavio — framextv RAW (без екранування).
// 6. Інше — без змін (прямий доступ).
func WrapStreamURL(sourceKey, playerMode, raw string) (playable, direct string, requiresProxy bool) {
	// 1. Ідемпотентність
	if strings.HasPrefix(raw, StreamProxyBase) || strings.HasPrefix(raw, SirkoProxyBase) {
		return raw, raw, false
	}
	// 2. Зовнішній плеєр
	if playerMode != "" && playerMode != "inner" {
		return raw, raw, false
	}
	// 3. Sirko — значення екранується
	if sourceKey == sourceKeySirko {
		return SirkoProxyBase + url.QueryEscape(raw), raw, true
	}
	// 4. uaflix + zetvideo — виняток, БЕЗ проксі
	if sourceKey == sourceKeyUAFlix && reZetvideo.MatchString(raw) {
		return raw, raw, false
	}
	// 5. ashdi / zetvideo / sniplyo / creavio — framextv, значення RAW
	if reAshdi.MatchString(raw) || reZetvideo.MatchString(raw) ||
		reSniplyo.MatchString(raw) || reCreavio.MatchString(raw) {
		return StreamProxyBase + raw, raw, true
	}
	// 6. Інше — без змін
	return raw, raw, false
}

// BuildStreamHeaders формує HTTP-заголовки для клієнтського плеєра libmpv.
// КРИТИЧНО: при requiresProxy=true Referer та Origin НЕ виставляються (лише User-Agent),
// оскільки запит іде до proxy api.framextv.tech, а сам проксі самостійно виставляє
// коректні заголовки для цільового CDN.
func BuildStreamHeaders(mediaURL string, requiresProxy bool) map[string]string {
	headers := map[string]string{
		"User-Agent": Chrome120UserAgent,
	}
	if requiresProxy {
		return headers
	}

	if u, err := url.Parse(mediaURL); err == nil && u.Scheme != "" && u.Host != "" {
		origin := u.Scheme + "://" + u.Host
		headers["Origin"] = origin
		headers["Referer"] = origin + "/"
	}
	return headers
}

// MergeSubtitles об'єднує списки субтитрів без дублікатів URL
func MergeSubtitles(primary, fallback []BanderaSubtitleItem) []domain.SubtitleSource {
	seen := make(map[string]bool)
	var out []domain.SubtitleSource

	add := func(items []BanderaSubtitleItem) {
		for _, it := range items {
			u := it.URL.String()
			if u == "" || seen[u] {
				continue
			}
			seen[u] = true
			lang := it.Lang.String()
			label := it.Label.String()
			if label == "" {
				label = lang
			}
			if label == "" {
				label = "Субтитри"
			}
			out = append(out, domain.SubtitleSource{
				URL:      u,
				Language: lang,
				Label:    label,
			})
		}
	}

	add(primary)
	add(fallback)
	return out
}
