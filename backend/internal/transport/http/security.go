package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnsafeURLScheme = errors.New("unsafe url scheme: only http and https are allowed")
	ErrSSRFBlocked     = errors.New("ssrf blocked: target points to internal, private or loopback address")

	// ErrURLTooLong bounds the input before it is parsed. middleware.BodyLimit
	// caps request *bodies* at 1 MiB, but `itemURL` travels in the query string,
	// which no middleware in this chain bounds.
	ErrURLTooLong = errors.New("url too long")
	// ErrUnsafeCharInput rejects control characters, which url.Parse happily
	// accepts and which are used to smuggle a second request line past naive
	// consumers of the validated value.
	ErrUnsafeCharInput = errors.New("url contains control characters")
	// ErrHostNotAllowed is returned when an allow-list is configured and the
	// host is not on it.
	ErrHostNotAllowed = errors.New("host is not on the upstream allow-list")
)

const (
	// maxURLLength bounds the URL before it is parsed.
	maxURLLength = 2048

	// resolveTimeout bounds a single DNS lookup. ValidateSafeURL has no context
	// parameter (its signature is frozen because every caller is synchronous and
	// context-free), so the timeout has to live here.
	resolveTimeout = 2 * time.Second
	// resolveCacheTTL keeps the happy path off the DNS path. 60s is short
	// enough that a legitimately re-pointed host recovers quickly.
	resolveCacheTTL = 60 * time.Second
	// resolveErrorCacheTTL applies when the lookup itself failed: a bogus host
	// must not cost a full DNS timeout on every request.
	resolveErrorCacheTTL = 5 * time.Second
	// resolveCacheMaxEntries bounds the cache itself; it is keyed by hostname,
	// so an attacker can only inflate it with distinct hostnames, and the reset
	// keeps that from becoming a memory sink.
	resolveCacheMaxEntries = 1024
)

// hostResolver is a package-level *net.Resolver so the DNS policy can be
// swapped in tests without a live resolver.
var (
	hostResolverMu sync.RWMutex
	hostResolver   = net.DefaultResolver

	allowlistMu   sync.RWMutex
	hostAllowlist []string
)

type resolveVerdict struct {
	safe    bool
	expires time.Time
}

var resolveCache = struct {
	mu      sync.Mutex
	entries map[string]resolveVerdict
}{entries: map[string]resolveVerdict{}}

// SetUpstreamHostAllowlist restricts ValidateSafeURL to the given hosts.
//
// SECURITY: an empty (or unset) list means "any host that resolves to a public
// address", which leaves the endpoint usable as a free request proxy against
// arbitrary third parties. It is NOT safe for production; operators must set it.
// Entries are matched case-insensitively, either exactly ("cdn.example"), as a
// suffix (".media.example" matches "www.media.example") or as a wildcard
// ("*.media.example"). A literal IP entry pins that address and deliberately
// overrides the private-address rejection, so it must only ever be used with a
// public address.
func SetUpstreamHostAllowlist(hosts []string) {
	normalized := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		normalized = append(normalized, h)
	}
	allowlistMu.Lock()
	hostAllowlist = normalized
	allowlistMu.Unlock()
}

func currentHostAllowlist() []string {
	allowlistMu.RLock()
	defer allowlistMu.RUnlock()
	if len(hostAllowlist) == 0 {
		return nil
	}
	out := make([]string, len(hostAllowlist))
	copy(out, hostAllowlist)
	return out
}

func hostOnAllowlist(host string, list []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, entry := range list {
		if entry == host {
			return true
		}
		if strings.HasPrefix(entry, "*.") {
			entry = entry[1:]
		}
		if strings.HasPrefix(entry, ".") && strings.HasSuffix(host, entry) {
			return true
		}
	}
	return false
}

// ValidateSafeURL перевіряє вхідний URL на наявність загроз SSRF.
//
// П'ять рівнів:
//  1. довжина + керуючі символи (у query string немає BodyLimit);
//  2. схема лише http/https;
//  3. імена, які завжди внутрішні (localhost, *.internal, *.local, metadata);
//  4. IP-літерали у приватних діапазонах;
//  5. resolve-then-check: hostname розкривається і блокується, якщо БУДЬ-який
//     з отриманих адрес приватний/loopback/link-local/ULA/unspecified.
//
// Крок 5 закриває обхід, який неможливо закрити синтаксично: ім'я, чий A-запис
// вказує на 169.254.169.254, не парситься як IP-літерал і раніше проходило
// перевірку без жодного DNS-запиту.
func ValidateSafeURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("empty url")
	}
	if hasControlChars(raw) {
		return ErrUnsafeCharInput
	}
	if len(raw) > maxURLLength {
		return fmt.Errorf("%w: %d bytes exceeds %d", ErrURLTooLong, len(raw), maxURLLength)
	}
	return validateSingleURL(raw)
}

func hasControlChars(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f {
			return true
		}
	}
	return false
}

func validateSingleURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("empty url")
	}
	if hasControlChars(raw) {
		return ErrUnsafeCharInput
	}

	// Безпечний відносний шлях у межах сайту (наприклад, /serial/123-dune.html)
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return nil
	}
	// "//host/path" — protocol-relative, тобто фактично абсолютний URL.
	if strings.HasPrefix(raw, "//") {
		return ErrSSRFBlocked
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed url: %w", err)
	}

	// Відносні ідентифікатори або шляхи без схеми
	if parsed.Scheme == "" {
		if strings.Contains(parsed.Host, "localhost") {
			return ErrSSRFBlocked
		}
		return nil
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrUnsafeURLScheme
	}

	host := parsed.Hostname()
	if host == "" {
		return errors.New("missing host in url")
	}

	// Allow-list має пріоритет: це явний вибір оператора. Див. doc SetUpstreamHostAllowlist.
	if list := currentHostAllowlist(); list != nil {
		if hostOnAllowlist(host, list) {
			return nil
		}
		// Список задано — він вичерпний: хост поза ним відкидається, навіть
		// якщо резолвиться в публічну адресу.
		return fmt.Errorf("%w: %s", ErrHostNotAllowed, host)
	}

	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" ||
		lowerHost == "metadata.google.internal" ||
		lowerHost == "instance-data" ||
		strings.HasSuffix(lowerHost, ".localhost") ||
		strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".local") {
		return ErrSSRFBlocked
	}

	// Перевірка IP-літерала: без DNS, однозначний результат.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrLocalIP(ip) {
			return ErrSSRFBlocked
		}
		return nil
	}

	// Resolve-then-check для імені хоста.
	if err := requirePublicResolution(host); err != nil {
		return err
	}
	return nil
}

// requirePublicResolution розкриває hostname і вимагає, щоб УСІ адреси були
// публічними. Перевіряється саме resolve-then-check, а не результат запиту:
// після цієї перевірки реальне з'єднання все ще робить власний резолв, тому
// захист не є атомарним (див. Risks у звіті).
//
// Помилка резолву НЕ відкидає запит. Причини:
//   - нерезолвний хост і так не буде використаний, тож fail-open тут не
//     відкриває «дірку» на читання;
//   - збій DNS перетворив би валідацію на 400, тобто вимкнення залежності
//     знецінювало б сервіс;
//   - залишається вікно DNS rebinding (NXDOMAIN під час валідації -> приватна
//     адреса під час з'єднання). Воно не більше за вікно, яке й так існує для
//     імені, що резолвиться в публічну адресу. Єдиний спосіб закрити обидва
//     вікна — задати SetUpstreamHostAllowlist: тоді резолв не виконується взагалі.
func requirePublicResolution(host string) error {
	verdict, cached := lookupCachedVerdict(host)
	if cached {
		if verdict {
			return nil
		}
		return fmt.Errorf("%w: %s resolves to a blocked address", ErrSSRFBlocked, host)
	}

	addrs, err := lookupIPAddrs(host)
	if err != nil {
		// Помилку резолву кешуємо нетривалим TTL: інакше кожен запит із
		// вигаданим хостом платив би повний таймаут DNS (2с) і тримав горутину.
		storeVerdict(host, true, resolveErrorCacheTTL)
		return nil
	}

	safe := len(addrs) > 0
	for _, addr := range addrs {
		if isPrivateOrLocalIP(addr.IP) {
			safe = false
			break
		}
	}
	storeVerdict(host, safe, resolveCacheTTL)

	if !safe {
		return fmt.Errorf("%w: %s resolves to a blocked address", ErrSSRFBlocked, host)
	}
	return nil
}

func lookupIPAddrs(host string) ([]net.IPAddr, error) {
	hostResolverMu.RLock()
	resolver := hostResolver
	hostResolverMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	return resolver.LookupIPAddr(ctx, host)
}

// SetHostResolver is a test seam: it swaps the resolver used for SSRF checks.
func SetHostResolver(resolver *net.Resolver) {
	hostResolverMu.Lock()
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	hostResolver = resolver
	hostResolverMu.Unlock()
	ClearResolveCache()
}

func lookupCachedVerdict(host string) (bool, bool) {
	resolveCache.mu.Lock()
	defer resolveCache.mu.Unlock()
	v, ok := resolveCache.entries[host]
	if !ok || time.Now().After(v.expires) {
		return false, false
	}
	return v.safe, true
}

func storeVerdict(host string, safe bool, ttl time.Duration) {
	resolveCache.mu.Lock()
	defer resolveCache.mu.Unlock()
	if len(resolveCache.entries) >= resolveCacheMaxEntries {
		resolveCache.entries = map[string]resolveVerdict{}
	}
	resolveCache.entries[host] = resolveVerdict{safe: safe, expires: time.Now().Add(ttl)}
}

// ClearResolveCache drops memoised DNS verdicts. Used by tests and by
// SetHostResolver.
func ClearResolveCache() {
	resolveCache.mu.Lock()
	resolveCache.entries = map[string]resolveVerdict{}
	resolveCache.mu.Unlock()
}

func isPrivateOrLocalIP(ip net.IP) bool {
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

	// Перевірка 169.254.169.254 (Cloud metadata service)
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// 0.0.0.0/8
		if ip4[0] == 0 {
			return true
		}
		// 100.64.0.0/10 — shared address space (RFC 6598): не публічний
		// диапазон, хоча і не зовсім приватний (CGNAT у контейнерних мережах).
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		if ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255 {
			return true // limited broadcast 255.255.255.255
		}
	}

	return false
}
