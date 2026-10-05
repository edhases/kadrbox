package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

// resetTrustedProxies повертає глобальний стан довірених проксі у fail-closed
// (довіри немає) і робить це після кожного тесту.
func resetTrustedProxies(t *testing.T) {
	t.Helper()
	clear := func() {
		if err := middleware.SetTrustedProxies(nil); err != nil {
			t.Fatalf("SetTrustedProxies(nil) failed: %v", err)
		}
	}
	clear()
	t.Cleanup(clear)
}

func TestRateLimitMiddleware(t *testing.T) {
	resetTrustedProxies(t)

	// Дозволяємо 2 токени з burst 2
	limiter := middleware.NewIPRateLimiter(2, 2)
	mw := middleware.RateLimitMiddleware(limiter)

	nextCalled := 0
	dummyHandler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "198.51.100.1:1234"

	// 1-й запит — дозволено (залишок 1)
	rr1 := httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected 200 on req 1, got %d", rr1.Code)
	}

	// 2-й запит — дозволено (залишок 0)
	rr2 := httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 on req 2, got %d", rr2.Code)
	}

	// 3-й запит миттєво — заблоковано (429)
	rr3 := httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on req 3, got %d", rr3.Code)
	}

	// Інший IP — дозволено!
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "203.0.113.5:5678"
	rrOther := httptest.NewRecorder()
	dummyHandler.ServeHTTP(rrOther, req2)
	if rrOther.Code != http.StatusOK {
		t.Fatalf("expected 200 for other IP, got %d", rrOther.Code)
	}

	// Після очікування токени поповнюються
	time.Sleep(600 * time.Millisecond) // поповнить >= 1 токен (2/сек * 0.6 = 1.2)
	rr4 := httptest.NewRecorder()
	dummyHandler.ServeHTTP(rr4, req)
	if rr4.Code != http.StatusOK {
		t.Fatalf("expected 200 on req 4 after wait, got %d", rr4.Code)
	}

	if nextCalled != 4 {
		t.Errorf("handler should have been reached exactly 4 times, got %d", nextCalled)
	}
}

// TestRateLimitMiddlewareResponseShape — 429 лишається JSON (Dart-клієнт
// парсить тіло через json.decode).
func TestRateLimitMiddlewareResponseShape(t *testing.T) {
	resetTrustedProxies(t)

	limiter := middleware.NewIPRateLimiter(1, 1)
	h := middleware.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "198.51.100.7:1234"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if i == 1 {
			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("expected 429, got %d", rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected application/json, got %q", ct)
			}
			if ra := rr.Header().Get("Retry-After"); ra == "" {
				t.Error("expected a Retry-After header on 429")
			}
			if body := rr.Body.String(); body != `{"error":"rate limit exceeded, please slow down"}` {
				t.Errorf("unexpected 429 body: %s", body)
			}
		}
	}
}

// TestRateLimitIgnoresForwardingHeadersFromUntrustedPeer — головний фікс:
// X-Forwarded-For від недовіреного peer'а більше не створює новий бакет.
func TestRateLimitIgnoresForwardingHeadersFromUntrustedPeer(t *testing.T) {
	resetTrustedProxies(t)

	limiter := middleware.NewIPRateLimiter(1, 1)
	h := middleware.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	spoofed := []string{"1.2.3.4", "5.6.7.8", "9.10.11.12", "13.14.15.16"}
	for i, ip := range spoofed {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "198.51.100.1:1234"
		req.Header.Set("X-Forwarded-For", ip)
		req.Header.Set("X-Real-IP", ip)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if i == 0 {
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 on the first request, got %d", rr.Code)
			}
			continue
		}
		if rr.Code != http.StatusTooManyRequests {
			t.Fatalf("rotating spoofed XFF must not mint a fresh bucket: request %d got %d, want 429", i, rr.Code)
		}
	}
}

// TestRateLimitTrustedProxyUsesRightmostUntrustedHop — за довіреним проксі
// береться перший НЕдовірений хоп зправа наліво.
func TestRateLimitTrustedProxyUsesRightmostUntrustedHop(t *testing.T) {
	resetTrustedProxies(t)
	if err := middleware.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("SetTrustedProxies failed: %v", err)
	}

	limiter := middleware.NewIPRateLimiter(1, 1)
	h := middleware.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func(remoteAddr, xff string) int {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = remoteAddr
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	// Перший запит від клієнта 203.0.113.7 через ланцюг 10.0.0.5 -> 10.0.0.9.
	if code := do("10.0.0.5:1234", "9.9.9.9, 203.0.113.7, 10.0.0.9"); code != http.StatusOK {
		t.Fatalf("first request from 203.0.113.7: got %d, want 200", code)
	}
	// Той самий клієнт із підміною лівих елементів — той самий бакет (429).
	if code := do("10.0.0.5:1234", "1.1.1.1, 203.0.113.7, 10.0.0.9"); code != http.StatusTooManyRequests {
		t.Fatalf("same rightmost untrusted hop must share a bucket: got %d, want 429", code)
	}
	// Клієнт 9.9.9.9 — інший бакет (200): довірятим є саме 203.0.113.7,
	// а не лівий елемент XFF.
	if code := do("10.0.0.5:1234", "9.9.9.9, 10.0.0.9"); code != http.StatusOK {
		t.Fatalf("a different leftmost XFF entry must not change the client bucket: got %d, want 200", code)
	}
	// Peer, якому не довіряють, але який сам прийшов без заголовків.
	if code := do("203.0.113.50:9999", ""); code != http.StatusOK {
		t.Fatalf("untrusted direct peer must get its own bucket: got %d, want 200", code)
	}
	if code := do("203.0.113.50:9999", ""); code != http.StatusTooManyRequests {
		t.Fatalf("untrusted direct peer must be limited: got %d, want 429", code)
	}
}

