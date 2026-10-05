package http_test

// Р СњР В°РЎРѓР С”РЎР‚РЎвЂ“Р В·Р Р…Р С‘Р в„– flow Р С—РЎР‚Р С•РЎвЂљР С‘ РЎРѓР С—РЎР‚Р В°Р Р†Р В¶Р Р…РЎвЂ“РЎвЂ¦ Postgres + Redis.
// Р вЂєР С•Р С”Р В°Р В»РЎРЉР Р…Р С• Р В±Р ВµР В· TEST_POSTGRES_DSN / TEST_REDIS_ADDR РІР‚вЂќ Р С—РЎР‚Р С•Р С—РЎС“РЎРѓР С” (t.Skip).
// Р Р€ CI Р Р†Р С‘Р С”Р С•Р Р…РЎС“РЎвЂќРЎвЂљРЎРЉРЎРѓРЎРЏ job РЎвЂ“Р В· services. Email-РЎРѓР ВµРЎР‚Р Р†РЎвЂ“РЎРѓ Р Р…Р ВµР Р…Р В°Р В»Р В°РЎв‚¬РЎвЂљР С•Р Р†Р В°Р Р…Р С‘Р в„–
// (RESEND_API="") РІвЂ вЂ™ РЎР‚Р ВµРЎвЂќРЎРѓРЎвЂљРЎР‚Р В°РЎвЂ РЎвЂ“РЎРЏ Р в„–Р Т‘Р Вµ Р С–РЎвЂ“Р В»Р С”Р С•РЎР‹ auto-verify.

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
		t.Skip("TEST_POSTGRES_DSN/TEST_REDIS_ADDR not set РІР‚вЂќ integration test skipped")
	}
	// Р вЂќР ВµРЎвЂљР ВµРЎР‚Р СРЎвЂ“Р Р…Р С•Р Р†Р В°Р Р…Р С•: auto-verify Р С–РЎвЂ“Р В»Р С”Р В° РЎР‚Р ВµРЎвЂќРЎРѓРЎвЂљРЎР‚Р В°РЎвЂ РЎвЂ“РЎвЂ”.
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
		router: transporthttp.NewRouter("test-secret", authH, contentH, syncH, hub, testAppURL),
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

// covFlowDecodeEnvelope decodes the `{"data": ..., "meta": ...}` envelope the
// list endpoints return. The bare decoder above is kept for the endpoints that
// still answer with a plain object or array.
func covFlowDecodeEnvelope[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope failed (status %d): %v", rr.Code, err)
	}
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("decode envelope data failed (status %d): %v", rr.Code, err)
	}
	return v
}

// toggleEnvelope mirrors the favourites toggle payload, which also carries the
// identifiers, so it cannot be decoded into map[string]bool.
type toggleEnvelope struct {
	IsFavorite bool   `json:"is_favorite"`
	MediaID    string `json:"media_id"`
	ProviderID string `json:"provider_id"`
}

