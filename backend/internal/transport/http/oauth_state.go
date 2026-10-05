package http

// OAuth `state` handling: generation, server-side storage, single-use consume,
// PKCE, and session binding.
//
// Why a server-side store rather than a signed state JWT: the callback must be
// rejected when the state is *replayed*, and a signed token cannot be made
// single-use without a store anyway. Once Redis is in the picture, keeping one
// mechanism (server-side record + atomic consume) gives CSRF protection,
// single-use and expiry from the same primitive, and it is the only variant
// that can carry the PKCE verifier without exposing it to the browser.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// oauthStateTTL bounds how long a login attempt stays valid. The window has
	// to cover the provider round trip (including account creation and MFA) but
	// no longer: 10 minutes.
	oauthStateTTL = 10 * time.Minute

	// oauthStateCookieName binds a login attempt to the browser that started
	// it. Without that binding an attacker can hand a victim a callback URL and
	// log the victim into the attacker's account (login CSRF).
	oauthStateCookieName = "oxide_oauth_nonce"

	oauthStateCookieMaxAge = 600

	// oauthStateSecretBytes is 32 bytes = 256 bits, well past the 128-bit floor
	// RFC 6749 asks for in section 10.10.
	oauthStateSecretBytes = 32

	pkceSecretBytes = 32
)

// ErrInvalidOAuthState is returned for every state rejection — absent, unknown,
// expired, already consumed, or bound to a different session or link token. The
// caller must not distinguish them in the response.
var ErrInvalidOAuthState = errors.New("invalid oauth state")

