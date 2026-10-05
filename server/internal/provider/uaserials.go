package provider

// Провайдер uaserials.com — DLE-сайт з українськими серіалами.
//
// Головна особливість цього сайту (на відміну від uakino/eneyida/lavakino):
// вкладки плеєра НЕ лежать у HTML відкритим текстом. Сторінка опису містить
// лише елемент
//
//	<player-control data-default='Плеєр'
//	    data-tag1='{"ciphertext":"…","iv":"…","salt":"…"}'>
//
// а сам розшифрований список вкладок приходить клієнту з бандлу
// templates/common/js/e7443126.min.js, де ключ передається у CryptoJSAesDecrypt:
// PBKDF2-SHA512, 999 ітерацій, 32-байтний ключ, AES-256-CBC.
//
// Повний цикл, перевірений на живому сайті 2026-10-05 на прикладі
// «Величне століття. Роксолана» (id 422):
//
//  1. data-tag1 → [{"tabName":"Плеєр","url":"https://tortuga.tw/embed/98"},
//     {"tabName":"Трейлер","url":"https://tortuga.tw/vod/5008"}]
//  2. tortuga.tw/embed/98 → зашифроване значення file: → дерево PlayerJS
//     у форматі «{1+1}https://calypso.tortuga.tw/hls/…/index.m3u8(subtitle:)»
//  3. m3u8 віддається клієнту напряму (медіатрафік НЕ йде через сервер).
//
// Дві речі, які тут коштували реального часу налагодження:
//
//   - «Трейлер» у розшифрованому списку має URL https://tortuga.tw/vod/5008,
//     у якому немає жодного слова «trailer». Фільтр лише за IsTrailerURL не
//     спрацьовує і користувач отримує рекламний ролик замість серії. Тому
//     фільтруємо і за tabName, і за URL.
//   - У картки каталогу .th-title — це <div>, а не <a>. Пошук «.th-title a»
//     (як у першій версії цього провайдера) повертає нуль карток навіть
//     тоді, коли сторінка їх містить. Посилання на картці — це a.short-img.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

const (
	// uaserialsDefaultBaseURL — кореневий домен сайту.
	uaserialsDefaultBaseURL = "https://uaserials.com"

	// uaserialsDefaultKey — пароль для data-tag.
	//
	// Це не секрет і не злой намір: значення лежить у клієнтському бандлі
	// й користувач бачить його в devtools. Воно існує лише для того, щоб
	// рекламу на сайті не можна було віддавати без першої сторінки.
	// Якщо його змінять, resolveUaserialsTabs перечитує ключ із бандлу
	// (uaserialsKeyBundlePath) і оновлює кеш.
	uaserialsDefaultKey = "297796CCB81D255125"

	// uaserialsKeyBundlePath — бандл, у якому лежить ключ. Хеш у імені
	// файлу змінюється при кожному релізі, тому шлях — лише орієнтир:
	// якщо регулярка нічого не знайшла, ми просто лишаємось на кешованому
	// ключі й повертаємо зрозумілу помилку.
	uaserialsKeyBundlePath = "/templates/uaserials2020/js/fe68ee32.min.js"

	// uaserialsPBKDF2Iterations — iterations: 0x3e7 у CryptoJS, тобто 999.
	uaserialsPBKDF2Iterations = 999

	// uaserialsKeyLen — keySize: 0x8 CryptoJS, тобто 8 слів по 4 байти.
	uaserialsKeyLen = 32

	// uaserialsPlayerLabel — назва вкладки в переліку потоків, коли
	// плеєр не впізнаний за хостом.
	uaserialsPlayerLabel = "Uaserials"
)

// Помилки провайдера. Обидві реальні (з underlying-причиною), а не
// «щось пішло не так»: реєстр і клієнт показують їх у діагностиці.
var (
	// ErrUaserialsNoTags — на сторінці немає жодного data-tag*.
	ErrUaserialsNoTags = errors.New("uaserials: page exposes no encrypted player tags")

	// ErrUaserialsDecrypt — теги є, але жоден не розшифрувався.
	ErrUaserialsDecrypt = errors.New("uaserials: cannot decrypt player tag")
)

// compile-time перевірка контракту.
var _ domain.Provider = (*UaserialsProvider)(nil)

// UaserialsProvider реалізує domain.Provider для uaserials.com.
type UaserialsProvider struct {
	client  *TLSClient
	baseURL string

	// keyMu захищає кеш ключа розшифровки. Одночасних розшифровок
	// буде багато (по одній на запит потоків), а блокування на випадку
	// зміни ключа не потрібне.
	keyMu sync.RWMutex
	key   string
	// probedKey — ключ, який ми вже пробули дістати з бандлу. Щоб не
	// смикати бандл на кожну невдалу розшифровку.
	probedKey string
}

// NewUaserialsProvider створює провайдера з клієнтом за замовчуванням.
func NewUaserialsProvider(client *TLSClient) *UaserialsProvider {
	return NewUaserialsProviderWithConfig("", client)
}

// NewUaserialsProviderWithConfig дозволяє підмінити базовий домен (тести,
// дзеркала). Порожній рядок означає uaserials.com.
func NewUaserialsProviderWithConfig(baseURL string, client *TLSClient) *UaserialsProvider {
	if baseURL == "" {
		baseURL = uaserialsDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if client == nil {
		client, _ = NewTLSClient()
	}
	return &UaserialsProvider{client: client, baseURL: baseURL, key: uaserialsDefaultKey}
}

func (p *UaserialsProvider) ID() string { return "uaserials" }

func (p *UaserialsProvider) Name() string { return "UA Serials" }

func (p *UaserialsProvider) BaseURL() string { return p.baseURL }

// Describe повертає публічні метадані для каталогу провайдерів.
func (p *UaserialsProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID:                   p.ID(),
		Name:                 p.Name(),
		BaseURL:              p.baseURL,
		ShowOnHome:           true,
		HasFixedStreams:      false,
		ContentTypes:         []string{"movie", "series", "cartoon", "anime"},
		SearchEnabledDefault: true,
	}
}

