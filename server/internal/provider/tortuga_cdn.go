package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// Екстрактор CDN Tortuga (calypso.tortuga.tw).
//
// Контекст. Значення file: у плеєрі Tortuga — це НЕ JSON, а
// зашифрований рядок:
//
//	file: "7aF6Knt/aUhOEANil++F6bPdocavO62luY6L3sLTp6mh+NvK3tTRJnh3eFNIQBcGYXtzdH0AAhgf5/uyrcXejtre4/Lt5/DVyd7bdG56eXkBEBPonpb4hduz..."
//
// Автор uafilms/core натрапив на такий рядок, не розпізнав його як
// PlayerJS-плейлист і вирішив, що джерело мертве, — викреслив Tortuga
// зі свого коду. Насправді CDN живий (перевірено енд-ту-енд
// 2026-10-05): розшифрований blob — звичайний PlayerJS-дерево
// «Сезон N → Серія M» на 155 серій, 57835 байт JSON.
//
// Розшифровка — base64 (з обрізаним padding) → байти, де байт[0] це
// ключ, а решта XOR-иться лінійним зсувом від нього. Див.
// DecodeTortugaPlaylist.
//
// Другий важливий факт про відповідь CDN: master-playlist віддає три
// якості (1080/720/480) як окремі варіанти EXT-X-STREAM-INF. Ми НЕ
// переписуємо master і НЕ підставляємо "/hls/1080/" — на Ashdi такі
// шляхи дають 404. Клієнт отримує master як є.

// Стелі для розшифрованого payload.
const (
	// tortugaMaxDecodedBytes — стеля на розшифрований JSON.
	//
	// Реальний плейлист «Величне століття» — 57835 байт на 155 серій.
	// 8 MiB — це стеля з запасом, але без ризину OOM на зловмисній
	// сторінці, яка підсуне гігантський blob. Значення навмисно те
	// саме, що й MaxUpstreamBodyBytes: два різні ліміти на два різні
	// етапи (відповідь CDN / розшифрований з неї текст) дрейфували б
	// один від одного.
	tortugaMaxDecodedBytes = MaxUpstreamBodyBytes

	// tortugaMaxNestingDepth — стеля глибини вкладеності JSON.
	//
	// Реальне дерево має глибину 2 («сезон» → «серія»). 32 — величезний
	// запас для будь-якого майбутнього формату, але «{{{{{…» на
	// півмегабайта не змусить нас рекурсивно спускатися.
	tortugaMaxNestingDepth = 32
)

// tortugaXorStep та tortugaXorOffset — параметри лінійного зсуву XOR.
//
// Вони НЕ довільні: алгоритм належить сторонньому плеєру, і «покращення
// його» (наприклад, «а чому б не взяти красивіше 7 і 13») ламає
// розшифровку на реальному трафіку. Коментар тут — нагадування для
// майбутнього «маленького рефакторингу».
const (
	tortugaXorStep   = 7
	tortugaXorOffset = 13
)

// ErrTortugaUndecodable — значення file: не розшифровується або не
// містить плейлиста. Окремий тип помилки, щоб викликач відрізнив
// «формат не наш» від «сторінка не завантажилась» (помилка HTTP) і не
// намагався б бездумно перебирати інші стратегії.
var ErrTortugaUndecodable = errors.New("tortuga: file value is not a decodable playlist")

// DecodeTortugaPlaylist розшифровує значення file: і повертає дерево
// PlayerJS у тому самому вигляді, що й решта пайплайну.
//
// Алгоритм (перевірений на живому трафіку, крок за кроком):
//
//  1. зрізати всі хвостові знаки "=" — padding у реальному рядку їх не
//     має: плеєр зрізає його перед тим, як віддати значення file:;
//  2. дописати '=' до кратності 4 — інакше StdEncoding відмовляється;
//  3. розшифрувати base64 У БАЙТИ, а не в символи: алгоритм оперує
//     байтами, і «Сезон» мусить пройти через XOR як шість байтів
//     0xD0/0xA1/0xB7/0xBE/0xBE/0xBD, а не як два руни. У Go це
//     означає одне: не робити жодного перетворення через []rune і не
//     декодувати UTF-8 — просто наскрізь байти;
//  4. raw[0] — ключ, решта — XOR із (key + i*7 + 13) % 256.
func DecodeTortugaPlaylist(encoded string) ([]playerJSPlaylistItem, error) {
	blob, err := decryptTortugaBlob(encoded)
	if err != nil {
		return nil, err
	}
	items, ok := ParsePlaylistJSON(string(blob))
	if !ok {
		return nil, fmt.Errorf("%w: decrypted payload is not a playlist JSON", ErrTortugaUndecodable)
	}
	return items, nil
}

