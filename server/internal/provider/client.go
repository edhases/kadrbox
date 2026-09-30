package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

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

type TLSClient struct {
	client tls_client.HttpClient
}

// NewTLSClient створює HTTP-клієнт з емуляцією браузерного TLS (JA3/JA4) для обходу Cloudflare
func NewTLSClient() (*TLSClient, error) {
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(15),
		tls_client.WithClientProfile(profiles.Chrome_120),
		tls_client.WithCookieJar(jar),
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create tls client: %w", err)
	}

	return &TLSClient{client: client}, nil
}

// Get виконує GET-запит з підміною реферера та заголовків
func (c *TLSClient) Get(ctx context.Context, targetURL, referer string) (string, error) {
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
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
		// Невідома метка не повинна ламати запит: повертаємо байти як є.
		return string(body), nil
	}
	decoded, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("transcode response body from %s: %w", label, err)
	}
	return string(decoded), nil
}

func (c *TLSClient) PostForm(ctx context.Context, targetURL, formData, referer string) (string, error) {
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
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
