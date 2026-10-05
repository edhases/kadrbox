package http

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestCovGenerateTokenFormat(t *testing.T) {
	tok, err := generateToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 32 випадкові байти в hex = 64 символи.
	if len(tok) != 64 {
		t.Errorf("expected 64-char token, got %d chars: %q", len(tok), tok)
	}
	if _, err := hex.DecodeString(tok); err != nil {
		t.Errorf("token is not valid hex: %v", err)
	}
	if strings.Trim(tok, "0123456789abcdef") != "" {
		t.Errorf("token contains non-hex chars: %q", tok)
	}
}

func TestCovGenerateTokenUnique(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		tok, err := generateToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, dup := seen[tok]; dup {
			t.Fatalf("duplicate token generated: %q", tok)
		}
		seen[tok] = struct{}{}
	}
}
