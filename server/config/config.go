package config

import (
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	ServerPort   string
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	DBSSLMode    string
	RedisAddr    string
	RedisPass    string
	JWTSecret           string
	BaseProxyURL        string
	AppURL              string
	GoogleClientID      string
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
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		DBHost:         getEnv("DB_HOST", "postgres"),
		DBPort:         getEnv("DB_PORT", "5432"),
		DBUser:         getEnv("DB_USER", "postgres"),
		DBPassword:     getEnv("DB_PASSWORD", "postgres"),
		DBName:         getEnv("DB_NAME", "oxide_film"),
		DBSSLMode:      getEnv("DB_SSLMODE", "disable"),
		RedisAddr:      getEnv("REDIS_ADDR", "redis:6379"),
		RedisPass:      getEnv("REDIS_PASSWORD", ""),
		JWTSecret:           getEnv("JWT_SECRET", "super-secret-jwt-key-oxide-film-2026"),
		BaseProxyURL:        getEnv("BASE_PROXY_URL", "http://127.0.0.1:8089"),
		AppURL:              getEnv("APP_URL", "https://film.oxideteam.pp.ua"),
		GoogleClientID:      getEnv("GOOGLE_CLIENT_ID", ""),
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

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}
