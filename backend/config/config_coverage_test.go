package config_test

import (
	"strings"
	"testing"

	"github.com/edhases/oxide-server/config"
)

// TestCovConfigDSNSpecialChars — ХАРАКТЕРИЗАЦІЯ: PostgresDSN будується через
// net/url (url.UserPassword), тому спецсимволи пароля percent-encode'яться
// (@ → %40 тощо) і DSN відрізняється від наївної конкатенації user:password@.
func TestCovConfigDSNSpecialChars(t *testing.T) {
	cfg := &config.Config{
		DBUser:     "u",
		DBPassword: "p@ss w:rd/x?#",
		DBHost:     "h",
		DBPort:     "5432",
		DBName:     "db",
		DBSSLMode:  "disable",
	}
	dsn := cfg.PostgresDSN()
	if !strings.Contains(dsn, "p%40ss") {
		t.Errorf("expected percent-encoded password (p%%40ss) in DSN, got %s", dsn)
	}
	if strings.Contains(dsn, "p@ss") {
		t.Errorf("expected raw @ to be encoded, got %s", dsn)
	}
}

func TestCovConfigEmptyStringIsUnset(t *testing.T) {
	t.Setenv("DB_HOST", "")
	cfg := config.Load()
	if cfg.DBHost != "postgres" {
		t.Errorf("expected empty DB_HOST to fall back to postgres, got %s", cfg.DBHost)
	}
}

func TestCovConfigAllCustom(t *testing.T) {
	// NOTE: getEnvInt не тестуємо напряму — він неекспортований і ніде
	// в кодовій базі не викликається (мертвий код).
	t.Setenv("SERVER_PORT", "1111")
	t.Setenv("DB_HOST", "custom-host")
	t.Setenv("DB_PORT", "5544")
	t.Setenv("DB_USER", "custom_user")
	t.Setenv("DB_PASSWORD", "custom_pass")
	t.Setenv("DB_NAME", "custom_db")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("REDIS_ADDR", "custom-redis:6380")
	t.Setenv("REDIS_PASSWORD", "redis-pass")
	t.Setenv("JWT_SECRET", "custom-jwt-secret")
	t.Setenv("BASE_PROXY_URL", "http://127.0.0.1:9999")

	cfg := config.Load()

	want := map[string]string{
		"ServerPort":   "1111",
		"DBHost":       "custom-host",
		"DBPort":       "5544",
		"DBUser":       "custom_user",
		"DBPassword":   "custom_pass",
		"DBName":       "custom_db",
		"DBSSLMode":    "require",
		"RedisAddr":    "custom-redis:6380",
		"RedisPass":    "redis-pass",
		"JWTSecret":    "custom-jwt-secret",
		"BaseProxyURL": "http://127.0.0.1:9999",
	}
	got := map[string]string{
		"ServerPort":   cfg.ServerPort,
		"DBHost":       cfg.DBHost,
		"DBPort":       cfg.DBPort,
		"DBUser":       cfg.DBUser,
		"DBPassword":   cfg.DBPassword,
		"DBName":       cfg.DBName,
		"DBSSLMode":    cfg.DBSSLMode,
		"RedisAddr":    cfg.RedisAddr,
		"RedisPass":    cfg.RedisPass,
		"JWTSecret":    cfg.JWTSecret,
		"BaseProxyURL": cfg.BaseProxyURL,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("field %s: expected %q, got %q", k, w, got[k])
		}
	}
}
