package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/html/charset"
)

// Chrome120UserAgent — єдиний User-Agent для всіх провайдерів.
//
// Раніше цей рядок був продубльований у client.go (2 рази), uakino.go,
// eneyida.go та lavakino.go (2 рази). Він мусить збігатися з профілем
// tls_client profiles.Chrome_120, інакше CDN бачить розбіжність між TLS
// відпечатком і заголовком.
const Chrome120UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// MaxUpstreamBodyBytes — стеля розміру тіла відповіді провайдера.
//
// Таймаут у 15с обмежує тривалість, але НІ байти: ворожій або скомпрометований
// CDN може за секунду віддати сотні мегабайт, і в контейнері з 256 MiB RAM це
// мгновений OOM. 8 MiB — із великим запасом понад реальні HTML-сторінки
// uakino/lavakino/eneyida (зазвичай 100–800 KiB) та m3u8-плейлисти.
//
// Прецедент: bandera_client.go робить те саме для сниппетів помилок —
// io.ReadAll(io.LimitReader(resp.Body, 512)) (bandera_client.go:108).
const MaxUpstreamBodyBytes = 8 << 20

// maxRedirects — скільки редиректів ми готові пройти. Стандарт net/http — 10;
// 5 вистачає для нормальних CDN-ланцюжків (http→https, www→bare) і обмежує
// ланцюги, зловмисно нарощені в бік внутрішніх адрес.
const maxRedirects = 5

// resolveRedirectTimeout — резолв хоста редиректа. Відбувається рідко (лише на
// ланцюжку 3xx), тому окремий таймаут; спільний ліміт запиту вже контролює
// tls_client.WithTimeoutSeconds(15).
const resolveRedirectTimeout = 2 * time.Second

// ErrUnsafeUpstreamTarget — редирект (або початкова ціль) веде на
// внутрішню/приватну адресу. Раніше редиректи йшли за замовчуванням (10
// переходів), тому "https://attacker/r" → 302 → "http://169.254.169.254/
// latest/meta-data/" повертав тіло метаданих клієнту: це SSRF на читання.
var ErrUnsafeUpstreamTarget = errors.New("refusing to follow redirect to unsafe upstream target")

type TLSClient struct {
	client       tls_client.HttpClient
	workerClient *nethttp.Client
	workerURL    string
	workerSecret string
	pacerMu      sync.Mutex
	hostLastReq  map[string]time.Time
	minInterval  time.Duration
}

// ClientOption дозволяє налаштовувати опції TLSClient.
type ClientOption func(*TLSClient)

// WithWorkerProxy налаштовує перенаправлення запитів через Cloudflare Worker Proxy.
func WithWorkerProxy(url, secret string, minInterval time.Duration) ClientOption {
	return func(c *TLSClient) {
		c.workerURL = strings.TrimRight(url, "/")
		c.workerSecret = secret
		c.minInterval = minInterval
	}
}

// NewTLSClient створює HTTP-клієнт з емуляцією браузерного TLS (JA3/JA4) для обходу Cloudflare
func NewTLSClient(opts ...ClientOption) (*TLSClient, error) {
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(15),
		tls_client.WithClientProfile(profiles.Chrome_120),
		tls_client.WithCookieJar(jar),
		// Кожен хоп редиректа проходить через validateUpstreamURL. Обрано саме
		// re-validation, а не повну забору редиректів (http.ErrUseLastResponse):
		// uakino/lavakino/eneyida реально редиректять (http→https, www→bare,
		// канонічні шляхи), і повна забора зламала б скрапінг. Перевірка
		// кожного хопу прибирає SSRF, зберігаючи функціональність.
		tls_client.WithCustomRedirectFunc(validateRedirectHop),
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create tls client: %w", err)
	}

	workerHTTP := &nethttp.Client{
		Timeout: 25 * time.Second,
		CheckRedirect: func(req *nethttp.Request, via []*nethttp.Request) error {
			return nethttp.ErrUseLastResponse
		},
	}

	c := &TLSClient{
		client:       client,
		workerClient: workerHTTP,
		hostLastReq:  make(map[string]time.Time),
		minInterval:  150 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// validateRedirectHop перевіряє кожен наступний хоп редиректа.
//
// Саме тут потрібен повторний виклик валідатора: початковий URL прийшов із
// transport/http (ValidateSafeURL), але Location у 302 віддає атакуючий, і без
// перевірки хопа SSRF лишається читабельним.
func validateRedirectHop(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if req == nil || req.URL == nil {
		return fmt.Errorf("redirect without target url")
	}
	if err := validateUpstreamURL(req.URL); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnsafeUpstreamTarget, req.URL.Redacted(), err)
	}
	return nil
}

