package auth_test

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const covJwtSecret = "coverage-test-secret-key-12345"

func TestCovJwtAlgNoneRejected(t *testing.T) {
	uid := uuid.New().String()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"user_id":%q,"email":"none@oxide.film","role":"user","sub":%q,"exp":%d}`,
		uid, uid, time.Now().Add(time.Hour).Unix(),
	)))
	tokenString := header + "." + payload + "."
	if _, err := auth.ValidateAccessToken(tokenString, covJwtSecret); err == nil {
		t.Errorf("expected alg=none token to be rejected, got nil error")
	}
}

// TestCovJwtHS512Accepted — ХАРАКТЕРИЗАЦІЯ: ValidateAccessToken перевіряє лише
// *jwt.SigningMethodHMAC (jwt.go:42), тому токен, підписаний HS512, приймається.
// Якщо політику посилять до строго HS256 — цей тест має впасти і це бажано.
func TestCovJwtHS512Accepted(t *testing.T) {
	uid := uuid.New()
	claims := auth.Claims{
		UserID: uid,
		Email:  "hs512@oxide.film",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   uid.String(),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(covJwtSecret))
	if err != nil {
		t.Fatalf("failed to sign HS512 token: %v", err)
	}
	got, err := auth.ValidateAccessToken(signed, covJwtSecret)
	if err != nil {
		t.Fatalf("HS512 token rejected: %v", err)
	}
	if got.UserID != uid {
		t.Errorf("expected userID %s, got %s", uid, got.UserID)
	}
}

func TestCovJwtMalformedTable(t *testing.T) {
	cases := map[string]string{
		"empty":        "",
		"single":       "abc",
		"threeDotless": "a.b.c",
	}
	for name, tokenString := range cases {
		if _, err := auth.ValidateAccessToken(tokenString, covJwtSecret); err == nil {
			t.Errorf("%s: expected error for malformed token, got nil", name)
		}
	}
}

func TestCovJwtSubjectIsUserID(t *testing.T) {
	uid := uuid.New()
	tokenString, err := auth.GenerateAccessToken(uid, "sub@oxide.film", "user", covJwtSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	claims, err := auth.ValidateAccessToken(tokenString, covJwtSecret)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}
	if claims.Subject != uid.String() {
		t.Errorf("expected subject %s, got %s", uid.String(), claims.Subject)
	}
	if claims.UserID != uid {
		t.Errorf("expected userID %s, got %s", uid, claims.UserID)
	}
}

func TestCovJwtRoleRoundTrip(t *testing.T) {
	for _, role := range []string{"user", "admin"} {
		uid := uuid.New()
		tokenString, err := auth.GenerateAccessToken(uid, "role@oxide.film", role, covJwtSecret, 15*time.Minute)
		if err != nil {
			t.Fatalf("role %s: failed to generate token: %v", role, err)
		}
		claims, err := auth.ValidateAccessToken(tokenString, covJwtSecret)
		if err != nil {
			t.Fatalf("role %s: failed to validate token: %v", role, err)
		}
		if claims.Role != role {
			t.Errorf("role %s: round trip failed, got %s", role, claims.Role)
		}
	}
}

func TestCovJwtRefreshTokenIsUUID(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		rt := auth.GenerateRefreshToken()
		if _, err := uuid.Parse(rt); err != nil {
			t.Fatalf("iteration %d: refresh token %q is not a UUID: %v", i, rt, err)
		}
		if _, dup := seen[rt]; dup {
			t.Fatalf("iteration %d: duplicate refresh token %q", i, rt)
		}
		seen[rt] = struct{}{}
	}
}
