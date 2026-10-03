package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/ws"
)

// covTestRouter будує роутер з nil-залежностями хендлерів і порожнім реєстром.
func covTestRouter() http.Handler {
	reg := provider.NewRegistry()
	hub := ws.NewHub(nil)
	contentH := transporthttp.NewContentHandler(reg, nil)
	authH := transporthttp.NewAuthHandler(nil, nil, nil, "secret", "")
	syncH := transporthttp.NewSyncHandler(nil, nil)
	return transporthttp.NewRouter("secret", authH, contentH, syncH, hub, testAppURL)
}

// TestCovHttpRouterProtectedRequireAuth — 6 захищених маршрутів без заголовка дають 401.
// Валідний токен на них НЕ тестуємо: за nil-репозиторіїв хендлер впав би у nil-panic.
func TestCovHttpRouterProtectedRequireAuth(t *testing.T) {
	router := covTestRouter()
	cases := []struct {
		name   string
		method string
		target string
	}{
		{"Me", http.MethodGet, "/api/v1/auth/me"},
		{"GetHistory", http.MethodGet, "/api/v1/sync/history"},
		{"SaveProgress", http.MethodPost, "/api/v1/sync/history"},
		{"GetContinueWatching", http.MethodGet, "/api/v1/sync/continue-watching"},
		{"GetFavorites", http.MethodGet, "/api/v1/sync/favorites"},
		{"ToggleFavorite", http.MethodPost, "/api/v1/sync/favorites/toggle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: очікувався 401, отримано %d", tc.method, tc.target, rr.Code)
			}
		})
	}
}

// TestCovHttpRouterPublicContentAccessible — публічні content-маршрути не вимагають
// авторизації: без q має бути 400 (а НЕ 401), без параметрів details — теж 400.
func TestCovHttpRouterPublicContentAccessible(t *testing.T) {
	router := covTestRouter()
	cases := []struct {
		name   string
		target string
	}{
		{"Search без q", "/api/v1/content/search"},
		{"Details без параметрів", "/api/v1/content/details"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: очікувався 400 (публічний маршрут), отримано %d", tc.target, rr.Code)
			}
		})
	}
}

// TestCovHttpRouterWatchPartyWithoutToken — ХАРАКТЕРИЗАЦІЯ: WS-ендпоінт
// знаходиться поза auth-групою (router.go:48), JWT там не перевіряється,
// тому без токена буде НЕ 401 (фактично 400 від невдалого upgrade handshake).
func TestCovHttpRouterWatchPartyWithoutToken(t *testing.T) {
	router := covTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/watch-party?room=A&user_id=1", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code == http.StatusUnauthorized {
		t.Errorf("характеризація порушена: WS-ендпоінт не має повертати 401, отримано %d", rr.Code)
	}
}
