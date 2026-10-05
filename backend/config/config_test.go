package config_test

import (
	"os"
	"testing"

	"github.com/edhases/oxide-server/config"
)

func TestConfigLoadDefaults(t *testing.T) {
	// Очищення змінних оточення для перевірки дефолтів
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")
	os.Unsetenv("REDIS_ADDR")

	cfg := config.Load()

	if cfg.ServerPort != "8080" {
		t.Errorf("expected default ServerPort 8080, got %s", cfg.ServerPort)
	}
	if cfg.DBHost != "postgres" {
		t.Errorf("expected default DBHost postgres, got %s", cfg.DBHost)
	}
	if cfg.DBPort != "5432" {
		t.Errorf("expected default DBPort 5432, got %s", cfg.DBPort)
	}
	if cfg.RedisAddr != "redis:6379" {
		t.Errorf("expected default RedisAddr redis:6379, got %s", cfg.RedisAddr)
	}

	expectedDSN := "postgres://postgres:postgres@postgres:5432/oxide_film?sslmode=disable"
	if cfg.PostgresDSN() != expectedDSN {
		t.Errorf("expected DSN %s, got %s", expectedDSN, cfg.PostgresDSN())
	}
}

func TestConfigLoadCustomEnv(t *testing.T) {
	os.Setenv("SERVER_PORT", "9999")
	os.Setenv("DB_HOST", "127.0.0.1")
	os.Setenv("DB_PORT", "5434")
	os.Setenv("DB_USER", "custom_user")
	os.Setenv("DB_PASSWORD", "custom_pass")
	os.Setenv("DB_NAME", "custom_db")
	os.Setenv("REDIS_ADDR", "127.0.0.1:6379")
	defer func() {
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
		os.Unsetenv("REDIS_ADDR")
	}()

	cfg := config.Load()

	if cfg.ServerPort != "9999" {
		t.Errorf("expected ServerPort 9999, got %s", cfg.ServerPort)
	}
	expectedDSN := "postgres://custom_user:custom_pass@127.0.0.1:5434/custom_db?sslmode=disable"
	if cfg.PostgresDSN() != expectedDSN {
		t.Errorf("expected DSN %s, got %s", expectedDSN, cfg.PostgresDSN())
	}
}
