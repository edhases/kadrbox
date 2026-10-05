package http_test

// Token issuance and the favourite-removal endpoint.
//
// respondWithTokens is the single place every successful sign-in converges on:
// it mints the access token, persists the refresh token with its TTL, and
// writes the envelope. It was uncovered, which meant the refresh-token TTL was
// never asserted by anything — a regression there would silently shorten every
// session in production.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/edhases/kadrbox-server/internal/auth"
	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
)

type authEnvelope struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         *domain.User `json:"user"`
}

func decodeEnvelope(t *testing.T, rr *httptest.ResponseRecorder) authEnvelope {
	t.Helper()
	var env authEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not a token envelope (%v): %s", err, rr.Body.String())
	}
	return env
}

func TestRegisterIssuesUsableTokens(t *testing.T) {
	h, users, refresh := newAuthRig(t)

	rr := postJSON(t, h.Register, "/api/v1/auth/register", context.Background(), map[string]string{
		"email":    "new@example.com",
		"password": "a-strong-password",
		"username": "newbie",
	})
	wantStatus(t, rr, http.StatusOK)

	env := decodeEnvelope(t, rr)
	if env.AccessToken == "" || env.RefreshToken == "" {
		t.Fatalf("expected both tokens, got %+v", env)
	}

	// The access token must verify against the same secret the client will
	// present, or every authenticated request fails after registration.
	claims, err := auth.ValidateAccessToken(env.AccessToken, testJWTSecret)
	if err != nil {
		t.Fatalf("issued access token does not validate: %v", err)
	}
	if claims.UserID != env.User.ID {
		t.Errorf("token subject %v does not match user %v", claims.UserID, env.User.ID)
	}

	// The refresh token must be live and point at this user, with the intended
	// 30-day lifetime.
	if refresh.liveTokens() != 1 {
		t.Fatalf("expected exactly one stored refresh token, got %d", refresh.liveTokens())
	}
	if refresh.lastTTL != 30*24*time.Hour {
		t.Errorf("refresh TTL = %v, want %v", refresh.lastTTL, 30*24*time.Hour)
	}
	storedID, err := refresh.GetUserIDByRefreshToken(context.Background(), env.RefreshToken)
	if err != nil {
		t.Fatalf("stored refresh token not retrievable: %v", err)
	}
	if storedID != env.User.ID {
		t.Errorf("refresh token maps to %v, want %v", storedID, env.User.ID)
	}

	// And the password must never be echoed back.
	if env.User.PasswordHash != "" {
		t.Error("password hash leaked into the response")
	}
	if _, err := users.GetUserByEmail(context.Background(), "new@example.com"); err != nil {
		t.Errorf("user was not persisted: %v", err)
	}
}