// validateUpstreamURL дзеркалить SSRF-політику transport/http ValidateSafeURL
// для редиректів. Дублювання свідоме: internal/provider не може імпортувати
// internal/transport/http — той імпортує provider (content_handler.go), тож
// виник би цикл. Коли з'явиться спільний пакет (напр. internal/netguard),
// обидві копії мають бути злиті.
func validateUpstreamURL(u *url.URL) error {
	if u == nil {
		return errors.New("nil url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsafe scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("missing host")
	}

	lower := strings.ToLower(host)
	if lower == "localhost" ||
		lower == "metadata.google.internal" ||
		lower == "instance-data" ||
		strings.HasSuffix(lower, ".localhost") ||
		strings.HasSuffix(lower, ".internal") ||
		strings.HasSuffix(lower, ".local") {
		return errors.New("blocked internal hostname")
	}

	if ip := net.ParseIP(host); ip != nil {
		if upstreamBlockedIP(ip) {
			return fmt.Errorf("blocked address %s", ip)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), resolveRedirectTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		// Fail-closed: нерезолвлений хост не може бути «підозрілим, але
		// пропущеним» — інакше DNS rebinding повертає SSRF.
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("%s resolved to no addresses", host)
	}
	for _, addr := range addrs {
		if upstreamBlockedIP(addr.IP) {
			return fmt.Errorf("%s resolves to blocked address %s", host, addr.IP)
		}
	}
	return nil
}

func upstreamBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		if ip4[0] == 0 {
			return true
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		if ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255 {
			return true // limited broadcast 255.255.255.255
		}
	}
	return false
}

// Get виконує GET-запит з підміною реферера та стандартним кешем для каталогу/пошуку (TTL 900c)
func (c *TLSClient) Get(ctx context.Context, targetURL, referer string) (string, error) {
	return c.GetWithTTL(ctx, targetURL, referer, 900)
}

// GetNoCache виконує GET-запит без кешування (TTL 0) для GetStreams, плейлистів та тимчасових токенів
func (c *TLSClient) GetNoCache(ctx context.Context, targetURL, referer string) (string, error) {
	return c.GetWithTTL(ctx, targetURL, referer, 0)
}

// GetWithTTL виконує GET-запит із заданим TTL кешу в секундах
func (c *TLSClient) GetWithTTL(ctx context.Context, targetURL, referer string, ttlSeconds int) (string, error) {
	if c.workerURL != "" && c.workerSecret != "" {
		return c.getViaWorker(ctx, targetURL, referer, ttlSeconds)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "uk,en-US;q=0.9,en;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := readLimitedBody(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(bodyBytes)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return "", fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, snippet)
	}

	return decodeBody(resp.Header.Get("Content-Type"), bodyBytes)
}

// readLimitedBody читає тіло відповіді з жорсткою стелею. Читаємо на 1 байт
// більше ліміту, щоб відрізнити «рівно ліміт» від «перевищено».
func readLimitedBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxUpstreamBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if len(body) > MaxUpstreamBodyBytes {
		return nil, fmt.Errorf("upstream response exceeds %d byte limit", MaxUpstreamBodyBytes)
	}
	return body, nil
}

// decodeBody транскодує тіло відповіді у UTF-8.
//
// golang.org/x/net/html (яким користується goquery) НЕ визначає кодування
// і НЕ перекодовує: він просто розбирає байти як UTF-8. Якщо DLE-сайт
// віддає windows-1251, усі кириличні рядки стають «мошибками», і — що
// набагато гірше — мовчки ламаються збіги за лейблами ("режисер", "актор",
// "жанр", "рік"): режисер, актори, жанри, країни, рік і тривалість приходять
// порожніми, без жодної помилки.
// charsetOf витягує значення параметра `charset` із заголовка Content-Type
// «як є»: без нормалізації регістру й без зміни вмісту значення.
//
// mime.ParseMediaType тут не годиться: він повертає помилку на заголовках
// виду "text/html; bad; charset=cp1251", де є безіменний параметр, і тоді
// ми втрачаємо корисну інформацію про кодування. Тому скануємо вручну.
func charsetOf(contentType string) string {
	const key = "charset="
	lower := strings.ToLower(contentType)
	i := strings.Index(lower, key)
	if i < 0 {
		return ""
	}
	v := contentType[i+len(key):]
	if j := strings.Index(v, ";"); j >= 0 {
		v = v[:j]
	}
	return strings.Trim(strings.TrimSpace(v), `"'`)
}

