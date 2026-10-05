package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	transporthttp "github.com/edhases/kadrbox-server/internal/transport/http"
)

// covAuthHandler будує AuthHandler з nil-залежностями.
// Безпечні лише шляхи, що повертаються до звернення до репозиторію.
func covAuthHandler() *transporthttp.AuthHandler {
	return transporthttp.NewAuthHandler(nil, nil, nil, "secret", "")
}

// TestCovHttpRegisterInvalidJSON — сміттєве тіло дає 400 "invalid request payload".
func TestCovHttpRegisterInvalidJSON(t *testing.T) {
	h := covAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader("{oops"))
	rr := httptest.NewRecorder()

	h.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("очікувався 400, отримано %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request payload") {
		t.Errorf("очікувалось повідомлення invalid request payload, отримано %q", rr.Body.String())
	}
}

// TestCovHttpRegisterMissingFields — таблиця: відсутність кожного поля дає 400.
func TestCovHttpRegisterMissingFields(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"без email", `{"password":"pw123456","username":"cov-user"}`},
		{"без password", `{"email":"cov@example.com","username":"cov-user"}`},
		{"без username", `{"email":"cov@example.com","password":"pw123456"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := covAuthHandler()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()

			h.Register(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("очікувався 400, отримано %d", rr.Code)
			}
		})
	}
}

// TestCovHttpLoginInvalidJSON — сміттєве тіло дає 400.
func TestCovHttpLoginInvalidJSON(t *testing.T) {
	h := covAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{oops"))
	rr := httptest.NewRecorder()

	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("очікувався 400, отримано %d", rr.Code)
	}
}

// TestCovHttpRefreshInvalidJSON — сміттєве тіло дає 400.
func TestCovHttpRefreshInvalidJSON(t *testing.T) {
	h := covAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader("{oops"))
	rr := httptest.NewRecorder()

	h.Refresh(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("очікувався 400, отримано %d", rr.Code)
	}
}

// TestCovHttpMeUnauthorized — без користувача в контексті має бути 401.
func TestCovHttpMeUnauthorized(t *testing.T) {
	h := covAuthHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rr := httptest.NewRecorder()

	h.Me(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("очікувався 401, отримано %d", rr.Code)
	}
}
