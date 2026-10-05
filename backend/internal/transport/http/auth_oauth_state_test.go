package http

// Behavioural tests for the OAuth state lifecycle: it is minted server-side, it
// is bound to the browser session that started it, it is bound to the link
// token when there is one, and it can be redeemed exactly once.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/auth"
	"github.com/edhases/kadrbox-server/internal/email"
	"github.com/google/uuid"
)

const stateTestJWTSecret = "oauth-state-test-secret"

func newStateHandler(t *testing.T) *AuthHandler {
	t.Helper()
	t.Setenv("RESEND_API", "")
	return newAuthHandler(nil, nil, nil, email.NewService(), stateTestJWTSecret, "")
}

// beginFlow drives the login endpoint and returns the state it minted plus the
// session cookie the caller must present on the callback.
func beginFlow(t *testing.T, h *AuthHandler, target string) (state string, cookie *http.Cookie, location string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/login", nil)
	if target != "" {
		req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/login?redirect_to="+url.QueryEscape(target), nil)
	}
	h.SetGoogleOAuth("google-client-id", "google-secret", "https://film.oxideteam.pp.ua/api/v1/auth/google/callback")
	rr := httptest.NewRecorder()
	h.GoogleLogin(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("GoogleLogin status = %d, want 307 (body: %s)", rr.Code, rr.Body.String())
	}
	location = rr.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse provider URL: %v", err)
	}
	state = parsed.Query().Get("state")
	if state == "" {
		t.Fatalf("no state in the provider URL: %s", location)
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == oauthStateCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("no %s cookie was issued", oauthStateCookieName)
	}
	return state, cookie, location
}

func callbackRequest(state string, cookie *http.Cookie, extra string) *http.Request {
	target := "/api/v1/auth/google/callback?state=" + url.QueryEscape(state)
	if extra != "" {
		target += "&" + extra
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

func TestOAuthStateIsSingleUse(t *testing.T) {
	h := newStateHandler(t)
	state, cookie, _ := beginFlow(t, h, "http://127.0.0.1:53123/callback")

	// First redemption succeeds (the code exchange then fails against the real
	// provider, which is irrelevant here: what matters is that the state is gone).
	h.GoogleCallback(httptest.NewRecorder(), callbackRequest(state, cookie, ""))

	if _, err := h.stateStore.ConsumeOAuthState(context.Background(), state); err == nil {
		t.Fatal("the state must be gone after one redemption")
	}

	rr := httptest.NewRecorder()
	h.GoogleCallback(rr, callbackRequest(state, cookie, ""))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("replayed callback status = %d, want 400", rr.Code)
	}
	if rr.Header().Get("Location") != "" {
		t.Errorf("a rejected callback must not redirect anywhere, got %s", rr.Header().Get("Location"))
	}
	if !strings.Contains(rr.Body.String(), "Недійсний") {
		t.Errorf("unexpected rejection body: %s", rr.Body.String())
	}
}

func TestOAuthStateRejectsUnknownAndMissing(t *testing.T) {
	h := newStateHandler(t)
	state, cookie, _ := beginFlow(t, h, "http://127.0.0.1:53123/callback")

	for _, tc := range []struct {
		name  string
		state string
	}{
		{"absent", ""},
		{"unknown", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		{"state from a different flow", state + "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.GoogleCallback(rr, callbackRequest(tc.state, cookie, ""))
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rr.Code)
			}
			if rr.Header().Get("Location") != "" {
				t.Errorf("must not redirect on a rejected state, got %s", rr.Header().Get("Location"))
			}
		})
	}
}

func TestOAuthStateExpires(t *testing.T) {
	h := newStateHandler(t)

	// A record whose stored age is past the TTL must be refused even if the
	// backing store still has it, so a store that loses its TTL cannot become an
	// eternal replay window.
	payload, err := json.Marshal(oauthStateRecord{
		Provider:  "google",
		CreatedAt: time.Now().Add(-oauthStateTTL - time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const stale = "stale-state-value"
	if err := h.stateStore.SetOAuthState(context.Background(), stale, payload, time.Hour); err != nil {
		t.Fatalf("SetOAuthState: %v", err)
	}

	rr := httptest.NewRecorder()
	h.GoogleCallback(rr, callbackRequest(stale, &http.Cookie{Name: oauthStateCookieName, Value: "anything"}, ""))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expired state status = %d, want 400", rr.Code)
	}
}

func TestOAuthStateIsBoundToBrowserSession(t *testing.T) {
	// One started flow, two callbacks from the "wrong" browser: neither may be
	// redeemed, and the second proves the first was not accepted either.
	t.Run("a callback from a different browser is refused", func(t *testing.T) {
		h := newStateHandler(t)
		state, _, _ := beginFlow(t, h, "http://127.0.0.1:53123/callback")
		stolen := &http.Cookie{Name: oauthStateCookieName, Value: "someone-elses-nonce"}
		rr := httptest.NewRecorder()
		h.GoogleCallback(rr, callbackRequest(state, stolen, ""))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rr.Code)
		}
		if rr.Header().Get("Location") != "" {
			t.Errorf("must not redirect, got %s", rr.Header().Get("Location"))
		}
	})

	t.Run("no cookie at all is refused", func(t *testing.T) {
		h := newStateHandler(t)
		state, _, _ := beginFlow(t, h, "http://127.0.0.1:53123/callback")
		rr := httptest.NewRecorder()
		h.GoogleCallback(rr, callbackRequest(state, nil, ""))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})
}

