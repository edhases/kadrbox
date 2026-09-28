package http_test

// Наскрізний flow проти справжніх Postgres + Redis.
// Локально без TEST_POSTGRES_DSN / TEST_REDIS_ADDR — пропуск (t.Skip).
// У CI виконується job із services. Email-сервіс неналаштований
// (RESEND_API="") → реєстрація йде гілкою auto-verify.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/email"
	"github.com/edhases/oxide-server/internal/provider"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/ws"
)

type covFlowRig struct {
	router http.Handler
}

func covFlowRigSetup(t *testing.T) *covFlowRig {
	t.Helper()
	if os.Getenv("TEST_POSTGRES_DSN") == "" || os.Getenv("TEST_REDIS_ADDR") == "" {
		t.Skip("TEST_POSTGRES_DSN/TEST_REDIS_ADDR not set — integration test skipped")
	}
	// Детерміновано: auto-verify гілка реєстрації.
	t.Setenv("RESEND_API", "")

	pool, err := postgres.InitDB(context.Background(), os.Getenv("TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(pool.Close)

	redisClient, err := redisRepo.NewRedisClient(os.Getenv("TEST_REDIS_ADDR"), "")
	if err != nil {
		t.Fatalf("redis connect failed: %v", err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })

	emailSvc := email.NewService()
	userRepo := postgres.NewUserRepository(pool)
	historyRepo := postgres.NewHistoryRepository(pool)
	favoritesRepo := postgres.NewFavoritesRepository(pool)
	cacheRepo := postgres.NewCacheRepository(pool)

	reg := provider.NewRegistry()
	authH := transporthttp.NewAuthHandler(userRepo, redisClient, emailSvc, "test-secret", "")
	contentH := transporthttp.NewContentHandler(reg, cacheRepo)
	syncH := transporthttp.NewSyncHandler(historyRepo, favoritesRepo)
	hub := ws.NewHub(redisClient)

	return &covFlowRig{
		router: transporthttp.NewRouter("test-secret", authH, contentH, syncH, hub),
	}
}

func (r *covFlowRig) do(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	r.router.ServeHTTP(rr, req)
	return rr
}

func covFlowDecode[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode failed (status %d): %v", rr.Code, err)
	}
	return v
}

func TestCovHttpFlowRegisterLoginRefresh(t *testing.T) {
	rig := covFlowRigSetup(t)
	mail := fmt.Sprintf("flow_%d@x.com", time.Now().UnixNano())

	// 1. Register → 200, auto-verified (email не налаштовано).
	rr := rig.do(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": mail, "password": "secret123", "username": "flowuser",
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("register: expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	reg := covFlowDecode[transporthttp.AuthResponse](t, rr)
	if reg.AccessToken == "" || reg.RefreshToken == "" {
		t.Fatal("register: missing tokens")
	}
	if !reg.User.IsVerified {
		t.Error("register: expected auto-verified user")
	}

	// 2. Дублікат → 409.
	rr = rig.do(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": mail, "password": "secret123", "username": "flowuser",
	}, "")
	if rr.Code != http.StatusConflict {
		t.Errorf("duplicate register: expected 409, got %d", rr.Code)
	}

	// 3. Me з токеном → 200.
	rr = rig.do(t, http.MethodGet, "/api/v1/auth/me", nil, reg.AccessToken)
	if rr.Code != http.StatusOK {
		t.Fatalf("me: expected 200, got %d", rr.Code)
	}
	me := covFlowDecode[domain.User](t, rr)
	if me.Email != mail {
		t.Errorf("me: unexpected user %+v", me)
	}

	// 4. Login з неправильним паролем → 401; з правильним → 200.
	rr = rig.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": mail, "password": "wrong",
	}, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("bad login: expected 401, got %d", rr.Code)
	}
	rr = rig.do(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": mail, "password": "secret123",
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", rr.Code)
	}
	login := covFlowDecode[transporthttp.AuthResponse](t, rr)

	// 5. Refresh ротація: новий токен працює, старий — 401.
	rr = rig.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": login.RefreshToken,
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh: expected 200, got %d", rr.Code)
	}
	refreshed := covFlowDecode[transporthttp.AuthResponse](t, rr)
	if refreshed.RefreshToken == login.RefreshToken {
		t.Error("refresh: token was not rotated")
	}
	rr = rig.do(t, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
		"refresh_token": login.RefreshToken,
	}, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("reused refresh: expected 401, got %d", rr.Code)
	}

	// 6. VerifyEmail з лівим токеном → 400 (гілка БД).
	rr = rig.do(t, http.MethodPost, "/api/v1/auth/verify-email", map[string]string{
		"token": "nope",
	}, "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad verify token: expected 400, got %d", rr.Code)
	}
}

func TestCovHttpFlowSync(t *testing.T) {
	rig := covFlowRigSetup(t)
	mail := fmt.Sprintf("sync_%d@x.com", time.Now().UnixNano())

	rr := rig.do(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": mail, "password": "secret123", "username": "syncuser",
	}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("register failed: %d", rr.Code)
	}
	token := covFlowDecode[transporthttp.AuthResponse](t, rr).AccessToken

	// SaveProgress → історія.
	rr = rig.do(t, http.MethodPost, "/api/v1/sync/history", map[string]any{
		"media_id": "m1", "provider_id": "uakino", "title": "Матриця",
		"position_ms": 50, "duration_ms": 100,
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("save progress: expected 200, got %d", rr.Code)
	}

	rr = rig.do(t, http.MethodGet, "/api/v1/sync/history?limit=10", nil, token)
	hist := covFlowDecode[[]domain.WatchHistory](t, rr)
	if len(hist) != 1 || hist[0].Title != "Матриця" || hist[0].PositionMs != 50 {
		t.Fatalf("unexpected history: %+v", hist)
	}

	rr = rig.do(t, http.MethodGet, "/api/v1/sync/continue-watching", nil, token)
	cont := covFlowDecode[[]domain.WatchHistory](t, rr)
	if len(cont) != 1 || cont[0].MediaID != "m1" {
		t.Errorf("unexpected continue-watching: %+v", cont)
	}

	// Toggle on → favorites містить; toggle off → порожньо.
	rr = rig.do(t, http.MethodPost, "/api/v1/sync/favorites/toggle", map[string]string{
		"media_id": "m1", "provider_id": "uakino", "title": "Матриця",
	}, token)
	on := covFlowDecode[map[string]bool](t, rr)
	if !on["is_favorite"] {
		t.Fatalf("toggle on failed: %v", on)
	}
	rr = rig.do(t, http.MethodGet, "/api/v1/sync/favorites", nil, token)
	favs := covFlowDecode[[]domain.Favorite](t, rr)
	if len(favs) != 1 {
		t.Fatalf("expected 1 favorite, got %+v", favs)
	}
	rr = rig.do(t, http.MethodPost, "/api/v1/sync/favorites/toggle", map[string]string{
		"media_id": "m1", "provider_id": "uakino", "title": "Матриця",
	}, token)
	off := covFlowDecode[map[string]bool](t, rr)
	if off["is_favorite"] {
		t.Fatalf("toggle off failed: %v", off)
	}
}
