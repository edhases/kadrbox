package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/edhases/kadrbox-server/internal/auth"
	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/edhases/kadrbox-server/internal/email"
	"github.com/edhases/kadrbox-server/internal/repository/postgres"
	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
	"github.com/google/uuid"
)

type AuthHandler struct {
	userRepo            UserStore
	redisClient         RefreshStore
	stateStore          oauthStateStore
	loginLimiter        *authRateLimiter
	emailSvc            *email.Service
	jwtSecret           string
	googleClientID      string
	googleClientSecret  string
	googleRedirectURI   string
	telegramBotToken    string
	telegramBotUsername string
	discordClientID     string
	discordClientSecret string
	discordRedirectURI  string
	appURL              string
}

func NewAuthHandler(
	userRepo UserStore,
	redisClient RefreshStore,
	emailSvc *email.Service,
	jwtSecret string,
	googleClientID string,
) *AuthHandler {
	// The production Redis client is also the OAuth state store; the assertion
	// keeps NewAuthHandler's signature unchanged for cmd/api.
	stateStore, _ := redisClient.(oauthStateStore)
	return newAuthHandler(userRepo, redisClient, stateStore, emailSvc, jwtSecret, googleClientID)
}

// newAuthHandler takes the OAuth state store separately from the session store.
//
// Splitting them keeps OAuthStateStore an explicit, testable dependency instead
// of a hidden type assertion on the Redis client, and lets a caller supply a
// different store for one without replacing both. Passing the same client twice
// is the production wiring (see cmd/api/main.go).
func newAuthHandler(
	userRepo UserStore,
	redisClient RefreshStore,
	stateStore oauthStateStore,
	emailSvc *email.Service,
	jwtSecret string,
	googleClientID string,
) *AuthHandler {
	if stateStore == nil {
		// No state store means no single-use CSRF protection, so login attempts
		// cannot be started safely. Falling back to an in-process store keeps a
		// single-instance deployment working; the log line is there so a
		// multi-instance deployment without shared state is noticed.
		log.Println("[Auth] WARNING: no OAuth state store configured; using in-process store (states will not survive a restart or be shared across instances)")
		stateStore = newMemoryOAuthStateStore()
	}
	return &AuthHandler{
		userRepo:       userRepo,
		redisClient:    redisClient,
		stateStore:     stateStore,
		loginLimiter:   newAuthRateLimiter(),
		emailSvc:       emailSvc,
		jwtSecret:      jwtSecret,
		googleClientID: googleClientID,
	}
}

// SetGoogleOAuth конфігурує параметри OAuth2 для Google
func (h *AuthHandler) SetGoogleOAuth(clientID, clientSecret, redirectURI string) {
	if clientID != "" {
		h.googleClientID = clientID
	}
	h.googleClientSecret = clientSecret
	h.googleRedirectURI = redirectURI
}

// SetOAuth конфігурує параметри сторонньої автентифікації (Telegram, Discord)
func (h *AuthHandler) SetOAuth(
	telegramBotToken, telegramBotUsername,
	discordClientID, discordClientSecret, discordRedirectURI,
	appURL string,
) {
	h.telegramBotToken = telegramBotToken
	h.telegramBotUsername = telegramBotUsername
	h.discordClientID = discordClientID
	h.discordClientSecret = discordClientSecret
	h.discordRedirectURI = discordRedirectURI
	h.appURL = appURL
}

// ---- request/response types -------------------------------------------------

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         *domain.User `json:"user"`
}

// ---- handlers ---------------------------------------------------------------

