package http_test

// Coverage for GoogleAuth — the mobile-client ID-token exchange — and for the
// handler paths that only run when a store fails.
//
// GoogleAuth posts nothing and calls one endpoint, so it is driven with the
// same transport seam as the browser callbacks.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/email"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/google/uuid"
)

const covGoogleTokenInfo = "oauth2.googleapis.com/tokeninfo"

// covGoogleInfoRig scripts the tokeninfo endpoint.
func covGoogleInfoRig(t *testing.T, responses map[string]covCannedResponse) {
	t.Helper()
	swapTransport(t, &covScriptedTransport{responses: responses})
}

func covTokenInfo(status int, body string) covCannedResponse {
	return covCannedResponse{status: status, body: body}
}

func TestCovGoogleAuthSignsInAnExistingVerifiedUser(t *testing.T) {
	covGoogleInfoRig(t, map[string]covCannedResponse{
		covGoogleTokenInfo: covTokenInfo(http.StatusOK,
			`{"aud":"cov-google-client","email":"known@example.com","email_verified":"true","name":"Known","sub":"g1"}`),
	})

	users := newMemUserStore()
	existing := covNewUser(t, users, "known@example.com", "known")
	if err := users.MarkEmailVerified(t.Context(), existing.id); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "cov-google-client")

	rec := httptest.NewRecorder()
	h.GoogleAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/google",
		map[string]string{"id_token": "good-token"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if sessions.stores != 1 {
		t.Errorf("%d sessions stored, want 1", sessions.stores)
	}
	if len(users.byEmail) != 1 {
		t.Errorf("%d accounts after sign-in with a known address, want 1", len(users.byEmail))
	}
}

// covNotFoundStore maps the shared fake's not-found error onto the repository's
// own sentinel.
//
// memUserStore answers with a package-local ErrNotFound, so a handler that
// branches on errors.Is(err, postgres.ErrUserNotFound) — GoogleAuth's
// create-an-account path — would otherwise be unreachable in tests that use the
// shared fake. The concrete repository returns the sentinel, so this wrapper
// restores the real contract rather than inventing one.
type covNotFoundStore struct {
	*memUserStore
}

func (s *covNotFoundStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, err := s.memUserStore.GetUserByEmail(ctx, email)
	if err != nil && errors.Is(err, ErrNotFound) {
		return nil, postgres.ErrUserNotFound
	}
	return u, err
}

func TestCovGoogleAuthCreatesAnAccountForAnUnknownAddress(t *testing.T) {
	covGoogleInfoRig(t, map[string]covCannedResponse{
		covGoogleTokenInfo: covTokenInfo(http.StatusOK,
			`{"aud":"cov-google-client","email":"fresh@example.com","email_verified":"true","name":"Fresh","picture":"https://cdn/p.png"}`),
	})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(&covNotFoundStore{memUserStore: users}, sessions,
		covNoEmailService(t), covJWTSecret, "cov-google-client")

	rec := httptest.NewRecorder()
	h.GoogleAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/google",
		map[string]string{"id_token": "t"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	created, err := users.GetUserByEmail(t.Context(), "fresh@example.com")
	if err != nil {
		t.Fatalf("the account was not created: %v", err)
	}
	if !created.IsVerified {
		t.Error("a Google-verified address produced an unverified row; the 403 gate would then reject the user the API just signed in")
	}
	if created.Username != "Fresh" {
		t.Errorf("username = %q, want the Google display name", created.Username)
	}
}

func TestCovGoogleAuthFallsBackToTheLocalPartForAUsername(t *testing.T) {
	covGoogleInfoRig(t, map[string]covCannedResponse{
		covGoogleTokenInfo: covTokenInfo(http.StatusOK,
			`{"aud":"cov-google-client","email":"only.local@example.com","email_verified":"1"}`),
	})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(&covNotFoundStore{memUserStore: users}, sessions,
		covNoEmailService(t), covJWTSecret, "cov-google-client")

	rec := httptest.NewRecorder()
	h.GoogleAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/google",
		map[string]string{"id_token": "t"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	created, _ := users.GetUserByEmail(t.Context(), "only.local@example.com")
	if created.Username != "only.local" {
		t.Errorf("username = %q, want the local part of the address", created.Username)
	}
}

func TestCovGoogleAuthRejectsEverythingThatIsNotAVerifiedMatchingToken(t *testing.T) {
	cases := []struct {
		name     string
		response covCannedResponse
		wantMsg  string
	}{
		{
			name:     "providerRejectsTheToken",
			response: covTokenInfo(http.StatusBadRequest, `{"error":"invalid_token"}`),
			wantMsg:  "invalid google id_token",
		},
		{
			name:     "payloadDoesNotParse",
			response: covTokenInfo(http.StatusOK, `not json`),
			wantMsg:  "invalid google token payload",
		},
		{
			name:     "payloadHasNoEmail",
			response: covTokenInfo(http.StatusOK, `{"sub":"g"}`),
			wantMsg:  "invalid google token payload",
		},
		{
			name:     "googleSaysTheEmailIsUnverified",
			response: covTokenInfo(http.StatusOK, `{"email":"u@example.com","email_verified":"false"}`),
			wantMsg:  "google email is not verified",
		},
		{
			name:     "audienceBelongsToAnotherApp",
			response: covTokenInfo(http.StatusOK, `{"aud":"someone-elses-client","email":"u@example.com","email_verified":"true"}`),
			wantMsg:  "google token audience mismatch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			covGoogleInfoRig(t, map[string]covCannedResponse{covGoogleTokenInfo: tc.response})

			users := newMemUserStore()
			sessions := newMemRefreshStore()
			h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "cov-google-client")

			rec := httptest.NewRecorder()
			h.GoogleAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/google",
				map[string]string{"id_token": "t"}))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401 (body %s)", rec.Code, rec.Body.String())
			}
			assertJSONError(t, rec, tc.wantMsg)
			if len(users.byEmail) != 0 {
				t.Error("an account was created from a rejected token")
			}
			if sessions.stores != 0 {
				t.Error("a session was issued for a rejected token")
			}
		})
	}
}