// oauthStateRecord is what the server keeps for the lifetime of one login
// attempt. It is stored server-side only; the browser sees nothing but the
// opaque nonce in the `state` parameter.
type oauthStateRecord struct {
	// Nonce is the value handed to the provider as `state`. It is echoed back by
	// the provider and matched against the store key, never parsed.
	Nonce string `json:"-"`
	// Provider binds the record to the flow that created it, so a state issued
	// for Telegram cannot be replayed against the Discord callback.
	Provider string `json:"provider"`
	// RedirectTo is the post-login destination, already validated against the
	// exact-origin allow-list when the flow started. It is deliberately NOT
	// taken from the callback's query string: that value is attacker-supplied.
	RedirectTo string `json:"redirect_to,omitempty"`
	// LinkUserID is the account a provider identity is being attached to. Empty
	// means plain sign-in.
	LinkUserID string `json:"link_user_id,omitempty"`
	// CodeVerifier is the PKCE verifier. It never leaves the server.
	CodeVerifier string `json:"code_verifier,omitempty"`
	// SessionHash binds the attempt to the browser that started it.
	SessionHash string `json:"session_hash,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// oauthStateStore is the persistence contract for login attempts. It is
// intentionally separate from RefreshStore: OAuth state is short-lived and
// single-use, refresh tokens are long-lived and rotating, and a test fake for
// one must not be usable as the other.
type oauthStateStore interface {
	SetOAuthState(ctx context.Context, state string, payload []byte, ttl time.Duration) error
	ConsumeOAuthState(ctx context.Context, state string) ([]byte, error)
}

// ---- nonce / PKCE generation -------------------------------------------------

// randomTokenBase64URL returns n cryptographically random bytes, base64url
// encoded without padding.
func randomTokenBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newOAuthStateNonce returns the value sent to the provider as `state`.
func newOAuthStateNonce() (string, error) {
	return randomTokenBase64URL(oauthStateSecretBytes)
}

// newPKCEVerifier returns an RFC 7636 code verifier: 43-128 characters of
// unreserved URL-safe text.
func newPKCEVerifier() (string, error) {
	return randomTokenBase64URL(pkceSecretBytes)
}

// pkceChallengeS256 is BASE64URL(SHA256(ASCII(code_verifier))).
func pkceChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// hashSessionNonce stores only a digest of the browser cookie, so a dump of the
// state store does not hand out a usable session binding.
func hashSessionNonce(nonce string) string {
	if nonce == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

// ---- session binding ---------------------------------------------------------

// issueSessionNonce returns a fresh browser-binding nonce, or "" when the client
// refuses cookies. An unbound attempt is still protected by the single-use
// server-side state and PKCE, so it is allowed rather than fatal.
func issueSessionNonce(w http.ResponseWriter, r *http.Request) string {
	nonce, err := randomTokenBase64URL(oauthStateSecretBytes)
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    nonce,
		Path:     "/",
		MaxAge:   oauthStateCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	return nonce
}

func readSessionNonce(r *http.Request) string {
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// sessionMatches reports whether the callback came from the browser that
// started the flow. An attempt that was bound to a session must be completed by
// that session.
func (rec oauthStateRecord) sessionMatches(r *http.Request) bool {
	if rec.SessionHash == "" {
		return true
	}
	return hashSessionNonce(readSessionNonce(r)) == rec.SessionHash
}

// ---- store plumbing ----------------------------------------------------------

// beginOAuthAttempt validates the requested post-login destination, mints the
// nonce / PKCE verifier / session binding, stores the record and returns the
// `state` value to hand to the provider.
func (h *AuthHandler) beginOAuthAttempt(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	provider string,
	withPKCE bool,
) (string, oauthStateRecord, error) {
	redirectTo, linkToken := requestedRedirect(r)

	// Strip the link token from the destination: it has already been consumed
	// for binding purposes and must not be replayed into a browser URL or a
	// loopback listener's access log.
	redirectTo = stripQueryParam(redirectTo, "link_token")

	nonce := issueSessionNonce(w, r)

	rec := oauthStateRecord{
		Provider:    provider,
		RedirectTo:  redirectTo,
		LinkUserID:  h.linkUserIDFromToken(linkToken),
		SessionHash: hashSessionNonce(nonce),
		CreatedAt:   time.Now().Unix(),
	}
	if withPKCE {
		verifier, err := newPKCEVerifier()
		if err != nil {
			return "", oauthStateRecord{}, err
		}
		rec.CodeVerifier = verifier
	}

	state, err := newOAuthStateNonce()
	if err != nil {
		return "", oauthStateRecord{}, err
	}
	rec.Nonce = state

	payload, err := json.Marshal(rec)
	if err != nil {
		return "", oauthStateRecord{}, err
	}
	if err := h.stateStore.SetOAuthState(ctx, state, payload, oauthStateTTL); err != nil {
		return "", oauthStateRecord{}, err
	}
	return state, rec, nil
}

// consumeOAuthAttempt atomically fetches and deletes the state record. A second
// call with the same state always fails, which is what makes the flow
// single-use.
func (h *AuthHandler) consumeOAuthAttempt(ctx context.Context, state string) (oauthStateRecord, error) {
	if state == "" {
		return oauthStateRecord{}, ErrInvalidOAuthState
	}
	payload, err := h.stateStore.ConsumeOAuthState(ctx, state)
	if err != nil || len(payload) == 0 {
		return oauthStateRecord{}, ErrInvalidOAuthState
	}
	var rec oauthStateRecord
	if err := json.Unmarshal(payload, &rec); err != nil {
		return oauthStateRecord{}, ErrInvalidOAuthState
	}
	// Defensive: TTL is enforced by the store, but a store that loses its TTL
	// must not turn into an eternal replay window.
	if rec.CreatedAt > 0 && time.Since(time.Unix(rec.CreatedAt, 0)) > oauthStateTTL {
		return oauthStateRecord{}, ErrInvalidOAuthState
	}
	return rec, nil
}

// verifyCallbackState performs every check a provider callback must pass before
// any code is exchanged or any user is looked up.
//
// Order matters only for cost; all failures are reported identically.
func (h *AuthHandler) verifyCallbackState(ctx context.Context, r *http.Request, provider string) (oauthStateRecord, error) {
	rec, err := h.consumeOAuthAttempt(ctx, r.URL.Query().Get("state"))
	if err != nil {
		return oauthStateRecord{}, err
	}
	if provider != "" && rec.Provider != provider {
		return oauthStateRecord{}, ErrInvalidOAuthState
	}
	if !rec.sessionMatches(r) {
		return oauthStateRecord{}, ErrInvalidOAuthState
	}
	// If the callback carries its own link_token it must resolve to the same
	// account the flow was started for; otherwise the binding could be swapped
	// between initiation and completion.
	if token := r.URL.Query().Get("link_token"); token != "" {
		if h.linkUserIDFromToken(token) != rec.LinkUserID {
			return oauthStateRecord{}, ErrInvalidOAuthState
		}
	}
	return rec, nil
}

// linkUserIDFromToken resolves a link_token JWT to a user id, or "" when the
// token is absent or invalid.
func (h *AuthHandler) linkUserIDFromToken(token string) string {
	if token == "" {
		return ""
	}
	claims, err := h.validateLinkToken(token)
	if err != nil {
		return ""
	}
	return claims.UserID.String()
}

// requestedRedirect extracts the post-login destination from the request.
//
// `redirect_to` is the documented parameter; `state` is still read for older
// clients, which used it to carry the same value. Both are only accepted after
// passing the exact-origin allow-list, and both end up stored server-side, so
// the callback never re-derives the destination from request input.
func requestedRedirect(r *http.Request) (redirectTo string, linkToken string) {
	q := r.URL.Query()
	raw := q.Get("redirect_to")
	if raw == "" {
		raw = q.Get("state")
	}
	if !isSafeRedirectURL(raw) {
		return "", ""
	}
	if u, err := parseAbsoluteURL(raw); err == nil {
		linkToken = queryParam(u, "link_token")
	}
	// An explicit top-level link_token is also honoured, so a client can bind
	// without having to smuggle it through the destination URL.
	if token := q.Get("link_token"); token != "" {
		linkToken = token
	}
	return raw, linkToken
}

func stripQueryParam(rawURL, key string) string {
	u, err := parseAbsoluteURL(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if !q.Has(key) {
		return rawURL
	}
	q.Del(key)
	u.RawQuery = q.Encode()
	return u.String()
}

func queryParam(u *url.URL, key string) string {
	if u == nil {
		return ""
	}
	return u.Query().Get(key)
}

func parseAbsoluteURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("empty url")
	}
	return url.Parse(raw)
}

// ---- in-memory fallback store -----------------------------------------------

// memoryOAuthStateStore is used only when the configured RefreshStore cannot
// store OAuth state (in tests, or a deployment without Redis). It is correct for
// a single process and single-use, but it is not shared between instances, so
// the constructor logs a warning when it is selected.
type memoryOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]memoryOAuthState
}

type memoryOAuthState struct {
	payload   []byte
	expiresAt time.Time
}

func newMemoryOAuthStateStore() *memoryOAuthStateStore {
	return &memoryOAuthStateStore{states: map[string]memoryOAuthState{}}
}

func (s *memoryOAuthStateStore) SetOAuthState(_ context.Context, state string, payload []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	s.states[state] = memoryOAuthState{payload: payload, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *memoryOAuthStateStore) ConsumeOAuthState(_ context.Context, state string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	entry, ok := s.states[state]
	if !ok {
		return nil, errors.New("oauth state not found")
	}
	delete(s.states, state)
	return entry.payload, nil
}

func (s *memoryOAuthStateStore) evictExpiredLocked() {
	now := time.Now()
	for key, entry := range s.states {
		if now.After(entry.expiresAt) {
			delete(s.states, key)
		}
	}
}