// ---- ключ розшифровки ----

// keyValue повертає кешований ключ.
func (p *UaserialsProvider) keyValue() string {
	p.keyMu.RLock()
	defer p.keyMu.RUnlock()
	return p.key
}

// rememberKey оновлює кеш ключа.
func (p *UaserialsProvider) rememberKey(k string) {
	if k == "" {
		return
	}
	p.keyMu.Lock()
	p.key = k
	p.keyMu.Unlock()
}

// reUaserialsBundleKey дістає ключ із бандлу, якщо той не обфусцований.
//
// Сирий regex, без webcrack: обфускація тут мінімальна і лише тоді, коли
// розробники її ввімкнули. На знімку 2026-10-05 ключ був складений
// обфускованим виразом (`var dd=_0x4bfc33(0x13f)+_0x4bfc33(0x185)+'25'`),
// і ця регулярка не збігалася — тому у разі провалу ми не ламаємо
// результат, а повертаємо помилку про розшифровку з реальною причиною.
var reUaserialsBundleKey = regexp.MustCompile(`var\s+dd\s*=\s*["']([^"']+)["']`)

// uaserialsKeyFromBundle шукає пароль data-tag у вмісті бандлу.
func uaserialsKeyFromBundle(js string) string {
	if m := reUaserialsBundleKey.FindStringSubmatch(js); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// refreshKeyFromBundle перечитує ключ із бандлу, якщо той ще не пробили.
//
// Повертає ключ, з яким варто повторити розшифровку. Порожній рядок
// означає «нічого нового немає, не смикай бандл знову».
func (p *UaserialsProvider) refreshKeyFromBundle(ctx context.Context) string {
	p.keyMu.RLock()
	current, probed := p.key, p.probedKey
	p.keyMu.RUnlock()

	if probed != "" && probed == current {
		return ""
	}
	if p.client == nil {
		return ""
	}

	js, err := p.client.Get(ctx, p.baseURL+uaserialsKeyBundlePath, p.baseURL+"/")
	if err != nil {
		// Не Fatal: можливо, ключ просто треба оновити вручну. Помилку
		// про це додасть той, хто не зміг розшифрувати.
		p.keyMu.Lock()
		p.probedKey = current
		p.keyMu.Unlock()
		return ""
	}

	if key := uaserialsKeyFromBundle(js); key != "" && key != current {
		p.rememberKey(key)
		return key
	}

	p.keyMu.Lock()
	p.probedKey = current
	p.keyMu.Unlock()
	return ""
}

// ---- розшифровка data-tag ----

// uaserialsTag — формат值 атрибута data-tag.
type uaserialsTag struct {
	Ciphertext string `json:"ciphertext"`
	IV         string `json:"iv"`
	Salt       string `json:"salt"`
}

// uaserialsTab — один розшифрований елемент списку вкладок.
type uaserialsTab struct {
	TabName string `json:"tabName"`
	URL     string `json:"url"`
}

// reUaserialsTagAttr — резервний шлях до data-tag, коли розмітка
// зламана і goquery не бачить елемент (наприклад, сторінка обрізана).
var reUaserialsTagAttr = regexp.MustCompile(`data-tag\d*\s*=\s*(?:'([^']*)'|"([^"]*)")`)

// extractUaserialsTags повертає всі значення атрибутів data-tag* зі сторінки.
//
// Основний шлях — goquery по <player-control>: його атрибути гарантовано
// розекрановані HTML-парсером (у JSON лежать base64-рядки з «/» та «+»,
// які ламають наївні регулярки).
func extractUaserialsTags(html string) []string {
	var out []string
	seen := make(map[string]bool)

	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			return
		}
		seen[raw] = true
		out = append(out, raw)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err == nil {
		doc.Find("player-control").Each(func(_ int, s *goquery.Selection) {
			for _, node := range s.Nodes {
				for _, attr := range node.Attr {
					if strings.HasPrefix(attr.Key, "data-tag") {
						add(attr.Val)
					}
				}
			}
		})
	}

	if len(out) > 0 {
		return out
	}

	for _, m := range reUaserialsTagAttr.FindAllStringSubmatch(html, -1) {
		if len(m) < 3 {
			continue
		}
		if m[1] != "" {
			add(m[1])
		} else {
			add(m[2])
		}
	}
	return out
}

// stripPKCS7 знімає стандартне вирівнювання PKCS#7.
//
// Обов'язково: CryptoJS знімає padding автоматично, а Go — ні. Без цієї
// перевірки ми б отримали JSON із 10 байтами \x0a у кінці, який json.Unmarshal
// відхилив би, і ми б вирішили, що ключ неправильний.
//
// Перевірка навмисно сувора: останній байт дорівнює довжині хвоста, і всі
// байти хвоста дорівнюють йому. Саме це відрізняє «правильний ключ» від
// «випадково вгаданого padding при неправильному ключі».
func stripPKCS7(b []byte) ([]byte, error) {
	n := int(b[len(b)-1])
	if n == 0 || n > len(b) {
		return nil, fmt.Errorf("invalid padding length %d for %d-byte plaintext", n, len(b))
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, errors.New("padding bytes are inconsistent")
		}
	}
	return b[:len(b)-n], nil
}