// TestRateLimitTrustedProxyFallsBackToCFConnectingIP — Cloudflare Tunnel не
// додає ланцюг у XFF так, як це робить nginx, але передає CF-Connecting-IP.
func TestRateLimitTrustedProxyFallsBackToCFConnectingIP(t *testing.T) {
	resetTrustedProxies(t)
	if err := middleware.SetTrustedProxies([]string{"172.18.0.0/16"}); err != nil {
		t.Fatalf("SetTrustedProxies failed: %v", err)
	}

	limiter := middleware.NewIPRateLimiter(1, 1)
	h := middleware.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func(cfIP string) int {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "172.18.0.2:40000"
		req.Header.Set("CF-Connecting-IP", cfIP)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	if code := do("198.51.100.22"); code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if code := do("198.51.100.22"); code != http.StatusTooManyRequests {
		t.Fatalf("same CF-Connecting-IP must share a bucket: got %d, want 429", code)
	}
}

// TestRateLimitUntrustedPeerIgnoresCFConnectingIP — без налаштованої довіри
// жоден forwarding-заголовок не впливає на ключ.
func TestRateLimitUntrustedPeerIgnoresCFConnectingIP(t *testing.T) {
	resetTrustedProxies(t)

	limiter := middleware.NewIPRateLimiter(1, 1)
	h := middleware.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func(cfIP string) int {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "198.51.100.9:1234"
		req.Header.Set("CF-Connecting-IP", cfIP)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	if code := do("203.0.113.1"); code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if code := do("203.0.113.2"); code != http.StatusTooManyRequests {
		t.Fatalf("CF-Connecting-IP from an untrusted peer must be ignored: got %d, want 429", code)
	}
}

// TestRateLimitPeerWithoutPortCollapsesToOneBucket — chimiddleware.RealIP
// переписує RemoteAddr у bare-IP, і тоді реального peer'а вже не дістати.
// Fail-closed: усі такі запити ділять один бакет.
func TestRateLimitPeerWithoutPortCollapsesToOneBucket(t *testing.T) {
	resetTrustedProxies(t)

	limiter := middleware.NewIPRateLimiter(1, 1)
	h := middleware.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	codes := []int{}
	for _, rewritten := range []string{"1.2.3.4", "5.6.7.8", "9.10.11.12"} {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = rewritten // рівно так робить chi RealIP
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		codes = append(codes, rr.Code)
	}

	if codes[0] != http.StatusOK {
		t.Fatalf("expected 200 on the first request, got %d", codes[0])
	}
	for i, code := range codes[1:] {
		if code != http.StatusTooManyRequests {
			t.Fatalf("request %d: rewriting RemoteAddr must not mint a new bucket, got %d want 429", i+1, code)
		}
	}
}

func TestSetTrustedProxiesParsing(t *testing.T) {
	resetTrustedProxies(t)

	if err := middleware.SetTrustedProxies([]string{
		"10.0.0.0/8",
		"192.168.1.1", // без маски -> /32
		"2001:db8::/32",
		"::1",
		"  ", // порожні рядки ігноруються
	}); err != nil {
		t.Fatalf("expected a valid CIDR list to parse, got %v", err)
	}

	for _, bad := range []string{"not-a-cidr", "10.0.0.0/99", "10.0.0.0/", "example.com"} {
		if err := middleware.SetTrustedProxies([]string{bad}); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}

func TestRateLimiterNilAndZeroRatePassThrough(t *testing.T) {
	resetTrustedProxies(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "198.51.100.1:1234"
		rr := httptest.NewRecorder()
		middleware.RateLimitMiddleware(nil)(next).ServeHTTP(rr, req)
		if rr.Code != http.StatusTeapot {
			t.Fatalf("nil limiter must pass through, got %d", rr.Code)
		}
		rr2 := httptest.NewRecorder()
		middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(0, 0))(next).ServeHTTP(rr2, req)
		if rr2.Code != http.StatusTeapot {
			t.Fatalf("zero-rate limiter must pass through, got %d", rr2.Code)
		}
	}
}

// TestRateLimiterMapIsBounded — перевіряється у ratelimit_internal_test.go
// (потрібен доступ до внутрішньої мапи).

func TestRateLimitPeerRewriteAndForwardedHeadersTogether(t *testing.T) {
	resetTrustedProxies(t)

	limiter := middleware.NewIPRateLimiter(1, 1)
	h := middleware.RateLimitMiddleware(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Повна імітація chimiddleware.RealIP: RemoteAddr переписано з XFF.
	codes := make([]int, 0, 3)
	for _, ip := range []string{"1.2.3.4", "5.6.7.8", "9.10.11.12"} {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "198.51.100.1:1234"
		req.Header.Set("X-Forwarded-For", ip)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		codes = append(codes, rr.Code)

		// Те саме, але коли RealIP вже переписав RemoteAddr у bare-IP.
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.RemoteAddr = ip
		rr2 := httptest.NewRecorder()
		h.ServeHTTP(rr2, req2)
	}

	if codes[0] != http.StatusOK {
		t.Fatalf("expected 200 on the first request, got %d", codes[0])
	}
	for i, code := range codes[1:] {
		if code != http.StatusTooManyRequests {
			t.Fatalf("request %d: rotated XFF must not mint a bucket, got %d want 429", i+1, code)
		}
	}
}
