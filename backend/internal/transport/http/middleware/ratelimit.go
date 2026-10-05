package middleware

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// TrustedProxyCIDRsEnv — CIDR- список (через кому) довірених реверс-проксі.
// Читається один раз, лише якщо SetTrustedProxies не викликали явно.
//
// Приклад для Portainer/nginx перед контейнером:
//
//	TRUSTED_PROXY_CIDRS=172.16.0.0/12,10.0.0.0/8
//
// Приклад для Cloudflare Tunnel (клієнтські з'єднання приходять з
// cloudflared всередині мережі; реальний IP передається у CF-Connecting-IP
// та X-Forwarded-For):
//
//	TRUSTED_PROXY_CIDRS=172.18.0.0/16
const TrustedProxyCIDRsEnv = "TRUSTED_PROXY_CIDRS"

// maxTrackedClients — абсолютна стеля розміру мапи.
// При виправленому extractIP ключі походять із реального сокета, тому
// мапа не може роздуватися підміною заголовків; ліміт — це друга лінія
// захисту від розростання при великому ботнеті (або від повільної
// ескалації «унікальний IP на запит»).
const maxTrackedClients = 50_000

// bucketIdleTTL — скільки часу бакет може простояти без запитів, перш ніж
// purge його видалить.
const bucketIdleTTL = 5 * time.Minute

// keyPeerRewritten — ключ для запитів, у яких RemoteAddr не має порту.
// chi-middleware RealIP (router.go) переписує RemoteAddr у bare-IP із
// X-Forwarded-For/X-Real-IP, і на момент нашого middleware реальний адресат
// уже втрачено. Ключ-константа — fail-closed: підроблені заголовки не можуть
// створити нові бакети (див. extractIP/limiterKey).
const keyPeerRewritten = "peer-rewritten-by-proxy-middleware"

type clientBucket struct {
	tokens     float64
	lastUpdate time.Time
}

var trustedProxies = struct {
	mu          sync.RWMutex
	nets        []*net.IPNet
	configured  bool
	initialized sync.Once
}{}

// SetTrustedProxies задає список довірених реверс-проксі у форматі CIDR
// ("10.0.0.0/8") або голого IP ("10.0.0.7" → /32). Порожній список означає
// «не довіряти нікому».
//
// Коли список порожній, усі forwarding-заголовки ігноруються повністю: без
// цієї конфігурації лімітер просто рахує байєти за RemoteAddr.
func SetTrustedProxies(cidrs []string) error {
	nets, err := parseCIDRs(cidrs)
	if err != nil {
		return err
	}
	trustedProxies.mu.Lock()
	trustedProxies.nets = nets
	trustedProxies.configured = true
	trustedProxies.mu.Unlock()
	return nil
}

func parseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, raw := range cidrs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			ip := net.ParseIP(raw)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy entry %q", raw)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy cidr %q: %w", raw, err)
		}
		nets = append(nets, network)
	}
	return nets, nil
}

// initTrustedProxiesOnce читає TRUSTED_PROXY_CIDRS лише тоді, коли
// SetTrustedProxies не викликали. Fail-closed: без змінної довіри немає.
func initTrustedProxiesOnce() {
	trustedProxies.initialized.Do(func() {
		trustedProxies.mu.RLock()
		alreadyConfigured := trustedProxies.configured
		trustedProxies.mu.RUnlock()
		if alreadyConfigured {
			return
		}
		raw := strings.TrimSpace(os.Getenv(TrustedProxyCIDRsEnv))
		if raw == "" {
			return
		}
		nets, err := parseCIDRs(strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == ';'
		}))
		if err != nil {
			// Помилкове значення не робить довіру ширшою — лише ігнорується.
			return
		}
		trustedProxies.mu.Lock()
		if !trustedProxies.configured {
			trustedProxies.nets = nets
			trustedProxies.configured = true
		}
		trustedProxies.mu.Unlock()
	})
}

func isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	trustedProxies.mu.RLock()
	defer trustedProxies.mu.RUnlock()
	for _, network := range trustedProxies.nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// IPRateLimiter обмежує кількість запитів від одного IP за алгоритмом Token Bucket
type IPRateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*clientBucket
	rate       float64 // токенів на секунду
	burst      float64 // максимальний запас токенів
	lastPurge  time.Time
	purgeEvery time.Duration

	// order — журнал ключів у порядку створення, head — позиція найстарішого
	// ще не витісненого ключа. Кільцевий FIFO дає O(1) витіснення замість
	// сканування всієї мапи на кожен новий ключ.
	order []string
	head  int
}

// NewIPRateLimiter створює новий per-IP лімітер.
// rate — допустима кількість запитів на секунду
// burst — максимальний сплеск одночасних запитів
func NewIPRateLimiter(rate float64, burst float64) *IPRateLimiter {
	return &IPRateLimiter{
		clients:    make(map[string]*clientBucket),
		rate:       rate,
		burst:      burst,
		lastPurge:  time.Now(),
		purgeEvery: 3 * time.Minute,
	}
}

