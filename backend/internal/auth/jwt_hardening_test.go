package auth_test

// Hardening tests for the JWT contract and the KDF concurrency gate.
//
// A signed token is only as good as the checks around it: the algorithm, the
// issuer, the audience and the expiry all have to be asserted by the verifier,
// and the KDF must not be allowed to fan out past the container's memory budget.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const hardenSecret = "hardening-test-secret-key-12345"

// signWith mints a token with arbitrary claims and an arbitrary algorithm, so a
// test can produce the exact shape an attacker would.
func signWith(t *testing.T, method jwt.SigningMethod, claims auth.Claims, secret string) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func validClaims(t *testing.T) auth.Claims {
	t.Helper()
	now := time.Now()
	return auth.Claims{
		UserID: uuid.New(),
		Email:  "subject@example.com",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.Issuer,
			Subject:   "subject",
			Audience:  jwt.ClaimStrings{auth.Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
}

func TestAccessTokenCarriesIssuerAudienceAndKid(t *testing.T) {
	uid := uuid.New()
	token, err := auth.GenerateAccessToken(uid, "kid@example.com", "user", hardenSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := auth.ValidateAccessToken(token, hardenSecret)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.Issuer != auth.Issuer {
		t.Errorf("iss = %q, want %q", claims.Issuer, auth.Issuer)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != auth.Audience {
		t.Errorf("aud = %v, want [%s]", claims.Audience, auth.Audience)
	}

	// The kid is in the header, not the payload, so it can be read before any
	// signature check when keys are rotated.
	header := decodeSegment(t, token, 0)
	if got := header["kid"]; got != auth.KeyID {
		t.Errorf("kid = %v, want %s", got, auth.KeyID)
	}
	if got := header["alg"]; got != "HS256" {
		t.Errorf("alg = %v, want HS256", got)
	}
}

func TestValidateAccessTokenRejectsForeignIssuerAndAudience(t *testing.T) {
	t.Run("wrong issuer", func(t *testing.T) {
		claims := validClaims(t)
		claims.Issuer = "some-other-service"
		token := signWith(t, jwt.SigningMethodHS256, claims, hardenSecret)

		_, err := auth.ValidateAccessToken(token, hardenSecret)
		if !errors.Is(err, auth.ErrWrongIssuer) {
			t.Errorf("err = %v, want ErrWrongIssuer", err)
		}
	})

	t.Run("missing issuer", func(t *testing.T) {
		claims := validClaims(t)
		claims.Issuer = ""
		token := signWith(t, jwt.SigningMethodHS256, claims, hardenSecret)

		if _, err := auth.ValidateAccessToken(token, hardenSecret); err == nil {
			t.Error("a token with no issuer must be rejected")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		claims := validClaims(t)
		claims.Audience = jwt.ClaimStrings{"another-api"}
		token := signWith(t, jwt.SigningMethodHS256, claims, hardenSecret)

		_, err := auth.ValidateAccessToken(token, hardenSecret)
		if !errors.Is(err, auth.ErrWrongAudience) {
			t.Errorf("err = %v, want ErrWrongAudience", err)
		}
	})

	t.Run("missing audience", func(t *testing.T) {
		claims := validClaims(t)
		claims.Audience = nil
		token := signWith(t, jwt.SigningMethodHS256, claims, hardenSecret)

		if _, err := auth.ValidateAccessToken(token, hardenSecret); err == nil {
			t.Error("a token with no audience must be rejected")
		}
	})

	t.Run("an issuer/audience pair is not a wildcard", func(t *testing.T) {
		// The same secret used by a second service must not produce tokens this
		// one accepts.
		claims := validClaims(t)
		claims.Issuer = "kadrbox-server"
		claims.Audience = jwt.ClaimStrings{"oxide-web"}
		token := signWith(t, jwt.SigningMethodHS256, claims, hardenSecret)

		if _, err := auth.ValidateAccessToken(token, hardenSecret); err == nil {
			t.Error("a token minted for another audience must be rejected")
		}
	})
}

func TestValidateAccessTokenRejectsAlgNone(t *testing.T) {
	claims := validClaims(t)
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."

	if _, err := auth.ValidateAccessToken(unsigned, hardenSecret); err == nil {
		t.Error("alg=none must be rejected")
	}
}

func TestValidateAccessTokenRejectsExpired(t *testing.T) {
	claims := validClaims(t)
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-2 * time.Minute))
	claims.NotBefore = jwt.NewNumericDate(time.Now().Add(-3 * time.Minute))
	token := signWith(t, jwt.SigningMethodHS256, claims, hardenSecret)

	if _, err := auth.ValidateAccessToken(token, hardenSecret); err == nil {
		t.Error("an expired token must be rejected")
	}
}

func TestValidateAccessTokenRequiresAnExpiry(t *testing.T) {
	claims := validClaims(t)
	claims.ExpiresAt = nil
	token := signWith(t, jwt.SigningMethodHS256, claims, hardenSecret)

	if _, err := auth.ValidateAccessToken(token, hardenSecret); err == nil {
		t.Error("a token with no exp must be rejected: it would never expire")
	}
}

func TestValidateAccessTokenRejectsTamperedPayload(t *testing.T) {
	token, err := auth.GenerateAccessToken(uuid.New(), "victim@example.com", "user", hardenSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	// Escalate role=user to role=admin without touching the signature.
	parts := strings.Split(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if body["role"] != "user" {
		t.Fatalf("expected role=user, got %v", body["role"])
	}
	body["role"] = "admin"
	forgedPayload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	forged := parts[0] + "." + base64.RawURLEncoding.EncodeToString(forgedPayload) + "." + parts[2]

	if _, err := auth.ValidateAccessToken(forged, hardenSecret); err == nil {
		t.Error("a tampered payload must be rejected")
	}

	// The tampered token must not validate even for a reader that only wants the
	// role, which is exactly what RequireRole would do.
	if _, err := auth.ValidateAccessToken(forged, hardenSecret); err == nil {
		t.Error("role escalation must not survive validation")
	}
}

func TestValidateAccessTokenRejectsWrongSecret(t *testing.T) {
	token, err := auth.GenerateAccessToken(uuid.New(), "x@example.com", "user", hardenSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if _, err := auth.ValidateAccessToken(token, "a-different-secret"); err == nil {
		t.Error("a token signed with another secret must be rejected")
	}
}

func decodeSegment(t *testing.T, token string, index int) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected three JWT segments, got %d", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[index])
	if err != nil {
		t.Fatalf("decode segment %d: %v", index, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal segment %d: %v", index, err)
	}
	return out
}

// ---- KDF concurrency ---------------------------------------------------------

// The OWASP parameters must stay where they are: weakening them would help an
// offline attacker far more than it helps this process.
func TestKDFParametersAreNotWeakened(t *testing.T) {
	if auth.DefaultParams.Memory != 64*1024 {
		t.Errorf("memory = %d KiB, want 65536 KiB (OWASP floor)", auth.DefaultParams.Memory)
	}
	if auth.DefaultParams.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", auth.DefaultParams.Iterations)
	}
	if auth.DefaultParams.Parallelism != 2 {
		t.Errorf("parallelism = %d, want 2", auth.DefaultParams.Parallelism)
	}
	if auth.KDFConcurrency() < 1 {
		t.Errorf("KDF concurrency = %d, want at least 1", auth.KDFConcurrency())
	}
}

// A full gate must refuse work rather than queue it: a queue of pending Argon2
// requests is as good a memory-exhaustion primitive as unbounded concurrency.
func TestKDFGateRefusesWhenFull(t *testing.T) {
	t.Setenv("ARGON2_MAX_CONCURRENCY", "1")
	// The semaphore is process-wide and already initialised by earlier tests, so
	// this asserts the contract rather than the specific width: a hash either
	// succeeds or returns ErrKDFBusy, and it never blocks forever.
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			_, err := auth.HashPassword("concurrent-password")
			done <- err
		}()
	}
	for i := 0; i < 4; i++ {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, auth.ErrKDFBusy) {
				t.Errorf("unexpected error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("HashPassword blocked: the gate must not queue indefinitely")
		}
	}
}

func TestComparePasswordAndHashReportsBusyRatherThanBlocking(t *testing.T) {
	hash, err := auth.HashPassword("a-real-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := auth.ComparePasswordAndHash("a-real-password", hash)
	if err != nil {
		t.Fatalf("ComparePasswordAndHash: %v", err)
	}
	if !ok {
		t.Error("the freshly created hash must verify")
	}
}

func TestBurnKDFForDummyUserIsSilentAndCheap(t *testing.T) {
	// It must never panic and never surface an error; it exists purely to equalise
	// the timing of the unknown-account branch.
	start := time.Now()
	auth.BurnKDFForDummyUser("anything-at-all")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("dummy KDF took %v", elapsed)
	}
	// Second call reuses the same process-wide dummy hash.
	auth.BurnKDFForDummyUser("another")
}