// Register — POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := h.loginLimiter.allowRegister(r); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" || req.Username == "" {
		jsonError(w, "email, password, and username are required", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		// The KDF gate is full: the process is protecting its memory budget.
		// 503 (not 500) tells the client this is worth retrying.
		if errors.Is(err, auth.ErrKDFBusy) {
			w.Header().Set("Retry-After", "1")
			jsonError(w, "server is busy hashing, retry shortly", http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	user, err := h.userRepo.CreateUser(r.Context(), req.Email, hash, req.Username)
	if err != nil {
		h.respondRegisterConflict(w, r, req.Email, err)
		return
	}

	// Відправляємо лист підтвердження (якщо Resend налаштований)
	if h.emailSvc.IsConfigured() {
		token, tokenErr := generateToken()
		if tokenErr == nil {
			if storeErr := h.userRepo.CreateVerificationToken(r.Context(), user.ID, token); storeErr == nil {
				if sendErr := h.emailSvc.SendVerificationEmail(user.Email, user.Username, token); sendErr != nil {
					log.Printf("[Email] Failed to send verification to %s: %v", user.Email, sendErr)
				} else {
					log.Printf("[Email] Verification email sent to %s", user.Email)
				}
			}
		}
	} else {
		// RESEND_API не задано — автоматично підтверджуємо email
		log.Printf("[Email] RESEND_API not configured — auto-verifying %s", user.Email)
		// Checked: answering 200 with a token claiming IsVerified=true while the
		// row says false produces exactly the contradiction
		// RequireVerifiedEmail then rejects the new user on.
		if err := h.userRepo.MarkEmailVerified(r.Context(), user.ID); err != nil {
			jsonError(w, "failed to record email verification", http.StatusInternalServerError)
			return
		}
		user.IsVerified = true
	}

	h.respondWithTokens(w, r, user)
}

// respondRegisterConflict answers a failed CreateUser without lying about the
// cause and without confirming more than we know.
//
// The repository returns one undifferentiated error for a unique-violation and
// for every other failure (connection lost, constraint violation, timeout), and
// the handler used to map all of them to 409 "email already registered". That
// turned any database hiccup into a false "this address is taken", and made the
// 409 a reliable account-existence oracle.
//
// So: confirm by lookup. Only when the address is positively confirmed to
// exist do we answer 409. Any other failure answers a uniform 202 with a
// generic body, which is indistinguishable from the "mail sent" case.
func (h *AuthHandler) respondRegisterConflict(w http.ResponseWriter, r *http.Request, emailAddr string, cause error) {
	if existing, lookupErr := h.userRepo.GetUserByEmail(r.Context(), emailAddr); lookupErr == nil && existing != nil {
		jsonError(w, "email already registered", http.StatusConflict)
		return
	}

	// Not confirmed as a duplicate: log the real cause server-side and answer
	// something the client cannot use as a probe.
	log.Printf("[Auth] registration failed for a non-confirmable reason: %v", cause)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"message":"if the address can be registered, a confirmation link has been sent"}`))
}

// Login — POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := h.loginLimiter.allowLogin(r); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	accountKey := normalizeEmail(req.Email)

	user, err := h.userRepo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		// Burn an equivalent KDF so an unknown address costs the same wall-clock
		// time as a known one. Without this, response time is a reliable
		// account-existence oracle.
		auth.BurnKDFForDummyUser(req.Password)
		h.loginLimiter.recordLoginFailure(accountKey)
		jsonError(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	if allowed, retryAfter := h.loginLimiter.allowAccount(accountKey); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	match, err := auth.ComparePasswordAndHash(req.Password, user.PasswordHash)
	if err != nil {
		if errors.Is(err, auth.ErrKDFBusy) {
			w.Header().Set("Retry-After", "1")
			jsonError(w, "server is busy verifying, retry shortly", http.StatusServiceUnavailable)
			return
		}
		// A hash we cannot parse is a stored-credential problem, not a wrong
		// password, but the client gets the same 401 either way.
		h.loginLimiter.recordLoginFailure(accountKey)
		jsonError(w, "invalid email or password", http.StatusUnauthorized)
		return
	}
	if !match {
		h.loginLimiter.recordLoginFailure(accountKey)
		jsonError(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	h.loginLimiter.recordLoginSuccess(accountKey)

	// Якщо email не підтверджено і RESEND налаштовано — повертаємо 403
	if !user.IsVerified && h.emailSvc.IsConfigured() {
		jsonError(w, "email not verified", http.StatusForbidden)
		return
	}

	h.respondWithTokens(w, r, user)
}

// Refresh — POST /api/v1/auth/refresh
//
// Rotation is a single atomic consume. The previous GET-then-DEL left a window
// in which the same token could be redeemed twice and discarded the delete
// error, so a failed delete silently kept the old token valid.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := h.loginLimiter.allowRefresh(r); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	if req.RefreshToken == "" {
		jsonError(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	userID, reused, err := h.redisClient.ConsumeRefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		jsonError(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}
	if reused {
		// The token was already redeemed, so a second copy exists somewhere. We
		// cannot tell the legitimate client from the thief, so both are cut off.
		log.Printf("[Auth] SECURITY: refresh token reuse detected for user %s — revoking all sessions", userID)
		h.revokeAllSessions(r.Context(), userID, "refresh token reuse")
		jsonError(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusUnauthorized)
		return
	}

	// Issue the replacement before answering. If storing it fails the old token
	// is already gone, so there is no way back — but the client gets a clear 500
	// and can sign in again, rather than silently holding a dead session.
	h.respondWithTokens(w, r, user)
}

// Me — GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// VerifyEmail — POST /api/v1/auth/verify-email
// Body: {"token": "..."}
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := h.loginLimiter.allowVerifyEmail(r); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		jsonError(w, "token is required", http.StatusBadRequest)
		return
	}

	userID, err := h.userRepo.GetUserByVerificationToken(r.Context(), body.Token)
	if err != nil {
		jsonError(w, "invalid or expired verification token", http.StatusBadRequest)
		return
	}

	if err := h.userRepo.MarkEmailVerified(r.Context(), userID); err != nil {
		jsonError(w, "failed to verify email", http.StatusInternalServerError)
		return
	}

	log.Printf("[Email] Email verified for user %s", userID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"email verified successfully"}`))
}

// ResendVerification — POST /api/v1/auth/resend-verification
// Body: {"email": "..."}
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := h.loginLimiter.allowMailRequest(r); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}

	// Charged BEFORE the service check, unlike the guard above. A 503 because the
	// mail vendor is down must not hand an attacker an unlimited supply of
	// attempts to spend the moment it recovers. The budget protects an inbox, and
	// the inbox is equally worth protecting while we cannot reach it.
	recipient := normalizeEmail(body.Email)
	if allowed, _ := h.loginLimiter.allowMailTo(recipient); !allowed {
		log.Printf("[Auth] verification mail budget exhausted for %s", recipient)
		respondGenericResend(w)
		return
	}

	if !h.emailSvc.IsConfigured() {
		jsonError(w, "email service not configured", http.StatusServiceUnavailable)
		return
	}

	user, err := h.userRepo.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		// Не розкриваємо що юзера не існує
		respondGenericResend(w)
		return
	}

	if user.IsVerified {
		// Previously answered "email already verified" here while an unknown
		// address got the generic sentence. Two different bodies for two
		// different states is an enumeration oracle with a 200 status code, which
		// is the hardest kind to notice in a log.
		respondGenericResend(w)
		return
	}

	token, err := generateToken()
	if err != nil {
		jsonError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	if err := h.userRepo.CreateVerificationToken(r.Context(), user.ID, token); err != nil {
		jsonError(w, "failed to store token", http.StatusInternalServerError)
		return
	}

	if err := h.emailSvc.SendVerificationEmail(user.Email, user.Username, token); err != nil {
		log.Printf("[Email] Failed to resend verification to %s: %v", user.Email, err)
		jsonError(w, "failed to send email", http.StatusInternalServerError)
		return
	}

	log.Printf("[Email] Verification email resent to %s", user.Email)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"verification email sent"}`))
}

// respondGenericResend answers every outcome of ResendVerification identically.
//
// One sentence for "sent", "already verified", "no such account" and "mail
// budget exhausted". Any difference between them is a statement about whether an
// address is registered, which is the one thing this endpoint must not say.
func respondGenericResend(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"if the email exists, a new verification link has been sent"}`))
}

// UpdateProfile — PUT /api/v1/auth/profile
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Username  string `json:"username"`
		Name      string `json:"name"`
		Bio       string `json:"bio"`
		AvatarURL string `json:"avatar_url"`
		Avatar    string `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	username := req.Username
	if username == "" {
		username = req.Name
	}
	avatarURL := req.AvatarURL
	if avatarURL == "" {
		avatarURL = req.Avatar
	}

	user, err := h.userRepo.UpdateProfile(r.Context(), userID, username, req.Bio, avatarURL)
	if err != nil {
		jsonError(w, "failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// ChangePassword — POST /api/v1/auth/change-password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OldPassword == "" || req.NewPassword == "" {
		jsonError(w, "old_password and new_password are required", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	match, err := auth.ComparePasswordAndHash(req.OldPassword, user.PasswordHash)
	if err != nil || !match {
		jsonError(w, "incorrect current password", http.StatusBadRequest)
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		if errors.Is(err, auth.ErrKDFBusy) {
			w.Header().Set("Retry-After", "1")
			jsonError(w, "server is busy hashing, retry shortly", http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "failed to hash new password", http.StatusInternalServerError)
		return
	}

	if err := h.userRepo.UpdatePassword(r.Context(), userID, newHash); err != nil {
		jsonError(w, "failed to update password", http.StatusInternalServerError)
		return
	}

	// A password change must end every other session. Without this, an attacker
	// holding a refresh token keeps access for the full 30-day TTL even after
	// the user changes the password in response to the compromise.
	h.revokeAllSessions(r.Context(), userID, "password change")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"password changed successfully"}`))
}

