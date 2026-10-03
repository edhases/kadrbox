package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	// MinJWTSecretLen — мінімальна довжина JWT_SIGNING ключа в байтах.
	// Рекомендована генерація: openssl rand -base64 48  (64 символи).
	MinJWTSecretLen = 32
	// MinDBPasswordLen — мінімальна довжина пароля PostgreSQL.
	MinDBPasswordLen = 16
)

type Config struct {
	ServerPort          string
	DBHost              string
	DBPort              string
	DBUser              string
	DBPassword          string
	DBName              string
	DBSSLMode           string
	RedisAddr           string
	RedisPass           string
	JWTSecret           string
	LogLevel            string
	LogFormat           string
	BaseProxyURL        string
	AppURL              string
	DisabledProviders   string
	GoogleClientID      string
	GoogleClientSecret  string
	GoogleRedirectURI   string
	TelegramBotToken    string
	TelegramBotUsername string
	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURI  string
}

func (c *Config) PostgresDSN() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.DBUser, c.DBPassword),
		Host:   c.DBHost + ":" + c.DBPort,
		Path:   c.DBName,
	}
	q := u.Query()
	q.Set("sslmode", c.DBSSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func Load() *Config {
	return &Config{
		ServerPort:          getEnv("SERVER_PORT", "8080"),
		DBHost:              getEnv("DB_HOST", "postgres"),
		DBPort:              getEnv("DB_PORT", "5432"),
		DBUser:              getEnv("DB_USER", "postgres"),
		DBPassword:          getEnv("DB_PASSWORD", "postgres"),
		DBName:              getEnv("DB_NAME", "oxide_film"),
		DBSSLMode:           getEnv("DB_SSLMODE", "disable"),
		RedisAddr:           getEnv("REDIS_ADDR", "redis:6379"),
		RedisPass:           os.Getenv("REDIS_PASSWORD"),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		LogFormat:           getEnv("LOG_FORMAT", "text"),
		BaseProxyURL:        getEnv("BASE_PROXY_URL", "http://127.0.0.1:8089"),
		AppURL:              getEnv("APP_URL", "https://film.oxideteam.pp.ua"),
		DisabledProviders:   getEnv("DISABLED_PROVIDERS", ""),
		GoogleClientID:      getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:  getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:   getEnv("GOOGLE_REDIRECT_URI", "https://film.oxideteam.pp.ua/api/v1/auth/google/callback"),
		TelegramBotToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramBotUsername: getEnv("TELEGRAM_BOT_USERNAME", "oxidefilmbot"),
		DiscordClientID:     getEnv("DISCORD_CLIENT_ID", ""),
		DiscordClientSecret: getEnv("DISCORD_CLIENT_SECRET", ""),
		DiscordRedirectURI:  getEnv("DISCORD_REDIRECT_URI", "https://film.oxideteam.pp.ua/api/v1/auth/discord/callback"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// Validate виконує fail-closed перевірку конфігурації.
// Сервер НЕ МУСИТЬ стартувати з порожнім або коротким JWT_SECRET:
// інакше HMAC-ключ стає відомим і будь-який може підробити access-токен.
func (c *Config) Validate() error {
	var errs []string

	if c.JWTSecret == "" {
		errs = append(errs, "JWT_SECRET is required (env / Portainer stack variable); generate with: openssl rand -base64 48")
	} else if n := len(c.JWTSecret); n < MinJWTSecretLen {
		errs = append(errs, fmt.Sprintf("JWT_SECRET must be >= %d bytes, got %d", MinJWTSecretLen, n))
	} else if isPlaceholderSecret(c.JWTSecret) {
		errs = append(errs, "JWT_SECRET looks like a known placeholder/default; generate a unique value with: openssl rand -base64 48")
	}

	if c.ServerPort == "" {
		errs = append(errs, "SERVER_PORT is required")
	}
	if c.DBHost == "" {
		errs = append(errs, "DB_HOST is required")
	}
	if c.DBUser == "" {
		errs = append(errs, "DB_USER is required")
	}
	if c.DBPassword == "" {
		errs = append(errs, "DB_PASSWORD is required")
	} else if len(c.DBPassword) < MinDBPasswordLen {
		errs = append(errs, fmt.Sprintf("DB_PASSWORD must be >= %d bytes, got %d", MinDBPasswordLen, len(c.DBPassword)))
	}
	if c.RedisPass == "" {
		errs = append(errs, "REDIS_PASSWORD is required (Redis holds refresh tokens; an open Redis leaks all sessions)")
	}
	if c.AppURL == "" {
		errs = append(errs, "APP_URL is required (used for OAuth redirect URIs)")
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// isPlaceholderSecret виявляє значення-заглушки, які могли залишитися
// у старих docker-compose файлах або прикладах.
func isPlaceholderSecret(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	switch l {
	case "super-secret-jwt-key-2026", "secret", "changeme", "jwt-secret", "test":
		return true
	}
	return strings.Contains(l, "super-secret")
}

// GetDisabledProviders парсить DISABLED_PROVIDERS (напр. "uakino, lavakino") у список ID.
func (c *Config) GetDisabledProviders() []string {
	var out []string
	for _, id := range strings.Split(c.DisabledProviders, ",") {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}
