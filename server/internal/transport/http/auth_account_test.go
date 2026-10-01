package http_test

// Coverage for the authenticated account-management endpoints and the
// password-reset flow.
//
// Every function here was previously at exactly zero percent. They matter more
// than their size suggests: ForgotPassword deliberately answers 200 for unknown
// addresses so it cannot be used to enumerate accounts, ResetPassword must burn
// its token so a reset link cannot be replayed, and ChangePassword must reject
// a wrong current password rather than letting anyone take over a session.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/edhases/oxide-server/internal/auth"
	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/email"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

const testJWTSecret = "test-secret-value"

// ctxWithUser injects an authenticated user id, standing in for the JWT
// middleware so the handlers under test are reached with a real request.
func ctxWithUser(userID uuid.UUID) context.Context {
	return context.WithValue(context.Background(), middleware.UserIDKey, userID)
}

func newAuthRig(t *testing.T) (*transporthttp.AuthHandler, *memUserStore, *memRefreshStore) {
	t.Helper()
	users := newMemUserStore()
	refresh := newMemRefreshStore()
	// With no RESEND_API the email service reports itself unconfigured, which
	// is the auto-verify branch used in local and CI runs.
	t.Setenv("RESEND_API", "")
	h := transporthttp.NewAuthHandler(users, refresh, email.NewService(), testJWTSecret, "")
	return h, users, refresh
}

// seedUser creates a password user and returns it with its plaintext password.
func seedUser(t *testing.T, store *memUserStore, emailAddr, password string) *domain.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	u, err := store.CreateUser(context.Background(), emailAddr, hash, "tester")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u
}

func postJSON(t *testing.T, h http.HandlerFunc, path string, ctx context.Context, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else if raw, ok := body.([]byte); ok {
		reader = bytes.NewReader(raw)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(http.MethodPost, path, reader)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func wantStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, want, rr.Body.String())
	}
}

func wantErrorBody(t *testing.T, rr *httptest.ResponseRecorder, fragment string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON (%v): %s", err, rr.Body.String())
	}
	msg, _ := payload["error"].(string)
	if !strings.Contains(msg, fragment) {
		t.Errorf("error = %q, want it to contain %q", msg, fragment)
	}
}

func TestUpdateProfile(t *testing.T) {
	h, users, _ := newAuthRig(t)
	u := seedUser(t, users, "profile@example.com", "secret-password")

	t.Run("unauthenticated", func(t *testing.T) {
		rr := postJSON(t, h.UpdateProfile, "/api/v1/auth/profile", context.Background(),
			map[string]string{"username": "new"})
		wantStatus(t, rr, http.StatusUnauthorized)
	})

	t.Run("rejects a malformed payload", func(t *testing.T) {
		rr := postJSON(t, h.UpdateProfile, "/api/v1/auth/profile", ctxWithUser(u.ID), []byte("{not json"))
		wantStatus(t, rr, http.StatusBadRequest)
		wantErrorBody(t, rr, "invalid request payload")
	})

	t.Run("updates username, bio and avatar", func(t *testing.T) {
		rr := postJSON(t, h.UpdateProfile, "/api/v1/auth/profile", ctxWithUser(u.ID), map[string]string{
			"username":   "renamed",
			"bio":        "hello",
			"avatar_url": "https://cdn/a.png",
		})
		wantStatus(t, rr, http.StatusOK)

		stored, err := users.GetUserByID(context.Background(), u.ID)
		if err != nil {
			t.Fatalf("GetUserByID: %v", err)
		}
		if stored.Username != "renamed" || stored.Bio != "hello" || stored.AvatarURL != "https://cdn/a.png" {
			t.Errorf("profile not applied: %+v", stored)
		}
	})

	// The mobile client sends "name" and "avatar"; the web client sends
	// "username" and "avatar_url". Both must land on the same fields.
	t.Run("accepts the legacy mobile field names", func(t *testing.T) {
		rr := postJSON(t, h.UpdateProfile, "/api/v1/auth/profile", ctxWithUser(u.ID), map[string]string{
			"name":   "legacy-name",
			"avatar": "https://cdn/legacy.png",
		})
		wantStatus(t, rr, http.StatusOK)

		stored, _ := users.GetUserByID(context.Background(), u.ID)
		if stored.Username != "legacy-name" {
			t.Errorf("name should map onto username, got %q", stored.Username)
		}
		if stored.AvatarURL != "https://cdn/legacy.png" {
			t.Errorf("avatar should map onto avatar_url, got %q", stored.AvatarURL)
		}
	})

	t.Run("reports a store failure as 500", func(t *testing.T) {
		ghost := uuid.New()
		rr := postJSON(t, h.UpdateProfile, "/api/v1/auth/profile", ctxWithUser(ghost),
			map[string]string{"username": "nobody"})
		wantStatus(t, rr, http.StatusInternalServerError)
	})
}

