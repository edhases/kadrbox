package auth_test

import (
	"testing"

	"github.com/edhases/kadrbox-server/internal/auth"
)

func TestArgon2idPasswordHashing(t *testing.T) {
	password := "SecretP@ssw0rd!2026"

	// 1. Хешування паролю
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == "" || len(hash) < 20 {
		t.Fatalf("generated hash is too short or empty: %s", hash)
	}

	// 2. Успішна валідація правильного паролю
	match, err := auth.ComparePasswordAndHash(password, hash)
	if err != nil {
		t.Fatalf("error comparing password: %v", err)
	}
	if !match {
		t.Errorf("expected password to match hash, but it did not")
	}

	// 3. Відхилення неправильного паролю
	wrongPassword := "WrongPassword!2026"
	match, err = auth.ComparePasswordAndHash(wrongPassword, hash)
	if err != nil {
		t.Fatalf("error comparing wrong password: %v", err)
	}
	if match {
		t.Errorf("expected wrong password to NOT match hash, but it matched")
	}
}
