package config

import (
	"testing"
)

// getEnvInt мертвий код (не викликається з Load), але тестуємо
// контракт щоб зафіксувати поведінку якщо його колись підключать.
func TestCovConfigGetEnvInt(t *testing.T) {
	const key = "COV_TEST_INT_KEY"

	t.Run("valid number", func(t *testing.T) {
		t.Setenv(key, "42")
		if got := getEnvInt(key, 7); got != 42 {
			t.Errorf("expected 42, got %d", got)
		}
	})
	t.Run("non-numeric falls back to default", func(t *testing.T) {
		t.Setenv(key, "abc")
		if got := getEnvInt(key, 7); got != 7 {
			t.Errorf("expected default 7, got %d", got)
		}
	})
	t.Run("empty means unset", func(t *testing.T) {
		t.Setenv(key, "")
		if got := getEnvInt(key, 7); got != 7 {
			t.Errorf("expected default 7, got %d", got)
		}
	})
	t.Run("unset returns default", func(t *testing.T) {
		if got := getEnvInt("COV_TEST_INT_KEY_DEFINITELY_UNSET", 7); got != 7 {
			t.Errorf("expected default 7, got %d", got)
		}
	})
}