// decryptUaserialsTag розшифровує один data-tag і повертає відкритий JSON.
func decryptUaserialsTag(passphrase, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: empty tag", ErrUaserialsDecrypt)
	}
	if passphrase == "" {
		return "", fmt.Errorf("%w: empty passphrase", ErrUaserialsDecrypt)
	}

	var tag uaserialsTag
	if err := json.Unmarshal([]byte(raw), &tag); err != nil {
		return "", fmt.Errorf("%w: tag is not json: %v", ErrUaserialsDecrypt, err)
	}
	if tag.Ciphertext == "" || tag.IV == "" || tag.Salt == "" {
		return "", fmt.Errorf("%w: tag misses ciphertext/iv/salt", ErrUaserialsDecrypt)
	}

	salt, err := hex.DecodeString(tag.Salt)
	if err != nil {
		return "", fmt.Errorf("%w: salt is not hex: %v", ErrUaserialsDecrypt, err)
	}
	iv, err := hex.DecodeString(tag.IV)
	if err != nil {
		return "", fmt.Errorf("%w: iv is not hex: %v", ErrUaserialsDecrypt, err)
	}
	ciphertext, err := decodeBase64Loose(tag.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: ciphertext is not base64: %v", ErrUaserialsDecrypt, err)
	}
	ct := []byte(ciphertext)

	block, err := aes.NewCipher(mustPBKDF2(passphrase, salt))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUaserialsDecrypt, err)
	}
	if len(iv) != block.BlockSize() {
		return "", fmt.Errorf("%w: iv is %d bytes, want %d", ErrUaserialsDecrypt, len(iv), block.BlockSize())
	}
	if len(ct) == 0 || len(ct)%block.BlockSize() != 0 {
		return "", fmt.Errorf("%w: ciphertext is %d bytes, not a multiple of the block size",
			ErrUaserialsDecrypt, len(ct))
	}

	plain := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ct)

	unpadded, err := stripPKCS7(plain)
	if err != nil {
		return "", fmt.Errorf("%w: %v (key mismatch is the usual cause)", ErrUaserialsDecrypt, err)
	}
	if !utf8.Valid(unpadded) {
		return "", fmt.Errorf("%w: plaintext is not valid utf-8 (key mismatch is the usual cause)",
			ErrUaserialsDecrypt)
	}
	return string(unpadded), nil
}

// mustPBKDF2 виводить ключ AES. Стандартний crypto/pbkdf2 повертає
// помилку лише для нерозбірливої пари алгоритм/довжина, тобто ніколи
// для наших констант.
func mustPBKDF2(passphrase string, salt []byte) []byte {
	key, err := pbkdf2.Key(sha512.New, passphrase, salt, uaserialsPBKDF2Iterations, uaserialsKeyLen)
	if err != nil {
		// Непомітно: далі aes.NewCipher все одно відхилить ключ неправильної
		// довжини й ми повернемо помилку з конкретною причиною.
		return nil
	}
	return key
}

// parseUaserialsTabs розбирає розшифрований список вкладок.
func parseUaserialsTabs(plain string) ([]uaserialsTab, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return nil, fmt.Errorf("%w: decrypted payload is empty", ErrUaserialsDecrypt)
	}
	var tabs []uaserialsTab
	if err := json.Unmarshal([]byte(plain), &tabs); err != nil {
		return nil, fmt.Errorf("%w: payload is not a tab array: %v", ErrUaserialsDecrypt, err)
	}
	kept := tabs[:0]
	for _, t := range tabs {
		if strings.TrimSpace(t.URL) != "" {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("%w: payload has no tab with a url", ErrUaserialsDecrypt)
	}
	return kept, nil
}

// resolveUaserialsTabs розшифровує всі data-tag сторінки й склеює вкладки.
func (p *UaserialsProvider) resolveUaserialsTabs(ctx context.Context, html string) ([]uaserialsTab, error) {
	raw := extractUaserialsTags(html)
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: page has no player-control", ErrUaserialsNoTags)
	}

	var (
		all       []uaserialsTab
		lastErr   error
		decrypted int
	)
	for _, one := range raw {
		plain, err := decryptUaserialsTag(p.keyValue(), one)
		if err != nil {
			// Можливо, змінився пароль. Пробуємо дістати свіжий з бандлу.
			if alt := p.refreshKeyFromBundle(ctx); alt != "" {
				plain, err = decryptUaserialsTag(alt, one)
			}
		}
		if err != nil {
			lastErr = err
			continue
		}
		tabs, err := parseUaserialsTabs(plain)
		if err != nil {
			lastErr = err
			continue
		}
		decrypted++
		all = append(all, tabs...)
	}

	if decrypted == 0 {
		return nil, fmt.Errorf("%w: %d tag(s), last error: %v", ErrUaserialsDecrypt, len(raw), lastErr)
	}
	return all, nil
}

// ---- відбір вкладок ----

// reUaserialsTrailerTab ловить вкладку-трейлер навіть тоді, коли в URL
// немає слова «trailer» (а на uaserials.com так і є:
// «Трейлер» → https://tortuga.tw/vod/5008).
var reUaserialsTrailerTab = regexp.MustCompile(`(?i)трейлер|trailer|teaser`)

// isSelfHostedURL повідомляє, чи URL веде на сам сайт uaserials.com.
//
// Такі посилання в розшифрованому списку — це не плеєри, а звичайні
// внутрішні переходи («Дивитися всі серії», «Реєстрація»). Віддавати їх
// резолверу безглуздо: це HTML-сторінка, яку mpv не відтворить.
func isSelfHostedURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "uaserials.com" || strings.HasSuffix(host, ".uaserials.com")
}

// isUaserialsPlayerURL перевіряє, що вкладку взагалі варто віддавати
// резолверу: абсолютний http(s) URL і не наш власний домен.
func isUaserialsPlayerURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, "<>\"' \t\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" || isSelfHostedURL(raw) {
		return false
	}
	return true
}