func TestCovGoogleAuthValidatesItsBody(t *testing.T) {
	covGoogleInfoRig(t, map[string]covCannedResponse{
		covGoogleTokenInfo: covTokenInfo(http.StatusOK, `{"email":"u@example.com","email_verified":"true"}`),
	})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "cov-google-client")

	for _, body := range []any{`{`, map[string]string{"token": "x"}, map[string]string{"id_token": ""}} {
		rec := httptest.NewRecorder()
		h.GoogleAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/google", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %v returned %d, want 400", body, rec.Code)
		}
	}
}

func TestCovGoogleAuthReportsAnUnreachableProviderAsABadGateway(t *testing.T) {
	// 502, not 500: the failure is upstream, and the client may retry.
	swapTransport(t, covSilentTransport{})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "cov-google-client")

	rec := httptest.NewRecorder()
	h.GoogleAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/google",
		map[string]string{"id_token": "t"}))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 (body %s)", rec.Code, rec.Body.String())
	}
}

// ---- store-failure paths ---------------------------------------------------

// failingUserStore wraps the in-memory store and fails one named call, so the
// handler's "the write did not happen" branches are reachable.
type failingUserStore struct {
	*memUserStore
	fail  map[string]error
	calls map[string]int
}

func newFailingUserStore() *failingUserStore {
	return &failingUserStore{memUserStore: newMemUserStore(), fail: map[string]error{}, calls: map[string]int{}}
}

func (s *failingUserStore) failOn(method string, err error) *failingUserStore {
	s.fail[method] = err
	return s
}

func (s *failingUserStore) countOf(method string) int { return s.calls[method] }

func (s *failingUserStore) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	s.calls["MarkEmailVerified"]++
	if err := s.fail["MarkEmailVerified"]; err != nil {
		return err
	}
	return s.memUserStore.MarkEmailVerified(ctx, id)
}

func (s *failingUserStore) CreateVerificationToken(ctx context.Context, id uuid.UUID, token string) error {
	s.calls["CreateVerificationToken"]++
	if err := s.fail["CreateVerificationToken"]; err != nil {
		return err
	}
	return s.memUserStore.CreateVerificationToken(ctx, id, token)
}

func (s *failingUserStore) UpdateProfile(ctx context.Context, id uuid.UUID, username, bio, avatar string) (*domain.User, error) {
	s.calls["UpdateProfile"]++
	if err := s.fail["UpdateProfile"]; err != nil {
		return nil, err
	}
	return s.memUserStore.UpdateProfile(ctx, id, username, bio, avatar)
}

