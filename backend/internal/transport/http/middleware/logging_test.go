package middleware_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/edhases/kadrbox-server/internal/logging"
	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
)

// captureRequestLogs перенаправляє логгер у буфер і повертає його.
func captureRequestLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := slog.Default()
	logging.SetDefault(slog.New(logging.NewHandler("debug", "json", buf)))
	t.Cleanup(func() { logging.SetDefault(prev) })
	return buf
}

// decodeLogRecords розбирає рядки JSON із буфера логів.
func decodeLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("запис не є JSON (%v): %s", err, line)
		}
		out = append(out, rec)
	}
	return out
}

// TestRequestLoggerEmitsExactlyOneRecord перевіряє, що на запит пишеться рівно один запис.
func TestRequestLoggerEmitsExactlyOneRecord(t *testing.T) {
	buf := captureRequestLogs(t)

	h := middleware.RequestContext(middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	})))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/content/search", nil))

	records := decodeLogRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("очікувався 1 запис логу, отримано %d: %s", len(records), buf.String())
	}

	rec := records[0]
	checks := map[string]any{
		"method": http.MethodPost,
		"path":   "/api/v1/content/search",
		"status": float64(http.StatusCreated),
		"bytes":  float64(5),
	}
	for key, want := range checks {
		if rec[key] != want {
			t.Errorf("поле %q: очікувалося %v, отримано %v", key, want, rec[key])
		}
	}
	for _, key := range []string{"duration_ms", "remote_ip", "trace_id"} {
		if _, ok := rec[key]; !ok {
			t.Errorf("у записі немає поля %q: %v", key, rec)
		}
	}
	if rec["msg"] != "http_request" {
		t.Errorf("очікувалося msg=http_request, отримано %v", rec["msg"])
	}
}

// TestRequestLoggerDoesNotLeakQuerySecrets гарантує, що query string з OAuth
// code/state та Telegram hash не потрапляє в лог.
func TestRequestLoggerDoesNotLeakQuerySecrets(t *testing.T) {
	buf := captureRequestLogs(t)

	const secretCode = "4/0AeanS0uSUPERCODETOKEN"
	const secretState = "st ate secret"
	const telegramHash = "tg-hash-secret"

	target := "/api/v1/auth/google/callback?code=" + secretCode +
		"&state=" + url.QueryEscape(secretState) +
		"&scope=email+profile&hash=" + telegramHash

	h := middleware.RequestContext(middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))

	out := buf.String()
	for _, secret := range []string{secretCode, secretState, telegramHash, "state=", "code=", "scope="} {
		if strings.Contains(out, secret) {
			t.Errorf("секрет %q потрапив у лог: %s", secret, out)
		}
	}

	records := decodeLogRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("очікувався 1 запис логу, отримано %d", len(records))
	}
	if records[0]["path"] != "/api/v1/auth/google/callback" {
		t.Errorf("очікувався чистий path, отримано %v", records[0]["path"])
	}
}

// TestRequestLoggerLevels перевіряє рівні: 5xx -> warn, 4xx/2xx -> info.
func TestRequestLoggerLevels(t *testing.T) {
	tests := []struct {
		name   string
		status int
		level  string
	}{
		{"2xx", http.StatusOK, "INFO"},
		{"4xx", http.StatusNotFound, "INFO"},
		{"5xx", http.StatusInternalServerError, "WARN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureRequestLogs(t)
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})
			h := middleware.RequestContext(middleware.RequestLogger(inner))

			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

			records := decodeLogRecords(t, buf)
			if len(records) != 1 {
				t.Fatalf("очікувався 1 запис, отримано %d", len(records))
			}
			if records[0]["level"] != tt.level {
				t.Errorf("очікувався рівень %s, отримано %v", tt.level, records[0]["level"])
			}
		})
	}
}

// TestRequestLoggerAttachesUserID перевіряє, що user_id з контексту потрапляє в лог.
func TestRequestLoggerAttachesUserID(t *testing.T) {
	buf := captureRequestLogs(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = middleware.WithUserID(r.Context(), "user-42")
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.RequestContext(middleware.RequestLogger(inner))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

	records := decodeLogRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("очікувався 1 запис, отримано %d", len(records))
	}
	if records[0]["user_id"] != "user-42" {
		t.Errorf("очікувався user_id=user-42, отримано %v", records[0]["user_id"])
	}
}

// hijackableRecorder імітує ResponseWriter з підтримкою WebSocket upgrade.
type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, errors.New("test hijack")
}

// TestRequestLoggerPreservesHijack гарантує, що обгортка не ламає WebSocket
// upgrade (gorilla вимагає http.Hijacker) і логує статус 101.
func TestRequestLoggerPreservesHijack(t *testing.T) {
	buf := captureRequestLogs(t)

	rec := &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
	h := middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("відсутній http.Hijacker на обгортці ResponseWriter")
			return
		}
		_, _, _ = hj.Hijack()
	}))

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ws/watch-party?room=A", nil))

	if !rec.hijacked {
		t.Fatal("Hijack не дійшов до справжнього ResponseWriter")
	}

	records := decodeLogRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("очікувався 1 запис, отримано %d", len(records))
	}
	if records[0]["status"] != float64(http.StatusSwitchingProtocols) {
		t.Errorf("очікувався статус 101 для WS, отримано %v", records[0]["status"])
	}
	if records[0]["path"] != "/api/v1/ws/watch-party" {
		t.Errorf("очікувався чистий path, отримано %v", records[0]["path"])
	}
}

// TestRequestLoggerWithoutRequestContext перевіряє, що логер працює і без
// RequestContext (trace_id порожній, запис все одно один).
func TestRequestLoggerWithoutRequestContext(t *testing.T) {
	buf := captureRequestLogs(t)

	h := middleware.RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/plain", nil))

	records := decodeLogRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("очікувався 1 запис, отримано %d", len(records))
	}
	if records[0]["trace_id"] != "" {
		t.Errorf("очікувався порожній trace_id, отримано %v", records[0]["trace_id"])
	}
}