func TestOAuthStateBindsTheLinkToken(t *testing.T) {
	linkUser := uuid.New()
	otherUser := uuid.New()

	linkTokenFor := func(id uuid.UUID) string {
		token, err := auth.GenerateAccessToken(id, "linker@example.com", "user", stateTestJWTSecret, time.Hour)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}
		return token
	}

	t.Run("a link token resolved at start is carried in the record", func(t *testing.T) {
		h := newStateHandler(t)
		target := "http://127.0.0.1:53123/callback?link_token=" + url.QueryEscape(linkTokenFor(linkUser))
		state, _, _ := beginFlow(t, h, target)

		rec, err := h.consumeOAuthAttempt(context.Background(), state)
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if rec.LinkUserID != linkUser.String() {
			t.Errorf("LinkUserID = %q, want %q", rec.LinkUserID, linkUser)
		}
		// The link token is a bearer credential for the caller's session; it must
		// not be echoed back into the destination URL.
		if strings.Contains(rec.RedirectTo, "link_token") {
			t.Errorf("link_token survived into the stored destination: %s", rec.RedirectTo)
		}
	})

	t.Run("a different link token at callback time is refused", func(t *testing.T) {
		h := newStateHandler(t)
		target := "http://127.0.0.1:53123/callback?link_token=" + url.QueryEscape(linkTokenFor(linkUser))
		state, cookie, _ := beginFlow(t, h, target)

		rr := httptest.NewRecorder()
		req := callbackRequest(state, cookie, "link_token="+url.QueryEscape(linkTokenFor(otherUser)))
		h.GoogleCallback(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
		}
		if rr.Header().Get("Location") != "" {
			t.Errorf("must not redirect, got %s", rr.Header().Get("Location"))
		}
	})

	t.Run("the same link token at callback time is accepted", func(t *testing.T) {
		h := newStateHandler(t)
		token := linkTokenFor(linkUser)
		target := "http://127.0.0.1:53123/callback?link_token=" + url.QueryEscape(token)
		state, cookie, _ := beginFlow(t, h, target)

		rr := httptest.NewRecorder()
		h.GoogleCallback(rr, callbackRequest(state, cookie, "link_token="+url.QueryEscape(token)))
		// It gets past state validation and fails later, on the code exchange.
		if strings.Contains(rr.Body.String(), "Недійсний або прострочений") {
			t.Errorf("the state should have validated: %s", rr.Body.String())
		}
	})
}

func TestOAuthStateRejectsAnotherProvidersState(t *testing.T) {
	h := newStateHandler(t)
	// A Telegram-issued state must not be redeemable against Discord.
	payload, err := json.Marshal(oauthStateRecord{Provider: "telegram", CreatedAt: time.Now().Unix()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	const state = "cross-provider-state"
	if err := h.stateStore.SetOAuthState(context.Background(), state, payload, time.Hour); err != nil {
		t.Fatalf("SetOAuthState: %v", err)
	}

	h.SetOAuth("", "", "discord-id", "discord-secret", "https://film.oxideteam.pp.ua/api/v1/auth/discord/callback", "")
	rr := httptest.NewRecorder()
	h.DiscordCallback(rr, callbackRequest(state, nil, ""))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestGoogleLoginPublishesPKCEChallenge(t *testing.T) {
	h := newStateHandler(t)
	_, _, location := beginFlow(t, h, "")

	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	challenge := q.Get("code_challenge")
	if challenge == "" {
		t.Fatal("no code_challenge was sent")
	}
	if challenge == q.Get("state") {
		t.Error("the challenge must not be the state")
	}

	// The verifier behind the challenge must be the one stored server-side, and
	// must never appear in the provider URL.
	verifier := storedVerifier(t, h, q.Get("state"))
	if pkceChallengeS256(verifier) != challenge {
		t.Error("the published challenge does not match the stored verifier")
	}
	if strings.Contains(location, verifier) {
		t.Error("the PKCE verifier leaked into the provider URL")
	}
}

func storedVerifier(t *testing.T, h *AuthHandler, state string) string {
	t.Helper()
	payload, err := h.stateStore.ConsumeOAuthState(context.Background(), state)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var rec oauthStateRecord
	if err := json.Unmarshal(payload, &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rec.CodeVerifier == "" {
		t.Fatal("no PKCE verifier was stored")
	}
	return rec.CodeVerifier
}
