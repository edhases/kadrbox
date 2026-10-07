package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The per-caller side of the mail and secret endpoints, and the guards on the
// endpoints that burn a token or run the KDF.
//
// The per-INBOX budget is pinned separately, in auth_mail_budget_test.go, which
// needs a working mail service to observe:
//
//   - the per-inbox budget is actually consulted by the handlers, and it is
//     SHARED between them, so alternating forgot-password and resend-verification
//     does not buy one inbox twice the mail it is owed;
//   - every outcome still answers the same body, because a 429 from the limiter
//     would otherwise become an oracle for how often an address has been asked
//     about -- and because the verified case used to answer differently at all.
func postFromAddr(
	t *testing.T, h http.HandlerFunc, path string, ctx context.Context,
	body any, remoteAddr string,
) *httptest.ResponseRecorder {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	req.RemoteAddr = remoteAddr
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestVerifyEmailAndResetAreRateLimited(t *testing.T) {
	// Both burn a secret or run the KDF, so both are bounded per caller.
	h, _, _ := newAuthRig(t)

	for _, tc := range []struct {
		name     string
		handler  http.HandlerFunc
		path     string
		body     map[string]string
		attempts int
	}{
		{
			name:     "verify-email",
			handler:  h.VerifyEmail,
			path:     "/api/v1/auth/verify-email",
			body:     map[string]string{"token": "not-a-real-token"},
			attempts: 12,
		},
		{
			name:     "reset-password",
			handler:  h.ResetPassword,
			path:     "/api/v1/auth/reset-password",
			body:     map[string]string{"token": "not-a-real-token", "password": "a-new-password"},
			attempts: 8,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var last int
			limited := false
			for range tc.attempts {
				rr := postFromAddr(t, tc.handler, tc.path, context.Background(),
					tc.body, "203.0.113.90:1")
				last = rr.Code
				if rr.Code == http.StatusTooManyRequests {
					limited = true
					break
				}
			}
			if !limited {
				t.Errorf("%d attempts were never limited (last status %d)", tc.attempts, last)
			}
		})
	}
}

func TestRefreshIsRateLimited(t *testing.T) {
	// A backstop, not a wall: the limiter-level test pins the generous budget so
	// this one only has to prove the guard is wired at all.
	h, _, _ := newAuthRig(t)

	limited := false
	for range 40 {
		rr := postFromAddr(t, h.Refresh, "/api/v1/auth/refresh", context.Background(),
			map[string]string{"refresh_token": "not-a-real-token"}, "198.51.100.95:1")
		if rr.Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("forty refreshes from one caller were never limited")
	}
}
