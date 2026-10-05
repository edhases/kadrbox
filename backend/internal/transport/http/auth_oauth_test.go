package http

// Table tests for the redirect allow-list, the PKCE helpers and the HTML
// renderers. These live in the package under test (not http_test) because the
// functions are unexported and their behaviour is the security boundary.

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestIsSafeRedirectURLTable(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		// --- the allow-list itself ---
		{"production web origin", "https://film.oxideteam.pp.ua/callback?x=1", true},
		{"production apex origin", "https://oxideteam.pp.ua/callback", true},
		{"loopback ephemeral port", "http://127.0.0.1:53123/callback", true},
		{"loopback named host", "http://localhost:53123/callback", true},
		{"ipv6 loopback", "http://[::1]:53123/callback", true},
		{"trailing dot is normalised", "https://film.oxideteam.pp.ua./callback", true},

		// --- ports ---
		{"explicit default https port", "https://film.oxideteam.pp.ua:443/callback", true},
		{"non-default https port", "https://film.oxideteam.pp.ua:8443/callback", false},
		{"http downgrade of the allow-listed origin", "http://film.oxideteam.pp.ua/callback", false},
		{"loopback without a port defaults to 80, which is not allowed", "http://127.0.0.1/callback", false},
		{"loopback on a low port", "http://127.0.0.1:8080/callback", false},
		{"loopback over https", "https://127.0.0.1:53123/callback", false},

		// --- host confusion ---
		{"suffix-lookalike host", "https://film.oxideteam.pp.ua.evil.com/callback", false},
		{"prefix-lookalike host", "https://evilfilm.oxideteam.pp.ua.example.com/callback", false},
		{"unlisted subdomain", "https://staging.oxideteam.pp.ua/callback", false},
		{"userinfo smuggling a trusted host", "https://oxideteam.pp.ua@evil.com/callback", false},
		{"userinfo plus password", "https://user:pass@oxideteam.pp.ua/callback", false},
		{"host in the query string", "https://evil.com/?next=film.oxideteam.pp.ua", false},
		{"host in the path", "https://evil.com/film.oxideteam.pp.ua", false},
		{"loopback lookalike host", "http://localhost.evil.com:53123/callback", false},
		{"decimal-encoded loopback", "http://2130706433:53123/callback", false},

		// --- scheme ---
		{"scheme relative", "//evil.com/callback", false},
		{"custom scheme", "oxide://auth/google", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"data scheme", "data:text/html,<script>alert(1)</script>", false},
		{"relative path", "/callback", false},

		// --- malformed ---
		{"empty", "", false},
		{"whitespace", "   ", false},
		{"control character", "https://film.oxideteam.pp.ua/cb\nSet-Cookie: x=1", false},
		{"over-long", "https://film.oxideteam.pp.ua/" + strings.Repeat("a", 4096), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSafeRedirectURL(tc.raw); got != tc.want {
				t.Errorf("isSafeRedirectURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// The loopback port list is operator-configurable, and narrowing it must narrow
// what is accepted.
func TestIsSafeRedirectURLHonoursConfiguredLoopbackPorts(t *testing.T) {
	t.Setenv("OAUTH_ALLOWED_LOOPBACK_PORTS", "5555,6000-6002")

	allowed := []string{
		"http://127.0.0.1:5555/callback",
		"http://127.0.0.1:6000/callback",
		"http://127.0.0.1:6002/callback",
	}
	for _, raw := range allowed {
		if !isSafeRedirectURL(raw) {
			t.Errorf("expected %q to be allowed by the configured port list", raw)
		}
	}

	denied := []string{
		"http://127.0.0.1:6003/callback", // just outside the range
		"http://127.0.0.1:53123/callback",
		"http://127.0.0.1:8080/callback",
	}
	for _, raw := range denied {
		if isSafeRedirectURL(raw) {
			t.Errorf("expected %q to be denied by the configured port list", raw)
		}
	}
}

func TestPKCEChallengeS256(t *testing.T) {
	verifier, err := newPKCEVerifier()
	if err != nil {
		t.Fatalf("newPKCEVerifier: %v", err)
	}
	// RFC 7636 allows 43-128 unreserved characters; 32 random bytes base64url
	// without padding is exactly 43.
	if len(verifier) != 43 {
		t.Errorf("verifier length = %d, want 43", len(verifier))
	}

	got := pkceChallengeS256(verifier)
	want := base64.RawURLEncoding.EncodeToString(
		argon2KeylessSHA256(verifier))
	if got != want {
		t.Errorf("challenge = %q, want BASE64URL(SHA256(verifier)) = %q", got, want)
	}
	// A challenge must never be the verifier itself, or PKCE would protect
	// nothing.
	if got == verifier {
		t.Error("challenge must differ from the verifier")
	}

	// Deterministic: the same verifier always produces the same challenge.
	if again := pkceChallengeS256(verifier); again != got {
		t.Error("challenge is not deterministic")
	}
	// Different verifiers must not collide.
	other, _ := newPKCEVerifier()
	if pkceChallengeS256(other) == got {
		t.Error("two verifiers produced the same challenge")
	}
}

func TestOAuthStateNonceIsUnpredictable(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		nonce, err := newOAuthStateNonce()
		if err != nil {
			t.Fatalf("newOAuthStateNonce: %v", err)
		}
		// 32 random bytes, base64url unpadded.
		if len(nonce) != 43 {
			t.Fatalf("nonce length = %d, want 43 (256 bits)", len(nonce))
		}
		if seen[nonce] {
			t.Fatalf("duplicate nonce after %d draws", i)
		}
		seen[nonce] = true
	}
}

func TestStripQueryParamDropsLinkToken(t *testing.T) {
	got := stripQueryParam("http://127.0.0.1:53123/callback?link_token=secret&keep=1", "link_token")
	if strings.Contains(got, "link_token") {
		t.Errorf("link_token survived: %s", got)
	}
	if !strings.Contains(got, "keep=1") {
		t.Errorf("unrelated params must survive: %s", got)
	}
	if got := stripQueryParam("http://127.0.0.1:53123/callback", "link_token"); got != "http://127.0.0.1:53123/callback" {
		t.Errorf("a URL without the param must be returned unchanged, got %s", got)
	}
}

func TestAppendQueryParamsPreservesExistingQuery(t *testing.T) {
	got := appendQueryParams("http://127.0.0.1:53123/callback?keep=1", map[string]string{
		"access_token":  "a b&c",
		"refresh_token": "rt",
	})
	parsed := errParse(t, got)
	if parsed.Query().Get("keep") != "1" {
		t.Errorf("existing query lost: %s", got)
	}
	if parsed.Query().Get("access_token") != "a b&c" {
		t.Errorf("token not encoded: %s", got)
	}
	if parsed.Query().Get("refresh_token") != "rt" {
		t.Errorf("refresh token not encoded: %s", got)
	}
}