// decryptTortugaBlob — вся розшифровка без розбору JSON, винесена окремо,
// щоб її можна було перевіряти тестами незалежно від дерева.
func decryptTortugaBlob(encoded string) ([]byte, error) {
	enc := strings.TrimSpace(encoded)

	// На початку іноді трапляється JSON-обгортка: значення file: могло
	// прийти вже як екрановане рядкове поле. Знімаємо її, інакше
	// перший символ був би лапкою, а не першим байтом ключа.
	if strings.HasPrefix(enc, `"`) {
		var unquoted string
		if err := json.Unmarshal([]byte(enc), &unquoted); err == nil && unquoted != "" {
			enc = strings.TrimSpace(unquoted)
		}
	}

	enc = strings.TrimRight(enc, "=")
	if enc == "" {
		return nil, fmt.Errorf("%w: empty payload", ErrTortugaUndecodable)
	}

	// Стеля на ЗАШИФРОВАНИЙ вхід. base64 роздуває на 4/3, тому ліміт
	// вищий за стелю на розшифрований результат — інакше ми відрізали б
	// легітимний плейлист, який ледь вкладається.
	maxEncoded := (tortugaMaxDecodedBytes/3 + 1) * 4
	if len(enc) > maxEncoded {
		return nil, fmt.Errorf("%w: encoded payload is %d bytes, limit %d", ErrTortugaUndecodable, len(enc), maxEncoded)
	}

	// Крок 2: padding. len%4==1 у base64 неможливий — такого рядка не
	// існує, і це не «битий input, пробуймо далі», а гарантований
	// збій. Повертаємо помилку одразу.
	var padded string
	switch rem := len(enc) % 4; rem {
	case 0:
		padded = enc
	case 2:
		padded = enc + "=="
	case 3:
		padded = enc + "="
	default:
		return nil, fmt.Errorf("%w: base64 length %d is not decodable", ErrTortugaUndecodable, len(enc))
	}

	raw, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		return nil, fmt.Errorf("%w: base64: %v", ErrTortugaUndecodable, err)
	}
	if len(raw) < 2 {
		// Один байт — це тільки ключ, шифрованого тіла немає.
		return nil, fmt.Errorf("%w: decrypted body is %d bytes, need at least a key and one byte", ErrTortugaUndecodable, len(raw))
	}
	if len(raw)-1 > tortugaMaxDecodedBytes {
		return nil, fmt.Errorf("%w: decrypted payload is %d bytes, limit %d", ErrTortugaUndecodable, len(raw)-1, tortugaMaxDecodedBytes)
	}

	key := int(raw[0])
	out := make([]byte, 0, len(raw)-1)
	for i := 1; i < len(raw); i++ {
		out = append(out, raw[i]^byte((key+(i-1)*tortugaXorStep+tortugaXorOffset)%256))
	}

	// Глибину рахуємо саме на РОЗШИФРОВАНОМ результаті, а не на
	// ciphertext. Ciphertext — випадкові байти, там дужок немає взагалі,
	// і перевірка завжди проходила б (тобто не робила нічого).
	// Мутаційна перевірка це й показала.
	if depth := jsonNestingDepth(out); depth > tortugaMaxNestingDepth {
		return nil, fmt.Errorf("%w: JSON nesting depth %d exceeds limit %d", ErrTortugaUndecodable, depth, tortugaMaxNestingDepth)
	}
	return out, nil
}