func decodeBody(contentType string, body []byte) (string, error) {
	label := charsetOf(contentType)
	if label == "" {
		// Найчастіший випадок: сервер не оголосив кодування. Не торкаємося
		// байтів, щоб не вносити регресію у звичайний шлях.
		return string(body), nil
	}
	switch strings.ToLower(label) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return string(body), nil
	}
	r, err := charset.NewReaderLabel(label, bytes.NewReader(body))
	if err != nil {
		// Невідома мітка не повинна ламати запит: повертаємо байти як є.
		return string(body), nil
	}
	// Другий прохід з тією самою стелею: конвертація може розширити тіло
	// (напр. UTF-16 → UTF-8 дає до 2x), тож ліміт треба застосувати і тут.
	decoded, err := io.ReadAll(io.LimitReader(r, MaxUpstreamBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("transcode response body from %s: %w", label, err)
	}
	if len(decoded) > MaxUpstreamBodyBytes {
		return "", fmt.Errorf("transcoded response body from %s exceeds %d byte limit", label, MaxUpstreamBodyBytes)
	}
	return string(decoded), nil
}

func (c *TLSClient) PostForm(ctx context.Context, targetURL, formData, referer string) (string, error) {
	if c.workerURL != "" && c.workerSecret != "" {
		return c.postFormViaWorker(ctx, targetURL, formData, referer)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, strings.NewReader(formData))
	if err != nil {
		return "", fmt.Errorf("create post request: %w", err)
	}

	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("do post request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := readLimitedBody(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(bodyBytes)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return "", fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, snippet)
	}

	return decodeBody(resp.Header.Get("Content-Type"), bodyBytes)
}

func (c *TLSClient) waitPacer(ctx context.Context, targetURL string) error {
	if c.minInterval <= 0 {
		return nil
	}

	host := ""
	if u, err := url.Parse(targetURL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	if host == "" {
		host = "default"
	}

	c.pacerMu.Lock()
	defer c.pacerMu.Unlock()

	now := time.Now()
	lastTime, exists := c.hostLastReq[host]
	if exists && !lastTime.IsZero() {
		elapsed := now.Sub(lastTime)
		if elapsed < c.minInterval {
			delay := c.minInterval - elapsed
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	c.hostLastReq[host] = time.Now()
	return nil
}

func (c *TLSClient) getViaWorker(ctx context.Context, targetURL, referer string, ttlSeconds int) (string, error) {
	currentTarget := targetURL
	for hop := 0; hop <= maxRedirects; hop++ {
		if err := c.waitPacer(ctx, currentTarget); err != nil {
			return "", err
		}

		req, err := nethttp.NewRequestWithContext(ctx, "GET", c.workerURL, nil)
		if err != nil {
			return "", fmt.Errorf("create worker request: %w", err)
		}

		req.Header.Set("X-Proxy-Secret", c.workerSecret)
		req.Header.Set("X-Target-URL", currentTarget)
		req.Header.Set("X-Cache-TTL", strconv.Itoa(ttlSeconds))
		req.Header.Set("User-Agent", Chrome120UserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "uk,en-US;q=0.9,en;q=0.8")
		if referer != "" {
			req.Header.Set("Referer", referer)
		}

		resp, err := c.workerClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("do worker request: %w", err)
		}

		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if loc == "" {
				return "", fmt.Errorf("redirect status %d without Location header", resp.StatusCode)
			}
			if hop == maxRedirects {
				return "", fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			nextURL, err := resolveRedirectURL(currentTarget, loc)
			if err != nil {
				return "", err
			}
			if err := validateUpstreamURL(nextURL); err != nil {
				return "", fmt.Errorf("%w: %s: %v", ErrUnsafeUpstreamTarget, nextURL.Redacted(), err)
			}
			currentTarget = nextURL.String()
			continue
		}

		defer resp.Body.Close()

		bodyBytes, err := readLimitedBody(resp.Body)
		if err != nil {
			return "", err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snippet := string(bodyBytes)
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			return "", fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, snippet)
		}

		return decodeBody(resp.Header.Get("Content-Type"), bodyBytes)
	}

	return "", fmt.Errorf("stopped after %d redirects", maxRedirects)
}

func (c *TLSClient) postFormViaWorker(ctx context.Context, targetURL, formData, referer string) (string, error) {
	if err := c.waitPacer(ctx, targetURL); err != nil {
		return "", err
	}

	req, err := nethttp.NewRequestWithContext(ctx, "POST", c.workerURL, strings.NewReader(formData))
	if err != nil {
		return "", fmt.Errorf("create worker post request: %w", err)
	}

	req.Header.Set("X-Proxy-Secret", c.workerSecret)
	req.Header.Set("X-Target-URL", targetURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", Chrome120UserAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := c.workerClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do worker post request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := readLimitedBody(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(bodyBytes)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return "", fmt.Errorf("upstream provider returned status %d: %s", resp.StatusCode, snippet)
	}

	return decodeBody(resp.Header.Get("Content-Type"), bodyBytes)
}

func resolveRedirectURL(baseStr, locStr string) (*url.URL, error) {
	base, err := url.Parse(baseStr)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	loc, err := url.Parse(locStr)
	if err != nil {
		return nil, fmt.Errorf("parse redirect location: %w", err)
	}
	return base.ResolveReference(loc), nil
}
