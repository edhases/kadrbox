package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

func TestRateLimitMiddleware(t *testing.T) {
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
}
