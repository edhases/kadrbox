package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
)

// TestRequestContextSetsHeaderAndTraceID перевіряє, що trace id генерується,
// доступний хендлеру через TraceID і повертається в X-Request-Id.
func TestRequestContextSetsHeaderAndTraceID(t *testing.T) {
	var handlerTraceID string
	h := middleware.RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerTraceID = middleware.TraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	header := rr.Header().Get("X-Request-Id")
	if header == "" {
		t.Fatal("заголовок X-Request-Id не встановлено")
	}
	if handlerTraceID != header {
		t.Errorf("TraceID у контексті (%q) != X-Request-Id (%q)", handlerTraceID, header)
	}
	if _, err := uuid.Parse(header); err != nil {
		t.Errorf("очікувався UUID у X-Request-Id, отримано %q: %v", header, err)
	}
}

// TestRequestContextReusesIncomingRequestID перевіряє, що вхідний X-Request-Id
// не перегенеровується (кореляція клієнт -> сервер).
func TestRequestContextReusesIncomingRequestID(t *testing.T) {
	var handlerTraceID string
	chain := chimiddleware.RequestID(
		middleware.RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerTraceID = middleware.TraceID(r.Context())
			w.WriteHeader(http.StatusOK)
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-Id", "incoming-trace-123")
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Request-Id"); got != "incoming-trace-123" {
		t.Errorf("очікувався збережений X-Request-Id, отримано %q", got)
	}
	if handlerTraceID != "incoming-trace-123" {
		t.Errorf("очікувався trace id з chi RequestID, отримано %q", handlerTraceID)
	}
}

// TestRequestContextGeneratesUUIDWithoutChi перевіряє, що без chi RequestID все
// одно генерується коректний UUID.
func TestRequestContextGeneratesUUIDWithoutChi(t *testing.T) {
	h := middleware.RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

	if _, err := uuid.Parse(rr.Header().Get("X-Request-Id")); err != nil {
		t.Fatalf("очікувався UUID, отримано %q: %v", rr.Header().Get("X-Request-Id"), err)
	}
}

// TestTraceIDOutsideRequest перевіряє поведінку геттера на контексті без запиту.
func TestTraceIDOutsideRequest(t *testing.T) {
	if id := middleware.TraceID(context.Background()); id != "" {
		t.Errorf("очікувався порожній trace id, отримано %q", id)
	}
	if _, ok := middleware.UserID(context.Background()); ok {
		t.Error("очікувалося ok=false для контексту без запиту")
	}
}

// TestWithUserIDWithoutRequestState перевіряє збереження user_id, коли
// RequestContext не встановлено.
func TestWithUserIDWithoutRequestState(t *testing.T) {
	ctx := middleware.WithUserID(context.Background(), "u-1")
	id, ok := middleware.UserID(ctx)
	if !ok || id != "u-1" {
		t.Errorf("очікувався user_id=u-1, отримано %q (ok=%v)", id, ok)
	}
}

// TestUserIDFallsBackToAuthContext перевіряє сумісність із UserIDKey з AuthMiddleware.
func TestUserIDFallsBackToAuthContext(t *testing.T) {
	id := uuid.New()
	h := middleware.RequestContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), middleware.UserIDKey, id)
		got, ok := middleware.UserID(ctx)
		if !ok || got != id.String() {
			t.Errorf("очікувався user_id=%s, отримано %q (ok=%v)", id, got, ok)
		}
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
}

// TestWithUserIDPropagatesThroughDerivedContext перевіряє, що user_id,
// записаний у похідному контексті, все одно видно RequestLogger.
func TestWithUserIDPropagatesThroughDerivedContext(t *testing.T) {
	buf := captureRequestLogs(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Похідний контекст: значення не повертається у RequestContext.
		ctx := middleware.WithUserID(r.Context(), "derived-user")
		_ = ctx
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.RequestContext(middleware.RequestLogger(inner))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

	records := decodeLogRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("очікувався 1 запис, отримано %d", len(records))
	}
	if records[0]["user_id"] != "derived-user" {
		t.Errorf("очікувався user_id=derived-user, отримано %v", records[0]["user_id"])
	}
}

// TestRequestContextTraceIDInLogRecord перевіряє, що X-Request-Id і trace_id у
// лозі збігаються.
func TestRequestContextTraceIDInLogRecord(t *testing.T) {
	buf := captureRequestLogs(t)

	h := chimiddleware.RequestID(middleware.RequestContext(middleware.RequestLogger(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/popular", nil)
	req.Header.Set("X-Request-Id", "corr-42")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	records := decodeLogRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("очікувався 1 запис, отримано %d", len(records))
	}
	if records[0]["trace_id"] != "corr-42" {
		t.Errorf("очікувався trace_id=corr-42, отримано %v", records[0]["trace_id"])
	}
	if rr.Header().Get("X-Request-Id") != "corr-42" {
		t.Errorf("очікувався X-Request-Id=corr-42, отримано %q", rr.Header().Get("X-Request-Id"))
	}
}

// TestHealthHandlerImplementsHTTPHandler гарантує, що HealthHandler можна
// використати як звичайний http.Handler (r.Mount).
func TestHealthHandlerImplementsHTTPHandler(t *testing.T) {
	var h http.Handler = middleware.New(nil, "kadrbox-server")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("не JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("очікувався status=ok, отримано %v", body)
	}
}
