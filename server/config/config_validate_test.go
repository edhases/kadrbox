package config

import (
	"strings"
	"testing"
)

// validBase повертає мінімально коректну конфігурацію,
// щоб кожен кейс міняв лише одне поле.
func validBase() *Config {
	return &Config{
		ServerPort: "8080",
		DBHost:     "postgres",
		DBUser:     "oxide",
		DBPassword: "a-strong-postgres-password",
		RedisPass:  "a-strong-redis-password",
		JWTSecret:  strings.Repeat("k", 64),
		AppURL:     "https://film.oxideteam.pp.ua",
	}
}

func TestValidate_AcceptsCorrectConfig(t *testing.T) {
	if err := validBase().Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestValidate_RejectsEmptyJWTSecret(t *testing.T) {
	c := validBase()
	c.JWTSecret = ""

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for empty JWT_SECRET")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET is required") {
		t.Fatalf("error should mention JWT_SECRET is required, got: %v", err)
	}
}

func TestValidate_RejectsShortJWTSecret(t *testing.T) {
	for _, n := range []int{1, 8, 16, 31} {
		c := validBase()
		c.JWTSecret = strings.Repeat("k", n)

		err := c.Validate()
		if err == nil {
			t.Fatalf("expected error for JWT_SECRET of %d bytes", n)
		}
		if !strings.Contains(err.Error(), "JWT_SECRET must be >=") {
			t.Fatalf("expected length error for %d bytes, got: %v", n, err)
		}
	}
}

func TestValidate_AcceptsExactlyMinLengthJWTSecret(t *testing.T) {
	c := validBase()
	c.JWTSecret = strings.Repeat("k", MinJWTSecretLen)

	if err := c.Validate(); err != nil {
		t.Fatalf("expected %d bytes to be accepted, got: %v", MinJWTSecretLen, err)
	}
}

func TestValidate_RejectsPlaceholderJWTSecret(t *testing.T) {
	// Значення-заглушка з попереднього docker-compose — не має проходити.
	placeholder := "super-secret-jwt-key-2026"
	c := validBase()
	c.JWTSecret = placeholder + strings.Repeat("x", 64-len(placeholder))

	err := c.Validate()
	if err == nil {
		t.Fatal("expected placeholder JWT_SECRET to be rejected")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("expected placeholder error, got: %v", err)
	}
}

func TestValidate_RejectsMissingDBAndRedisCredentials(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"empty DB_PASSWORD", func(c *Config) { c.DBPassword = "" }, "DB_PASSWORD is required"},
		{"short DB_PASSWORD", func(c *Config) { c.DBPassword = "short" }, "DB_PASSWORD must be >="},
		{"empty REDIS_PASSWORD", func(c *Config) { c.RedisPass = "" }, "REDIS_PASSWORD is required"},
		{"empty SERVER_PORT", func(c *Config) { c.ServerPort = "" }, "SERVER_PORT is required"},
		{"empty DB_HOST", func(c *Config) { c.DBHost = "" }, "DB_HOST is required"},
		{"empty DB_USER", func(c *Config) { c.DBUser = "" }, "DB_USER is required"},
		{"empty APP_URL", func(c *Config) { c.AppURL = "" }, "APP_URL is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validBase()
			tt.mutate(c)

			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q in error, got: %v", tt.want, err)
			}
		})
	}
}

func TestValidate_ReportsAllProblemsAtOnce(t *testing.T) {
	// Неправильна конфігурація повинна падати одразу з повним списком,
	// а не по одному полю за запуск.
	c := &Config{}

	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for completely empty config")
	}

	for _, want := range []string{
		"JWT_SECRET is required",
		"SERVER_PORT is required",
		"DB_HOST is required",
		"DB_USER is required",
		"DB_PASSWORD is required",
		"REDIS_PASSWORD is required",
		"APP_URL is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in aggregated error, got: %v", want, err)
		}
	}
}

func TestLoad_JWTSecretHasNoFallback(t *testing.T) {
	// Ключовий регресійний тест: JWT_SECRET НЕ МУСИТЬ мати дефолт.
	// Раніше getEnv("JWT_SECRET", "") маскував відсутність змінної.
	t.Setenv("JWT_SECRET", "")

	if got := Load().JWTSecret; got != "" {
		t.Fatalf("Load() must not substitute a JWT_SECRET default, got %q", got)
	}

	if err := Load().Validate(); err == nil {
		t.Fatal("Load() with no JWT_SECRET must fail Validate()")
	}
}

func TestLoad_JWTSecretReadFromEnvironment(t *testing.T) {
	secret := strings.Repeat("S", 64)
	t.Setenv("JWT_SECRET", secret)

	if got := Load().JWTSecret; got != secret {
		t.Fatalf("expected JWT_SECRET from env, got %q", got)
	}
}

func TestLoad_RedisPassHasNoFallback(t *testing.T) {
	// Redis без пароля = витік усіх refresh-токенів.
	t.Setenv("REDIS_PASSWORD", "")

	if got := Load().RedisPass; got != "" {
		t.Fatalf("Load() must not substitute a REDIS_PASSWORD default, got %q", got)
	}
}