// DeleteAccount — DELETE /api/v1/auth/account
func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.userRepo.DeleteUser(r.Context(), userID); err != nil {
		jsonError(w, "failed to delete user", http.StatusInternalServerError)
		return
	}

	// Deleting the row does not touch Redis. Revoking first would revoke even
	// when the delete failed; revoking after means a delete that succeeded
	// always leaves no live session behind.
	h.revokeAllSessions(r.Context(), userID, "account deleted")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"account deleted successfully"}`))
}

// ForgotPassword — POST /api/v1/auth/forgot-password
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := h.loginLimiter.allowMailRequest(r); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}

	// The per-inbox budget, not a per-IP one. This endpoint answers 200 for
	// unknown and known addresses alike -- that is deliberate, it removes the
	// enumeration oracle -- so the rate limit is the only thing standing between
	// a caller and using somebody else's inbox as a mail cannon. A per-IP limit
	// does not: the abuse is many addresses and one victim.
	recipient := normalizeEmail(req.Email)
	if allowed, _ := h.loginLimiter.allowMailTo(recipient); !allowed {
		// Answered with the same generic body as the success case. A 429 here
		// would turn the limiter itself into an oracle for "this inbox has been
		// asked three times already", which is a smaller leak but still a leak.
		log.Printf("[Auth] password reset mail budget exhausted for %s", recipient)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"if the email exists, a password reset link has been sent"}`))
		return
	}

	user, err := h.userRepo.GetUserByEmail(r.Context(), req.Email)
	if err == nil && h.emailSvc.IsConfigured() {
		if token, tokenErr := generateToken(); tokenErr == nil {
			if storeErr := h.userRepo.CreatePasswordResetToken(r.Context(), user.ID, token); storeErr == nil {
				if sendErr := h.emailSvc.SendPasswordResetEmail(user.Email, user.Username, token); sendErr != nil {
					log.Printf("[Email] Failed to send password reset email to %s: %v", user.Email, sendErr)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"if the email exists, a password reset link has been sent"}`))
}

// ResetPassword — POST /api/v1/auth/reset-password
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if allowed, retryAfter := h.loginLimiter.allowResetPassword(r); !allowed {
		rejectRateLimited(w, retryAfter)
		return
	}

	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || req.Password == "" {
		jsonError(w, "token and password are required", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrKDFBusy) {
			w.Header().Set("Retry-After", "1")
			jsonError(w, "server is busy hashing, retry shortly", http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	// Burn and hash write are one transaction in the repository. The previous
	// three-step sequence (lookup -> burn -> write) let a crash or a dropped
	// error leave a freshly written password next to a still-live reset token,
	// replayable for its full 1-hour TTL.
	resetUserID, err := h.userRepo.ConsumePasswordResetTokenAndUpdatePassword(r.Context(), req.Token, hash)
	if err != nil {
		if errors.Is(err, postgres.ErrResetTokenNotFound) || errors.Is(err, postgres.ErrUserNotFound) {
			jsonError(w, "invalid or expired reset token", http.StatusBadRequest)
			return
		}
		jsonError(w, "failed to reset password", http.StatusInternalServerError)
		return
	}

	// Same reasoning as ChangePassword: a reset is the recovery path from a
	// compromise, so every pre-existing session has to die with it.
	h.revokeAllSessions(r.Context(), resetUserID, "password reset")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"password reset successfully"}`))
}

