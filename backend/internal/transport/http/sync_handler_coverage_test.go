package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	transporthttp "github.com/edhases/kadrbox-server/internal/transport/http"
	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
	"github.com/google/uuid"
)

// covSyncHandler будує SyncHandler з nil-репозиторіями.
// Безпечні лише шляхи до звернення до репозиторію: 401 без контексту
// та 400 при невалідному JSON з валідним контекстом.
func covSyncHandler() *transporthttp.SyncHandler {
	return transporthttp.NewSyncHandler(nil, nil)
}

// covAuthedSyncRequest додає userID у контекст запиту.
func covAuthedSyncRequest(method, target, body string) *http.Request {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, uuid.New())
	return req.WithContext(ctx)
}

// TestCovHttpSyncUnauthorized — таблиця 5 маршрутів з порожнім контекстом дає 401.
// Коментар: clamp limit у GetHistory та перезапис item.UserID недосяжні
// без репозиторію (виклики з валідним контекстом впали б у nil-panic),
// тому тут покрито лише гілку відсутності користувача.
func TestCovHttpSyncUnauthorized(t *testing.T) {
	h := covSyncHandler()
	cases := []struct {
		name    string
		method  string
		target  string
		handler func(w http.ResponseWriter, r *http.Request)
	}{
		{"GetHistory", http.MethodGet, "/api/v1/sync/history", h.GetHistory},
		{"SaveProgress", http.MethodPost, "/api/v1/sync/history", h.SaveProgress},
		{"GetContinueWatching", http.MethodGet, "/api/v1/sync/continue-watching", h.GetContinueWatching},
		{"GetFavorites", http.MethodGet, "/api/v1/sync/favorites", h.GetFavorites},
		{"ToggleFavorite", http.MethodPost, "/api/v1/sync/favorites/toggle", h.ToggleFavorite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			rr := httptest.NewRecorder()

			tc.handler(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Errorf("очікувався 401, отримано %d", rr.Code)
			}
		})
	}
}

// TestCovHttpSaveProgressInvalidJSON — валідний контекст, сміттєве тіло дає 400.
func TestCovHttpSaveProgressInvalidJSON(t *testing.T) {
	h := covSyncHandler()
	req := covAuthedSyncRequest(http.MethodPost, "/api/v1/sync/history", "{oops")
	rr := httptest.NewRecorder()

	h.SaveProgress(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("очікувався 400, отримано %d", rr.Code)
	}
}

// TestCovHttpToggleFavoriteInvalidJSON — валідний контекст, сміттєве тіло дає 400.
func TestCovHttpToggleFavoriteInvalidJSON(t *testing.T) {
	h := covSyncHandler()
	req := covAuthedSyncRequest(http.MethodPost, "/api/v1/sync/favorites/toggle", "{oops")
	rr := httptest.NewRecorder()

	h.ToggleFavorite(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("очікувався 400, отримано %d", rr.Code)
	}
}
