package http_test

// Session-lifecycle tests: what happens to refresh tokens when a credential
// changes, when a token is replayed, and when the client logs out.
//
// Each of these used to be a silent no-op — the handlers changed the password,
// reset it, deleted the account or rotated a token without ever touching the
// session store, so a stolen refresh token kept working for its full 30-day TTL.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/auth"
	"github.com/edhases/oxide-server/internal/email"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
	"github.com/google/uuid"
)

// openSessions creates n live refresh tokens for a user.
func openSessions(t *testing.T, store *memRefreshStore, userID uuid.UUID, n int) []string {
	t.Helper()
	tokens := make([]string, 0, n)
	for i := 0; i < n; i++ {
		token := uuid.New().String()
		if err := store.StoreRefreshToken(context.Background(), token, userID, 30*24*time.Hour); err != nil {
			t.Fatalf("StoreRefreshToken: %v", err)
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func TestChangePasswordRevokesEverySession(t *testing.T) {
	h, users, refresh := newAuthRig(t)
	u := seedUser(t, users, "sessions@example.com", "old-password-1")
	tokens := openSessions(t, refresh, u.ID, 3)

	rr := postJSON(t, h.ChangePassword, "/api/v1/auth/change-password", ctxWithUser(u.ID), map[string]string{
		"old_password": "old-password-1",
		"new_password": "new-password-2",
	})
	wantStatus(t, rr, http.StatusOK)

	if live := refresh.liveTokensForUser(u.ID); live != 0 {
		t.Errorf("%d session(s) survived a password change", live)
	}
	for _, token := range tokens {
		if refresh.tokenIsLive(token) {
			t.Errorf("token %s still works after a password change", token)
		}
	}
}

func TestResetPasswordRevokesEverySession(t *testing.T) {
	h, users, refresh := newAuthRig(t)
	u := seedUser(t, users, "reset-sessions@example.com", "first-password")
	tokens := openSessions(t, refresh, u.ID, 2)

	if err := users.CreatePasswordResetToken(context.Background(), u.ID, "reset-token"); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}
	rr := postJSON(t, h.ResetPassword, "/api/v1/auth/reset-password", context.Background(), map[string]string{
		"token": "reset-token", "password": "brand-new-password",
	})
	wantStatus(t, rr, http.StatusOK)

	if live := refresh.liveTokensForUser(u.ID); live != 0 {
		t.Errorf("%d session(s) survived a password reset", live)
	}
	for _, token := range tokens {
		if refresh.tokenIsLive(token) {
			t.Errorf("token %s still works after a password reset", token)
		}
	}
}

func TestDeleteAccountRevokesEverySession(t *testing.T) {
	h, users, refresh := newAuthRig(t)
	u := seedUser(t, users, "gone@example.com", "some-password")
	tokens := openSessions(t, refresh, u.ID, 2)

	rr := deleteJSON(t, h.DeleteAccount, "/api/v1/auth/account", ctxWithUser(u.ID), nil)
	wantStatus(t, rr, http.StatusOK)

	if live := refresh.liveTokensForUser(u.ID); live != 0 {
		t.Errorf("%d session(s) survived account deletion", live)
	}
	for _, token := range tokens {
		if refresh.tokenIsLive(token) {
			t.Errorf("token %s still works after account deletion", token)
		}
	}
}

// Revoking "all" must mean all: the whole point is cutting off an attacker who
// holds a copy.
func TestRevokeAllForUserLeavesOtherUsersAlone(t *testing.T) {
	_, _, refresh := newAuthRig(t)
	victim, bystander := uuid.New(), uuid.New()
	victimTokens := openSessions(t, refresh, victim, 3)
	bystanderTokens := openSessions(t, refresh, bystander, 1)

	revoked, err := refresh.RevokeAllForUser(context.Background(), victim)
	if err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if revoked != 3 {
		t.Errorf("RevokeAllForUser = %d, want 3", revoked)
	}
	for _, token := range victimTokens {
		if refresh.tokenIsLive(token) {
			t.Errorf("victim token %s survived", token)
		}
	}
	for _, token := range bystanderTokens {
		if !refresh.tokenIsLive(token) {
			t.Errorf("bystander token %s was revoked: bulk revocation must be scoped to one user", token)
		}
	}
}

// Rotation must be a single atomic consume, and a replayed token must be treated
// as a compromise: all sessions of that user die.
func TestRefreshRotationDetectsReuse(t *testing.T) {
	h, users, refresh := newAuthRig(t)
	u := seedUser(t, users, "rotate@example.com", "correct-password")

	login := postJSON(t, h.Login, "/api/v1/auth/login", context.Background(), map[string]string{
		"email": "rotate@example.com", "password": "correct-password",
	})
	wantStatus(t, login, http.StatusOK)
	first := decodeEnvelope(t, login)

	// A second, independent session, so we can prove the reuse response is not
	// just "this one token is gone".
	other := openSessions(t, refresh, u.ID, 1)[0]

	rotation := postJSON(t, h.Refresh, "/api/v1/auth/refresh", context.Background(),
		map[string]string{"refresh_token": first.RefreshToken})
	wantStatus(t, rotation, http.StatusOK)
	rotated := decodeEnvelope(t, rotation)
	if rotated.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if refresh.consumeCount() == 0 {
		t.Error("rotation did not go through the atomic consume")
	}

	// Replaying the consumed token: someone has a copy.
	replay := postJSON(t, h.Refresh, "/api/v1/auth/refresh", context.Background(),
		map[string]string{"refresh_token": first.RefreshToken})
	wantStatus(t, replay, http.StatusUnauthorized)

	if live := refresh.liveTokensForUser(u.ID); live != 0 {
		t.Errorf("%d session(s) survived a refresh-token replay; a replay means one holder is an attacker", live)
	}
	for _, token := range []string{rotated.RefreshToken, other} {
		if refresh.tokenIsLive(token) {
			t.Errorf("token %s survived the compromise response", token)
		}
	}

	// And the freshly issued replacement is refused too, rather than becoming a
	// way back in.
	after := postJSON(t, h.Refresh, "/api/v1/auth/refresh", context.Background(),
		map[string]string{"refresh_token": rotated.RefreshToken})
	wantStatus(t, after, http.StatusUnauthorized)
}

func TestRefreshRejectsUnknownToken(t *testing.T) {
	h, _, _ := newAuthRig(t)
	rr := postJSON(t, h.Refresh, "/api/v1/auth/refresh", context.Background(),
		map[string]string{"refresh_token": uuid.New().String()})
	wantStatus(t, rr, http.StatusUnauthorized)
}

func TestLogoutRevokesThePresentedToken(t *testing.T) {
	h, users, refresh := newAuthRig(t)
	u := seedUser(t, users, "logout@example.com", "correct-password")
	tokens := openSessions(t, refresh, u.ID, 2)

	rr := postJSON(t, h.Logout, "/api/v1/auth/logout", ctxWithUser(u.ID),
		map[string]string{"refresh_token": tokens[0]})
	wantStatus(t, rr, http.StatusOK)

	if refresh.tokenIsLive(tokens[0]) {
		t.Error("the presented token was not revoked")
	}
	if !refresh.tokenIsLive(tokens[1]) {
		t.Error("logout revoked a session it was not given")
	}
}

func TestLogoutRequiresAToken(t *testing.T) {
	h, _, _ := newAuthRig(t)
	rr := postJSON(t, h.Logout, "/api/v1/auth/logout", ctxWithUser(uuid.New()), map[string]string{})
	wantStatus(t, rr, http.StatusBadRequest)
}

// A duplicate registration must be indistinguishable from another duplicate: no
// timing or body differences that leak anything beyond "this address is taken".
func TestRegisterTwiceIsIdempotentlyReported(t *testing.T) {
	h, _, _ := newAuthRig(t)
	body := map[string]string{
		"email":    "duplicate@example.com",
		"password": "a-strong-password",
		"username": "duplicate",
	}

	first := postJSON(t, h.Register, "/api/v1/auth/register", context.Background(), body)
	wantStatus(t, first, http.StatusOK)

	second := postJSON(t, h.Register, "/api/v1/auth/register", context.Background(), body)
	third := postJSON(t, h.Register, "/api/v1/auth/register", context.Background(), body)

	if second.Code != third.Code {
		t.Errorf("repeat registrations disagree: %d vs %d", second.Code, third.Code)
	}
	if second.Body.String() != third.Body.String() {
		t.Errorf("repeat registrations differ and so leak state:\n  2nd: %s\n  3rd: %s",
			second.Body.String(), third.Body.String())
	}
}

// A create failure that is NOT a confirmed duplicate must not be reported as a
// duplicate: the old code mapped every error to 409 "email already registered",
// which turned a database hiccup into a false existence claim.
func TestRegisterDoesNotClaimADuplicateItCannotConfirm(t *testing.T) {
	t.Setenv("RESEND_API", "re_test_key") // so the handler does not auto-verify
	failing := &failingCreateStore{memUserStore: newMemUserStore()}
	h := transporthttp.NewAuthHandler(failing, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

	rr := postJSON(t, h.Register, "/api/v1/auth/register", context.Background(), map[string]string{
		"email":    "ghost@example.com",
		"password": "a-strong-password",
		"username": "ghost",
	})

	if rr.Code == http.StatusConflict {
		t.Errorf("an unconfirmable create failure must not be reported as a duplicate: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "already registered") {
		t.Errorf("response leaks a duplicate claim: %s", rr.Body.String())
	}
	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202 for an unconfirmable failure", rr.Code)
	}
}

// The limiter exists because the router-level budget (30/min shared with every
// other route) is far too generous for credential endpoints.
func TestLoginIsRateLimitedPerSourceAddress(t *testing.T) {
	users := newMemUserStore()
	seedUser(t, users, "brute@example.com", "correct-password")
	h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

	statuses := map[int]int{}
	for i := 0; i < 8; i++ {
		body, err := json.Marshal(map[string]string{"email": "brute@example.com", "password": "guess"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
		req.RemoteAddr = "203.0.113.9:5555"
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		statuses[rr.Code]++
	}
	if statuses[http.StatusTooManyRequests] == 0 {
		t.Fatalf("eight guesses were never throttled: %v", statuses)
	}
}

func TestRegisterIsRateLimitedPerSourceAddress(t *testing.T) {
	users := newMemUserStore()
	h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

	statuses := map[int]int{}
	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
			strings.NewReader(`{"email":"a@b.c","password":"password1","username":"u"}`))
		req.RemoteAddr = "198.51.100.4:5555"
		rr := httptest.NewRecorder()
		h.Register(rr, req)
		statuses[rr.Code]++
	}
	if statuses[http.StatusTooManyRequests] == 0 {
		t.Fatalf("eight registrations were never throttled: %v", statuses)
	}
}

// Login must not be a timing oracle for account existence: the unknown-email
// branch has to cost a KDF too.
func TestLoginUnknownEmailStillRunsTheKDF(t *testing.T) {
	t.Setenv("RESEND_API", "")
	users := newMemUserStore()
	h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

	// Warm the process-wide dummy hash first so the measurement is of the
	// derivation, not of a one-off lazy initialisation.
	auth.BurnKDFForDummyUser("warm-up")

	elapsed := timeLogin(t, h, "nobody@example.com", "a-strong-password")
	if elapsed < 20*time.Millisecond {
		t.Errorf("unknown address answered in %v: no KDF ran, so timing enumerates accounts", elapsed)
	}
}

// The KDF must not be allowed to fan out past the container's memory budget; the
// limiter is what keeps the request path from queueing unbounded Argon2 work.
func TestKDFGateDoesNotBlockTheRequestPath(t *testing.T) {
	t.Setenv("RESEND_API", "")
	users := newMemUserStore()
	h := transporthttp.NewAuthHandler(users, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 12; i++ {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
				strings.NewReader(`{"email":"race@example.com","password":"password1","username":"u"}`))
			req.RemoteAddr = "192.0.2.55:1234"
			h.Register(httptest.NewRecorder(), req)
		}
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("registration requests blocked: the KDF gate must fail fast, not queue")
	}
}

// MarkEmailVerified failing must not produce a token that claims verification.
func TestRegisterDoesNotIssueTokensWhenVerificationCannotBeStored(t *testing.T) {
	t.Setenv("RESEND_API", "") // auto-verify branch
	base := newMemUserStore()
	failing := &failingVerifyStore{memUserStore: base}
	h := transporthttp.NewAuthHandler(failing, newMemRefreshStore(), email.NewService(), testJWTSecret, "")

	rr := postJSON(t, h.Register, "/api/v1/auth/register", context.Background(), map[string]string{
		"email":    "unverifiable@example.com",
		"password": "a-strong-password",
		"username": "unverifiable",
	})

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the verification write fails", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "access_token") {
		t.Error("a token was issued for an account whose verification could not be stored")
	}
}

// RequireRole exists because the role claim was carried in the context and never
// checked. No admin route is registered by this change; the helper is the
// enforcement point waiting for one.
func TestRequireRoleEnforcesTheRoleClaim(t *testing.T) {
	adminCtx := context.WithValue(context.Background(), middleware.RoleKey, "admin")
	userCtx := context.WithValue(context.Background(), middleware.RoleKey, "user")
	noRoleCtx := context.Background()

	cases := []struct {
		name string
		role string
		ctx  context.Context
		want int
	}{
		{"admin reaches an admin route", "admin", adminCtx, http.StatusOK},
		{"user is refused an admin route", "admin", userCtx, http.StatusForbidden},
		{"no role claim is unauthenticated", "admin", noRoleCtx, http.StatusUnauthorized},
		{"wildcard admits any authenticated caller", "*", userCtx, http.StatusOK},
		{"wildcard still refuses a missing claim", "*", noRoleCtx, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			handler := transporthttp.RequireRole(tc.role)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				reached = true
			}))
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/admin", nil).WithContext(tc.ctx)
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.want {
				t.Errorf("status = %d, want %d", rr.Code, tc.want)
			}
			if reached != (tc.want == http.StatusOK) {
				t.Errorf("handler reached = %v, want %v", reached, tc.want == http.StatusOK)
			}
		})
	}
}

// timeLogin measures one login attempt end to end.
func timeLogin(t *testing.T, h *transporthttp.AuthHandler, emailAddr, password string) time.Duration {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": emailAddr, "password": password})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()

	start := time.Now()
	h.Login(rr, req)
	elapsed := time.Since(start)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("login for %s = %d, want 401", emailAddr, rr.Code)
	}
	return elapsed
}