func (limiter *IPRateLimiter) allow(ip string) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()

	// Періодичне очищення старих записів для запобігання витоку пам'яті
	if now.Sub(limiter.lastPurge) > limiter.purgeEvery {
		limiter.purgeLocked(now)
		limiter.lastPurge = now
	}

	b, exists := limiter.clients[ip]
	if !exists {
		limiter.makeRoomLocked(ip)
		limiter.clients[ip] = &clientBucket{
			tokens:     limiter.burst - 1,
			lastUpdate: now,
		}
		limiter.order = append(limiter.order, ip)
		return true
	}

	// Поповнення токенів за минулий час
	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.tokens += elapsed * limiter.rate
	if b.tokens > limiter.burst {
		b.tokens = limiter.burst
	}
	b.lastUpdate = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

func (limiter *IPRateLimiter) purgeLocked(now time.Time) {
	for k, b := range limiter.clients {
		if now.Sub(b.lastUpdate) > bucketIdleTTL {
			delete(limiter.clients, k)
		}
	}
}

// makeRoomLocked гарантує місце для нового ключа.
//
// Відмова («повернути false» для нових IP) була б DoS-вектором: атакуючий
// міг би наповнити мапу і назавжди заблокувати легітимних нових клієнтів. Тому
// замість відмови витісняємо найстаріший бакет — FIFO за O(1).
func (limiter *IPRateLimiter) makeRoomLocked(ip string) {
	for len(limiter.clients) >= maxTrackedClients {
		if limiter.head >= len(limiter.order) {
			// Усі ключі, які колись додавали, вже або витіснені, або видалені
			// purge'ом, отже мапа порожня і місця вистачає.
			limiter.order = limiter.order[:0]
			limiter.head = 0
			return
		}
		stale := limiter.order[limiter.head]
		limiter.head++
		if _, ok := limiter.clients[stale]; ok {
			delete(limiter.clients, stale)
		}
	}
	if limiter.head >= 1024 && limiter.head*2 >= len(limiter.order) {
		limiter.order = append(limiter.order[:0], limiter.order[limiter.head:]...)
		limiter.head = 0
	}
}

// RateLimitMiddleware створює HTTP middleware для обмеження частоти запитів
func RateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || limiter.rate <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			ip := limiterKey(r)
			if !limiter.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"rate limit exceeded, please slow down"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractIP повертає найкращу доступну адресу клієнта для логів.
//
// Вона НЕ придатна для лімітера: якщо peer не є довіреним проксі, значення
// заголовків ігнорується і повертається адреса сокета, яка в цьому ланцюжку
// вже могла бути переписана chimiddleware.RealIP. Для лімітера —
// limiterKey(), якій ця проблема не властива.
func extractIP(r *http.Request) string {
	initTrustedProxiesOnce()

	peer, ok := peerHostPort(r.RemoteAddr)
	if !ok {
		return strings.TrimSpace(r.RemoteAddr)
	}
	if !isTrustedProxy(net.ParseIP(peer)) {
		return peer
	}
	if forwarded := rightmostUntrustedForwardedIP(r); forwarded != "" {
		return forwarded
	}
	return peer
}

// limiterKey повертає ключ бакета.
//
// Правила:
//  1. RemoteAddr без порту → хтось (chimiddleware.RealIP у router.go) уже
//     підставив у нього значення forwarding-заголовка. Довіряти цьому
//     не можна, а real peer невідомий — повертаємо спільний ключ, щоб
//     підроблені заголовки не видавали нові бакети.
//  2. Peer не в списку довірених → ігноруємо X-Forwarded-For / X-Real-IP /
//     CF-Connecting-IP повністю і рахуємо за адресою сокета.
//  3. Peer — довірений проксі → йдемо по X-Forwarded-For справа наліво і
//     беремо перший НЕдовірений хоп (класичний алгоритм, стійкий до
//     підміни лівих елементів). Fallback — CF-Connecting-IP (Cloudflare
//     Tunnel), потім X-Real-IP, потім сам peer.
func limiterKey(r *http.Request) string {
	initTrustedProxiesOnce()

	peer, ok := peerHostPort(r.RemoteAddr)
	if !ok {
		return keyPeerRewritten
	}
	if !isTrustedProxy(net.ParseIP(peer)) {
		return peer
	}
	if forwarded := rightmostUntrustedForwardedIP(r); forwarded != "" {
		return forwarded
	}
	return peer
}

func rightmostUntrustedForwardedIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil {
				continue
			}
			if !isTrustedProxy(ip) {
				return ip.String()
			}
		}
	}
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		if ip := net.ParseIP(cf); ip != nil && !isTrustedProxy(ip) {
			return ip.String()
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		if ip := net.ParseIP(xrip); ip != nil && !isTrustedProxy(ip) {
			return ip.String()
		}
	}
	return ""
}

func peerHostPort(remoteAddr string) (string, bool) {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return "", false
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil && host != "" {
		return host, true
	}
	// Деякі тести й httptest дають bare-IP; Go-сервер завжди пише host:port,
	// тому bare-IP трактуємо як такий, що не має порту (тобто переписаний).
	return "", false
}
