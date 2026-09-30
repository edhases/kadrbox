package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type clientBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// IPRateLimiter обмежує кількість запитів від одного IP за алгоритмом Token Bucket
type IPRateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*clientBucket
	rate       float64 // токенів на секунду
	burst      float64 // максимальний запас токенів
	lastPurge  time.Time
	purgeEvery time.Duration
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
		for k, b := range limiter.clients {
			if now.Sub(b.lastUpdate) > 5*time.Minute {
				delete(limiter.clients, k)
			}
		}
		limiter.lastPurge = now
	}

	b, exists := limiter.clients[ip]
	if !exists {
		limiter.clients[ip] = &clientBucket{
			tokens:     limiter.burst - 1,
			lastUpdate: now,
		}
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

// RateLimitMiddleware створює HTTP middleware для обмеження частоти запитів
func RateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter == nil || limiter.rate <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			ip := extractIP(r)
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

func extractIP(r *http.Request) string {
	// Якщо RealIP middleware вже виставило RemoteAddr або є X-Real-IP / X-Forwarded-For
	if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
		return xRealIP
	}
	if xForwarded := r.Header.Get("X-Forwarded-For"); xForwarded != "" {
		parts := strings.Split(xForwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
