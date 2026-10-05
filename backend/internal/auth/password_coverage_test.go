package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/edhases/kadrbox-server/internal/auth"
)

// covPasswordMutateSegment повертає валідний хеш із заміненим $-сегментом idx (0..5).
func covPasswordMutateSegment(t *testing.T, hash string, idx int, replacement string) string {
	t.Helper()
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("expected valid hash with 6 segments, got %d: %s", len(parts), hash)
	}
	parts[idx] = replacement
	return strings.Join(parts, "$")
}

func covPasswordValidHash(t *testing.T) string {
	t.Helper()
	h, err := auth.HashPassword("coverage-password-2026")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	return h
}

func TestCovPasswordMalformedTable(t *testing.T) {
	valid := covPasswordValidHash(t)
	parts := strings.Split(valid, "$")
	cases := map[string]string{
		"noDollar":      "notahashwithoutdollars",
		"fiveSegments":  strings.Join(parts[:5], "$"),
		"sevenSegments": valid + "$extra",
	}
	for name, h := range cases {
		if _, err := auth.ComparePasswordAndHash("coverage-password-2026", h); !errors.Is(err, auth.ErrInvalidHash) {
			t.Errorf("%s: expected ErrInvalidHash, got %v", name, err)
		}
	}
}

func TestCovPasswordWrongAlgorithm(t *testing.T) {
	valid := covPasswordValidHash(t)
	mutated := strings.Replace(valid, "argon2id", "argon2i", 1)
	if _, err := auth.ComparePasswordAndHash("coverage-password-2026", mutated); !errors.Is(err, auth.ErrIncompatibleVersion) {
		t.Errorf("expected ErrIncompatibleVersion, got %v", err)
	}
}

func TestCovPasswordIncompatibleVersion(t *testing.T) {
	valid := covPasswordValidHash(t)
	mutated := strings.Replace(valid, "v=19", "v=16", 1)
	if mutated == valid {
		t.Skip("argon2 version prefix v=19 not found, layout changed")
	}
	if _, err := auth.ComparePasswordAndHash("coverage-password-2026", mutated); !errors.Is(err, auth.ErrIncompatibleVersion) {
		t.Errorf("expected ErrIncompatibleVersion, got %v", err)
	}
}

func TestCovPasswordUnparsableParams(t *testing.T) {
	valid := covPasswordValidHash(t)
	mutated := covPasswordMutateSegment(t, valid, 3, "m=abc,t=1,p=1")
	if _, err := auth.ComparePasswordAndHash("coverage-password-2026", mutated); err == nil {
		t.Errorf("expected parse error for unparsable params, got nil")
	}
}

func TestCovPasswordInvalidBase64Salt(t *testing.T) {
	valid := covPasswordValidHash(t)
	mutated := covPasswordMutateSegment(t, valid, 4, "!!!")
	if _, err := auth.ComparePasswordAndHash("coverage-password-2026", mutated); err == nil {
		t.Errorf("expected base64 error for corrupted salt segment, got nil")
	}
}

func TestCovPasswordInvalidBase64Hash(t *testing.T) {
	valid := covPasswordValidHash(t)
	mutated := covPasswordMutateSegment(t, valid, 5, "!!!")
	if _, err := auth.ComparePasswordAndHash("coverage-password-2026", mutated); err == nil {
		t.Errorf("expected base64 error for corrupted hash segment, got nil")
	}
}

func TestCovPasswordHashFormat(t *testing.T) {
	h, err := auth.HashPassword("format-check")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Errorf("expected prefix $argon2id$v=19$, got %s", h)
	}
	if n := strings.Count(h, "$"); n != 5 {
		t.Errorf("expected exactly 5 $ characters, got %d in %s", n, h)
	}
}

func TestCovPasswordSaltsDiffer(t *testing.T) {
	pw := "same-password-2026"
	h1, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	h2, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if h1 == h2 {
		t.Errorf("expected different salts to produce different hashes")
	}
	for i, h := range []string{h1, h2} {
		ok, err := auth.ComparePasswordAndHash(pw, h)
		if err != nil || !ok {
			t.Errorf("hash %d: expected match without error, got ok=%v err=%v", i, ok, err)
		}
	}
}

func TestCovPasswordEmptyPassword(t *testing.T) {
	h, err := auth.HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword(\"\") failed: %v", err)
	}
	ok, err := auth.ComparePasswordAndHash("", h)
	if err != nil || !ok {
		t.Errorf("expected empty password to validate, got ok=%v err=%v", ok, err)
	}
}
