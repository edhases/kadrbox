package http_test

// Coverage for the auth handlers whose bodies were previously unreachable.
//
// Two groups live here:
//
//   - Local handlers that needed a signed request context or a multipart body:
//     Me, VerifyEmail, ResendVerification, VerifyEmailWeb, ResetPasswordWeb and
//     UploadAvatar.
//   - The OAuth code-exchange handlers, which talk to Google and Discord. Those
//     use a client with a nil Transport, so swapping http.DefaultTransport for a
//     scripted RoundTripper intercepts them without a socket. Everything else
//     stays real: the state store, the user store and the session store.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/email"
	transporthttp "github.com/edhases/kadrbox-server/internal/transport/http"
	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
	"github.com/google/uuid"
)

const covJWTSecret = "cov-secret-key-that-is-long-enough"

// ---- harness ---------------------------------------------------------------

func covUserCtx(userID uuid.UUID) context.Context {
	return context.WithValue(context.Background(), middleware.UserIDKey, userID)
}

func covNewUser(t *testing.T, users *memUserStore, emailAddr, username string) *covUser {
	t.Helper()
	u, err := users.CreateUser(t.Context(), emailAddr, "argon2-hash", username)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return &covUser{id: u.ID, email: u.Email}
}

type covUser struct {
	id    uuid.UUID
	email string
}

func covJSONRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			reader = strings.NewReader(v)
		default:
			raw, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			reader = bytes.NewReader(raw)
		}
	}
	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// covAuth builds a handler with in-memory stores, mirroring production wiring.
func covAuth(users *memUserStore, sessions *memRefreshStore, emailSvc *email.Service) *transporthttp.AuthHandler {
	h := transporthttp.NewAuthHandler(users, sessions, emailSvc, covJWTSecret, "cov-google-client")
	h.SetOAuth("123:telegram-token", "oxidebot", "discord-client", "discord-secret", "https://app.example/auth/discord/callback", "https://app.example")
	return h
}

// covNoEmailService is a Service with no RESEND_API, i.e. the auto-verify mode.
// email.NewService reads the environment at construction, so the key is cleared
// for the duration of the test.
func covNoEmailService(t *testing.T) *email.Service {
	t.Helper()
	t.Setenv("RESEND_API", "")
	return email.NewService()
}

// ---- Me --------------------------------------------------------------------