// selectUaserialsPlayers ділить вкладки на «плеєри» та «трейлери».
//
// trailerUrl повертається окремо, щоб GetDetails міг показати його як
// TrailerURL, а не втратити разом із гравцями.
func selectUaserialsPlayers(tabs []uaserialsTab) (players []uaserialsTab, trailerURL string) {
	for _, t := range tabs {
		target := strings.TrimSpace(t.URL)
		if target == "" {
			continue
		}
		isTrailer := reUaserialsTrailerTab.MatchString(t.TabName) || IsTrailerURL(target)
		if !isTrailer && !isUaserialsPlayerURL(target) {
			continue
		}
		if isTrailer {
			if trailerURL == "" && isUaserialsPlayerURL(target) {
				trailerURL = target
			}
			continue
		}
		players = append(players, t)
	}
	return players, trailerURL
}

// ---- каталог ----

// uaserialsSection віддає шлях розділу каталогу для типу контенту.
//
// Перевірено живими запитами 2026-10-05: /films/, /series/, /anime/,
// /cartoons/ — усі 200. /multfilmy/ — 404 (розділ перейменовано),
// тому «multfilmy» з ТЗ не використовуємо.
func uaserialsSection(contentType string) string {
	switch contentType {
	case "movie":
		return "films"
	case "series":
		return "series"
	case "cartoon":
		return "cartoons"
	case "anime":
		return "anime"
	default:
		return ""
	}
}

// absolutize перетворює відносний шлях на абсолютний URL.
func (p *UaserialsProvider) absolutize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return p.baseURL + "/" + strings.TrimLeft(raw, "/")
}

// reUaserialsPostID дістає числовий id посту з URL виду /12753-vbyty-dzheki.html.
var reUaserialsPostID = regexp.MustCompile(`(?i)^https?://[^/]+/(\d+)[-.]`)

// uaserialsPostID повертає id посту або "".
func uaserialsPostID(rawURL string) string {
	if m := reUaserialsPostID.FindStringSubmatch(strings.TrimSpace(rawURL)); m != nil {
		return m[1]
	}
	return ""
}

// Search шукає через POST-форму DLE.
//
// Важлива деталь, знайдена на живому сайті: сторінка результатів
// (/search/<запит>/) НЕ використовує картки .short-item. Там інша розмітка —
// <a class="uas-card">. Тому парсер має знати обидва варіанти, інакше пошук
// мовчки повертає нуль карток.
func (p *UaserialsProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		// Не помилка і не порожній nil: клієнт просто нічого не ввів.
		return []domain.MediaItem{}, nil
	}

	form := "do=search&subaction=search&story=" + url.QueryEscape(query)
	html, err := p.client.PostForm(ctx, p.baseURL+"/index.php?do=search", form, p.baseURL+"/")
	if err != nil {
		return nil, fmt.Errorf("uaserials search request: %w", err)
	}

	items := p.parseUaserialsCards(html, "")
	if len(items) == 0 {
		return items, nil
	}
	p.applySearchTypes(ctx, query, items)
	return items, nil
}

// searchPath будує канонічний шлях сторінки пошуку.
//
// DLE очікує percent-encoded запит у шляху. QueryEscape дав би «+» замість
// пробілу, тож міняємо на %20.
func (p *UaserialsProvider) searchPath(query string) string {
	return p.baseURL + "/search/" + strings.ReplaceAll(url.PathEscape(query), "+", "%20") + "/"
}

// uaserialsSearchCats — вкладки фільтрів на сторінці результатів.
//
// На live-сайті картки не мають ознаки типу (на відміну від карток
// каталогу, де є .short-label-level-1 «1 сезон»). Єдине джерело типу —
// самі вкладки «Серіали» / «Мультсеріали», які повертають відфільтровану
// добірку. Тому ми робимо два додаткові запити (паралельно) і звіряємо id.
var uaserialsSearchCats = []struct{ cat, mediaType string }{
	{"serial", "series"},
	{"cartoonserial", "cartoon"},
}

// applySearchTypes проставляє тип у картки пошуку за вкладками категорій.
//
// Найкраще-effort: будь-яка помилка тут НЕ повартає пошук — картки лишаться
// типу movie, що краще, ніж порожній результат.
func (p *UaserialsProvider) applySearchTypes(ctx context.Context, query string, items []domain.MediaItem) {
	if len(items) == 0 || p.client == nil {
		return
	}
	base := p.searchPath(query)
	sets := make([]map[string]bool, len(uaserialsSearchCats))

	var wg sync.WaitGroup
	for i, c := range uaserialsSearchCats {
		wg.Add(1)
		go func(idx int, cat string) {
			defer wg.Done()
			html, err := p.client.Get(ctx, base+"?cat="+cat, p.baseURL+"/")
			if err != nil {
				return
			}
			sets[idx] = uaserialsCollectPostIDs(html)
		}(i, c.cat)
	}
	wg.Wait()

	for i, c := range uaserialsSearchCats {
		for j := range items {
			id := uaserialsPostID(items[j].URL)
			if id != "" && sets[i][id] {
				items[j].Type = c.mediaType
			}
		}
	}
}

// uaserialsCollectPostIDs збирає id карток (не людей!) зі сторінки.
func uaserialsCollectPostIDs(html string) map[string]bool {
	out := make(map[string]bool)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return out
	}
	doc.Find("a.uas-card[data-uas-id]").Each(func(_ int, s *goquery.Selection) {
		if id, ok := s.Attr("data-uas-id"); ok && id != "" {
			out[id] = true
		}
	})
	return out
}

// uaserialsSectionMediaType перекладає шлях розділу (/films/, /cartoons/…)
// у тип матеріалу з domain.MediaItem.
func uaserialsSectionMediaType(section string) string {
	switch section {
	case "films":
		return "movie"
	case "cartoons", "fcartoon":
		return "cartoon"
	case "anime":
		return "anime"
	case "series":
		return "series"
	default:
		return ""
	}
}