func TestChangePassword(t *testing.T) {
	h, users, _ := newAuthRig(t)
	u := seedUser(t, users, "chpw@example.com", "old-password-1")

	t.Run("unauthenticated", func(t *testing.T) {
		rr := postJSON(t, h.ChangePassword, "/api/v1/auth/change-password", context.Background(),
			map[string]string{"old_password": "old-password-1", "new_password": "new-password-2"})
		wantStatus(t, rr, http.StatusUnauthorized)
	})

	t.Run("requires both passwords", func(t *testing.T) {
		for _, body := range []map[string]string{
			{},
			{"old_password": "old-password-1"},
			{"new_password": "new-password-2"},
		} {
			rr := postJSON(t, h.ChangePassword, "/api/v1/auth/change-password", ctxWithUser(u.ID), body)
			wantStatus(t, rr, http.StatusBadRequest)
		}
	})

	t.Run("rejects a wrong current password", func(t *testing.T) {
		rr := postJSON(t, h.ChangePassword, "/api/v1/auth/change-password", ctxWithUser(u.ID),
			map[string]string{"old_password": "totally-wrong", "new_password": "new-password-2"})
		wantStatus(t, rr, http.StatusBadRequest)
		wantErrorBody(t, rr, "incorrect current password")

		if users.updatesPassword != 0 {
			t.Error("a rejected attempt must not write a new hash")
		}
	})

	t.Run("changes the password when the current one matches", func(t *testing.T) {
		rr := postJSON(t, h.ChangePassword, "/api/v1/auth/change-password", ctxWithUser(u.ID),
			map[string]string{"old_password": "old-password-1", "new_password": "new-password-2"})
		wantStatus(t, rr, http.StatusOK)

		stored, _ := users.GetUserByID(context.Background(), u.ID)
		ok, err := auth.ComparePasswordAndHash("new-password-2", stored.PasswordHash)
		if err != nil || !ok {
			t.Errorf("new password should verify, got ok=%v err=%v", ok, err)
		}
		// The plaintext must never be stored.
		if strings.Contains(stored.PasswordHash, "new-password-2") {
			t.Error("password was stored in plaintext")
		}
	})

	t.Run("reports an unknown user as 404", func(t *testing.T) {
		rr := postJSON(t, h.ChangePassword, "/api/v1/auth/change-password", ctxWithUser(uuid.New()),
			map[string]string{"old_password": "a", "new_password": "b"})
		wantStatus(t, rr, http.StatusNotFound)
	})
}

func TestDeleteAccount(t *testing.T) {
	h, users, _ := newAuthRig(t)
	u := seedUser(t, users, "bye@example.com", "some-password")

	t.Run("unauthenticated", func(t *testing.T) {
		rr := postJSON(t, h.DeleteAccount, "/api/v1/auth/delete-account", context.Background(), nil)
		wantStatus(t, rr, http.StatusUnauthorized)
	})

	t.Run("removes the user", func(t *testing.T) {
		rr := postJSON(t, h.DeleteAccount, "/api/v1/auth/delete-account", ctxWithUser(u.ID), nil)
		wantStatus(t, rr, http.StatusOK)

		if _, err := users.GetUserByID(context.Background(), u.ID); err == nil {
			t.Error("user should be gone after deletion")
		}
		// The email must be released so it can be registered again.
		if _, err := users.CreateUser(context.Background(), "bye@example.com", "h", "again"); err != nil {
			t.Errorf("email should be reusable after deletion: %v", err)
		}
	})

	t.Run("reports a store failure as 500", func(t *testing.T) {
		rr := postJSON(t, h.DeleteAccount, "/api/v1/auth/delete-account", ctxWithUser(uuid.New()), nil)
		wantStatus(t, rr, http.StatusInternalServerError)
	})
}

