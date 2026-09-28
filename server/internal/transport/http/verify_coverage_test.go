package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/email"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

// NOTE: ці тести покривають код паралельного агента (email-верифікація
// в auth_handler.go). Гілки без БД: валідація вхідних даних і "сервіс
// не налаштовано". Гілки з userRepo/redis лишаються поза досяжністю
// без інтеграційного оточення.

func covVerifyHandler() *transporthttp.AuthHandler {
	return transporthttp.NewAuthHandler(nil, nil, nil, "secret")
}

func covVerifyError(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	return body["error"]
}

func TestCovHttpVerifyEmailInvalidJSON(t *testing.T) {
	h := covVerifyHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-email",
		strings.NewReader("{not-json"))
	rr := httptest.NewRecorder()

	h.VerifyEmail(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if msg := covVerifyError(t, rr); msg != "token is required" {
		t.Errorf("unexpected error: %q", msg)
	}
}

func TestCovHttpVerifyEmailEmptyToken(t *testing.T) {
	h := covVerifyHandler()
	for _, body := range []string{`{}`, `{"token":""}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-email",
			strings.NewReader(body))
		rr := httptest.NewRecorder()

		h.VerifyEmail(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, rr.Code)
		}
	}
}

func TestCovHttpResendUnconfigured(t *testing.T) {
	// RESEND_API порожній → NewService повертає неналаштований сервіс,
	// жодного звернення до БД чи мережі.
	t.Setenv("RESEND_API", "")
	svc := email.NewService()
	if svc.IsConfigured() {
		t.Fatal("precondition: service must be unconfigured")
	}
	h := transporthttp.NewAuthHandler(nil, nil, svc, "secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/resend-verification",
		strings.NewReader(`{"email":"u@example.com"}`))
	rr := httptest.NewRecorder()

	h.ResendVerification(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
	if msg := covVerifyError(t, rr); msg != "email service not configured" {
		t.Errorf("unexpected error: %q", msg)
	}
}

func TestCovHttpResendConfiguredInvalidJSON(t *testing.T) {
	// Сервіс "налаштований" ключем-заглушкою, але тіло невалідне —
	// повертаємось до звернення до БД, мережі немає.
	t.Setenv("RESEND_API", "re_test_key")
	svc := email.NewService()
	if !svc.IsConfigured() {
		t.Fatal("precondition: service must be configured")
	}
	h := transporthttp.NewAuthHandler(nil, nil, svc, "secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/resend-verification",
		strings.NewReader("{not-json"))
	rr := httptest.NewRecorder()

	h.ResendVerification(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if msg := covVerifyError(t, rr); msg != "email is required" {
		t.Errorf("unexpected error: %q", msg)
	}
}

func TestCovHttpResendConfiguredEmptyEmail(t *testing.T) {
	t.Setenv("RESEND_API", "re_test_key")
	svc := email.NewService()
	h := transporthttp.NewAuthHandler(nil, nil, svc, "secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/resend-verification",
		strings.NewReader(`{}`))
	rr := httptest.NewRecorder()

	h.ResendVerification(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
