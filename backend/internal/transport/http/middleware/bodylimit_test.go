package middleware_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

// TestBodyLimitRejectsOversizedContentLength перевіряє 413 для відомого розміру.
func TestBodyLimitRejectsOversizedContentLength(t *testing.T) {
	called := false
	h := middleware.BodyLimit(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/history",
		strings.NewReader(strings.Repeat("a", 2048)))

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("очікувався 413, отримано %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("очікувався JSON content type, отримано %q", ct)
	}
	if called {
		t.Error("хендлер не мав викликатися для завеликого тіла")
	}

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("не JSON: %v (%s)", err, rr.Body.String())
	}
	if body["error"] == "" {
		t.Errorf("очікувався ключ error, отримано %v", body)
	}
}

// TestBodyLimitAllowsBodyUnderLimit перевіряє проходження нормальних тіл.
func TestBodyLimitAllowsBodyUnderLimit(t *testing.T) {
	payload := `{"progress":1}`
	var got string
	h := middleware.BodyLimit(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("помилка читання тіла: %v", err)
		}
		got = string(b)
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/sync/history", strings.NewReader(payload)))

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d", rr.Code)
	}
	if got != payload {
		t.Errorf("тіло не дійшло незмінним: %q", got)
	}
}

// TestBodyLimitTruncatesChunkedBody перевіряє, що MaxBytesReader обриває потокове
// тіло, коли Content-Length невідомий (-1).
func TestBodyLimitTruncatesChunkedBody(t *testing.T) {
	var readErr error
	h := middleware.BodyLimit(16)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync/history",
		strings.NewReader(strings.Repeat("b", 512)))
	req.ContentLength = -1 // імітуємо chunked transfer-encoding

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if readErr == nil {
		t.Fatal("очікувалася помилка MaxBytesReader при читанні понад ліміт")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(readErr, &maxErr) {
		t.Errorf("очікувався *http.MaxBytesError, отримано %v", readErr)
	} else if maxErr.Limit != 16 {
		t.Errorf("очікувався Limit=16, отримано %d", maxErr.Limit)
	}
}

// TestBodyLimitDisabled перевіряє, що max <= 0 вимикає обмеження.
func TestBodyLimitDisabled(t *testing.T) {
	called := false
	h := middleware.BodyLimit(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("c", 4096)))
	h.ServeHTTP(rr, req)

	if !called || rr.Code != http.StatusOK {
		t.Errorf("обмеження вимкнено неправильно: called=%v code=%d", called, rr.Code)
	}
}

// TestDefaultMaxBodyBytes перевіряє документований дефолт (1 MiB).
func TestDefaultMaxBodyBytes(t *testing.T) {
	if middleware.DefaultMaxBodyBytes != 1<<20 {
		t.Errorf("очікувався дефолт 1 MiB, отримано %d", middleware.DefaultMaxBodyBytes)
	}
}