func TestCovHttpFlowRegisterLoginRefresh(t *testing.T) {
	rig := covFlowRigSetup(t)
	mail := fmt.Sprintf("flow_%d@x.com", time.Now().UnixNano())

	// 1. Register РІвЂ вЂ™ 200, auto-verified (email Р Р…Р Вµ Р Р…Р В°Р В»Р В°РЎв‚¬РЎвЂљР С•Р Р†Р В°Р Р…Р С•).
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

	// 2. Р вЂќРЎС“Р В±Р В»РЎвЂ“Р С”Р В°РЎвЂљ РІвЂ вЂ™ 409.
	rr = rig.do(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": mail, "password": "secret123", "username": "flowuser",
	}, "")
	if rr.Code != http.StatusConflict {
		t.Errorf("duplicate register: expected 409, got %d", rr.Code)
	}

	// 3. Me Р В· РЎвЂљР С•Р С”Р ВµР Р…Р С•Р С РІвЂ вЂ™ 200.
	rr = rig.do(t, http.MethodGet, "/api/v1/auth/me", nil, reg.AccessToken)
	if rr.Code != http.StatusOK {
		t.Fatalf("me: expected 200, got %d", rr.Code)
	}
	me := covFlowDecode[domain.User](t, rr)
	if me.Email != mail {
		t.Errorf("me: unexpected user %+v", me)
	}

	// 4. Login Р В· Р Р…Р ВµР С—РЎР‚Р В°Р Р†Р С‘Р В»РЎРЉР Р…Р С‘Р С Р С—Р В°РЎР‚Р С•Р В»Р ВµР С РІвЂ вЂ™ 401; Р В· Р С—РЎР‚Р В°Р Р†Р С‘Р В»РЎРЉР Р…Р С‘Р С РІвЂ вЂ™ 200.
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

	// 5. Refresh РЎР‚Р С•РЎвЂљР В°РЎвЂ РЎвЂ“РЎРЏ: Р Р…Р С•Р Р†Р С‘Р в„– РЎвЂљР С•Р С”Р ВµР Р… Р С—РЎР‚Р В°РЎвЂ РЎР‹РЎвЂќ, РЎРѓРЎвЂљР В°РЎР‚Р С‘Р в„– РІР‚вЂќ 401.
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

	// 6. VerifyEmail Р В· Р В»РЎвЂ“Р Р†Р С‘Р С РЎвЂљР С•Р С”Р ВµР Р…Р С•Р С РІвЂ вЂ™ 400 (Р С–РЎвЂ“Р В»Р С”Р В° Р вЂР вЂќ).
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

	// SaveProgress РІвЂ вЂ™ РЎвЂ“РЎРѓРЎвЂљР С•РЎР‚РЎвЂ“РЎРЏ.
	rr = rig.do(t, http.MethodPost, "/api/v1/sync/history", map[string]any{
		"media_id": "m1", "provider_id": "uakino", "title": "Р СљР В°РЎвЂљРЎР‚Р С‘РЎвЂ РЎРЏ",
		"position_ms": 50, "duration_ms": 100,
	}, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("save progress: expected 200, got %d", rr.Code)
	}

	rr = rig.do(t, http.MethodGet, "/api/v1/sync/history?limit=10", nil, token)
	hist := covFlowDecodeEnvelope[[]domain.WatchHistory](t, rr)
	if len(hist) != 1 || hist[0].Title != "Р СљР В°РЎвЂљРЎР‚Р С‘РЎвЂ РЎРЏ" || hist[0].PositionMs != 50 {
		t.Fatalf("unexpected history: %+v", hist)
	}

	rr = rig.do(t, http.MethodGet, "/api/v1/sync/continue-watching", nil, token)
	cont := covFlowDecodeEnvelope[[]domain.WatchHistory](t, rr)
	if len(cont) != 1 || cont[0].MediaID != "m1" {
		t.Errorf("unexpected continue-watching: %+v", cont)
	}

	// Toggle on РІвЂ вЂ™ favorites Р СРЎвЂ“РЎРѓРЎвЂљР С‘РЎвЂљРЎРЉ; toggle off РІвЂ вЂ™ Р С—Р С•РЎР‚Р С•Р В¶Р Р…РЎРЉР С•.
	rr = rig.do(t, http.MethodPost, "/api/v1/sync/favorites/toggle", map[string]string{
		"media_id": "m1", "provider_id": "uakino", "title": "Р СљР В°РЎвЂљРЎР‚Р С‘РЎвЂ РЎРЏ",
	}, token)
	on := covFlowDecodeEnvelope[toggleEnvelope](t, rr)
	if !on.IsFavorite {
		t.Fatalf("toggle on failed: %+v", on)
	}
	rr = rig.do(t, http.MethodGet, "/api/v1/sync/favorites", nil, token)
	favs := covFlowDecodeEnvelope[[]domain.Favorite](t, rr)
	if len(favs) != 1 {
		t.Fatalf("expected 1 favorite, got %+v", favs)
	}
	rr = rig.do(t, http.MethodPost, "/api/v1/sync/favorites/toggle", map[string]string{
		"media_id": "m1", "provider_id": "uakino", "title": "Р СљР В°РЎвЂљРЎР‚Р С‘РЎвЂ РЎРЏ",
	}, token)
	off := covFlowDecodeEnvelope[toggleEnvelope](t, rr)
	if off.IsFavorite {
		t.Fatalf("toggle off failed: %+v", off)
	}
}
