package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/auth"
	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
	"github.com/google/uuid"
)

// covMiddlewareDoRequest проганяє запит через AuthMiddleware з заданим заголовком.
func covMiddlewareDoRequest(t *testing.T, secret string, authHeader string, next http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	h := middleware.AuthMiddleware(secret)(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// covMiddlewareNextOK повертає хендлер-заглушку, що фіксує виклик і повертає 200.
func covMiddlewareNextOK(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

// TestCovMiddlewareMissingHeader перевіряє 401 без заголовка Authorization.
func TestCovMiddlewareMissingHeader(t *testing.T) {
	called := false
	rr := covMiddlewareDoRequest(t, "secret", "", covMiddlewareNextOK(&called))

	// Без заголовка middleware має відповісти 401 і не викликати next.
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("очікувався статус 401, отримано %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "missing authorization header") {
		t.Fatalf("тіло має містити %q, отримано %q", "missing authorization header", rr.Body.String())
	}
	if called {
		t.Fatalf("next не мав викликатися без заголовка")
	}
}

// TestCovMiddlewareMalformedHeader перевіряє 401 при невірному форматі заголовка.
func TestCovMiddlewareMalformedHeader(t *testing.T) {
	// Готуємо валідний токен для кейса з малої літери "bearer".
	valid, err := auth.GenerateAccessToken(uuid.New(), "t@e.st", "user", "secret", time.Hour)
	if err != nil {
		t.Fatalf("не вдалося згенерувати токен: %v", err)
	}

	tests := []struct {
		name   string
		header string
	}{
		{"Bearer без токена", "Bearer"},
		{"чужа схема", "Basic xyz"},
		{"три частини", "Bearer a b"},
		{"мала літера bearer", "bearer " + valid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			rr := covMiddlewareDoRequest(t, "secret", tt.header, covMiddlewareNextOK(&called))

			// Формат має бути строго "Bearer <token>".
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("очікувався статус 401, отримано %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "invalid authorization header format") {
				t.Fatalf("тіло має містити %q, отримано %q", "invalid authorization header format", rr.Body.String())
			}
			if called {
				t.Fatalf("next не мав викликатися при невірному форматі")
			}
		})
	}
}

// TestCovMiddlewareInvalidToken перевіряє 401 при невалідному або простроченому токені.
func TestCovMiddlewareInvalidToken(t *testing.T) {
	// Токен, підписаний чужим секретом.
	wrongSecret, err := auth.GenerateAccessToken(uuid.New(), "t@e.st", "user", "other-secret", time.Hour)
	if err != nil {
		t.Fatalf("не вдалося згенерувати токен: %v", err)
	}
	// Прострочений токен.
	expired, err := auth.GenerateAccessToken(uuid.New(), "t@e.st", "user", "secret", -time.Hour)
	if err != nil {
		t.Fatalf("не вдалося згенерувати токен: %v", err)
	}

	tests := []struct {
		name   string
		header string
	}{
		{"сміття", "Bearer garbage"},
		{"чужий секрет", "Bearer " + wrongSecret},
		{"прострочений", "Bearer " + expired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			rr := covMiddlewareDoRequest(t, "secret", tt.header, covMiddlewareNextOK(&called))

			// Невалідний токен має давати 401 без виклику next.
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("очікувався статус 401, отримано %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "invalid or expired token") {
				t.Fatalf("тіло має містити %q, отримано %q", "invalid or expired token", rr.Body.String())
			}
			if called {
				t.Fatalf("next не мав викликатися при невалідному токені")
			}
		})
	}
}

// TestCovMiddlewareValidToken перевіряє, що валідний токен пропускає запит і кладе UserID у контекст.
func TestCovMiddlewareValidToken(t *testing.T) {
	id := uuid.New()
	token, err := auth.GenerateAccessToken(id, "t@e.st", "user", "secret", time.Hour)
	if err != nil {
		t.Fatalf("не вдалося згенерувати токен: %v", err)
	}

	var gotID uuid.UUID
	var gotOK bool
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Дістаємо UserID з контексту всередині next-хендлера.
		gotID, gotOK = middleware.GetUserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	rr := covMiddlewareDoRequest(t, "secret", "Bearer "+token, next)

	// Валідний токен має викликати next зі статусом 200.
	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався статус 200, отримано %d", rr.Code)
	}
	if !called {
		t.Fatalf("next мав викликатися для валідного токена")
	}
	if !gotOK {
		t.Fatalf("GetUserIDFromContext повернув ok=false для валідного токена")
	}
	if gotID != id {
		t.Fatalf("очікувався UserID %s, отримано %s", id, gotID)
	}
}

// TestCovMiddlewareRolePropagated перевіряє, що роль з токена потрапляє в контекст.
func TestCovMiddlewareRolePropagated(t *testing.T) {
	// RoleKey пишеться, але жоден хендлер його не читає (рольової авторизації немає) — відома прогалина.
	token, err := auth.GenerateAccessToken(uuid.New(), "t@e.st", "user", "secret", time.Hour)
	if err != nil {
		t.Fatalf("не вдалося згенерувати токен: %v", err)
	}

	var gotRole string
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Читаємо роль напряму з контексту за ключем RoleKey.
		gotRole, gotOK = r.Context().Value(middleware.RoleKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	rr := covMiddlewareDoRequest(t, "secret", "Bearer "+token, next)

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався статус 200, отримано %d", rr.Code)
	}
	if !gotOK {
		t.Fatalf("роль не знайдена в контексті під RoleKey")
	}
	if gotRole != "user" {
		t.Fatalf("очікувалася роль %q, отримано %q", "user", gotRole)
	}
}

// TestCovMiddlewareGetUserIDFromContext перевіряє поведінку геттера на порожньому та чужому значеннях.
func TestCovMiddlewareGetUserIDFromContext(t *testing.T) {
	// Порожній контекст не містить UserID.
	id, ok := middleware.GetUserIDFromContext(context.Background())
	if ok {
		t.Fatalf("очікувався ok=false для порожнього контексту")
	}
	if id != uuid.Nil {
		t.Fatalf("очікувався uuid.Nil, отримано %s", id)
	}

	// Чужий тип значення під UserIDKey не має розпізнаватися як UUID.
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, "not-a-uuid")
	if _, ok := middleware.GetUserIDFromContext(ctx); ok {
		t.Fatalf("очікувався ok=false для чужого типу значення")
	}
}