// jsonNestingDepth рахує глибину вкладеності {}, [] у сирому JSON,
// ігноруючи дужки всередині рядкових літералів. Повертає min(depth,
// limit+1): нам не потрібна точна глибина, тільки «перевищено чи ні».
func jsonNestingDepth(raw []byte) int {
	depth := 0
	inString := false
	escaped := false

	for _, c := range raw {
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
			// решта символів усередині рядка не впливає на глибину
		case c == '{' || c == '[':
			depth++
			if depth > tortugaMaxNestingDepth {
				return depth
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	if depth < 0 {
		return 0
	}
	return depth
}

// tortugaDropQueryParams — query-параметри, які ламають відповідь
// Tortuga (і, за тим самим класом проблем, Ashdi).
//
//	geoblock=1 — CDN починає віддавати 403 «регіон заблоковано».
//	season/episode — приводять до відповіді-«однієї серії» замість
//	                  дерева: клієнт втрачає можливість вибору.
//	quality — змушує CDN віддати один варіант замість master.
//	lang/language, sub/subs — фільтри озвуки/субтитрів, які нам не
//	                  потрібні, бо озвучку ми беремо з файлів дерева.
//
// Усе інше (?id=, ?vid=, ?t=…) зберігається: ці параметри або нешкідливі,
// або потрібні сторінці.
var tortugaDropQueryParams = map[string]bool{
	"geoblock": true,
	"geo":      true,
	"season":   true,
	"episode":  true,
	"quality":  true,
	"lang":     true,
	"language": true,
	"sub":      true,
	"subs":     true,
	"cc":       true,
}

// sanitizeTortugaURL знімає руйнуючі query-параметри й перевіряє, що
// це взагалі відтворюване медіа.
//
// Повертає помилку, а не мовчки порожній рядок: викликач має відрізнити
// «нема чого грати» від «зіпсоване посилання».
func sanitizeTortugaURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("empty url")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("missing host")
	}

	q := u.Query()
	for key := range tortugaDropQueryParams {
		q.Del(key)
	}
	u.RawQuery = q.Encode()
	// Фрагмент (#...) плеєр не надсилає на сервер, але у StreamSource
	// він лише заважає — клієнт склеює його сам, якщо треба.
	u.Fragment = ""

	return u.String(), nil
}

// reTortugaSubtitleLabel — мітка перед URL у субтитрах:
// «[Українська]https://…». parseSubtitlesFromPlayerHTML такий
// формат не бере: він шукає ключ «subtitle:» або <track>. Значення
// file: від Tortuga містить уже «чистий» список міток, без ключа,
// тому розбираємо його тут.
var reTortugaSubtitleLabel = regexp.MustCompile(`^\s*\[([^\]]*)\]\s*(.+)$`)

// parseTortugaSubtitles розбирає список субтитрів у формі
//
//	[Українська]https://a.uk.vtt,[English]https://a.en.vtt
//
// і, якщо це не схоже, віддає значення загальному парсеру плеєра
// (форма «subtitle:[…]» або <track>).
func parseTortugaSubtitles(text string) []domain.SubtitleSource {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	var subs []domain.SubtitleSource
	seen := make(map[string]bool)

	for _, part := range strings.Split(trimmed, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		m := reTortugaSubtitleLabel.FindStringSubmatch(part)
		if m == nil {
			continue
		}
		u := strings.TrimSpace(m[2])
		if !strings.HasPrefix(u, "http") || seen[u] {
			continue
		}
		seen[u] = true

		label := strings.TrimSpace(m[1])
		lang := ""
		switch strings.ToLower(label) {
		case "украинская", "українська", "uk", "ukrainian":
			lang = "uk"
		case "english", "en", "английская", "англійська":
			lang = "en"
		case "russian", "ru", "русская":
			lang = "ru"
		}
		subs = append(subs, domain.SubtitleSource{
			URL:      u,
			Label:    label,
			Language: lang,
		})
	}

	if len(subs) > 0 {
		return subs
	}
	return parseSubtitlesFromPlayerHTML(trimmed)
}

