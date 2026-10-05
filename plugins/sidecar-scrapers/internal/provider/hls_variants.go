package provider

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// Розбір HLS master-playlist і розкриття варіантів у джерела.
//
// Навіщо це. Раніше ми віддавали клієнту master-playlist «як є» і
// ставили Quality = "Auto". Клієнт показував «Auto» замість
// переліку 1080p/720p/480p. Але master містить у собі
// #EXT-X-STREAM-INF з реальними варіантами — достатньо один раз
// його прочитати.
//
// Чому не можна «просто підставити шлях»
//
// Спокуса: з `/hls/index.m3u8` зробити `/hls/1080/index.m3u8`.
// Це працює на Tortuga, але на Ashdi дає фантом. Перевірено
// жив'яком 2026-10-05 на «Величне століття. Роксолана»:
//
//	ashdi.vip/.../hls/480/index.m3u8   -> 200
//	ashdi.vip/.../hls/1080/index.m3u8  -> 200   плейлист існує!
//	ashdi.vip/.../hls/480/segment1.ts  -> 200, 309636 байт
//	ashdi.vip/.../hls/1080/segment1.ts -> 404     сегментів за ним немає
//
// Тобто плейлист 1080 віддає 200, хоча за ним нічого не відтворюється.
// Показувати користувачу «1080p», який при натисканні зупиниться на
// першому сегменті — гірше, ніж чесне «Auto».
//
// Тому правило одне: Quality береться тільки з того, що master
// оголосив сам. Ніколи не конструюємо шлях.

// reStreamInf розбирає атрибути #EXT-X-STREAM-INF.
var reStreamInf = regexp.MustCompile(`^#EXT-X-STREAM-INF:(.*)$`)

// reStreamAttr дістає один атрибут із рядка атрибутів.
var reStreamAttr = regexp.MustCompile(`([A-Z0-9-]+)=("[^"]*"|[^,]*)`)

// HLSCVariant — один варіант із master-playlist.
type HLSCVariant struct {
	URL       string
	Width     int
	Height    int
	Bandwidth int
}

// Quality повертає нормалізовану назву якості за висотою кадру.
//
// nil-результат означає, що роздільної здатності не оголошено: тоді
// Quality лишається Auto, а не вигадується з Bandwidth.
func (v HLSCVariant) Quality() string {
	switch {
	case v.Height <= 0:
		return defaultStreamQuality
	case v.Height >= 2160:
		return "4K"
	default:
		return strconv.Itoa(v.Height) + "p"
	}
}

// ParseMasterPlaylist розбирає master-playlist і повертає варіанти
// у порядку спадання висоти (від найкращої до найгіршої).
//
// baseURL потрібен, бо сегменти й сам варіант можуть бути
// відносними: на Tortuga URI варіанту абсолютний, але на інших CDN
// відносний, і без baseURL ми отримали б URL, який плеєр не розуміє.
//
// Порожній результат + ok=false означає, що це не master, а медіа-
// плейлист (перелік сегментів). Тоді джерело має залишитися одним.
func ParseMasterPlaylist(body, baseURL string) ([]HLSCVariant, bool) {
	if !strings.Contains(body, "#EXT-X-STREAM-INF") {
		return nil, false
	}

	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var variants []HLSCVariant

	for i := 0; i < len(lines); i++ {
		m := reStreamInf.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			continue
		}

		variant := HLSCVariant{}
		for _, attr := range reStreamAttr.FindAllStringSubmatch(m[1], -1) {
			key := attr[1]
			val := strings.Trim(attr[2], `"`)
			switch key {
			case "RESOLUTION":
				variant.Width, variant.Height = parseResolution(val)
			case "BANDWIDTH":
				variant.Bandwidth, _ = strconv.Atoi(val)
			}
		}

		// URL варіанта стоїть у наступному непорожньому рядку.
		for j := i + 1; j < len(lines); j++ {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" {
				continue
			}
			if strings.HasPrefix(candidate, "#") {
				break
			}
			variant.URL = absolutizeURL(candidate, baseURL)
			break
		}

		if variant.URL != "" {
			variants = append(variants, variant)
		}
	}

	if len(variants) == 0 {
		return nil, false
	}

	// Найкраща якість першою: клієнт показує список у цьому порядку,
	// і тим самим визначається «типова» якість відтворення.
	sort.SliceStable(variants, func(a, b int) bool {
		return variants[a].Height > variants[b].Height
	})
	return variants, true
}

// parseResolution розбирає "1920x1080" у ширину й висоту.
func parseResolution(v string) (width, height int) {
	parts := strings.SplitN(strings.ToLower(v), "x", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	width, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	height, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	return width, height
}

// ExpandMasterStream перетворює один master-потік на набір джерел
// по якостях.
//
// Повертає nil, коли розкриття не вдалося: тоді викликач має
// віддати master як є з Quality = Auto. Краще «Auto», ніж вигадана
// якість, за якою потім нічого не відтворюється.
//
// baseHeaders копіюється в кожне джерело: CDN перевіряє Referer на
// сегментах так само, як і на плейлисті.
func ExpandMasterStream(master domain.StreamSource, variants []HLSCVariant) []domain.StreamSource {
	if len(variants) == 0 {
		return nil
	}

	out := make([]domain.StreamSource, 0, len(variants))
	seen := make(map[string]bool, len(variants))

	for _, v := range variants {
		if v.URL == "" || seen[v.URL] {
			continue
		}
		seen[v.URL] = true

		src := master
		src.URL = v.URL
		src.DirectURL = v.URL
		src.Quality = v.Quality()
		out = append(out, src)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// MasterQualityOf повертає найкращу оголошену якість для
// послідовності джерел. Корисно, коли треба заповнити Quality у
// тих джерелах, які розкрити не вдалося.
func MasterQualityOf(sources []domain.StreamSource) string {
	best := defaultStreamQuality
	for _, s := range sources {
		if s.Quality == "" || s.Quality == defaultStreamQuality {
			continue
		}
		if qualityRank(s.Quality) > qualityRank(best) {
			best = s.Quality
		}
	}
	return best
}

// qualityRank переводить назву якості в число для порівняння.
func qualityRank(q string) int {
	switch normaliseQualityRank(q) {
	case "4k", "2160":
		return 2160
	case "1440":
		return 1440
	case "1080":
		return 1080
	case "720":
		return 720
	case "480":
		return 480
	case "360":
		return 360
	default:
		return 0
	}
}

// normaliseQualityRank приводить «1080P», «Full HD», «FHD», «HD» до
// форми, яку розуміє qualityRank.
//
// УВАЖА: літера «p» зрізається, тому далі ключі пишуться БЕЗ неї
// («1080», а не «1080p»). Раніше тут були ключі з «p», через що
// qualityRank завжди повертав 0, а MasterQualityOf — «Auto» для
// будь-якого набору джерел. Це зловив тест.
func normaliseQualityRank(q string) string {
	s := strings.ToLower(strings.TrimSpace(q))
	s = strings.NewReplacer(" ", "", "p", "").Replace(s)
	switch s {
	case "fhd", "fullhd":
		return "1080"
	case "2k":
		return "1440"
	case "2160":
		return "2160"
	case "hd":
		return "720"
	case "sd":
		return "480"
	case "auto":
		return ""
	}
	return s
}