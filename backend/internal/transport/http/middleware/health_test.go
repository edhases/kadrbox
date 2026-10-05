package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

// stubPing будує probe-заглушку: повертає err після опційної затримки.
func stubPing(err error, delay time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return err
	}
}

func decodeJSONBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("очікувався JSON content type, отримано %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("не JSON: %v (%s)", err, rr.Body.String())
	}
	return body
}

// TestLivezAlwaysOK перевіряє, що liveness не залежить від залежностей.
func TestLivezAlwaysOK(t *testing.T) {
	h := middleware.New(stubPing(errors.New("redis down"), 0), "oxide-server").
		WithPostgresPing(stubPing(errors.New("pg down"), 0))

	rr := httptest.NewRecorder()
	h.Livez(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d", rr.Code)
	}
	body := decodeJSONBody(t, rr)
	if body["status"] != "ok" || body["service"] != "oxide-server" {
		t.Errorf("очікувався {ok, oxide-server}, отримано %v", body)
	}
}

// TestReadyzReady перевіряє 200, коли обидві залежності живі.
func TestReadyzReady(t *testing.T) {
	h := middleware.New(stubPing(nil, 0), "oxide-server").
		WithPostgresPing(stubPing(nil, 0))

	rr := httptest.NewRecorder()
	h.Readyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d (%s)", rr.Code, rr.Body.String())
	}
	if body := decodeJSONBody(t, rr); body["status"] != "ready" {
		t.Errorf("очікувався status=ready, отримано %v", body)
	}
}

// TestReadyzDegraded перевіряє, що падіння однієї залежності повертає 503 із
// назвою саме цієї залежності.
func TestReadyzDegraded(t *testing.T) {
	tests := []struct {
		name     string
		pgErr    error
		redisErr error
		wantDep  string
	}{
		{"postgres down", errors.New("pg down"), nil, "postgres"},
		{"redis down", nil, errors.New("redis down"), "redis"},
		{"both down", errors.New("pg down"), errors.New("redis down"), "postgres"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := middleware.New(stubPing(tt.redisErr, 0), "oxide-server").
				WithPostgresPing(stubPing(tt.pgErr, 0))

			rr := httptest.NewRecorder()
			h.Readyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("очікувався 503, отримано %d", rr.Code)
			}
			body := decodeJSONBody(t, rr)
			if body["status"] != "unavailable" || body["dependency"] != tt.wantDep {
				t.Errorf("очікувався unavailable/%s, отримано %v", tt.wantDep, body)
			}
		})
	}
}

// TestReadyzFailsClosedWithoutProbes перевіряє, що без налаштованих пінгів
// сервіс ніколи не повідомляє про готовність.
func TestReadyzFailsClosedWithoutProbes(t *testing.T) {
	rr := httptest.NewRecorder()
	middleware.New(nil, "oxide-server").Readyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("очікувався 503, отримано %d", rr.Code)
	}
	if body := decodeJSONBody(t, rr); body["dependency"] != "postgres" {
		t.Errorf("очікувався dependency=postgres, отримано %v", body)
	}
}

// TestReadyzTimesOutSlowDependency перевіряє, що повільна залежність обривається
// таймаутом (2s), а не блокує probe назавжди.
func TestReadyzTimesOutSlowDependency(t *testing.T) {
	if middleware.ReadinessTimeout != 2*time.Second {
		t.Fatalf("очікувався таймаут 2s, отримано %s", middleware.ReadinessTimeout)
	}

	h := middleware.New(stubPing(nil, 0), "oxide-server").
		WithPostgresPing(stubPing(nil, 3*time.Second))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	// Контекст запиту обмежено швидше за таймаут readiness, щоб тест був швидким.
	ctx, cancel := context.WithTimeout(req.Context(), 50*time.Millisecond)
	defer cancel()

	rr := httptest.NewRecorder()
	h.Readyz(rr, req.WithContext(ctx))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("очікувався 503, отримано %d", rr.Code)
	}
}

// TestHealthServeHTTPDispatch перевіряє диспетчеризацію за шляхом (для r.Mount).
func TestHealthServeHTTPDispatch(t *testing.T) {
	h := middleware.New(stubPing(nil, 0), "oxide-server").WithPostgresPing(stubPing(nil, 0))

	tests := []struct {
		path string
		code int
	}{
		{"/healthz", http.StatusOK},
		{"/health", http.StatusOK},
		{"/readyz", http.StatusOK},
		{"/nope", http.StatusNotFound},
	}

	for _, tt := range tests {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if rr.Code != tt.code {
			t.Errorf("%s: очікувався %d, отримано %d", tt.path, tt.code, rr.Code)
		}
		if tt.code == http.StatusNotFound {
			if body := decodeJSONBody(t, rr); body["error"] == "" {
				t.Errorf("%s: очікувався JSON error", tt.path)
			}
		}
	}
}

// TestSetProbesAreProcessWide перевіряє сеттери пінгів, якими користується main.go.
func TestSetProbesAreProcessWide(t *testing.T) {
	t.Cleanup(func() {
		middleware.SetPostgresPing(nil)
		middleware.SetRedisPing(nil)
	})

	if middleware.RedisPing() != nil || middleware.PostgresPing() != nil {
		t.Fatal("очікувалися nil-пінги за замовчуванням")
	}

	middleware.SetRedisPing(stubPing(nil, 0))
	middleware.SetPostgresPing(stubPing(nil, 0))

	if middleware.RedisPing() == nil || middleware.PostgresPing() == nil {
		t.Fatal("сеттери не зберегли пінги")
	}

	// New підхоплює процесні пінги, тож main.go достатньо викликати сеттери.
	rr := httptest.NewRecorder()
	middleware.New(middleware.RedisPing(), "oxide-server").
		Readyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d (%s)", rr.Code, rr.Body.String())
	}
}

// pgxPoolStub ілюструє, що Pinger задовольняється пулом з одним методом Ping.
type pgxPoolStub struct{ err error }

func (p pgxPoolStub) Ping(ctx context.Context) error { return p.err }

func TestPingerInterfaceIsSatisfiedBySingleMethod(t *testing.T) {
	var p middleware.Pinger = pgxPoolStub{}
	h := middleware.New(stubPing(nil, 0), "oxide-server").
		WithPostgresPing(p.Ping)

	rr := httptest.NewRecorder()
	h.Readyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d", rr.Code)
	}
}