// ForgotPassword must never reveal whether an address is registered: both
// branches answer 200 with the same message.
func TestForgotPasswordNeverLeaksAccountExistence(t *testing.T) {
	h, users, _ := newAuthRig(t)
	seedUser(t, users, "known@example.com", "some-password")

	t.Run("rejects a missing email", func(t *testing.T) {
		rr := postJSON(t, h.ForgotPassword, "/api/v1/auth/forgot-password", context.Background(), map[string]string{})
		wantStatus(t, rr, http.StatusBadRequest)
	})

	unknown := postJSON(t, h.ForgotPassword, "/api/v1/auth/forgot-password", context.Background(),
		map[string]string{"email": "nobody@example.com"})
	known := postJSON(t, h.ForgotPassword, "/api/v1/auth/forgot-password", context.Background(),
		map[string]string{"email": "known@example.com"})

	wantStatus(t, unknown, http.StatusOK)
	wantStatus(t, known, http.StatusOK)

	if unknown.Body.String() != known.Body.String() {
		t.Errorf("responses differ and so leak account existence:\n  unknown: %s\n  known:   %s",
			unknown.Body.String(), known.Body.String())
	}
}

func TestResetPassword(t *testing.T) {
	h, users, _ := newAuthRig(t)
	u := seedUser(t, users, "reset@example.com", "first-password")

	token := "reset-token-abc"
	if err := users.CreatePasswordResetToken(context.Background(), u.ID, token); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}

	t.Run("requires token and password", func(t *testing.T) {
		for _, body := range []map[string]string{
			{},
			{"token": token},
			{"password": "brand-new-password"},
		} {
			rr := postJSON(t, h.ResetPassword, "/api/v1/auth/reset-password", context.Background(), body)
			wantStatus(t, rr, http.StatusBadRequest)
		}
	})

	t.Run("rejects an unknown token", func(t *testing.T) {
		rr := postJSON(t, h.ResetPassword, "/api/v1/auth/reset-password", context.Background(),
			map[string]string{"token": "never-issued", "password": "brand-new-password"})
		wantStatus(t, rr, http.StatusBadRequest)
		wantErrorBody(t, rr, "invalid or expired reset token")
	})

	t.Run("sets the new password", func(t *testing.T) {
		rr := postJSON(t, h.ResetPassword, "/api/v1/auth/reset-password", context.Background(),
			map[string]string{"token": token, "password": "brand-new-password"})
		wantStatus(t, rr, http.StatusOK)

		stored, _ := users.GetUserByID(context.Background(), u.ID)
		ok, err := auth.ComparePasswordAndHash("brand-new-password", stored.PasswordHash)
		if err != nil || !ok {
			t.Errorf("reset password should verify, got ok=%v err=%v", ok, err)
		}
	})

	// The whole point of MarkPasswordResetUsed: a leaked reset link must stop
	// working the moment it is redeemed.
	t.Run("a redeemed token cannot be replayed", func(t *testing.T) {
		rr := postJSON(t, h.ResetPassword, "/api/v1/auth/reset-password", context.Background(),
			map[string]string{"token": token, "password": "attacker-chosen-password"})
		wantStatus(t, rr, http.StatusBadRequest)

		stored, _ := users.GetUserByID(context.Background(), u.ID)
		ok, _ := auth.ComparePasswordAndHash("brand-new-password", stored.PasswordHash)
		if !ok {
			t.Error("a replayed reset must not overwrite the password")
		}
	})
}