func TestCovRegisterDoesNotClaimVerificationItCouldNotRecord(t *testing.T) {
	// The mail service is configured, so registration mints a verification
	// token and marks the row verified. If that write fails, answering 200 with
	// IsVerified=true leaves the API and the database in contradiction and the
	// RequireVerifiedEmail gate then rejects the new user.
	t.Setenv("RESEND_API", "cov-key")
	t.Setenv("APP_URL", "https://app.example")
	swapTransport(t, covSilentTransport{})

	users := newFailingUserStore()
	users.failOn("CreateVerificationToken", errors.New("write timed out"))
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, email.NewService(), covJWTSecret, "")

	rec := httptest.NewRecorder()
	h.Register(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "new@example.com", "password": "Str0ng-Passw0rd!", "username": "new",
	}))

	// A failed token store is not a failed registration: the account exists.
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var env struct {
		User struct {
			IsVerified bool `json:"is_verified"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.User.IsVerified {
		t.Error("the response claims IsVerified=true although no verification token could be stored")
	}
	if users.countOf("CreateVerificationToken") != 1 {
		t.Errorf("CreateVerificationToken called %d times, want 1", users.countOf("CreateVerificationToken"))
	}
}

func TestCovVerifyEmailReportsAFailedVerificationWrite(t *testing.T) {
	users := newFailingUserStore()
	u := covNewUser(t, users.memUserStore, "v@example.com", "v")
	if err := users.CreateVerificationToken(t.Context(), u.id, "tok"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	users.failOn("MarkEmailVerified", errors.New("deadlock detected"))
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")

	rec := httptest.NewRecorder()
	h.VerifyEmail(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/verify-email",
		map[string]string{"token": "tok"}))

	// 500, not 200: the success page would tell the user their address is
	// verified while the row says otherwise.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestCovVerifyEmailWebReportsAFailedVerificationWrite(t *testing.T) {
	users := newFailingUserStore()
	u := covNewUser(t, users.memUserStore, "vw@example.com", "vw")
	if err := users.CreateVerificationToken(t.Context(), u.id, "tok"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	users.failOn("MarkEmailVerified", errors.New("deadlock detected"))
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")

	rec := httptest.NewRecorder()
	h.VerifyEmailWeb(rec, httptest.NewRequest(http.MethodGet, "/verify-email?token=tok", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestCovResetPasswordRejectsEveryTokenFailureIndistinguishably(t *testing.T) {
	// A burned token, a token for a vanished user and an outright database
	// failure must not be distinguishable, or the endpoint becomes an oracle
	// for which reset links were ever issued.
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")

	t.Run("tokenNotFound", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ResetPassword(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/reset-password",
			map[string]string{"token": "never-issued", "password": "New-Passw0rd!"}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
		assertJSONError(t, rec, "invalid or expired reset token")
	})

	t.Run("userRowGone", func(t *testing.T) {
		u := covNewUser(t, users, "gone@example.com", "gone")
		if err := users.CreatePasswordResetToken(t.Context(), u.id, "tok-gone"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := users.DeleteUser(t.Context(), u.id); err != nil {
			t.Fatalf("delete: %v", err)
		}
		rec := httptest.NewRecorder()
		h.ResetPassword(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/reset-password",
			map[string]string{"token": "tok-gone", "password": "New-Passw0rd!"}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
		assertJSONError(t, rec, "invalid or expired reset token")
	})

	t.Run("resetTokenIsSingleUse", func(t *testing.T) {
		u := covNewUser(t, users, "once@example.com", "once")
		if err := users.CreatePasswordResetToken(t.Context(), u.id, "tok-once"); err != nil {
			t.Fatalf("seed: %v", err)
		}
		first := httptest.NewRecorder()
		h.ResetPassword(first, covJSONRequest(t, http.MethodPost, "/api/v1/auth/reset-password",
			map[string]string{"token": "tok-once", "password": "New-Passw0rd!"}))
		if first.Code != http.StatusOK {
			t.Fatalf("first use got %d, want 200 (body %s)", first.Code, first.Body.String())
		}

		second := httptest.NewRecorder()
		h.ResetPassword(second, covJSONRequest(t, http.MethodPost, "/api/v1/auth/reset-password",
			map[string]string{"token": "tok-once", "password": "Another-Passw0rd!"}))
		if second.Code != http.StatusBadRequest {
			t.Errorf("a replayed reset link got %d, want 400", second.Code)
		}
	})
}

func TestCovResetPasswordValidatesItsInput(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")

	for _, body := range []any{
		`{`,
		map[string]string{"token": "t"},
		map[string]string{"password": "p"},
		map[string]string{"token": "", "password": "p"},
	} {
		rec := httptest.NewRecorder()
		h.ResetPassword(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/reset-password", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %v returned %d, want 400", body, rec.Code)
		}
	}
}

func TestCovChangePasswordRejectsAWrongCurrentPassword(t *testing.T) {
	// The hash comes from a real Register, so the comparison below is a real
	// one and a wrong password really does fail it.
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")

	seed := httptest.NewRecorder()
	h.Register(seed, covJSONRequest(t, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "cp@example.com", "password": "Original-Passw0rd!", "username": "cp",
	}))
	if seed.Code != http.StatusOK {
		t.Fatalf("seed registration got %d: %s", seed.Code, seed.Body.String())
	}
	var reg struct {
		User struct {
			ID uuid.UUID `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(seed.Body.Bytes(), &reg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	t.Run("wrongCurrent", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/change-password", map[string]string{
			"old_password": "not-it", "new_password": "New-Passw0rd!",
		})
		req = req.WithContext(covUserCtx(reg.User.ID))
		h.ChangePassword(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
		}
	})

	t.Run("missingFields", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/change-password", map[string]string{"old_password": "x"})
		req = req.WithContext(covUserCtx(reg.User.ID))
		h.ChangePassword(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
	})

	t.Run("vanishedUser", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/change-password", map[string]string{
			"old_password": "Original-Passw0rd!", "new_password": "New-Passw0rd!",
		})
		req = req.WithContext(covUserCtx(uuid.New()))
		h.ChangePassword(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	})
}