// PruneTortugaTrailers прибирає з дерева вузли з трейлерами.
//
// Навіщо, якщо TortugaStreamsFromPlaylist і так їх фільтрує: трейлер у
// нашій фікстурі стоїть на позиції «Серія 3» усередині сезону. За
// гальним обходом (playlist_tree.go) він отримує номер 3 — і «Серія 3»,
// яка йде за ним, відкидається як дублікат (dub, season, episode).
// В результаті season 1 має 3 епізоди замість 4, і клієнт фізично не
// може вибрати третю серію.
//
// Вузли оголошують номери в title («Серія 3»), а не позицією, тому
// вирізання трейлера НЕ змінює нумерацію решти — саме тому це
// безпечніше за фільтрацію за позицією.
//
// Експортовано, бо це частина контракту: бачити сезони без трейлерів
// мусить і GetDetails, а не лише GetStreams.
func PruneTortugaTrailers(items []playerJSPlaylistItem) []playerJSPlaylistItem {
	out := make([]playerJSPlaylistItem, 0, len(items))
	for _, node := range items {
		file := rawPlaylistFile(node.File)
		if file != "" {
			_, mediaURL, _ := ParseTortugaFileField(file)
			if IsTrailerURL(mediaURL) {
				continue
			}
		}
		if len(node.Folder) > 0 {
			node.Folder = PruneTortugaTrailers(node.Folder)
		}
		out = append(out, node)
	}
	return out
}

// TortugaExtractor розбирає сторінки виду
//
//	https://tortuga.tw/embed/{id}
//	https://tortuga.tw/vod/{id}
//
// Обидва шляхи віддають той самий плеєр із зашифрованим file:, тому
// окремого обробника для vod не потрібно.
type TortugaExtractor struct {
	client *TLSClient
}

// NewTortugaExtractor створює екстрактор поверх наявного HTTP-клієнта.
func NewTortugaExtractor(client *TLSClient) *TortugaExtractor {
	return &TortugaExtractor{client: client}
}

// TortugaResult — все, що потрібно і GetDetails, і GetStreams, в одній
// структурі: дерево для деталей і готові потоки для відтворення.
type TortugaResult struct {
	Seasons    []domain.Season
	Voiceovers []domain.Voiceover
	Streams    []domain.StreamSource
	Subtitles  []domain.SubtitleSource

	// PageURL — адреса сторінки ПРОВАЙДЕРА, з якої ми прийшли.
	//
	// Не embed-сторінка Tortuga, а сторінка uaserials: саме її
	// клієнт надсилає назад у запиті за потоком. Embed-сторінка
	// одноразова й не є ідентифікатором елемента.
	//
	// Без цього поля ref кожної серії виходив компактним
	// («v:1:2:1+1») без адреси, і GetStreams не знав, що
	// завантажувати: падав «invalid URL scheme: [v]».
	PageURL string
}

// FetchTortugaEmbed завантажує embed-сторінку та повертає сезони,
// озвучки й потоки.
//
// Потоки повертаються «усі разом» (без фільтра за серією) — саме так
// їх хоче бачити GetDetails: клієнт сам віддає потрібний таб. Точечний
// запит робить FetchTortugaEpisode.
func (e *TortugaExtractor) FetchTortugaEmbed(ctx context.Context, rawURL, pageURL string) (TortugaResult, error) {
	items, playerURL, err := e.fetchPlaylist(ctx, rawURL)
	if err != nil {
		return TortugaResult{}, err
	}

	// Дерево для потоків і для сезонів — одне й те саме, вже без
	// трейлерів: інакше трейлер займає номер реальної серії (див.
	// PruneTortugaTrailers).
	items = PruneTortugaTrailers(items)

	streams, subs := TortugaStreamsFromPlaylist(items, playerURL, tortugaPlayerLabel, 0, 0, "")

	return TortugaResult{
		Seasons:    BuildSeasonsForItem(items, pageURL),
		Voiceovers: BuildVoiceoversForItem(items, pageURL),
		Streams:    streams,
		Subtitles:  subs,
		PageURL:    pageURL,
	}, nil
}

// FetchTortugaEpisode повертає потоки конкретної серії в конкретній
// озвучці — це те, що потрібно GetStreams.
//
// Важливо: сезон і серія НІКОЛИ не додаються до URL. Tortuga (як і
// Ashdi) у відповідь на такий запит віддає замість дерева одну серію, і
// клієнт втрачає можливість перемикати озвучку та серії.
func (e *TortugaExtractor) FetchTortugaEpisode(ctx context.Context, rawURL string, season, episode int, voiceID string) ([]domain.StreamSource, []domain.SubtitleSource, error) {
	items, playerURL, err := e.fetchPlaylist(ctx, rawURL)
	if err != nil {
		return nil, nil, err
	}
	items = PruneTortugaTrailers(items)
	streams, subs := TortugaStreamsFromPlaylist(items, playerURL, tortugaPlayerLabel, season, episode, voiceID)
	if len(streams) == 0 && (season > 0 || episode > 0 || voiceID != "") {
		// Фолбек, як у SelectPlaylistStream: один невірно розпізнаний
		// номер не повинен робити деталь порожньою.
		streams, subs = TortugaStreamsFromPlaylist(items, playerURL, tortugaPlayerLabel, 0, 0, "")
	}

	// Розкриваємо якість із master-плейлиста.
	//
	// Робочо робимо саме тут, а не у FetchTortugaEmbed: деталі
	// віддаємо десятками серій, і один запит на кожну перетворив би
	// відкриття картки на 155 HTTP-запитів. GetStreams викликається
	// для однієї серії — це рівно один додатковий запит.
	streams = e.expandQualities(ctx, streams)
	return streams, subs, nil
}

