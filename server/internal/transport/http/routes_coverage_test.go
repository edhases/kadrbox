package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/email"
	"github.com/edhases/oxide-server/internal/provider"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/ws"
)

// Маршрути verify-email / resend-verification (додані в router.go):
// перевірка що вони зареєстровані як публічні і делегують хендлерам.

func qwAuthRouter(t *testing.T, svc *email.Service) http.Handler {
	t.Helper()
	reg := provider.NewRegistry()
	hub := ws.NewHub(nil)
	contentH := transporthttp.NewContentHandler(reg, nil)
	authH := transporthttp.NewAuthHandler(nil, nil, svc, "secret", "")
	syncH := transporthttp.NewSyncHandler(nil, nil)
	return transporthttp.NewRouter("secret", authH, contentH, syncH, hub)
}

func qwPost(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestCovHttpRouteVerifyEmailBadJSON(t *testing.T) {
	t.Setenv("RESEND_API", "")
	router := qwAuthRouter(t, email.NewService())

	rr := qwPost(t, router, "/api/v1/auth/verify-email", "{bad")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	var decoded map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&decoded); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if decoded["error"] != "token is required" {
		t.Errorf("unexpected error: %v", decoded)
	}
}

func TestCovHttpRouteResendUnconfigured(t *testing.T) {
	t.Setenv("RESEND_API", "")
	router := qwAuthRouter(t, email.NewService())

	rr := qwPost(t, router, "/api/v1/auth/resend-verification", `{"email":"u@x.com"}`)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestCovHttpRouteUnknownAuthPath(t *testing.T) {
	t.Setenv("RESEND_API", "")
	router := qwAuthRouter(t, email.NewService())

	rr := qwPost(t, router, "/api/v1/auth/no-such-endpoint", `{}`)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}
