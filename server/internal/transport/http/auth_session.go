package http

// Session-management methods for the Redis-backed RefreshStore, split out of
// auth_handler.go so the god object does not grow further.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/edhases/oxide-server/internal/auth"
	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
	"github.com/google/uuid"
)

// refreshTokenTTL is how long a refresh token stays valid. Sessions are meant
// to be long-lived, which is exactly why every credential-changing operation has
// to revoke them explicitly.
const refreshTokenTTL = 30 * 24 * time.Hour

// Logout — POST /api/v1/auth/logout
//
// Revokes the presented refresh token. Registering the route lives in
// router.go, which this change does not own.
//
// The access token is deliberately not invalidated: it is a stateless JWT that
// expires within 15 minutes, and killing it immediately would require
// server-side session state this codebase does not keep. The refresh token is
// the long-lived credential, so that is what logout revokes.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	// A malformed or empty body must not stop the revocation: logout is
	// idempotent from the client's point of view.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	token := strings.TrimSpace(req.RefreshToken)
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-Refresh-Token"))
	}
	if token == "" {
		jsonError(w, "refresh_token is required", http.StatusBadRequest)
		return
	}

	if err := h.redisClient.RevokeRefreshToken(r.Context(), token); err != nil {
		// Telling the client we could not revoke is the only honest answer.
		jsonError(w, "failed to revoke session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"logged out"}`))
}

// revokeAllSessions invalidates every refresh token of a user.
//
// Called after any credential change (password change, password reset, account
// deletion) and on refresh-token reuse. Errors are logged loudly and then
// suppressed: the caller's own operation has already succeeded, and reporting a
// failure would tell the client the password change did not happen when it did.
func (h *AuthHandler) revokeAllSessions(ctx context.Context, userID uuid.UUID, reason string) int {
	revoked, err := h.redisClient.RevokeAllForUser(ctx, userID)
	if err != nil {
		log.Printf("[Auth] CRITICAL: failed to revoke sessions for user %s (reason=%s): %v", userID, reason, err)
		return 0
	}
	if revoked > 0 {
		log.Printf("[Auth] revoked %d session(s) for user %s (reason=%s)", revoked, userID, reason)
	}
	return revoked
}

// respondWithTokens mints an access token, persists a refresh token and writes
// the envelope. Every successful sign-in converges here, so this is the one
// place where a refresh token that failed to persist must abort the response —
// otherwise the client stores a session the server cannot honour.
func (h *AuthHandler) respondWithTokens(w http.ResponseWriter, r *http.Request, user *domain.User) {
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, user.Role, h.jwtSecret, 15*time.Minute)
	if err != nil {
		jsonError(w, "failed to generate access token", http.StatusInternalServerError)
		return
	}

	refreshToken := auth.GenerateRefreshToken()
	if err := h.redisClient.StoreRefreshToken(r.Context(), refreshToken, user.ID, refreshTokenTTL); err != nil {
		jsonError(w, "failed to store session", http.StatusInternalServerError)
		return
	}

	writeAuthEnvelope(w, AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	})
}

// issueOAuthTokens returns a freshly minted token pair for a browser-side OAuth
// completion, or an error if the session could not be persisted.
func (h *AuthHandler) issueOAuthTokens(ctx context.Context, user *domain.User) (accessToken, refreshToken string, err error) {
	accessToken, err = auth.GenerateAccessToken(user.ID, user.Email, user.Role, h.jwtSecret, 15*time.Minute)
	if err != nil {
		return "", "", err
	}
	refreshToken = auth.GenerateRefreshToken()
	if err := h.redisClient.StoreRefreshToken(ctx, refreshToken, user.ID, refreshTokenTTL); err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

// RequireRole builds a middleware admitting only the named role.
//
// The role claim is put into the request context by middleware.AuthMiddleware
// but nothing enforced it, so a token minted with role "admin" and one minted
// with role "user" were equally powerful. This is the missing enforcement point;
// no admin route is registered by this change.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, ok := roleFromContext(r.Context())
			if !ok {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !roleMatches(got, role) {
				// 403, not 401: the caller is authenticated, just not allowed.
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin is the admin shorthand of RequireRole.
func RequireAdmin() func(http.Handler) http.Handler {
	return RequireRole("admin")
}

// roleFromContext reads the role middleware.AuthMiddleware stored.
//
// The key is unexported in that package, so it is read through the exported
// value the middleware put there; a missing value means "no role claim", which
// is treated as unauthenticated rather than as a default role.
func roleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(middleware.RoleKey).(string)
	if !ok || role == "" {
		return "", false
	}
	return role, true
}

// roleMatches compares roles case-insensitively; a configured role of "*"
// admits any authenticated caller.
func roleMatches(got, want string) bool {
	if want == "*" {
		return got != ""
	}
	return strings.EqualFold(strings.TrimSpace(got), strings.TrimSpace(want))
}