// uaserialsCardType визначає тип картки.
//
// Пріоритет — підказка розділу (вона точна: /anime/ — це anime), далі
// мітка «1 сезон» на картці каталогу, далі сегмент /series/ в URL.
func uaserialsCardType(sectionHint, href, seasonLabel string) string {
	if mt := uaserialsSectionMediaType(sectionHint); mt != "" && mt != "series" {
		return mt
	}
	if strings.TrimSpace(seasonLabel) != "" || strings.Contains(strings.ToLower(href), "/series/") {
		return "series"
	}
	return "movie"
}

// reUaserialsSectionLink ловить посилання на розділ каталогу — у деталях
// це єдине місце, де видно, серіал це чи фільм: URL самих сторінок
// (/422-velychne-stolittya-roksolana-2011s.html) жодного типу не містить.
var reUaserialsSectionLink = regexp.MustCompile(`(?i)^https?://[^/]+/(films|series|anime|cartoons|fcartoon)/`)

// uaserialsDetailsType визначає тип матеріалу зі сторінки опису.
func uaserialsDetailsType(doc *goquery.Document) string {
	mediaType := ""
	doc.Find("ul.short-list a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		m := reUaserialsSectionLink.FindStringSubmatch(strings.TrimSpace(href))
		if m == nil {
			return true
		}
		mediaType = uaserialsSectionMediaType(strings.ToLower(m[1]))
		return false
	})
	if mediaType != "" {
		return mediaType
	}
	return "movie"
}

// parseUaserialsCards дістає картки з будь-якої сторінки каталогу.
//
// sectionHint — «films»/«series»/«anime»/«cartoons» для сторінок розділів
// або "" для пошуку, де розділ невідомий.
func (p *UaserialsProvider) parseUaserialsCards(html, sectionHint string) []domain.MediaItem {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return []domain.MediaItem{}
	}

	// Ініціалізовано не-nil, щоб порожній каталог серіалізувався як [],
	// а не null (json.Marshal перетворює nil на null, і клієнт потім
	// змушений окремо це обробляти).
	items := []domain.MediaItem{}

	// 1. Картки розділів: .short-item.
	doc.Find(".short-item").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Find("a.short-img, a.short-item-link").First().Attr("href")
		if !ok || strings.TrimSpace(href) == "" {
			return
		}
		href = strings.TrimSpace(href)

		// .th-title — це <div>, а не <a>: посилання треба брати з
		// a.short-img (перша версія провайдера шукала «.th-title a» і
		// повертала 0 карток).
		title := strings.TrimSpace(s.Find(".th-title").First().Text())
		origTitle := strings.TrimSpace(s.Find(".th-title-oname").First().Text())
		if title == "" {
			title = origTitle
		}
		if title == "" {
			return
		}

		poster := p.posterFromImage(s.Find("img").First())

		// «1 сезон», «1-3 сезон» — ознака серіалу.
		seasonLabel := strings.TrimSpace(s.Find(".short-label-level-1 span").First().Text())

		items = append(items, domain.MediaItem{
			ID:            href,
			ProviderID:    p.ID(),
			Title:         title,
			OriginalTitle: origTitle,
			PosterURL:     poster,
			Type:          uaserialsCardType(sectionHint, href, seasonLabel),
			URL:           href,
		})
	})

	if len(items) > 0 {
		return items
	}

	// 2. Картки пошуку: a.uas-card.
	doc.Find("a.uas-card[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		href = strings.TrimSpace(href)
		title := strings.TrimSpace(s.Find(".uas-card__title").First().Text())
		if href == "" || title == "" {
			return
		}
		origTitle := strings.TrimSpace(s.Find(".uas-card__orig").First().Text())
		var rating float64
		if v := strings.TrimSpace(s.Find(".uas-card__rating").First().Text()); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				rating = f
			}
		}
		items = append(items, domain.MediaItem{
			ID:            href,
			ProviderID:    p.ID(),
			Title:         title,
			OriginalTitle: origTitle,
			PosterURL:     p.posterFromImage(s.Find("img").First()),
			Year:          parseYear(s.Find(".uas-card__year").First().Text()),
			Rating:        rating,
			Type:          "movie", // уточнює applySearchTypes
			URL:           href,
		})
	})

	return items
}

// posterFromImage дістає лінк на постер.
//
// На цьому сайті картки — lazyload: src завжди /media/default.png, а
// справжній лінк лежить у data-src.
func (p *UaserialsProvider) posterFromImage(img *goquery.Selection) string {
	if img == nil || img.Length() == 0 {
		return ""
	}
	for _, attr := range []string{"data-src", "src"} {
		if v, ok := img.Attr(attr); ok {
			v = strings.TrimSpace(v)
			if v == "" || strings.Contains(v, "/media/default.png") {
				continue
			}
			return p.absolutize(v)
		}
	}
	return ""
}

// uaserialsCatalogURL збирає URL сторінки каталогу.
//
// Винесено окремо від GetPopular/GetByCategory, щоб правила побудови
// шляху можна було перевірити тестами без мережі: саме тут легко
// «додати сторінку» у неправильному місці (а DLE на таку помилку
// мовчки віддає корінь розділу — фільтрація втрачається без жодної
// помилки).
func uaserialsCatalogURL(baseURL, section, genre string, page int) string {
	if page < 1 {
		page = 1
	}
	switch {
	case section == "" && genre == "" && page == 1:
		return baseURL + "/"
	case section == "" && genre == "":
		return fmt.Sprintf("%s/page/%d/", baseURL, page)
	case genre == "" && page == 1:
		return fmt.Sprintf("%s/%s/", baseURL, section)
	case genre == "":
		return fmt.Sprintf("%s/%s/page/%d/", baseURL, section, page)
	case page == 1:
		return fmt.Sprintf("%s/%s/%s/", baseURL, section, genre)
	default:
		return fmt.Sprintf("%s/%s/%s/page/%d/", baseURL, section, genre, page)
	}
}

// GetPopular повертає популярне за розділом.
func (p *UaserialsProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	section := uaserialsSection(contentType)
	return p.fetchCatalog(ctx, uaserialsCatalogURL(p.baseURL, section, "", page), section)
}

