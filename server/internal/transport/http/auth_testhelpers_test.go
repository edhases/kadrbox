package http

// Small helpers shared by the package's internal tests. They exist here rather
// than being spelled out at each call site so the assertions stay about
// behaviour instead of parsing.

import (
	"crypto/sha256"
	"net/url"
	"testing"
)

// argon2KeylessSHA256 exposes the raw digest the PKCE challenge is computed
// from, so the test can assert the RFC 7636 formula itself rather than a copy of
// the implementation.
func argon2KeylessSHA256(verifier string) []byte {
	sum := sha256.Sum256([]byte(verifier))
	return sum[:]
}

func errParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return parsed
}