func TestUnlinkProvider(t *testing.T) {
	h, users, _ := newAuthRig(t)

	telegram := int64(4242)
	discord := "discord-abc"
	linked, err := users.CreateOAuthUser(context.Background(), "linked@example.com", "", "linked", "",
		&telegram, &discord)
	if err != nil {
		t.Fatalf("CreateOAuthUser: %v", err)
	}

	t.Run("unauthenticated", func(t *testing.T) {
		rr := postJSON(t, h.UnlinkProvider, "/api/v1/auth/unlink", context.Background(),
			map[string]string{"provider": "telegram"})
		wantStatus(t, rr, http.StatusUnauthorized)
	})

	t.Run("only telegram and discord can be unlinked", func(t *testing.T) {
		for _, name := range []string{"", "google", "facebook", "Telegram"} {
			rr := postJSON(t, h.UnlinkProvider, "/api/v1/auth/unlink", ctxWithUser(linked.ID),
				map[string]string{"provider": name})
			wantStatus(t, rr, http.StatusBadRequest)
			wantErrorBody(t, rr, "telegram or discord")
		}
	})

	t.Run("reports an unknown user as 404", func(t *testing.T) {
		rr := postJSON(t, h.UnlinkProvider, "/api/v1/auth/unlink", ctxWithUser(uuid.New()),
			map[string]string{"provider": "telegram"})
		wantStatus(t, rr, http.StatusNotFound)
	})

	// A placeholder address exists only because OAuth supplied no real email.
	// Unlinking the only login method would lock the account out entirely, so
	// the handler must refuse even though the client believes one method
	// remains.
	t.Run("refuses to remove the last login method", func(t *testing.T) {
		placeholder, err := users.CreateOAuthUser(context.Background(), "x@telegram.oxide", "", "tg-only", "",
			&telegram, nil)
		if err != nil {
			t.Fatalf("CreateOAuthUser: %v", err)
		}
		rr := postJSON(t, h.UnlinkProvider, "/api/v1/auth/unlink", ctxWithUser(placeholder.ID),
			map[string]string{"provider": "telegram"})
		wantStatus(t, rr, http.StatusBadRequest)

		stored, _ := users.GetUserByID(context.Background(), placeholder.ID)
		if stored.TelegramID == nil {
			t.Error("telegram must remain linked")
		}
	})

	t.Run("unlinks telegram when another method remains", func(t *testing.T) {
		rr := postJSON(t, h.UnlinkProvider, "/api/v1/auth/unlink", ctxWithUser(linked.ID),
			map[string]string{"provider": "telegram"})
		wantStatus(t, rr, http.StatusOK)

		stored, _ := users.GetUserByID(context.Background(), linked.ID)
		if stored.TelegramID != nil {
			t.Error("telegram should have been unlinked")
		}
		if stored.DiscordID == nil {
			t.Error("discord must be untouched")
		}
	})

	t.Run("unlinks discord", func(t *testing.T) {
		rr := postJSON(t, h.UnlinkProvider, "/api/v1/auth/unlink", ctxWithUser(linked.ID),
			map[string]string{"provider": "discord"})
		wantStatus(t, rr, http.StatusOK)

		stored, _ := users.GetUserByID(context.Background(), linked.ID)
		if stored.DiscordID != nil {
			t.Error("discord should have been unlinked")
		}
	})
}

// RequireVerifiedEmail gates sync endpoints. With email delivery unconfigured
// it must let everyone through; otherwise an unverified account is refused.
func TestRequireVerifiedEmail(t *testing.T) {
	t.Run("passes through when email is not configured", func(t *testing.T) {
		t.Setenv("RESEND_API", "")
		users := newMemUserStore()
		h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

		u := seedUser(t, users, "auto@example.com", "pw") // never verified
		reached := false
		mw := h.RequireVerifiedEmail()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached = true
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/history", nil)
		req = req.WithContext(ctxWithUser(u.ID))
		mw.ServeHTTP(httptest.NewRecorder(), req)

		if !reached {
			t.Error("handler should run when email delivery is disabled")
		}
	})

	t.Run("refuses an unverified account when email is configured", func(t *testing.T) {
		t.Setenv("RESEND_API", "re_test_key")
		users := newMemUserStore()
		h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

		u := seedUser(t, users, "unverified@example.com", "pw")
		reached := false
		mw := h.RequireVerifiedEmail()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached = true
		}))

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/history", nil)
		req = req.WithContext(ctxWithUser(u.ID))
		mw.ServeHTTP(rr, req)

		if reached {
			t.Error("unverified account must not reach the sync handler")
		}
		wantStatus(t, rr, http.StatusForbidden)
		wantErrorBody(t, rr, "email not verified")
	})

	t.Run("allows a verified account", func(t *testing.T) {
		t.Setenv("RESEND_API", "re_test_key")
		users := newMemUserStore()
		h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

		u := seedUser(t, users, "verified@example.com", "pw")
		if err := users.MarkEmailVerified(context.Background(), u.ID); err != nil {
			t.Fatalf("MarkEmailVerified: %v", err)
		}

		reached := false
		mw := h.RequireVerifiedEmail()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached = true
		}))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/history", nil)
		req = req.WithContext(ctxWithUser(u.ID))
		mw.ServeHTTP(httptest.NewRecorder(), req)

		if !reached {
			t.Error("verified account should reach the sync handler")
		}
	})

	t.Run("passes through when there is no user in context", func(t *testing.T) {
		t.Setenv("RESEND_API", "re_test_key")
		users := newMemUserStore()
		h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

		reached := false
		mw := h.RequireVerifiedEmail()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached = true
		}))
		mw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

		if !reached {
			t.Error("an unauthenticated request should be left to the auth middleware")
		}
	})
}