// GetByCategory повертає список за жанром.
//
// У site's DLE жанр — це тег у шляхах: /series/drama/, /films/f-comedy/.
// Клієнт передає власний slug (ContentGenres.toSlug: drama, sci-fi,
// romance, …), а сайт використовує власний (drama, fantastic, melodrama,
// …), тому потрібен словник відповідності. Усі значення перевірені живими
// запитами; словник у uaserialsGenreSlug.
func (p *UaserialsProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	if strings.TrimSpace(category) == "" {
		return p.GetPopular(ctx, contentType, page)
	}

	slug, ok := uaserialsGenreSlug(category)
	if !ok {
		return nil, fmt.Errorf("uaserials: no site slug for category %q", category)
	}

	section := uaserialsSection(contentType)
	if section == "" {
		section = "series"
	}
	return p.fetchCatalog(ctx, uaserialsCatalogURL(p.baseURL, section, slug, page), section)
}

// fetchCatalog завантажує сторінку каталогу та розбирає картки.
func (p *UaserialsProvider) fetchCatalog(ctx context.Context, reqURL, sectionHint string) ([]domain.MediaItem, error) {
	html, err := p.client.Get(ctx, reqURL, p.baseURL+"/")
	if err != nil {
		return nil, fmt.Errorf("uaserials fetch catalog (%s): %w", reqURL, err)
	}
	// Порожня сторінка — це не помилка (DLE так віддає «нема чого»), але
	// й не тиша: клієнт покаже «нічого не знайдено» лише з правильним
	// порожнім (не nil) списком, а не з помилкою 500.
	return p.parseUaserialsCards(html, sectionHint), nil
}

// uaserialsGenreSlug перекладає slug клієнта в slug сайту.
//
// ok=false, коли відповідника немає. Тоді GetByCategory повертає помилку з
// назвою категорії, а не мовчки віддає весь розділ під виглядом фільтра —
// мовча підміна гірша за чесна помилка.
func uaserialsGenreSlug(category string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(category))
	if key == "" {
		return "", false
	}
	if slug, ok := uaserialsGenreAliases[key]; ok {
		return slug, true
	}
	if uaserialsKnownGenreSlugs[key] {
		return key, true
	}
	return "", false
}

// uaserialsGenreAliases — slug клієнта → slug uaserials.com.
//
// Ліві ключі взяті з lib/core/constants/content_constants.dart
// (ContentGenres.toSlug) — саме те клієнт передає у запиті. Праві значення
// перевірені живими запитами 2026-10-05: /series/<slug>/ віддає 200,
// неіснуючий slug — 404.
//
// Чого немає: «musical» (/series/musical/ → 404). Словник не вигадує
// значень, яких не існує на сайті.
var uaserialsGenreAliases = map[string]string{
	"action":      "action",
	"adventure":   "adventure",
	"anime":       "anime",
	"biography":   "biography",
	"comedy":      "comedy",
	"crime":       "crime",
	"documentary": "documentary",
	"drama":       "drama",
	"family":      "family",
	"fantasy":     "fantasy",
	"history":     "history",
	"horror":      "horror",
	"mystery":     "detective",
	"romance":     "melodrama",
	"sci-fi":      "fantastic",
	"sport":       "sport",
	"thriller":    "thriller",
	"war":         "war",
	"western":     "western",
	// Назви, а не slug: клієнт у деяких місцях передає саме їх
	// (MediaDetails.Genres — це те, що показано людині). Ключі — уже
	// у нижньому регістрі, бо uaserialsGenreSlug нормалізує запит.
	"бойовик":        "action",
	"пригоди":        "adventure",
	"аніме":          "anime",
	"біографія":      "biography",
	"комедія":        "comedy",
	"кримінал":       "crime",
	"документальний": "documentary",
	"драма":          "drama",
	"сімейний":       "family",
	"фентезі":        "fantasy",
	"історичний":     "history",
	"жахи":           "horror",
	"детектив":       "detective",
	"мелодрама":      "melodrama",
	"фантастика":     "fantastic",
	"спорт":          "sport",
	"трилер":         "thriller",
	"військовий":     "war",
	"вестерн":        "western",
}

// uaserialsKnownGenreSlugs — власні слоги сайту, які клієнт може передати
// як є (бо вони збігаються з його власними).
var uaserialsKnownGenreSlugs = map[string]bool{
	"detective": true,
	"fantastic": true,
	"melodrama": true,
}

// ---- деталі ----

// GetDetails розбирає сторінку опису, сезони та озвучки.
//
// Сезони й озвучки беруться з розшифрованого списку вкладок: у кожній
// повноцінній вкладці сидить Tortuga embed, а в ньому — повне дерево
// PlayerJS. Для фільмів сезонів немає, і це нормально.
func (p *UaserialsProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	ctx, cancel := context.WithTimeout(ctx, PlayerResolveTimeout)
	defer cancel()

	html, err := p.client.Get(ctx, itemURL, p.baseURL+"/")
	if err != nil {
		return nil, fmt.Errorf("uaserials get details: %w", err)
	}

	details := p.parseUaserialsDetails(html, itemURL)

	tabs, err := p.resolveUaserialsTabs(ctx, html)
	if err != nil {
		// Деталі без плеєра — корисніше, ніж помилка: клієкт покаже
		// опис, постер і жанри, а відтворення не запуститься.
		return details, nil
	}

	players, trailerURL := selectUaserialsPlayers(tabs)
	details.TrailerURL = trailerURL

	var lastErr error
	for _, pl := range players {
		res, err := p.fetchPlayerTree(ctx, pl.URL, itemURL)
		if err != nil {
			lastErr = err
			continue
		}
		if len(res.Seasons) > 0 && len(details.Seasons) == 0 {
			details.Seasons = res.Seasons
		}
		if len(res.Voiceovers) > 0 && len(details.Voiceovers) == 0 {
			details.Voiceovers = res.Voiceovers
		}
		if len(details.Seasons) > 0 {
			break
		}
	}

	if len(details.Seasons) == 0 && lastErr != nil {
		// Не помилка: сезони просто не вдалося дістати. Повертаємо те,
		// що є, і не змушуємо клієнт показувати помилку замість
		// нормальної картки фільму.
		_ = lastErr
	}
	return details, nil
}