// UploadAvatar — POST /api/v1/auth/avatar
func (h *AuthHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 5 MB max
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		jsonError(w, "file too large (max 5MB)", http.StatusBadRequest)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("avatar")
	if err != nil {
		jsonError(w, "avatar file is required in 'avatar' field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		jsonError(w, "only .jpg, .jpeg, .png, and .webp images are allowed", http.StatusBadRequest)
		return
	}

	// Валідація реального вмісту файлу (Magic Bytes / MIME-тип)
	buff := make([]byte, 512)
	n, readErr := file.Read(buff)
	if readErr != nil && readErr != io.EOF {
		jsonError(w, "failed to read file content", http.StatusBadRequest)
		return
	}
	contentType := http.DetectContentType(buff[:n])
	if !strings.HasPrefix(contentType, "image/jpeg") &&
		!strings.HasPrefix(contentType, "image/png") &&
		!strings.HasPrefix(contentType, "image/webp") {
		jsonError(w, "invalid image content type: "+contentType, http.StatusBadRequest)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		jsonError(w, "failed to process file", http.StatusInternalServerError)
		return
	}

	uploadDir := "./data/uploads/avatars"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		jsonError(w, "failed to create upload directory", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s%s", userID.String(), ext)
	targetPath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(targetPath)
	if err != nil {
		jsonError(w, "failed to save avatar file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		jsonError(w, "failed to write avatar file", http.StatusInternalServerError)
		return
	}

	avatarURL := fmt.Sprintf("/uploads/avatars/%s?v=%d", filename, time.Now().Unix())

	user, err := h.userRepo.UpdateProfile(r.Context(), userID, "", "", avatarURL)
	if err != nil {
		jsonError(w, "failed to update user avatar in database", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// GoogleAuth — POST /api/v1/auth/google
func (h *AuthHandler) GoogleAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDToken == "" {
		jsonError(w, "id_token is required", http.StatusBadRequest)
		return
	}

	// Валідація ID Token через Google TokenInfo API з таймаутом
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tokenURL := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", url.QueryEscape(req.IDToken))
	reqHttp, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		jsonError(w, "failed to build google request", http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(reqHttp)
	if err != nil {
		jsonError(w, "failed to contact google oauth", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		jsonError(w, "invalid google id_token", http.StatusUnauthorized)
		return
	}

	var info struct {
		Aud           string `json:"aud"`
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		Sub           string `json:"sub"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Email == "" {
		jsonError(w, "invalid google token payload", http.StatusUnauthorized)
		return
	}

	// Перевірка статусу верифікації пошти в Google
	if info.EmailVerified != "true" && info.EmailVerified != "1" {
		jsonError(w, "google email is not verified", http.StatusUnauthorized)
		return
	}

	// Перевірка Audience (Client ID) якщо налаштовано на сервері
	if h.googleClientID != "" && info.Aud != h.googleClientID {
		jsonError(w, "google token audience mismatch", http.StatusUnauthorized)
		return
	}

	// Шукаємо користувача за email
	user, err := h.userRepo.GetUserByEmail(r.Context(), info.Email)
	if err != nil {
		if errors.Is(err, postgres.ErrUserNotFound) {
			// Створюємо нового користувача
			randomPass, _ := generateToken()
			hash, hashErr := auth.HashPassword(randomPass)
			if hashErr != nil {
				jsonError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}

			username := info.Name
			if username == "" {
				username = strings.Split(info.Email, "@")[0]
			}

			newUser, createErr := h.userRepo.CreateUser(r.Context(), info.Email, hash, username)
			if createErr != nil {
				jsonError(w, "failed to create user", http.StatusInternalServerError)
				return
			}

			// Google email is already verified. The write is checked: claiming
			// verification in the response while the DB row says otherwise puts
			// the API and the database in contradictory states, and the
			// RequireVerifiedEmail gate then 403s a user the API just signed in.
			if err := h.userRepo.MarkEmailVerified(r.Context(), newUser.ID); err != nil {
				jsonError(w, "failed to record email verification", http.StatusInternalServerError)
				return
			}
			newUser.IsVerified = true

			if info.Picture != "" {
				updatedUser, updateErr := h.userRepo.UpdateProfile(r.Context(), newUser.ID, "", "", info.Picture)
				if updateErr == nil {
					newUser = updatedUser
				}
			}
			user = newUser
		} else {
			jsonError(w, "database error", http.StatusInternalServerError)
			return
		}
	} else {
		// The account exists: Google has just asserted the address is verified.
		// Checked, for the same reason as the create path above.
		if !user.IsVerified {
			if err := h.userRepo.MarkEmailVerified(r.Context(), user.ID); err != nil {
				jsonError(w, "failed to record email verification", http.StatusInternalServerError)
				return
			}
			user.IsVerified = true
		}
		if user.AvatarURL == "" && info.Picture != "" {
			if updatedUser, updateErr := h.userRepo.UpdateProfile(r.Context(), user.ID, "", "", info.Picture); updateErr == nil && updatedUser != nil {
				user = updatedUser
			}
		}
	}

	h.respondWithTokens(w, r, user)
}

// GoogleLogin — GET /api/v1/auth/google/login та GET /auth/google
func (h *AuthHandler) GoogleLogin(w http.ResponseWriter, r *http.Request) {
	if h.googleClientID == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(renderOAuthStatusHTML(false, "Google недоступний", "GOOGLE_CLIENT_ID не налаштований на сервері", "", "", "")))
		return
	}

	redirectURI := h.googleRedirectURI
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/v1/auth/google/callback", h.appURL)
	}

	// The state is minted here, never taken from the request. The previous code
	// echoed the caller's `state`/`redirect_to` straight into the provider URL,
	// so the "state" the provider echoed back proved nothing.
	state, rec, err := h.beginOAuthAttempt(r.Context(), w, r, "google", true)
	if err != nil {
		log.Printf("[Auth] failed to start Google OAuth attempt: %v", err)
		writeHTMLStatus(w, http.StatusServiceUnavailable,
			renderOAuthStatusHTML(false, "Google недоступний", "Не вдалося почати авторизацію. Спробуйте пізніше.", "", "", ""))
		return
	}

	params := url.Values{}
	params.Set("client_id", h.googleClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "openid email profile")
	params.Set("access_type", "offline")
	params.Set("prompt", "select_account")
	params.Set("state", state)
	// PKCE S256: an intercepted authorization code is useless without the
	// verifier, which never leaves the server.
	params.Set("code_challenge", pkceChallengeS256(rec.CodeVerifier))
	params.Set("code_challenge_method", "S256")

	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type googleUserInfoResponse struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

// processGoogleCode exchanges an authorization code for tokens and resolves the
// local user. codeVerifier is the PKCE verifier from the state record.
func (h *AuthHandler) processGoogleCode(ctx context.Context, code, redirectURI, codeVerifier string) (*domain.User, error) {
	if h.googleClientID == "" {
		return nil, errors.New("GOOGLE_CLIENT_ID not configured")
	}
	if codeVerifier == "" {
		// Every authorization code this server accepts came from a flow it
		// started with a challenge, so a missing verifier means the callback
		// did not come from such a flow.
		return nil, ErrInvalidOAuthState
	}

	form := url.Values{}
	form.Set("client_id", h.googleClientID)
	if h.googleClientSecret != "" {
		form.Set("client_secret", h.googleClientSecret)
	}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	reqHTTP.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(reqHTTP)
	if err != nil {
		return nil, fmt.Errorf("exchange token request failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || tokenResp.AccessToken == "" {
		msg := tokenResp.ErrorDesc
		if msg == "" {
			msg = tokenResp.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("google token error (status %d)", resp.StatusCode)
		}
		return nil, errors.New(msg)
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("build user info request: %w", err)
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	userResp, err := client.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("fetch google user: %w", err)
	}
	defer userResp.Body.Close()

	var gUser googleUserInfoResponse
	if err := json.NewDecoder(userResp.Body).Decode(&gUser); err != nil {
		return nil, fmt.Errorf("decode user response: %w", err)
	}
	if userResp.StatusCode != http.StatusOK || gUser.Email == "" {
		return nil, fmt.Errorf("failed to fetch google user info (status %d)", userResp.StatusCode)
	}

	user, err := h.userRepo.GetUserByEmail(ctx, gUser.Email)
	if err == nil {
		if !user.IsVerified {
			// Checked: reporting IsVerified=true while the row disagrees leaves
			// the API and the database in contradictory states.
			if err := h.userRepo.MarkEmailVerified(ctx, user.ID); err != nil {
				return nil, fmt.Errorf("mark email verified: %w", err)
			}
			user.IsVerified = true
		}
		if user.AvatarURL == "" && gUser.Picture != "" {
			if updated, err := h.userRepo.UpdateProfile(ctx, user.ID, "", "", gUser.Picture); err == nil {
				user = updated
			}
		}
		return user, nil
	}

	username := gUser.Name
	if username == "" {
		username = strings.Split(gUser.Email, "@")[0]
	}
	if len([]rune(username)) > 100 {
		username = string([]rune(username)[:100])
	}

	randomPass, _ := generateToken()
	hash, err := auth.HashPassword(randomPass)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newUser, err := h.userRepo.CreateOAuthUser(ctx, gUser.Email, hash, username, gUser.Picture, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create google user: %w", err)
	}
	return newUser, nil
}

// GoogleCallback — GET /api/v1/auth/google/callback
//
// The state is consumed before anything else: an absent, unknown, expired,
// replayed or session-mismatched state is a flat 400 with no detail about which
// check failed.
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	rec, err := h.verifyCallbackState(r.Context(), r, "google")
	if err != nil {
		h.rejectOAuthCallback(w, "Помилка Google", "Недійсний або прострочений запит авторизації. Почніть вхід знову.")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		errDesc := sanitizeProviderMessage(r.URL.Query().Get("error_description"))
		if errDesc == "" {
			errDesc = sanitizeProviderMessage(r.URL.Query().Get("error"))
		}
		if errDesc == "" {
			errDesc = "Авторизацію через Google було скасовано."
		}
		h.failOAuthAttempt(w, r, rec, errDesc)
		return
	}

	redirectURI := h.googleRedirectURI
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("%s/api/v1/auth/google/callback", h.appURL)
	}

	user, err := h.processGoogleCode(r.Context(), code, redirectURI, rec.CodeVerifier)
	if err != nil {
		h.failOAuthAttempt(w, r, rec, err.Error())
		return
	}

	// Google identifies the user by email: linking is only possible to an
	// account with that same address, otherwise it would be someone else's.
	if rec.LinkUserID != "" && user.ID.String() != rec.LinkUserID {
		h.failOAuthAttempt(w, r, rec, "Цей Google-акаунт належить іншому користувачу")
		return
	}

	accessToken, refreshToken, err := h.issueOAuthTokens(r.Context(), user)
	if err != nil {
		writeHTMLStatus(w, http.StatusInternalServerError,
			renderOAuthStatusHTML(false, "Помилка сервера", "Не вдалося зберегти сесію. Спробуйте пізніше.", "", "", ""))
		return
	}

	h.completeOAuthAttempt(w, r, rec, "google", "Вхід через Google успішний!", accessToken, refreshToken)
}

// ---- Telegram Auth ----------------------------------------------------------

type TelegramAuthRequest struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date"`
	Hash      string `json:"hash"`
}

func (h *AuthHandler) verifyTelegramAuth(req TelegramAuthRequest) bool {
	if h.telegramBotToken == "" || req.Hash == "" || req.ID == 0 || req.AuthDate == 0 {
		return false
	}
	now := time.Now().Unix()
	if now-req.AuthDate > 86400 || req.AuthDate > now+300 {
		return false
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("auth_date=%d", req.AuthDate))
	if req.FirstName != "" {
		parts = append(parts, fmt.Sprintf("first_name=%s", req.FirstName))
	}
	parts = append(parts, fmt.Sprintf("id=%d", req.ID))
	if req.LastName != "" {
		parts = append(parts, fmt.Sprintf("last_name=%s", req.LastName))
	}
	if req.PhotoURL != "" {
		parts = append(parts, fmt.Sprintf("photo_url=%s", req.PhotoURL))
	}
	if req.Username != "" {
		parts = append(parts, fmt.Sprintf("username=%s", req.Username))
	}
	sort.Strings(parts)
	dataCheckString := strings.Join(parts, "\n")

	sha := sha256.Sum256([]byte(h.telegramBotToken))
	mac := hmac.New(sha256.New, sha[:])
	mac.Write([]byte(dataCheckString))
	expectedHash := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(strings.ToLower(expectedHash)), []byte(strings.ToLower(req.Hash)))
}

func (h *AuthHandler) getOrCreateTelegramUser(ctx context.Context, req TelegramAuthRequest) (*domain.User, error) {
	user, err := h.userRepo.GetUserByTelegramID(ctx, req.ID)
	if err == nil {
		if req.PhotoURL != "" && user.AvatarURL == "" {
			if updated, err := h.userRepo.UpdateProfile(ctx, user.ID, "", "", req.PhotoURL); err == nil {
				user = updated
			}
		}
		return user, nil
	}

	displayName := req.Username
	if displayName == "" {
		displayName = strings.TrimSpace(req.FirstName + " " + req.LastName)
	}
	if displayName == "" {
		displayName = fmt.Sprintf("tg_user_%d", req.ID)
	}
	if len([]rune(displayName)) > 100 {
		displayName = string([]rune(displayName)[:100])
	}

	placeholderEmail := fmt.Sprintf("tg_%d@telegram.oxide", req.ID)
	randomPass, _ := generateToken()
	hash, err := auth.HashPassword(randomPass)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newUser, err := h.userRepo.CreateOAuthUser(ctx, placeholderEmail, hash, displayName, req.PhotoURL, &req.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return newUser, nil
}

// TelegramAuth — POST /api/v1/auth/telegram
func (h *AuthHandler) TelegramAuth(w http.ResponseWriter, r *http.Request) {
	var req TelegramAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	if !h.verifyTelegramAuth(req) {
		jsonError(w, "invalid telegram authentication signature", http.StatusUnauthorized)
		return
	}

	user, err := h.getOrCreateTelegramUser(r.Context(), req)
	if err != nil {
		jsonError(w, "failed to process user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.respondWithTokens(w, r, user)
}

// TelegramLoginWeb — GET /api/v1/auth/telegram/login та GET /auth/telegram
func (h *AuthHandler) TelegramLoginWeb(w http.ResponseWriter, r *http.Request) {
	if h.telegramBotToken == "" {
		writeHTMLStatus(w, http.StatusServiceUnavailable,
			renderOAuthStatusHTML(false, "Telegram недоступний", "TELEGRAM_BOT_TOKEN не налаштований на сервері", "", "", ""))
		return
	}

	botUser := h.telegramBotUsername
	if botUser == "" {
		botUser = "oxidefilmbot"
	}

	// The state the widget will carry back is a server nonce, not the caller's
	// redirect target. Telegram has no authorization-code exchange, so there is
	// no PKCE here; the signed id/hash Telegram returns is the credential, and
	// single-use server-side state still provides CSRF protection.
	state, _, err := h.beginOAuthAttempt(r.Context(), w, r, "telegram", false)
	if err != nil {
		log.Printf("[Auth] failed to start Telegram OAuth attempt: %v", err)
		writeHTMLStatus(w, http.StatusServiceUnavailable,
			renderOAuthStatusHTML(false, "Telegram недоступний", "Не вдалося почати авторизацію. Спробуйте пізніше.", "", "", ""))
		return
	}

	authURL := "/api/v1/auth/telegram/callback?state=" + url.QueryEscape(state)

	setNoTokenCacheHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(renderTelegramWidgetHTML(botUser, authURL)))
}

// TelegramCallbackWeb — GET /api/v1/auth/telegram/callback
func (h *AuthHandler) TelegramCallbackWeb(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, _ := strconv.ParseInt(q.Get("id"), 10, 64)
	authDate, _ := strconv.ParseInt(q.Get("auth_date"), 10, 64)

	req := TelegramAuthRequest{
		ID:        id,
		FirstName: q.Get("first_name"),
		LastName:  q.Get("last_name"),
		Username:  q.Get("username"),
		PhotoURL:  q.Get("photo_url"),
		AuthDate:  authDate,
		Hash:      q.Get("hash"),
	}

	rec, err := h.verifyCallbackState(r.Context(), r, "telegram")
	if err != nil {
		h.rejectOAuthCallback(w, "Помилка Telegram", "Недійсний або прострочений запит авторизації. Почніть вхід знову.")
		return
	}

	if !h.verifyTelegramAuth(req) {
		h.failOAuthAttempt(w, r, rec, "Недійсний підпис авторизації Telegram")
		return
	}

	user, err := func() (*domain.User, error) {
		if rec.LinkUserID != "" {
			return h.linkTelegramToUser(r.Context(), uuid.MustParse(rec.LinkUserID), req.ID)
		}
		return h.getOrCreateTelegramUser(r.Context(), req)
	}()
	if err != nil {
		h.failOAuthAttempt(w, r, rec, "Не вдалося зберегти користувача: "+err.Error())
		return
	}

	accessToken, refreshToken, err := h.issueOAuthTokens(r.Context(), user)
	if err != nil {
		writeHTMLStatus(w, http.StatusInternalServerError,
			renderOAuthStatusHTML(false, "Помилка сервера", "Не вдалося зберегти сесію. Спробуйте пізніше.", "", "", ""))
		return
	}

	h.completeOAuthAttempt(w, r, rec, "telegram", "Вхід успішний!", accessToken, refreshToken)
}

// ---- Discord OAuth2 ---------------------------------------------------------

type discordTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type discordUserResponse struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name"`
	Avatar        string `json:"avatar"`
	Email         string `json:"email"`
	Verified      bool   `json:"verified"`
}

// processDiscordCode exchanges a code and resolves the local user. codeVerifier
// is the PKCE verifier from the state record.
func (h *AuthHandler) processDiscordCode(ctx context.Context, code, redirectURI, codeVerifier string) (*domain.User, error) {
	discordUser, err := h.fetchDiscordProfile(ctx, code, redirectURI, codeVerifier)
	if err != nil {
		return nil, err
	}

	var avatarURL string
	if discordUser.Avatar != "" {
		avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", discordUser.ID, discordUser.Avatar)
	}

	user, err := h.userRepo.GetUserByDiscordID(ctx, discordUser.ID)
	if err == nil {
		if avatarURL != "" && user.AvatarURL == "" {
			if updated, err := h.userRepo.UpdateProfile(ctx, user.ID, "", "", avatarURL); err == nil {
				user = updated
			}
		}
		return user, nil
	}

	if discordUser.Email != "" {
		if existing, err := h.userRepo.GetUserByEmail(ctx, discordUser.Email); err == nil {
			// Checked: a discarded LinkDiscord error means the caller is told the
			// account is linked when no discord_id was ever written, and the link
			// silently fails for good.
			if err := h.userRepo.LinkDiscord(ctx, existing.ID, discordUser.ID); err != nil {
				return nil, fmt.Errorf("link discord account: %w", err)
			}
			if !existing.IsVerified && discordUser.Verified {
				if err := h.userRepo.MarkEmailVerified(ctx, existing.ID); err != nil {
					return nil, fmt.Errorf("mark email verified: %w", err)
				}
				existing.IsVerified = true
			}
			if existing.AvatarURL == "" && avatarURL != "" {
				if updated, err := h.userRepo.UpdateProfile(ctx, existing.ID, "", "", avatarURL); err == nil {
					existing = updated
				}
			}
			return existing, nil
		}
	}

	emailAddr := discordUser.Email
	if emailAddr == "" {
		emailAddr = fmt.Sprintf("discord_%s@discord.oxide", discordUser.ID)
	}

	displayName := discordUser.GlobalName
	if displayName == "" {
		displayName = discordUser.Username
	}
	if displayName == "" {
		displayName = "discord_user"
	}
	if len([]rune(displayName)) > 100 {
		displayName = string([]rune(displayName)[:100])
	}

	randomPass, _ := generateToken()
	hash, err := auth.HashPassword(randomPass)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newUser, err := h.userRepo.CreateOAuthUser(ctx, emailAddr, hash, displayName, avatarURL, nil, &discordUser.ID)
	if err != nil {
		return nil, fmt.Errorf("create discord user: %w", err)
	}
	return newUser, nil
}

// fetchDiscordProfile обмінює OAuth-код на профіль Discord
// (без створення користувача — для флоу прив'язки до існуючого акаунту).
func (h *AuthHandler) fetchDiscordProfile(ctx context.Context, code, redirectURI, codeVerifier string) (*discordUserResponse, error) {
	if h.discordClientID == "" || h.discordClientSecret == "" {
		return nil, errors.New("discord credentials not configured")
	}
	if codeVerifier == "" {
		// A code this server issued always came from a flow it started with a
		// PKCE challenge; a missing verifier means it did not.
		return nil, ErrInvalidOAuthState
	}

	form := url.Values{}
	form.Set("client_id", h.discordClientID)
	form.Set("client_secret", h.discordClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)

	reqHTTP, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://discord.com/api/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	reqHTTP.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(reqHTTP)
	if err != nil {
		return nil, fmt.Errorf("exchange token request failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp discordTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || tokenResp.AccessToken == "" {
		msg := tokenResp.ErrorDesc
		if msg == "" {
			msg = tokenResp.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("discord token error (status %d)", resp.StatusCode)
		}
		return nil, errors.New(msg)
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/users/@me", nil)
	if err != nil {
		return nil, fmt.Errorf("build user request: %w", err)
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	userResp, err := client.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("fetch discord user: %w", err)
	}
	defer userResp.Body.Close()

	var discordUser discordUserResponse
	if err := json.NewDecoder(userResp.Body).Decode(&discordUser); err != nil {
		return nil, fmt.Errorf("decode user response: %w", err)
	}
	if userResp.StatusCode != http.StatusOK || discordUser.ID == "" {
		return nil, fmt.Errorf("failed to fetch discord user info (status %d)", userResp.StatusCode)
	}
	return &discordUser, nil
}

// DiscordLogin — GET /api/v1/auth/discord/login та GET /auth/discord
func (h *AuthHandler) DiscordLogin(w http.ResponseWriter, r *http.Request) {
	if h.discordClientID == "" {
		writeHTMLStatus(w, http.StatusServiceUnavailable,
			renderOAuthStatusHTML(false, "Discord недоступний", "DISCORD_CLIENT_ID не налаштований на сервері", "", "", ""))
		return
	}

	// Server-minted state plus PKCE; see GoogleLogin for the reasoning.
	state, rec, err := h.beginOAuthAttempt(r.Context(), w, r, "discord", true)
	if err != nil {
		log.Printf("[Auth] failed to start Discord OAuth attempt: %v", err)
		writeHTMLStatus(w, http.StatusServiceUnavailable,
			renderOAuthStatusHTML(false, "Discord недоступний", "Не вдалося почати авторизацію. Спробуйте пізніше.", "", "", ""))
		return
	}

	params := url.Values{}
	params.Set("client_id", h.discordClientID)
	params.Set("redirect_uri", h.discordRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "identify email")
	params.Set("prompt", "consent")
	params.Set("state", state)
	params.Set("code_challenge", pkceChallengeS256(rec.CodeVerifier))
	params.Set("code_challenge_method", "S256")

	authURL := "https://discord.com/api/oauth2/authorize?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// DiscordCallback — GET /api/v1/auth/discord/callback
func (h *AuthHandler) DiscordCallback(w http.ResponseWriter, r *http.Request) {
	rec, err := h.verifyCallbackState(r.Context(), r, "discord")
	if err != nil {
		h.rejectOAuthCallback(w, "Помилка Discord", "Недійсний або прострочений запит авторизації. Почніть вхід знову.")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		errDesc := sanitizeProviderMessage(r.URL.Query().Get("error_description"))
		if errDesc == "" {
			errDesc = "Авторизацію через Discord було скасовано."
		}
		h.failOAuthAttempt(w, r, rec, errDesc)
		return
	}

	user, err := func() (*domain.User, error) {
		if rec.LinkUserID != "" {
			profile, fetchErr := h.fetchDiscordProfile(r.Context(), code, h.discordRedirectURI, rec.CodeVerifier)
			if fetchErr != nil {
				return nil, fetchErr
			}
			return h.linkDiscordToUser(r.Context(), uuid.MustParse(rec.LinkUserID), profile.ID)
		}
		return h.processDiscordCode(r.Context(), code, h.discordRedirectURI, rec.CodeVerifier)
	}()
	if err != nil {
		h.failOAuthAttempt(w, r, rec, err.Error())
		return
	}

	accessToken, refreshToken, err := h.issueOAuthTokens(r.Context(), user)
	if err != nil {
		writeHTMLStatus(w, http.StatusInternalServerError,
			renderOAuthStatusHTML(false, "Помилка сервера", "Не вдалося зберегти сесію. Спробуйте пізніше.", "", "", ""))
		return
	}

	h.completeOAuthAttempt(w, r, rec, "discord", "Вхід через Discord успішний!", accessToken, refreshToken)
}

type DiscordAuthRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri,omitempty"`
	// CodeVerifier is the PKCE verifier the client generated. Optional for
	// backwards compatibility, but without it this endpoint can no longer
	// exchange a code, because the exchange now requires a verifier.
	CodeVerifier string `json:"code_verifier,omitempty"`
}

// DiscordAuthAPI — POST /api/v1/auth/discord (прямий обмін коду на JWT з клієнта)
//
// The browser redirect flow uses the state-bound verifier; a native client that
// drives the authorize URL itself must send its own verifier, since the server
// never saw the challenge.
func (h *AuthHandler) DiscordAuthAPI(w http.ResponseWriter, r *http.Request) {
	var req DiscordAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		jsonError(w, "code is required", http.StatusBadRequest)
		return
	}
	redirectURI := req.RedirectURI
	if redirectURI == "" {
		redirectURI = h.discordRedirectURI
	}

	user, err := h.processDiscordCode(r.Context(), req.Code, redirectURI, req.CodeVerifier)
	if err != nil {
		if errors.Is(err, ErrInvalidOAuthState) {
			jsonError(w, "code_verifier is required", http.StatusBadRequest)
			return
		}
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.respondWithTokens(w, r, user)
}

// validateLinkToken resolves a link_token JWT to its claims.
func (h *AuthHandler) validateLinkToken(token string) (*auth.Claims, error) {
	return auth.ValidateAccessToken(token, h.jwtSecret)
}

// ---- OAuth link/unlink --------------------------------------------------------

// The link target now lives in the server-side state record (rec.LinkUserID)
// rather than being parsed out of a `state` query parameter at callback time.
// Parsing it there meant trusting a caller-supplied URL, and the token in it was
// validated only for signature, not against the flow that is completing.

// linkTelegramToUser прив'язує Telegram ID до вказаного користувача.
// Повертає помилку, якщо ID вже належить іншому користувачу.
func (h *AuthHandler) linkTelegramToUser(ctx context.Context, linkUserID uuid.UUID, telegramID int64) (*domain.User, error) {
	if owner, err := h.userRepo.GetUserByTelegramID(ctx, telegramID); err == nil {
		if owner.ID == linkUserID {
			return owner, nil
		}
		return nil, errors.New("цей Telegram-акаунт вже прив'язано до іншого користувача")
	}
	linkUser, err := h.userRepo.GetUserByID(ctx, linkUserID)
	if err != nil {
		return nil, errors.New("сесія для прив'язки недійсна, увійдіть заново")
	}
	if err := h.userRepo.LinkTelegram(ctx, linkUserID, telegramID); err != nil {
		return nil, fmt.Errorf("не вдалося прив'язати Telegram: %w", err)
	}
	linkUser.TelegramID = &telegramID
	return linkUser, nil
}

// linkDiscordToUser прив'язує Discord ID до вказаного користувача.
func (h *AuthHandler) linkDiscordToUser(ctx context.Context, linkUserID uuid.UUID, discordID string) (*domain.User, error) {
	if owner, err := h.userRepo.GetUserByDiscordID(ctx, discordID); err == nil {
		if owner.ID == linkUserID {
			return owner, nil
		}
		return nil, errors.New("цей Discord-акаунт вже прив'язано до іншого користувача")
	}
	linkUser, err := h.userRepo.GetUserByID(ctx, linkUserID)
	if err != nil {
		return nil, errors.New("сесія для прив'язки недійсна, увійдіть заново")
	}
	if err := h.userRepo.LinkDiscord(ctx, linkUserID, discordID); err != nil {
		return nil, fmt.Errorf("не вдалося прив'язати Discord: %w", err)
	}
	linkUser.DiscordID = &discordID
	return linkUser, nil
}

// UnlinkProvider — POST /api/v1/auth/unlink (авторизований)
// Body: {"provider": "telegram" | "discord"}
func (h *AuthHandler) UnlinkProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Provider != "telegram" && body.Provider != "discord") {
		jsonError(w, "provider must be telegram or discord", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	// Не даємо відв'язати останній спосіб входу: OAuth-акаунти з
	// плейсхолдер-поштою не мають пароля, тож втратять доступ назавжди.
	if isPlaceholderEmail(user.Email) {
		remaining := 0
		if body.Provider != "telegram" && user.TelegramID != nil {
			remaining++
		}
		if body.Provider != "discord" && user.DiscordID != nil {
			remaining++
		}
		if remaining == 0 {
			jsonError(w, "cannot unlink the last login method", http.StatusBadRequest)
			return
		}
	}

	switch body.Provider {
	case "telegram":
		if err := h.userRepo.UnlinkTelegram(r.Context(), userID); err != nil {
			jsonError(w, "failed to unlink telegram", http.StatusInternalServerError)
			return
		}
	case "discord":
		if err := h.userRepo.UnlinkDiscord(r.Context(), userID); err != nil {
			jsonError(w, "failed to unlink discord", http.StatusInternalServerError)
			return
		}
	}

	updated, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "failed to reload profile", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

// isPlaceholderEmail перевіряє службові пошти OAuth-користувачів без пароля.
func isPlaceholderEmail(email string) bool {
	return strings.HasSuffix(email, "@telegram.oxide") || strings.HasSuffix(email, "@discord.oxide")
}

// RequireVerifiedEmail — middleware для ендпоінтів цінності акаунту (синхронізація).
// Без підтвердженої пошти акаунт вважається неактивним: повертає 403.
// Якщо поштовий сервіс не налаштовано, всі акаунти авто-верифіковані — пропускає.
//
// Fail-closed on everything else. The previous version called next.ServeHTTP
// when GetUserByID returned *any* error, including a database outage, so a
// caller who could make the lookup fail reached the sync endpoints with an
// unverified account. Only the documented "email service not configured" case
// may pass.
func (h *AuthHandler) RequireVerifiedEmail() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !h.emailSvc.IsConfigured() {
				next.ServeHTTP(w, r)
				return
			}
			userID, ok := middleware.GetUserIDFromContext(r.Context())
			if !ok {
				// Reaching a value-gated endpoint with no user must not pass.
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			user, err := h.userRepo.GetUserByID(r.Context(), userID)
			if err != nil {
				// We cannot tell whether this account is verified, so we must
				// not let it through. 503 says "retry", not "forbidden".
				log.Printf("[Auth] verification lookup failed for %s: %v", userID, err)
				jsonError(w, "unable to verify account state", http.StatusServiceUnavailable)
				return
			}
			if !user.IsVerified {
				jsonError(w, "email not verified", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---- helpers ----------------------------------------------------------------

// respondWithTokens lives in auth_session.go, next to the other session
// operations.

// normalizeEmail lowercases and trims an address for use as a limiter key.
func normalizeEmail(emailAddr string) string {
	return strings.ToLower(strings.TrimSpace(emailAddr))
}

func writeAuthEnvelope(w http.ResponseWriter, resp AuthResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// VerifyEmailWeb - GET /verify-email?token=...
func (h *AuthHandler) VerifyEmailWeb(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	if token == "" {
		writeHTMLStatus(w, http.StatusBadRequest,
			renderEmailStatusHTML(false, "Токен відсутній", "Посилання не містить токена підтвердження."))
		return
	}

	userID, err := h.userRepo.GetUserByVerificationToken(r.Context(), token)
	if err != nil {
		writeHTMLStatus(w, http.StatusBadRequest,
			renderEmailStatusHTML(false, "Посилання недійсне або застаріло", "Термін дії посилання закінчився (24 години) або воно вже було використане."))
		return
	}

	if err := h.userRepo.MarkEmailVerified(r.Context(), userID); err != nil {
		writeHTMLStatus(w, http.StatusInternalServerError,
			renderEmailStatusHTML(false, "Помилка сервера", "Не вдалося зберегти підтвердження email. Спробуйте пізніше."))
		return
	}

	log.Printf("[Email] Web verification successful for user %s", userID)
	writeHTMLStatus(w, http.StatusOK,
		renderEmailStatusHTML(true, "Пошту успішно підтверджено!", "Ваш акаунт активовано. Тепер ви можете увійти у застосунок Oxide Film."))
}

// ResetPasswordWeb - GET /reset-password?token=...
//
// The form is rendered from html/template (html_render.go): the reset token is
// request data and the previous fmt.Sprintf with %q interpolated it straight
// into a <script> body, where a token containing </script> breaks out.
func (h *AuthHandler) ResetPasswordWeb(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	if token == "" {
		writeHTMLStatus(w, http.StatusBadRequest,
			renderEmailStatusHTML(false, "Токен відсутній", "Посилання не містить токена скидання пароля."))
		return
	}

	if _, err := h.userRepo.GetUserByPasswordResetToken(r.Context(), token); err != nil {
		writeHTMLStatus(w, http.StatusBadRequest,
			renderEmailStatusHTML(false, "Посилання недійсне або застаріло", "Термін дії посилання для скидання пароля минув (1 година) або воно вже використане."))
		return
	}

	writeHTMLStatus(w, http.StatusOK, renderResetPasswordForm(token))
}