func TestCovUnlinkProviderRefusesTheLastLoginMethod(t *testing.T) {
	// An OAuth-only account has a placeholder address and no password, so
	// unlinking its last provider locks the user out permanently.
	users := newMemUserStore()
	u := covNewUser(t, users, "tg_42@telegram.oxide", "tele")
	if err := users.LinkTelegram(t.Context(), u.id, 42); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")

	rec := httptest.NewRecorder()
	req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/unlink", map[string]string{"provider": "telegram"})
	req = req.WithContext(covUserCtx(u.id))
	h.UnlinkProvider(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec, "cannot unlink the last login method")

	// An account with a real address may unlink freely.
	v := covNewUser(t, users, "real@example.com", "real")
	if err := users.LinkTelegram(t.Context(), v.id, 43); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ok := httptest.NewRecorder()
	okReq := covJSONRequest(t, http.MethodPost, "/api/v1/auth/unlink", map[string]string{"provider": "telegram"})
	okReq = okReq.WithContext(covUserCtx(v.id))
	h.UnlinkProvider(ok, okReq)
	if ok.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 for an account with a password (body %s)", ok.Code, ok.Body.String())
	}
	if users.unlinkedTm != 1 {
		t.Errorf("UnlinkTelegram ran %d times, want 1", users.unlinkedTm)
	}
}

func TestCovUnlinkProviderValidatesItsInput(t *testing.T) {
	users := newMemUserStore()
	u := covNewUser(t, users, "u@example.com", "u")
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")

	t.Run("unknownProvider", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/unlink", map[string]string{"provider": "myspace"})
		req = req.WithContext(covUserCtx(u.id))
		h.UnlinkProvider(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
	})

	t.Run("malformedBody", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/unlink", `{`)
		req = req.WithContext(covUserCtx(u.id))
		h.UnlinkProvider(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
	})

	t.Run("vanishedUser", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/unlink", map[string]string{"provider": "discord"})
		req = req.WithContext(covUserCtx(uuid.New()))
		h.UnlinkProvider(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	})
}

func TestCovGoogleAuthReportsADatabaseFailureAsAServerError(t *testing.T) {
	// A lookup error that is not "no such user" is a database fault. Mapping it
	// to "create a new account" would duplicate an identity the store merely
	// failed to return.
	covGoogleInfoRig(t, map[string]covCannedResponse{
		covGoogleTokenInfo: covTokenInfo(http.StatusOK,
			`{"aud":"cov-google-client","email":"flaky@example.com","email_verified":"true"}`),
	})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := transporthttp.NewAuthHandler(&covLookupsFailStore{memUserStore: users}, sessions,
		covNoEmailService(t), covJWTSecret, "cov-google-client")

	rec := httptest.NewRecorder()
	h.GoogleAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/google",
		map[string]string{"id_token": "t"}))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500 on a database fault (body %s)", rec.Code, rec.Body.String())
	}
}

// covLookupsFailStore fails every email lookup with an error that is not the
// repository's not-found sentinel.
type covLookupsFailStore struct {
	*memUserStore
}

func (s *covLookupsFailStore) GetUserByEmail(context.Context, string) (*domain.User, error) {
	return nil, errors.New("connection reset by peer")
}

func TestCovRepositorySentinelsAreDistinct(t *testing.T) {
	// The handler distinguishes these by identity, so collapsing them would
	// turn a database fault into "this address is free".
	if errors.Is(postgres.ErrUserNotFound, postgres.ErrEmailTaken) {
		t.Error("ErrUserNotFound and ErrEmailTaken are the same error")
	}
	if errors.Is(postgres.ErrUserNotFound, postgres.ErrResetTokenNotFound) {
		t.Error("ErrUserNotFound and ErrResetTokenNotFound are the same error")
	}
}