func TestCovMeReturnsTheAuthenticatedProfile(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "me@example.com", "me")
	h := covAuth(users, sessions, covNoEmailService(t))

	rec := httptest.NewRecorder()
	req := covJSONRequest(t, http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(covUserCtx(u.id))

	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got struct {
		ID       uuid.UUID `json:"id"`
		Email    string    `json:"email"`
		Username string    `json:"username"`
		Password string    `json:"password_hash"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != u.id {
		t.Errorf("id = %s, want %s", got.ID, u.id)
	}
	if got.Email != "me@example.com" {
		t.Errorf("email = %q, want me@example.com", got.Email)
	}
	// The password hash must never leave the server.
	if got.Password != "" {
		t.Errorf("the response carried a password field: %q", got.Password)
	}
}

func TestCovMeRefusesAnUnauthenticatedOrVanishedAccount(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covAuth(users, sessions, covNoEmailService(t))

	t.Run("noUserInContext", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.Me(rec, covJSONRequest(t, http.MethodGet, "/api/v1/auth/me", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", rec.Code)
		}
	})

	t.Run("userRowIsGone", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodGet, "/api/v1/auth/me", nil)
		req = req.WithContext(covUserCtx(uuid.New()))
		h.Me(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404 for a token whose user no longer exists", rec.Code)
		}
	})
}

// ---- VerifyEmail -----------------------------------------------------------

func TestCovVerifyEmailWalksTheTokenToTheVerifiedFlag(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "verify@example.com", "verify")
	h := covAuth(users, sessions, covNoEmailService(t))

	if err := users.CreateVerificationToken(t.Context(), u.id, "tok-verify"); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	rec := httptest.NewRecorder()
	h.VerifyEmail(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/verify-email",
		map[string]string{"token": "tok-verify"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	stored, err := users.GetUserByID(t.Context(), u.id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !stored.IsVerified {
		t.Error("a 200 was written but the user is still unverified; the API and the database now disagree")
	}
}

func TestCovVerifyEmailRejectsBadAndUnknownTokens(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covAuth(users, sessions, covNoEmailService(t))

	cases := []struct {
		name string
		body any
	}{
		{name: "malformedJSON", body: `{`},
		{name: "noTokenField", body: map[string]string{"tokenn": "x"}},
		{name: "emptyToken", body: map[string]string{"token": ""}},
		{name: "unknownToken", body: map[string]string{"token": "never-issued"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.VerifyEmail(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/verify-email", tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCovVerifyEmailWebRendersAnOutcomePage(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "web@example.com", "web")
	h := covAuth(users, sessions, covNoEmailService(t))

	t.Run("unknownToken", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.VerifyEmailWeb(rec, httptest.NewRequest(http.MethodGet, "/verify-email?token=nope", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "недійсне") {
			t.Errorf("body does not explain the failure: %s", rec.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		if err := users.CreateVerificationToken(t.Context(), u.id, "tok-web"); err != nil {
			t.Fatalf("seed token: %v", err)
		}
		rec := httptest.NewRecorder()
		h.VerifyEmailWeb(rec, httptest.NewRequest(http.MethodGet, "/verify-email?token=tok-web", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		stored, _ := users.GetUserByID(t.Context(), u.id)
		if !stored.IsVerified {
			t.Error("the success page was shown but the user is still unverified")
		}
	})
}

func TestCovResetPasswordWebRendersTheFormOnlyForALiveToken(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "reset-web@example.com", "resetweb")
	h := covAuth(users, sessions, covNoEmailService(t))

	rec := httptest.NewRecorder()
	h.ResetPasswordWeb(rec, httptest.NewRequest(http.MethodGet, "/reset-password?token=nope", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 for an unknown token", rec.Code)
	}

	if err := users.CreatePasswordResetToken(t.Context(), u.id, "tok-reset"); err != nil {
		t.Fatalf("seed reset token: %v", err)
	}
	rec = httptest.NewRecorder()
	h.ResetPasswordWeb(rec, httptest.NewRequest(http.MethodGet, "/reset-password?token=tok-reset", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tok-reset") {
		t.Error("the rendered form does not carry the token back")
	}

	// The lookup consumed the token, so a second visit must not re-render the
	// form with a dead token.
	rec = httptest.NewRecorder()
	h.ResetPasswordWeb(rec, httptest.NewRequest(http.MethodGet, "/reset-password?token=tok-reset", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d on the second visit, want 400: a reset link must be single-use", rec.Code)
	}
}

// ---- ResendVerification ----------------------------------------------------

func TestCovResendVerificationRequiresAConfiguredEmailService(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covAuth(users, sessions, covNoEmailService(t))

	rec := httptest.NewRecorder()
	h.ResendVerification(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/resend-verification",
		map[string]string{"email": "someone@example.com"}))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 with no mail service configured", rec.Code)
	}
}

func TestCovResendVerificationNeverRevealsWhetherAnAccountExists(t *testing.T) {
	t.Setenv("RESEND_API", "cov-resend-key")
	t.Setenv("SMTP_FROM", "noreply@example.com")
	t.Setenv("APP_URL", "https://app.example")
	swapTransport(t, covSilentTransport{})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covAuth(users, sessions, email.NewService())

	rec := httptest.NewRecorder()
	h.ResendVerification(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/resend-verification",
		map[string]string{"email": "ghost@example.com"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("an unknown address returned %d, want the same 200 as a known one", rec.Code)
	}
	unknownBody := rec.Body.String()
	if !strings.Contains(unknownBody, "if the email exists") {
		t.Errorf("body = %s, want the non-committal message", unknownBody)
	}
	if len(users.verificationTokens) != 0 {
		t.Error("a verification token was minted for an address that does not exist")
	}
}

func TestCovResendVerificationTellsAnAlreadyVerifiedUserNothingNew(t *testing.T) {
	// This used to assert the opposite: that the body said "already verified".
	// That was an enumeration oracle behind a 200 -- registered-and-verified
	// answered differently from unknown, so the endpoint said which addresses
	// exist. Now all three outcomes share one sentence.
	t.Setenv("RESEND_API", "cov-resend-key")
	t.Setenv("APP_URL", "https://app.example")
	swapTransport(t, covSilentTransport{})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "done@example.com", "done")
	if err := users.MarkEmailVerified(t.Context(), u.id); err != nil {
		t.Fatalf("mark verified: %v", err)
	}
	h := covAuth(users, sessions, email.NewService())

	post := func(emailAddr string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ResendVerification(rec, covJSONRequest(t, http.MethodPost,
			"/api/v1/auth/resend-verification", map[string]string{"email": emailAddr}))
		return rec
	}

	verified := post("done@example.com")
	unknown := post("nobody@example.com")

	if verified.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", verified.Code)
	}
	if verified.Body.String() != unknown.Body.String() {
		t.Errorf("a verified account and an unknown address answer differently, "+
			"which leaks which addresses exist:\n  verified: %s\n  unknown:   %s",
			verified.Body.String(), unknown.Body.String())
	}
	if strings.Contains(strings.ToLower(verified.Body.String()), "already verified") {
		t.Errorf("body = %s, still states that the address is registered", verified.Body.String())
	}
	if len(users.verificationTokens) != 0 {
		t.Error("a new token was minted for an already-verified account")
	}
}

func TestCovResendVerificationValidatesTheEmailField(t *testing.T) {
	t.Setenv("RESEND_API", "cov-resend-key")
	t.Setenv("APP_URL", "https://app.example")
	swapTransport(t, covSilentTransport{})

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covAuth(users, sessions, email.NewService())

	for _, body := range []any{`{`, map[string]string{"email": ""}, map[string]string{"emai": "a@b.c"}} {
		rec := httptest.NewRecorder()
		h.ResendVerification(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/resend-verification", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %v returned %d, want 400", body, rec.Code)
		}
	}
}

func TestCovForgotPasswordValidatesItsInput(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covAuth(users, sessions, covNoEmailService(t))

	for _, body := range []any{`{`, map[string]string{"email": ""}} {
		rec := httptest.NewRecorder()
		h.ForgotPassword(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/forgot-password", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %v returned %d, want 400", body, rec.Code)
		}
	}
}

// ---- UploadAvatar ----------------------------------------------------------

// covAvatarRequest builds a multipart avatar upload. content is the file body,
// so a test can present a .png name with non-image bytes.
func covAvatarRequest(t *testing.T, userID uuid.UUID, field, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if field != "" {
		part, err := w.CreateFormFile(field, filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/avatar", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req.WithContext(covUserCtx(userID))
}

// PNG magic bytes, enough for http.DetectContentType to report image/png.
var covPNGMagic = append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 600)...)

func TestCovUploadAvatarRejectsUnauthenticatedAndMalformedRequests(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "avatar@example.com", "avatar")
	h := covAuth(users, sessions, covNoEmailService(t))

	t.Run("noUserInContext", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covAvatarRequest(t, u.id, "avatar", "a.png", covPNGMagic)
		req = req.WithContext(context.Background())
		h.UploadAvatar(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401", rec.Code)
		}
	})

	t.Run("notMultipart", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := covJSONRequest(t, http.MethodPost, "/api/v1/auth/avatar", map[string]string{"x": "y"})
		req = req.WithContext(covUserCtx(u.id))
		h.UploadAvatar(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 for a non-multipart body", rec.Code)
		}
	})

	t.Run("noAvatarField", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.UploadAvatar(rec, covAvatarRequest(t, u.id, "photo", "a.png", covPNGMagic))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 when the field is named something else", rec.Code)
		}
	})

	t.Run("fieldMissingEntirely", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.UploadAvatar(rec, covAvatarRequest(t, u.id, "", "", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 with no file part", rec.Code)
		}
	})
}

func TestCovUploadAvatarChecksTheExtensionAndThenTheBytes(t *testing.T) {
	// Two separate gates: the extension is the cheap filter, the magic bytes
	// are the one that matters. A .png name over a text body is the case an
	// extension-only check lets through.
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "ext@example.com", "ext")
	h := covAuth(users, sessions, covNoEmailService(t))
	t.Chdir(t.TempDir())

	cases := []struct {
		name     string
		filename string
		content  []byte
		wantMsg  string
	}{
		{name: "executableExtension", filename: "payload.php", content: covPNGMagic, wantMsg: "only .jpg, .jpeg, .png, and .webp images are allowed"},
		{name: "noExtension", filename: "avatar", content: covPNGMagic, wantMsg: "only .jpg, .jpeg, .png, and .webp images are allowed"},
		{name: "svgIsRejected", filename: "vector.svg", content: covPNGMagic, wantMsg: "only .jpg, .jpeg, .png, and .webp images are allowed"},
		{name: "pngNameOverTextBytes", filename: "a.png", content: []byte("<?php system($_GET[0]); ?>"), wantMsg: "invalid image content type: text/plain"},
		{name: "htmlNameOverHTMLBytes", filename: "a.jpg", content: []byte("<html><script>alert(1)</script></html>"), wantMsg: "invalid image content type: text/html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.UploadAvatar(rec, covAvatarRequest(t, u.id, "avatar", tc.filename, tc.content))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			// The message names the detected type, which is what lets an
			// operator tell a mis-named file from a non-image upload.
			assertJSONErrorPrefix(t, rec, tc.wantMsg)
		})
	}

	// Nothing may have been written.
	if entries, err := os.ReadDir("./data/uploads/avatars"); err == nil && len(entries) != 0 {
		t.Errorf("rejected uploads left %d files on disk: %v", len(entries), entries)
	}
}

func TestCovUploadAvatarStoresTheFileAndUpdatesTheProfile(t *testing.T) {
	users := newMemUserStore()
	sessions := newMemRefreshStore()
	u := covNewUser(t, users, "ok@example.com", "ok")
	h := covAuth(users, sessions, covNoEmailService(t))
	// The handler writes to a fixed relative path, so the working directory
	// decides where: point it at a temp dir rather than the repo.
	t.Chdir(t.TempDir())

	rec := httptest.NewRecorder()
	h.UploadAvatar(rec, covAvatarRequest(t, u.id, "avatar", "me.PNG", covPNGMagic))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var got struct {
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The extension is lowercased before it is used, so an uppercase name
	// cannot produce a URL the static handler will not serve.
	if !strings.Contains(got.AvatarURL, u.id.String()+".png") {
		t.Errorf("avatar_url = %q, want it to name %s.png", got.AvatarURL, u.id)
	}
	if !strings.Contains(got.AvatarURL, "?v=") {
		t.Errorf("avatar_url = %q, want a cache-busting version query", got.AvatarURL)
	}

	stored, err := users.GetUserByID(t.Context(), u.id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.AvatarURL != got.AvatarURL {
		t.Errorf("stored avatar = %q, response said %q", stored.AvatarURL, got.AvatarURL)
	}

	want := filepath.Join("data", "uploads", "avatars", u.id.String()+".png")
	raw, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("the uploaded bytes were not written to %s: %v", want, err)
	}
	if !bytes.HasPrefix(raw, covPNGMagic[:8]) {
		t.Errorf("stored %d bytes, want the uploaded PNG verbatim", len(raw))
	}
}

// ---- OAuth: transport interception ----------------------------------------

// covScriptedTransport answers each registered URL with a canned response and
// records the requests it saw, so a test can assert on the token request's form.
type covScriptedTransport struct {
	responses map[string]covCannedResponse
	seen      []*http.Request
	forms     map[string]url.Values
}

type covCannedResponse struct {
	status int
	body   string
}

func (s *covScriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Read and restore the body so the recorded request is still usable, and
	// so a form-encoded POST can be asserted on. A GET has no body at all.
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	s.seen = append(s.seen, req)

	if form, err := url.ParseQuery(string(body)); err == nil && len(form) > 0 {
		if s.forms == nil {
			s.forms = map[string]url.Values{}
		}
		s.forms[req.URL.String()] = form
	}

	canned, ok := s.responses[req.URL.Host+req.URL.Path]
	if !ok {
		return nil, fmt.Errorf("cov transport: unexpected request to %s", req.URL.String())
	}
	return &http.Response{
		StatusCode: canned.status,
		Body:       io.NopCloser(strings.NewReader(canned.body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

func (s *covScriptedTransport) requestsTo(hostPath string) []*http.Request {
	var out []*http.Request
	for _, r := range s.seen {
		if r.URL.Host+r.URL.Path == hostPath {
			out = append(out, r)
		}
	}
	return out
}

// covSilentTransport refuses every request. It stands in for a mail provider
// that is unreachable, so a handler's "the send failed" branch is reachable
// without a network.
type covSilentTransport struct{}

func (covSilentTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("cov transport: network disabled")
}

// swapTransport installs tr as the process-wide default transport for the
// duration of the test and restores it afterwards. Every client in this
// package is built with a nil Transport, so this is the single seam.
func swapTransport(t *testing.T, tr http.RoundTripper) {
	t.Helper()
	prev := http.DefaultTransport
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = prev })
}

const (
	covGoogleTokenURL = "oauth2.googleapis.com/token"
	covGoogleInfoURL  = "www.googleapis.com/oauth2/v3/userinfo"
	covDiscordToken   = "discord.com/api/oauth2/token"
	covDiscordUser    = "discord.com/api/users/@me"
)

func covGoogleTransport(t *testing.T, userinfo string) *covScriptedTransport {
	t.Helper()
	tr := &covScriptedTransport{responses: map[string]covCannedResponse{
		covGoogleTokenURL: {status: http.StatusOK, body: `{"access_token":"ga","token_type":"Bearer","expires_in":3600}`},
		covGoogleInfoURL:  {status: http.StatusOK, body: userinfo},
	}}
	swapTransport(t, tr)
	return tr
}

func covDiscordTransport(t *testing.T, userJSON string) *covScriptedTransport {
	t.Helper()
	tr := &covScriptedTransport{responses: map[string]covCannedResponse{
		covDiscordToken: {status: http.StatusOK, body: `{"access_token":"da","token_type":"Bearer"}`},
		covDiscordUser:  {status: http.StatusOK, body: userJSON},
	}}
	swapTransport(t, tr)
	return tr
}

// mintState writes one OAuth state record and returns the opaque nonce to echo
// back on the callback.
//
// The record mirrors the production shape — provider, PKCE verifier, link
// target and creation time — with the session hash left empty, which the
// handler treats as "unbound" and accepts. Going through the handler's own
// store means the consume is single-use, exactly as in production.
// covCallbackRedirect is a validated post-login destination: an allow-listed
// origin, so the flow that stores it would have accepted it.
const covCallbackRedirect = "https://film.oxideteam.pp.ua/auth/done"

func mintState(t *testing.T, store transporthttp.OAuthStateStore, providerName string, withPKCE bool, linkUserID string) string {
	t.Helper()
	return mintStateTo(t, store, providerName, withPKCE, linkUserID, covCallbackRedirect)
}

// mintStateTo additionally records a validated post-login destination. With one
// the handler redirects to it; without one it renders a status page. The
// destination is stored server-side precisely so the callback cannot change it.
func mintStateTo(t *testing.T, store transporthttp.OAuthStateStore, providerName string, withPKCE bool, linkUserID, redirectTo string) string {
	t.Helper()
	rec := map[string]any{
		"provider":   providerName,
		"created_at": time.Now().Unix(),
	}
	if redirectTo != "" {
		rec["redirect_to"] = redirectTo
	}
	if linkUserID != "" {
		rec["link_user_id"] = linkUserID
	}
	if withPKCE {
		rec["code_verifier"] = "cov-pkce-verifier"
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal state record: %v", err)
	}
	state := fmt.Sprintf("cov-state-%s-%d", providerName, time.Now().UnixNano())
	if err := store.SetOAuthState(t.Context(), state, payload, 10*time.Minute); err != nil {
		t.Fatalf("store state: %v", err)
	}
	return state
}

// covSessionStore is a session store that is also the OAuth state store, which
// is what NewAuthHandler discovers by type assertion on the production Redis
// client.
type covSessionStore struct {
	*memRefreshStore
}

func newCovSessionStore() *covSessionStore {
	return &covSessionStore{memRefreshStore: newMemRefreshStore()}
}

var (
	_ transporthttp.RefreshStore    = (*covSessionStore)(nil)
	_ transporthttp.OAuthStateStore = (*covSessionStore)(nil)
)

// covOAuthRig wires a handler whose OAuth state lives in covSessionStore, so a
// test can mint a state record and drive the callback directly.
func covOAuthRig(t *testing.T, users *memUserStore, googleClientID string) (*transporthttp.AuthHandler, *covSessionStore) {
	t.Helper()
	sessions := newCovSessionStore()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, googleClientID)
	return h, sessions
}

func covGoogleRig(t *testing.T, users *memUserStore) (*transporthttp.AuthHandler, *covSessionStore) {
	t.Helper()
	h, sessions := covOAuthRig(t, users, "cov-google-client")
	h.SetGoogleOAuth("cov-google-client", "cov-google-secret", "https://app.example/auth/google/callback")
	h.SetOAuth("123:telegram-token", "oxidebot", "cid", "csecret", "https://app.example/cb", "https://app.example")
	return h, sessions
}

func TestCovGoogleCallbackSignsInANewUser(t *testing.T) {
	tr := covGoogleTransport(t, `{"sub":"g1","email":"new@example.com","email_verified":true,"name":"New User","picture":"https://cdn/p.png"}`)

	users := newMemUserStore()
	h, sessions := covGoogleRig(t, users)
	state := mintState(t, sessions, "google", true, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/google/callback?state="+state+"&code=auth-code", nil)
	h.GoogleCallback(rec, req)

	assertCovRedirectCarriesTokens(t, rec)

	stored, err := users.GetUserByEmail(t.Context(), "new@example.com")
	if err != nil {
		t.Fatalf("the account was not created: %v", err)
	}
	if !stored.IsVerified {
		t.Error("a Google identity is verified by the provider, but the row says otherwise")
	}
	if stored.AvatarURL != "https://cdn/p.png" {
		t.Errorf("avatar = %q, want the Google picture carried through", stored.AvatarURL)
	}

	// The exchange must send the PKCE verifier and the redirect URI the flow
	// was started with; without the verifier the code is useless to an
	// interceptor and the exchange would fail against the real provider.
	forms := tr.forms["https://"+covGoogleTokenURL]
	if forms == nil {
		t.Fatal("no token request was made")
	}
	if forms.Get("code_verifier") == "" {
		t.Error("the token request carried no code_verifier")
	}
	if forms.Get("grant_type") != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", forms.Get("grant_type"))
	}
	if forms.Get("code") != "auth-code" {
		t.Errorf("code = %q, want the code from the callback", forms.Get("code"))
	}
	if got := tr.requestsTo(covGoogleInfoURL)[0].Header.Get("Authorization"); got != "Bearer ga" {
		t.Errorf("userinfo Authorization = %q, want %q", got, "Bearer ga")
	}
}

func TestCovGoogleCallbackUpgradesAnExistingUnverifiedAccount(t *testing.T) {
	covGoogleTransport(t, `{"sub":"g2","email":"old@example.com","email_verified":true,"name":"Old"}`)

	users := newMemUserStore()
	existing := covNewUser(t, users, "old@example.com", "old")
	h, sessions := covGoogleRig(t, users)
	state := mintState(t, sessions, "google", true, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/google/callback?state="+state+"&code=c", nil)
	h.GoogleCallback(rec, req)

	assertCovRedirectCarriesTokens(t, rec)
	stored, _ := users.GetUserByID(t.Context(), existing.id)
	if !stored.IsVerified {
		t.Error("Google asserted the address is verified but the row was left unverified; the 403 gate would now reject this user")
	}
}

func TestCovGoogleCallbackReportsProviderFailures(t *testing.T) {
	cases := []struct {
		name      string
		responses map[string]covCannedResponse
		wantText  string
	}{
		{
			name: "tokenExchangeRejected",
			responses: map[string]covCannedResponse{
				covGoogleTokenURL: {status: http.StatusBadRequest, body: `{"error":"invalid_grant","error_description":"code already used"}`},
			},
			wantText: "code already used",
		},
		{
			name: "tokenExchangeReturnsNoAccessToken",
			responses: map[string]covCannedResponse{
				covGoogleTokenURL: {status: http.StatusOK, body: `{"token_type":"Bearer"}`},
			},
			wantText: "google token error",
		},
		{
			name: "userinfoRejects",
			responses: map[string]covCannedResponse{
				covGoogleTokenURL: {status: http.StatusOK, body: `{"access_token":"ga"}`},
				covGoogleInfoURL:  {status: http.StatusUnauthorized, body: `{}`},
			},
			wantText: "failed to fetch google user info",
		},
		{
			name: "userinfoHasNoEmail",
			responses: map[string]covCannedResponse{
				covGoogleTokenURL: {status: http.StatusOK, body: `{"access_token":"ga"}`},
				covGoogleInfoURL:  {status: http.StatusOK, body: `{"sub":"g"}`},
			},
			wantText: "failed to fetch google user info",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			swapTransport(t, &covScriptedTransport{responses: tc.responses})

			users := newMemUserStore()
			h, sessions := covGoogleRig(t, users)
			state := mintState(t, sessions, "google", true, "")

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/auth/google/callback?state="+state+"&code=c", nil)
			h.GoogleCallback(rec, req)

			assertCovRedirectCarriesError(t, rec, tc.wantText)
		})
	}
}

func TestCovGoogleCallbackRejectsAnUnverifiableState(t *testing.T) {
	covGoogleTransport(t, `{"sub":"g","email":"x@example.com","email_verified":true}`)

	users := newMemUserStore()
	h, sessions := covGoogleRig(t, users)

	cases := []struct {
		name string
		url  string
	}{
		{name: "noState", url: "/api/v1/auth/google/callback?code=c"},
		{name: "unknownState", url: "/api/v1/auth/google/callback?state=never-issued&code=c"},
		{name: "telegramState", url: "/api/v1/auth/google/callback?state=" + mintState(t, sessions, "telegram", true, "") + "&code=c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.GoogleCallback(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if len(users.byEmail) != 0 {
				t.Error("a callback with an unusable state still reached the user store")
			}
		})
	}
}

func TestCovGoogleCallbackReportsAMissingCodeWithoutEchoingTheProvider(t *testing.T) {
	covGoogleTransport(t, `{}`)

	users := newMemUserStore()
	h, sessions := covGoogleRig(t, users)
	state := mintState(t, sessions, "google", true, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/google/callback?state="+state+"&error=access_denied&error_description=%3Cscript%3Ealert(1)%3C/script%3E", nil)
	h.GoogleCallback(rec, req)

	// The provider's description reaches the client, so it has to be escaped.
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Errorf("the provider message was not escaped: %s", body)
	}
}

func TestCovGoogleCallbackRefusesToLinkSomebodyElsesGoogleAccount(t *testing.T) {
	// The link target is carried in the state record. Completing with a
	// different Google identity must not attach it to the linker's account.
	covGoogleTransport(t, `{"sub":"other","email":"attacker@example.com","email_verified":true}`)

	users := newMemUserStore()
	victim := covNewUser(t, users, "victim@example.com", "victim")
	h, sessions := covGoogleRig(t, users)
	state := mintState(t, sessions, "google", true, victim.id.String())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/google/callback?state="+state+"&code=c", nil)
	h.GoogleCallback(rec, req)

	assertCovRedirectCarriesError(t, rec, "іншому користувачу")

	attacker, err := users.GetUserByEmail(t.Context(), "attacker@example.com")
	if err == nil && attacker.ID == victim.id {
		t.Error("the attacker's Google identity was attached to the victim's account")
	}
}

// ---- OAuth: Discord --------------------------------------------------------

// covDiscordRig wires a handler with Discord credentials configured; the API
// exchange refuses outright without them, since it has no other client id.
func covDiscordRig(t *testing.T, users *memUserStore, sessions transporthttp.RefreshStore) *transporthttp.AuthHandler {
	t.Helper()
	h := transporthttp.NewAuthHandler(users, sessions, covNoEmailService(t), covJWTSecret, "")
	h.SetOAuth("", "", "cov-discord-client", "cov-discord-secret", covDiscordRedirectURI, "https://app.example")
	return h
}

const covDiscordRedirectURI = "https://app.example/auth/discord/callback"

func TestCovDiscordAuthAPICreatesAUserWithADiscordIdentity(t *testing.T) {
	covDiscordTransport(t, `{"id":"d1","username":"disc","global_name":"Disc User","email":"disc@example.com","verified":true,"avatar":"abc"}`)

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covDiscordRig(t, users, sessions)

	rec := httptest.NewRecorder()
	h.DiscordAuthAPI(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/discord", map[string]string{
		"code":          "discord-code",
		"code_verifier": "the-verifier",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	stored, err := users.GetUserByDiscordID(t.Context(), "d1")
	if err != nil {
		t.Fatalf("the discord account was not linked: %v", err)
	}
	if stored.Email != "disc@example.com" {
		t.Errorf("email = %q, want the Discord address", stored.Email)
	}
	if stored.AvatarURL != "https://cdn.discordapp.com/avatars/d1/abc.png" {
		t.Errorf("avatar = %q, want the CDN URL built from the Discord id and hash", stored.AvatarURL)
	}
	if sessions.stores != 1 {
		t.Errorf("%d refresh tokens stored, want 1", sessions.stores)
	}
}

func TestCovDiscordAuthAPIUsesAPlaceholderWhenDiscordHasNoEmail(t *testing.T) {
	// Discord can be configured without the email scope. Without a placeholder
	// there would be no unique account to attach the identity to.
	covDiscordTransport(t, `{"id":"d2","username":"noemail"}`)

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	h := covDiscordRig(t, users, sessions)

	rec := httptest.NewRecorder()
	h.DiscordAuthAPI(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/discord", map[string]string{
		"code":          "c",
		"code_verifier": "v",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	stored, err := users.GetUserByDiscordID(t.Context(), "d2")
	if err != nil {
		t.Fatalf("not linked: %v", err)
	}
	if stored.Email != "discord_d2@discord.oxide" {
		t.Errorf("email = %q, want the documented placeholder", stored.Email)
	}
	if stored.Username != "noemail" {
		t.Errorf("username = %q, want the Discord username", stored.Username)
	}
}

func TestCovDiscordAuthAPILinksToAnExistingAccountWithTheSameEmail(t *testing.T) {
	covDiscordTransport(t, `{"id":"d3","username":"linked","email":"both@example.com","verified":true}`)

	users := newMemUserStore()
	existing := covNewUser(t, users, "both@example.com", "existing")
	sessions := newMemRefreshStore()
	h := covDiscordRig(t, users, sessions)

	rec := httptest.NewRecorder()
	h.DiscordAuthAPI(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/discord", map[string]string{
		"code": "c", "code_verifier": "v",
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	linked, err := users.GetUserByDiscordID(t.Context(), "d3")
	if err != nil {
		t.Fatalf("the discord id was not linked: %v", err)
	}
	// Linking must attach to the existing account, not create a second one for
	// the same address.
	if linked.ID != existing.id {
		t.Errorf("linked to %s, want the existing account %s", linked.ID, existing.id)
	}
	if !linked.IsVerified {
		t.Error("Discord asserted the address is verified but the row was left unverified")
	}
	if len(users.byEmail) != 1 {
		t.Errorf("%d accounts for one address, want 1", len(users.byEmail))
	}
}

func TestCovDiscordAuthAPIRequiresTheCodeAndAVerifier(t *testing.T) {
	users := newMemUserStore()
	h := covDiscordRig(t, users, newCovSessionStore())

	t.Run("noCode", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.DiscordAuthAPI(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/discord", map[string]string{}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
	})

	t.Run("malformedBody", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.DiscordAuthAPI(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/discord", `{`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
	})

	t.Run("noVerifier", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.DiscordAuthAPI(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/discord",
			map[string]string{"code": "c"}))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
		}
		// This is a distinct, actionable message: the client knows to send a
		// PKCE verifier rather than that its code is bad.
		assertJSONError(t, rec, "code_verifier is required")
	})
}

func TestCovDiscordCallbackSignsInThroughTheBrowserFlow(t *testing.T) {
	covDiscordTransport(t, `{"id":"d4","username":"browser","email":"browser@example.com","verified":true}`)

	users := newMemUserStore()
	sessions := newCovSessionStore()
	h := covDiscordRig(t, users, sessions)
	state := mintState(t, sessions, "discord", true, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/discord/callback?state="+state+"&code=dc", nil)
	h.DiscordCallback(rec, req)

	assertCovRedirectCarriesTokens(t, rec)
	if _, err := users.GetUserByDiscordID(t.Context(), "d4"); err != nil {
		t.Errorf("the identity was not stored: %v", err)
	}
}

func TestCovDiscordCallbackLinksWhenTheStateNamesAnAccount(t *testing.T) {
	covDiscordTransport(t, `{"id":"d5","username":"linkme","email":"linkme@example.com","verified":true}`)

	users := newMemUserStore()
	target := covNewUser(t, users, "linkme@example.com", "target")
	sessions := newCovSessionStore()
	h := covDiscordRig(t, users, sessions)
	state := mintState(t, sessions, "discord", true, target.id.String())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/discord/callback?state="+state+"&code=dc", nil)
	h.DiscordCallback(rec, req)

	assertCovRedirectCarriesTokens(t, rec)
	linked, err := users.GetUserByDiscordID(t.Context(), "d5")
	if err != nil {
		t.Fatalf("the link did not happen: %v", err)
	}
	if linked.ID != target.id {
		t.Errorf("linked to %s, want the state record's account %s", linked.ID, target.id)
	}
}

func TestCovDiscordCallbackRefusesToStealAnIdentityHeldByAnotherAccount(t *testing.T) {
	covDiscordTransport(t, `{"id":"d6","username":"owned"}`)

	users := newMemUserStore()
	owner := covNewUser(t, users, "owner@example.com", "owner")
	if err := users.LinkDiscord(t.Context(), owner.id, "d6"); err != nil {
		t.Fatalf("seed discord link: %v", err)
	}
	attacker := covNewUser(t, users, "attacker@example.com", "attacker")

	sessions := newCovSessionStore()
	h := covDiscordRig(t, users, sessions)
	state := mintState(t, sessions, "discord", true, attacker.id.String())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/discord/callback?state="+state+"&code=dc", nil)
	h.DiscordCallback(rec, req)

	assertCovRedirectCarriesError(t, rec, "вже прив'язано")
}

// ---- OAuth: Telegram -------------------------------------------------------

// covTelegramHash computes the Telegram Login Widget HMAC so the tests can
// present a genuinely valid payload rather than a hand-waved one.
func covTelegramHash(botToken string, req map[string]string) string {
	keys := make([]string, 0, len(req))
	for k := range req {
		if k == "hash" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+req[k])
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

func covTelegramPayload(botToken string, id int64) map[string]any {
	fields := map[string]string{
		"id":         strconv.FormatInt(id, 10),
		"first_name": "Tele",
		"username":   "teleuser",
		"auth_date":  strconv.FormatInt(time.Now().Unix(), 10),
	}
	return map[string]any{
		"id":         id,
		"first_name": fields["first_name"],
		"username":   fields["username"],
		"auth_date":  mustParseInt(fields["auth_date"]),
		"hash":       covTelegramHash(botToken, fields),
	}
}

func mustParseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func TestCovTelegramAuthCreatesAndThenReusesAnAccount(t *testing.T) {
	const botToken = "123:telegram-token"
	sessions := newMemRefreshStore()

	t.Run("firstSignInCreatesTheAccount", func(t *testing.T) {
		users := newMemUserStore()
		h := covDiscordRig(t, users, sessions)
		h.SetOAuth(botToken, "oxidebot", "", "", "", "https://app.example")

		rec := httptest.NewRecorder()
		h.TelegramAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/telegram",
			covTelegramPayload(botToken, 555)))

		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		created, err := users.GetUserByTelegramID(t.Context(), 555)
		if err != nil {
			t.Fatalf("the telegram account was not created: %v", err)
		}
		// An OAuth-only account has no usable password, so it gets a
		// placeholder address rather than an empty one that would collide
		// with every other OAuth user.
		if created.Email != "tg_555@telegram.oxide" {
			t.Errorf("email = %q, want the documented placeholder", created.Email)
		}
		if created.Username != "teleuser" {
			t.Errorf("username = %q, want the telegram username", created.Username)
		}
	})

	t.Run("secondSignInReusesTheSameAccount", func(t *testing.T) {
		users := newMemUserStore()
		first := covNewUser(t, users, "tg_777@telegram.oxide", "teleuser")
		if err := users.LinkTelegram(t.Context(), first.id, 777); err != nil {
			t.Fatalf("seed link: %v", err)
		}
		h := covDiscordRig(t, users, sessions)
		h.SetOAuth(botToken, "oxidebot", "", "", "", "https://app.example")

		rec := httptest.NewRecorder()
		h.TelegramAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/telegram",
			covTelegramPayload(botToken, 777)))

		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
		}
		if len(users.byEmail) != 1 {
			t.Errorf("%d accounts after a second sign-in, want 1", len(users.byEmail))
		}
	})
}

func TestCovTelegramAuthRejectsUnsignedAndStalePayloads(t *testing.T) {
	const botToken = "123:telegram-token"
	users := newMemUserStore()
	h := covDiscordRig(t, users, newMemRefreshStore())
	h.SetOAuth(botToken, "oxidebot", "", "", "", "https://app.example")

	stale := covTelegramPayload(botToken, 9)
	stale["auth_date"] = time.Now().Add(-48 * time.Hour).Unix()
	stale["hash"] = covTelegramHash(botToken, map[string]string{
		"id": "9", "first_name": "Tele", "username": "teleuser",
		"auth_date": strconv.FormatInt(time.Now().Add(-48*time.Hour).Unix(), 10),
	})

	future := covTelegramPayload(botToken, 10)
	future["auth_date"] = time.Now().Add(time.Hour).Unix()
	future["hash"] = covTelegramHash(botToken, map[string]string{
		"id": "10", "first_name": "Tele", "username": "teleuser",
		"auth_date": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
	})

	// id 0 is Telegram's "unassigned" value, so it must be refused even with a
	// signature that would otherwise check out: an account keyed on 0 would be
	// shared by every such login.
	zeroID := map[string]any{
		"id": 0, "first_name": "Tele", "username": "teleuser",
		"auth_date": time.Now().Unix(),
	}
	zeroID["hash"] = covTelegramHash(botToken, map[string]string{
		"id": "0", "first_name": "Tele", "username": "teleuser",
		"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
	})

	noAuthDate := map[string]any{
		"id": 5, "first_name": "Tele", "username": "teleuser",
		"auth_date": 0,
	}
	noAuthDate["hash"] = covTelegramHash(botToken, map[string]string{
		"id": "5", "first_name": "Tele", "username": "teleuser", "auth_date": "0",
	})

	cases := []struct {
		name string
		body any
	}{
		{name: "noHash", body: map[string]any{"id": 1, "auth_date": time.Now().Unix()}},
		{name: "wrongHash", body: map[string]any{"id": 1, "auth_date": time.Now().Unix(), "hash": strings.Repeat("0", 64)}},
		{name: "noID", body: map[string]any{"auth_date": time.Now().Unix(), "hash": strings.Repeat("0", 64)}},
		{name: "zeroIDWithAValidSignature", body: zeroID},
		{name: "zeroAuthDateWithAValidSignature", body: noAuthDate},
		{name: "staleAuthDate", body: stale},
		{name: "authDateFromTheFuture", body: future},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.TelegramAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/telegram", tc.body))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}

	// A body that does not parse is a client error, not a signature failure,
	// so it is a 400 rather than a 401.
	t.Run("malformedJSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.TelegramAuth(rec, covJSONRequest(t, http.MethodPost, "/api/v1/auth/telegram", `{`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400 for a body that does not parse", rec.Code)
		}
	})

	if len(users.byEmail) != 0 {
		t.Errorf("%d accounts created from unsigned payloads", len(users.byEmail))
	}
}

func TestCovTelegramCallbackWebCompletesTheBrowserFlow(t *testing.T) {
	const botToken = "123:telegram-token"
	users := newMemUserStore()
	sessions := newCovSessionStore()
	h := covDiscordRig(t, users, sessions)
	h.SetOAuth(botToken, "oxidebot", "", "", "", "https://app.example")
	state := mintState(t, sessions, "telegram", false, "")

	payload := covTelegramPayload(botToken, 888)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/telegram/callback?state="+state+
			"&id=888&first_name=Tele&username=teleuser&auth_date="+
			strconv.FormatInt(payload["auth_date"].(int64), 10)+
			"&hash="+payload["hash"].(string), nil)
	h.TelegramCallbackWeb(rec, req)

	assertCovRedirectCarriesTokens(t, rec)
	if _, err := users.GetUserByTelegramID(t.Context(), 888); err != nil {
		t.Errorf("the telegram identity was not stored: %v", err)
	}
}

func TestCovTelegramCallbackWebLinksToTheNamedAccount(t *testing.T) {
	const botToken = "123:telegram-token"
	users := newMemUserStore()
	target := covNewUser(t, users, "target@example.com", "target")
	sessions := newCovSessionStore()
	h := covDiscordRig(t, users, sessions)
	h.SetOAuth(botToken, "oxidebot", "", "", "", "https://app.example")
	state := mintState(t, sessions, "telegram", false, target.id.String())

	payload := covTelegramPayload(botToken, 999)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/telegram/callback?state="+state+
			"&id=999&first_name=Tele&username=teleuser&auth_date="+
			strconv.FormatInt(payload["auth_date"].(int64), 10)+
			"&hash="+payload["hash"].(string), nil)
	h.TelegramCallbackWeb(rec, req)

	assertCovRedirectCarriesTokens(t, rec)
	linked, err := users.GetUserByTelegramID(t.Context(), 999)
	if err != nil {
		t.Fatalf("the link did not happen: %v", err)
	}
	if linked.ID != target.id {
		t.Errorf("linked to %s, want the state record's account %s", linked.ID, target.id)
	}
}

func TestCovTelegramCallbackWebRefusesAnIdentityHeldElsewhere(t *testing.T) {
	const botToken = "123:telegram-token"
	users := newMemUserStore()
	owner := covNewUser(t, users, "owner2@example.com", "owner")
	if err := users.LinkTelegram(t.Context(), owner.id, 111); err != nil {
		t.Fatalf("seed: %v", err)
	}
	attacker := covNewUser(t, users, "attacker2@example.com", "attacker")

	sessions := newCovSessionStore()
	h := covDiscordRig(t, users, sessions)
	h.SetOAuth(botToken, "oxidebot", "", "", "", "https://app.example")
	state := mintState(t, sessions, "telegram", false, attacker.id.String())

	payload := covTelegramPayload(botToken, 111)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/auth/telegram/callback?state="+state+
			"&id=111&first_name=Tele&username=teleuser&auth_date="+
			strconv.FormatInt(payload["auth_date"].(int64), 10)+
			"&hash="+payload["hash"].(string), nil)
	h.TelegramCallbackWeb(rec, req)

	assertCovRedirectCarriesError(t, rec, "вже прив'язано")
}

func TestCovTelegramCallbackWebRejectsABadStateOrSignature(t *testing.T) {
	const botToken = "123:telegram-token"
	users := newMemUserStore()
	sessions := newCovSessionStore()
	h := covDiscordRig(t, users, sessions)
	h.SetOAuth(botToken, "oxidebot", "", "", "", "https://app.example")

	t.Run("noState", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/auth/telegram/callback?id=1&auth_date=1&hash=zz", nil)
		h.TelegramCallbackWeb(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", rec.Code)
		}
	})

	t.Run("validStateBadSignature", func(t *testing.T) {
		state := mintState(t, sessions, "telegram", false, "")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/auth/telegram/callback?state="+state+
				"&id=42&auth_date="+strconv.FormatInt(time.Now().Unix(), 10)+"&hash="+strings.Repeat("a", 64), nil)
		h.TelegramCallbackWeb(rec, req)
		// The state is spent, so the attempt has to fail somewhere; the point
		// is that no account was created from an unsigned payload.
		if len(users.byEmail) != 0 {
			t.Error("an account was created from an invalid signature")
		}
	})
}

// ---- RequireAdmin ----------------------------------------------------------

func TestCovRequireAdminEnforcesTheAdminRole(t *testing.T) {
	// RequireAdmin is what protects account administration; nothing enforces
	// the role claim otherwise.
	handler := transporthttp.RequireAdmin()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("noRoleClaimIsUnauthorized", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d, want 401 with no role claim", rec.Code)
		}
	})

	t.Run("wrongRoleIsForbidden", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req = req.WithContext(context.WithValue(context.Background(), middleware.RoleKey, "user"))
		handler.ServeHTTP(rec, req)
		// 403, not 401: the caller is authenticated, just not allowed.
		if rec.Code != http.StatusForbidden {
			t.Fatalf("got %d, want 403", rec.Code)
		}
	})

	t.Run("adminPasses", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req = req.WithContext(context.WithValue(context.Background(), middleware.RoleKey, "admin"))
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200 for an admin", rec.Code)
		}
	})
}

// ---- helpers ---------------------------------------------------------------

func assertCovRedirectCarriesTokens(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatalf("no Location header on a successful callback (status %d, body %s)", rec.Code, rec.Body.String())
	}
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("Location %q does not parse: %v", loc, err)
	}
	q := parsed.Query()
	if q.Get("access_token") == "" || q.Get("refresh_token") == "" {
		t.Errorf("Location %q carries no token pair", loc)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("Cache-Control = %q; a URL carrying tokens must not be cached", got)
	}
}

func assertCovRedirectCarriesError(t *testing.T, rec *httptest.ResponseRecorder, wantSubstring string) {
	t.Helper()
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatalf("no Location header; expected a redirect carrying %q (status %d, body %s)", wantSubstring, rec.Code, rec.Body.String())
	}
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("Location does not parse: %v", err)
	}
	if got := parsed.Query().Get("error"); !strings.Contains(got, wantSubstring) {
		t.Errorf("redirect error = %q, want it to mention %q", got, wantSubstring)
	}
	if parsed.Query().Get("access_token") != "" {
		t.Error("a failed attempt leaked an access token")
	}
}

var _ = errors.New