func TestLoginIssuesTokensAndRejectsBadCredentials(t *testing.T) {
	h, users, _ := newAuthRig(t)
	seedUser(t, users, "login@example.com", "correct-password")

	t.Run("successful login", func(t *testing.T) {
		rr := postJSON(t, h.Login, "/api/v1/auth/login", context.Background(), map[string]string{
			"email":    "login@example.com",
			"password": "correct-password",
		})
		wantStatus(t, rr, http.StatusOK)

		env := decodeEnvelope(t, rr)
		if _, err := auth.ValidateAccessToken(env.AccessToken, testJWTSecret); err != nil {
			t.Errorf("access token does not validate: %v", err)
		}
		if env.RefreshToken == "" {
			t.Error("expected a refresh token")
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		rr := postJSON(t, h.Login, "/api/v1/auth/login", context.Background(), map[string]string{
			"email":    "login@example.com",
			"password": "wrong-password",
		})
		wantStatus(t, rr, http.StatusUnauthorized)
	})

	t.Run("unknown email", func(t *testing.T) {
		rr := postJSON(t, h.Login, "/api/v1/auth/login", context.Background(), map[string]string{
			"email":    "ghost@example.com",
			"password": "anything-at-all",
		})
		wantStatus(t, rr, http.StatusUnauthorized)
	})
}

// A refresh token that fails to persist must not be reported as a successful
// login: the client would store a session the server cannot honour.
func TestLoginFailsWhenRefreshTokenCannotBeStored(t *testing.T) {
	h, users, refresh := newAuthRig(t)
	seedUser(t, users, "nostore@example.com", "correct-password")
	refresh.storeErr = context.DeadlineExceeded

	rr := postJSON(t, h.Login, "/api/v1/auth/login", context.Background(), map[string]string{
		"email":    "nostore@example.com",
		"password": "correct-password",
	})
	wantStatus(t, rr, http.StatusInternalServerError)
	wantErrorBody(t, rr, "failed to store session")
}

// ---- favourites removal ----------------------------------------------------

func ctxWithUserCtx(userID uuid.UUID) context.Context {
	return context.WithValue(context.Background(), middleware.UserIDKey, userID)
}

func deleteJSON(t *testing.T, h http.HandlerFunc, path string, ctx context.Context, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, strings.NewReader(""))
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req = httptest.NewRequest(http.MethodDelete, path, strings.NewReader(string(encoded)))
	}
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestRemoveFavorite(t *testing.T) {
	user := uuid.New()
	other := uuid.New()

	t.Run("unauthenticated", func(t *testing.T) {
		s := newSyncRig()
		rr := deleteJSON(t, s.handler.RemoveFavorite, "/api/v1/sync/favorites?media_id=m1&provider_id=example-provider",
			context.Background(), nil)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rr.Code)
		}
	})

	// The client has shipped both snake_case and camelCase over its life, and
	// sends identifiers in either the query string or a JSON body.
	t.Run("accepts snake_case query parameters", func(t *testing.T) {
		s := newSyncRig()
		rr := deleteJSON(t, s.handler.RemoveFavorite, "/api/v1/sync/favorites?media_id=m1&provider_id=example-provider",
			ctxWithUserCtx(user), nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
		}
		// The single-object envelope replaced {"success":true}.
		if !strings.Contains(rr.Body.String(), `{"data":{"removed":true}}`) {
			t.Errorf("unexpected body: %s", rr.Body.String())
		}
		if s.favs.removed != 1 {
			t.Errorf("expected one removal, got %d", s.favs.removed)
		}
		if s.favs.lastUser != user || s.favs.lastMedia != "m1" || s.favs.lastProv != "example-provider" {
			t.Errorf("wrong removal arguments: user=%v media=%q provider=%q",
				s.favs.lastUser, s.favs.lastMedia, s.favs.lastProv)
		}
	})

	t.Run("falls back to the JSON body", func(t *testing.T) {
		s := newSyncRig()
		rr := deleteJSON(t, s.handler.RemoveFavorite, "/api/v1/sync/favorites", ctxWithUserCtx(user),
			map[string]string{"media_id": "m2", "provider_id": "example-provider-b"})
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
		}
		if s.favs.lastMedia != "m2" || s.favs.lastProv != "example-provider-b" {
			t.Errorf("body fallback not used: media=%q provider=%q", s.favs.lastMedia, s.favs.lastProv)
		}
	})

	t.Run("falls back to camelCase body fields", func(t *testing.T) {
		s := newSyncRig()
		rr := deleteJSON(t, s.handler.RemoveFavorite, "/api/v1/sync/favorites", ctxWithUserCtx(user),
			map[string]string{"mediaId": "m3", "providerId": "example-provider"})
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rr.Code, rr.Body.String())
		}
		if s.favs.lastMedia != "m3" || s.favs.lastProv != "example-provider" {
			t.Errorf("camelCase fallback not used: media=%q provider=%q", s.favs.lastMedia, s.favs.lastProv)
		}
	})

	t.Run("requires both identifiers", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/sync/favorites",
			"/api/v1/sync/favorites?media_id=m1",
			"/api/v1/sync/favorites?provider_id=example-provider",
		} {
			s := newSyncRig()
			rr := deleteJSON(t, s.handler.RemoveFavorite, path, ctxWithUserCtx(user), nil)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", path, rr.Code)
			}
			if s.favs.removed != 0 {
				t.Errorf("%s: store was touched on an invalid request", path)
			}
		}
	})

	t.Run("maps a store failure to 500", func(t *testing.T) {
		s := newSyncRig()
		s.favs.removeErr = context.DeadlineExceeded
		rr := deleteJSON(t, s.handler.RemoveFavorite, "/api/v1/sync/favorites?media_id=m&provider_id=p",
			ctxWithUserCtx(user), nil)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rr.Code)
		}
	})

	// Removing someone else's favourite must not be possible, so the store is
	// always scoped by the authenticated user id.
	t.Run("always scopes the removal to the caller", func(t *testing.T) {
		s := newSyncRig()
		rr := deleteJSON(t, s.handler.RemoveFavorite, "/api/v1/sync/favorites?media_id=m1&provider_id=example-provider",
			ctxWithUserCtx(other), nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if s.favs.lastUser != other {
			t.Errorf("removal used user %v, want the authenticated %v", s.favs.lastUser, other)
		}
	})
}