// parseUaserialsDetails витягує метадані зі сторінки опису.
func (p *UaserialsProvider) parseUaserialsDetails(html, itemURL string) *domain.MediaDetails {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return &domain.MediaDetails{MediaItem: domain.MediaItem{
			ID:         itemURL,
			ProviderID: p.ID(),
			URL:        itemURL,
		}}
	}

	title := strings.TrimSpace(doc.Find(".short-title .oname_ua").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	origTitle := strings.TrimSpace(doc.Find(".oname").First().Text())
	poster := p.posterFromImage(doc.Find(".fimg img").First())
	if poster == "" {
		if v, ok := doc.Find(`meta[property="og:image"]`).Attr("content"); ok {
			poster = p.absolutize(v)
		}
	}
	description := strings.TrimSpace(doc.Find(".ftext.full-text").First().Text())

	var rating float64
	if v := strings.TrimSpace(doc.Find(".short-rate-imdb span").First().Text()); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			rating = f
		}
	}

	meta := parseUaserialsShortList(doc)

	details := &domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:            itemURL,
			ProviderID:    p.ID(),
			Title:         title,
			OriginalTitle: origTitle,
			PosterURL:     poster,
			Year:          meta.year,
			Rating:        rating,
			Type:          uaserialsDetailsType(doc),
			URL:           itemURL,
		},
		Description: description,
		Genres:      meta.genres,
		Countries:   meta.countries,
		Director:    meta.director,
		Actors:      meta.actors,
		Duration:    meta.duration,
	}
	return details
}

// uaserialsMeta — метадані з блоку ul.short-list.
type uaserialsMeta struct {
	genres    []string
	countries []string
	actors    []string
	director  string
	duration  string
	year      int
}

// reUaserialsSectionOnlyLink ловить перший жанр-посилання, яке веде на
// корінь розділу («Серіал» → /series/, «Фільм» → /films/). Це не жанр,
// а тип матеріалу, тому в Genres він не потрапляє.
var reUaserialsSectionOnlyLink = regexp.MustCompile(`(?i)^https?://[^/]+/(?:films|series|anime|cartoons|fcartoon)/?$`)

// parseUaserialsShortList читає ul.short-list — єдине місце, де сайт
// тримає жанри, країни, акторів, режисера, рік і тривалість.
//
// Мітки реальні (перевірено на живому сайті): «Списки», «Гасло», «Рік»,
// «Жанр», «Країна», «Телеканал», «Переклад», «Тривалість», «Режисер»,
// «Актори».
func parseUaserialsShortList(doc *goquery.Document) uaserialsMeta {
	var meta uaserialsMeta

	doc.Find("ul.short-list > li").Each(func(_ int, li *goquery.Selection) {
		label := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(li.Find("span").First().Text()), ":"))
		if label == "" {
			return
		}

		switch label {
		case "Жанр", "Жанри":
			li.Find("a").Each(func(_ int, a *goquery.Selection) {
				href, _ := a.Attr("href")
				if reUaserialsSectionOnlyLink.MatchString(strings.TrimSpace(href)) {
					return
				}
				if g := strings.TrimSpace(a.Text()); g != "" {
					meta.genres = append(meta.genres, g)
				}
			})
		case "Рік":
			if meta.year == 0 {
				meta.year = parseYear(li.Find("a").First().Text())
			}
		case "Країна":
			meta.countries = appendUniqueTexts(meta.countries, li.Find("a"))
		case "Тривалість":
			meta.duration = cleanUaserialsValue(li)
		case "Режисер":
			meta.director = cleanUaserialsValue(li)
		case "Актори":
			meta.actors = splitUaserialsList(cleanUaserialsValue(li))
		}
	})

	return meta
}

// cleanUaserialsValue повертає текст li без мітки та без технічних
// підказок (title=, «(завершений)» тощо).
func cleanUaserialsValue(li *goquery.Selection) string {
	clone := li.Clone()
	clone.Find("span.cursor-help").Remove()
	clone.Find("span").First().Remove()
	text := strings.TrimSpace(clone.Text())
	text = strings.TrimSuffix(text, ":")
	text = strings.TrimSpace(reStudioTrailingParen.ReplaceAllString(text, ""))
	return strings.Join(strings.Fields(text), " ")
}

// splitUaserialsList розбиває список імен або стран по комі.
func splitUaserialsList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// appendUniqueTexts збирає тексти посилань без дублікатів.
func appendUniqueTexts(dst []string, links *goquery.Selection) []string {
	links.Each(func(_ int, a *goquery.Selection) {
		v := strings.TrimSpace(a.Text())
		if v == "" {
			return
		}
		for _, existing := range dst {
			if existing == v {
				return
			}
		}
		dst = append(dst, v)
	})
	return dst
}

// ---- потоки ----

