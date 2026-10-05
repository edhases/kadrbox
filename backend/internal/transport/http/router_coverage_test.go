package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	transporthttp "github.com/edhases/kadrbox-server/internal/transport/http"
	"github.com/edhases/kadrbox-server/internal/transport/ws"
)

// covTestRouter будує роутер з nil-залежностями хендлерів.
func covTestRouter() http.Handler {
	hub := ws.NewHub(nil)
	authH := transporthttp.NewAuthHandler(nil, nil, nil, "secret", "")
	syncH := transporthttp.NewSyncHandler(nil, nil)
	return transporthttp.NewRouter("secret", authH, syncH, hub, testAppURL)
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