// expandQualities замінює кожен master-потік на перелік джерел по
// якостях, якщо master оголошує варіанти.
//
// Джерело, яке не вдалося розкрити, лишається як є: краще
// «Auto», ніж вигадана якість.
func (e *TortugaExtractor) expandQualities(ctx context.Context, streams []domain.StreamSource) []domain.StreamSource {
	if e == nil || e.client == nil || len(streams) == 0 {
		return streams
	}

	out := make([]domain.StreamSource, 0, len(streams)*2)
	changed := false

	for _, s := range streams {
		expanded := e.expandOneQuality(ctx, s)
		if len(expanded) > 0 {
			changed = true
			out = append(out, expanded...)
			continue
		}
		out = append(out, s)
	}

	if !changed {
		return streams
	}
	SortStreamsByDubWeight(out)
	return out
}

// expandOneQuality завантажує один master і розкриває його.
// nil означає «розкрити не вдалося» — викликач лишає джерело з Auto.
func (e *TortugaExtractor) expandOneQuality(ctx context.Context, src domain.StreamSource) []domain.StreamSource {
	if src.URL == "" || !isHLSManifestURL(src.URL) {
		return nil
	}

	body, err := e.client.GetNoCache(ctx, src.URL, src.Headers["Referer"])
	if err != nil {
		return nil
	}

	variants, ok := ParseMasterPlaylist(body, src.URL)
	if !ok {
		return nil
	}
	return ExpandMasterStream(src, variants)
}

// isHLSManifestURL відсікає те, що точно не є HLS-плейлистом, щоб
// не робити зайвий запит на mp4 або картинку.
func isHLSManifestURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, ".m3u8") || strings.Contains(lower, ".m3u")
}

// tortugaPlayerLabel — назва вкладки в переліку потоків.
const tortugaPlayerLabel = "Tortuga"

// fetchPlaylist завантажує сторінку, дістає значення file: і
// розшифровує його. Другим значенням повертає нормалізовану сторінку:
// вона потрібна для Referer/Origin у StreamSource.
func (e *TortugaExtractor) fetchPlaylist(ctx context.Context, rawURL string) ([]playerJSPlaylistItem, string, error) {
	target, err := sanitizeTortugaURL(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("tortuga: bad player url: %w", err)
	}
	if e == nil || e.client == nil {
		return nil, "", errors.New("tortuga: extractor without http client")
	}

	// Referer на саму себе: Tortuga не перевіряє, але деякі сторінки
	// віддають порожній плеєр без Referer.
	page, err := e.client.GetNoCache(ctx, target, target)
	if err != nil {
		return nil, "", fmt.Errorf("tortuga: fetch %s: %w", target, err)
	}

	rawFile, found := ExtractPlaylistFromPlayerHTML(page)
	if !found || strings.TrimSpace(rawFile) == "" {
		return nil, "", fmt.Errorf("%w: no file: value on %s", ErrTortugaUndecodable, target)
	}

	items, err := DecodeTortugaPlaylist(rawFile)
	if err == nil {
		return items, target, nil
	}

	// Фолбек: не всі сторінки Tortuga шифрують file:. Старі
	// кінематографічні сторінки віддають звичайний JSON-масив, і
	// знецінювати їх через помилку розшифровки не можна.
	if items, ok := ParsePlaylistJSON(rawFile); ok {
		return items, target, nil
	}
	return nil, "", fmt.Errorf("tortuga: %s: %w", target, err)
}