// GetStreams розшифровує вкладки й віддає прямі CDN-URLи.
//
// Медіатрафік НЕ йде через сервер: StreamSource.URL == DirectURL,
// RequiresProxy == false. Заголовки обов'язкові — calypso.tortuga.tw
// віддає m3u8 і без них, але клієнтський плеєр (media_kit/libmpv) має
// отримати ті самі заголовки, що бачив би браузер.
func (p *UaserialsProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, PlayerResolveTimeout)
	defer cancel()

	resp := &domain.ContentStreamsResponse{
		ProviderID: p.ID(),
		Streams:    []domain.StreamSource{},
		Subtitles:  []domain.SubtitleSource{},
	}

	html, err := p.client.Get(ctx, itemURL, p.baseURL+"/")
	if err != nil {
		return resp, fmt.Errorf("uaserials get streams html: %w", err)
	}

	tabs, err := p.resolveUaserialsTabs(ctx, html)
	if err != nil {
		return resp, fmt.Errorf("uaserials %s: %w: %w", itemURL, ErrUnresolvablePlayer, err)
	}

	players, _ := selectUaserialsPlayers(tabs)
	if len(players) == 0 {
		return resp, fmt.Errorf("uaserials %s: %w: decrypted tabs expose no playable player",
			itemURL, ErrUnresolvablePlayer)
	}

	dub := uaserialsVoiceName(voiceID)

	var (
		seenStreams = make(map[string]bool)
		seenSubs    = make(map[string]bool)
		lastErr     error
	)
	for _, pl := range players {
		streams, subs, err := p.resolvePlayerStreams(ctx, pl.URL, itemURL, pl.TabName, season, episode, dub)
		if err != nil {
			lastErr = err
			continue
		}
		for _, s := range finalizeUaserialsStreams(streams, itemURL, p.baseURL) {
			if seenStreams[s.URL] {
				continue
			}
			seenStreams[s.URL] = true
			resp.Streams = append(resp.Streams, s)
		}
		for _, sub := range subs {
			if seenSubs[sub.URL] {
				continue
			}
			seenSubs[sub.URL] = true
			resp.Subtitles = append(resp.Subtitles, sub)
		}
	}

	if len(resp.Streams) == 0 {
		if lastErr == nil {
			lastErr = ErrUnresolvablePlayer
		}
		return resp, fmt.Errorf("uaserials %s streams: %w: %w", itemURL, ErrUnresolvablePlayer, lastErr)
	}

	SortStreamsByDubWeight(resp.Streams)
	return resp, nil
}

// uaserialsVoiceName переводить Voiceover.ID клієнта на назву, якою
// позначено доріжки в плеєрі.
//
// Voiceover.ID для «1+1» — це «1plus1» (canonical id зі словника
// студій), а в плеєрі доріжка зветися «1+1». Без цього перетворення
// фільтр за озвучкою не збігається й доводиться щоразу падати на
// фолбек «віддати всі серії».
func uaserialsVoiceName(voiceID string) string {
	voiceID = strings.TrimSpace(voiceID)
	if voiceID == "" {
		return ""
	}
	if info, ok := ResolveStudio(voiceID); ok {
		return info.Name
	}
	return voiceID
}

// resolvePlayerStreams перетворює URL вкладки на готові потоки.
func (p *UaserialsProvider) resolvePlayerStreams(ctx context.Context, playerURL, itemURL, label string, season, episode int, dub string) ([]domain.StreamSource, []domain.SubtitleSource, error) {
	name := label
	if name == "" || reUaserialsTrailerTab.MatchString(name) {
		name = uaserialsPlayerLabel
	}

	// Tortuga має власний екстрактор (tortuga_cdn.go): там зашифроване
	// значення file:, яке загальний резолвер не розбирає.
	if isTortugaPlayerURL(playerURL) {
		return NewTortugaExtractor(p.client).FetchTortugaEpisode(ctx, playerURL, season, episode, dub)
	}

	if detected := detectPlayerBalancer(playerURL); detected != "" {
		name = detected
	}
	return extractAllStreamsFromPlayer(ctx, p.client, playerURL, itemURL, name, season, episode, dub)
}

// fetchPlayerTree дістає сезони та озвучки з гравця вкладки.
//
// pageURL — адреса сторінки uaserials, а не embed-сторінки Tortuga.
// Вона їде в ref кожної серії, інакше клієнт поверне «v:1:2:1+1» без
// адреси, і GetStreams впаде з «invalid URL scheme: [v]».
func (p *UaserialsProvider) fetchPlayerTree(ctx context.Context, playerURL, pageURL string) (TortugaResult, error) {
	if !isTortugaPlayerURL(playerURL) {
		return TortugaResult{}, fmt.Errorf("uaserials: %s is not a known playlist host", playerURL)
	}
	return NewTortugaExtractor(p.client).FetchTortugaEmbed(ctx, playerURL, pageURL)
}

// isTortugaPlayerURL — вкладка веде на плеєр Tortuga.
func isTortugaPlayerURL(raw string) bool {
	return strings.Contains(strings.ToLower(raw), "tortuga")
}

// finalizeUaserialsStreams приводить джерела до контракту провайдера:
// прямий URL, без проксі, з обов'язковими заголовками.
func finalizeUaserialsStreams(streams []domain.StreamSource, itemURL, baseURL string) []domain.StreamSource {
	origin := uaserialsOrigin(itemURL, baseURL)

	for i := range streams {
		s := &streams[i]
		// Media is never proxied by us: the client plays the CDN URL
		// itself. Тому DirectURL — це те саме, що й URL, навіть якщо
		// джерело прийшло без заповненого DirectURL.
		if s.DirectURL == "" {
			s.DirectURL = s.URL
		}
		s.URL = s.DirectURL
		s.RequiresProxy = false
		// Карту перебудовуємо, а не доповнюємо: у джерела міг
		// залишитися Referer стороннього CDN, який суперечить нашому
		// і змусив би клієнтський плеєр віддавати неправильний запит.
		s.Headers = map[string]string{
			"User-Agent": Chrome120UserAgent,
			"Referer":    itemURL,
			"Origin":     origin,
		}
	}
	return streams
}

// uaserialsOrigin повертає origin сторінки опису для заголовка Origin.
func uaserialsOrigin(itemURL, baseURL string) string {
	if u, err := url.Parse(strings.TrimSpace(itemURL)); err == nil && u.Scheme != "" && u.Host != "" {
		return originOf(u)
	}
	return strings.TrimRight(baseURL, "/")
}
