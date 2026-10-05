package auth_test

import (
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/auth"
	"github.com/google/uuid"
)

func TestJWTAccessAndRefreshTokens(t *testing.T) {
	secret := "test-super-secret-key-12345"
	userID := uuid.New()
	email := "test@oxide.film"
	role := "user"

	// 1. Генерація токена
	tokenStr, err := auth.GenerateAccessToken(userID, email, role, secret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate access token: %v", err)
	}

	// 2. Валідація токена
	claims, err := auth.ValidateAccessToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("failed to validate access token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, claims.UserID)
	}
	if claims.Email != email {
		t.Errorf("expected email %s, got %s", email, claims.Email)
	}
	if claims.Role != role {
		t.Errorf("expected role %s, got %s", role, claims.Role)
	}

	// 3. Відхилення з некоректним секретом
	_, err = auth.ValidateAccessToken(tokenStr, "wrong-secret-key")
	if err == nil {
		t.Errorf("expected error when validating with wrong secret, got nil")
	}

	// 4. Перевірка простроченого токена
	expiredTokenStr, err := auth.GenerateAccessToken(userID, email, role, secret, -1*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}
	_, err = auth.ValidateAccessToken(expiredTokenStr, secret)
	if err == nil {
		t.Errorf("expected expired token to fail validation, got nil")
	}

	// 5. Перевірка Refresh Token
	refreshToken := auth.GenerateRefreshToken()
	if _, err := uuid.Parse(refreshToken); err != nil {
		t.Errorf("expected refresh token to be a valid UUID, got %s: %v", refreshToken, err)
	}
}