// TortugaStreamsFromPlaylist перетворює дерево на готові StreamSource.
//
// Чому окрема функція, а не виклик SelectPlaylistStream: той крутить
// значення file: через parseMultiQualityString, а isPlayableMediaURL
// відхиляє рядок з фігурними дужками ({1+1}https://…) — тобто для
// Tortuga він не дав би ЖОДНОГО потоку, ані для фільмів, ані для
// серій. Розбір формату file: мусить бути ДО перевірки URL.
//
// season/episode/voiceID фільтрують лише в пам'яті; у сам URL вони
// ніколи не потрапляють (див. FetchTortugaEpisode).
func TortugaStreamsFromPlaylist(items []playerJSPlaylistItem, playerURL, playerLabel string, season, episode int, voiceID string) ([]domain.StreamSource, []domain.SubtitleSource) {
	if len(items) == 0 {
		return nil, nil
	}

	var streams []domain.StreamSource
	var subs []domain.SubtitleSource
	seen := make(map[string]bool)

	ForEachPlaylistLeaf(items, func(l PlaylistLeaf) {
		if season > 0 && l.Ctx.Season > 0 && l.Ctx.Season != season {
			return
		}
		if episode > 0 && l.Ctx.Episode > 0 && l.Ctx.Episode != episode {
			return
		}
		if voiceID != "" && l.Ctx.Dub != "" && !strings.EqualFold(l.Ctx.Dub, voiceID) {
			return
		}

		dub, mediaURL, fileSub := ParseTortugaFileField(l.File)
		if dub == "" {
			dub = l.Ctx.Dub
		}

		// Трейлери — не епізоди. На живому трафіку вони приходять
		// тим самим розшифрованим деревом, і без фільтра користувач
		// отримує «Серія 7» у вигляді рекламного ролика.
		if mediaURL == "" || IsTrailerURL(mediaURL) {
			return
		}

		cleanURL, err := sanitizeTortugaURL(mediaURL)
		if err != nil || !isPlayableMediaURL(cleanURL) {
			return
		}
		if seen[cleanURL] {
			return
		}
		seen[cleanURL] = true

		_, dubName, language := ResolveVoiceoverNameNormalised(dub)
		src := newStreamSource(cleanURL, playerURL)

		// Quality НЕ дорівнює назві озвучки.
		//
		// Раніше тут стояло src.Quality = playlistLabel(dubName, ...),
		// і клієнт показував «1+1 (Tortuga)» у колонці «Якість» —
		// тобто озвучка в полі якості. Озвучка вже живе у
		// src.Voiceover, а якість тут Auto: ми віддаємо master-
		// playlist як є, і варіантів усередині три (1080/720/480).
		// Розкладати їх у окремі джерела не можна — на Ashdi шлях
		// /hls/1080/ дає 404, перевірено.
		src.Quality = defaultStreamQuality

		src.Player = detectPlayerBalancer(playerURL)
		if src.Player == "" {
			src.Player = playerLabel
		}
		src.Voiceover = dubName
		src.Language = language
		streams = append(streams, src)

		if l.Subtitle != "" {
			subs = append(subs, parseTortugaSubtitles(l.Subtitle)...)
		}
		if fileSub != "" {
			subs = append(subs, parseTortugaSubtitles(fileSub)...)
		}
	})

	SortStreamsByDubWeight(streams)
	return streams, subs
}

// tortugaEpisodeCount рахує серії в розшифрованому дереві — використовується
// в діагностиці та тестах масштабу (155 серій на реальному «Величне століття»).
func tortugaEpisodeCount(items []playerJSPlaylistItem) int {
	count := 0
	ForEachPlaylistLeaf(items, func(PlaylistLeaf) { count++ })
	return count
}

// tortugaDescribe — короткий опис дерева для логів: «4 сезони / 155 серій».
func tortugaDescribe(items []playerJSPlaylistItem) string {
	seasons := BuildSeasonsFromPlaylist(items)
	parts := make([]string, 0, len(seasons))
	for _, s := range seasons {
		parts = append(parts, "S"+strconv.Itoa(s.Number)+"("+strconv.Itoa(len(s.Episodes))+")")
	}
	return "seasons=" + strconv.Itoa(len(seasons)) + " [" + strings.Join(parts, " ") + "] episodes=" + strconv.Itoa(tortugaEpisodeCount(items))
}
