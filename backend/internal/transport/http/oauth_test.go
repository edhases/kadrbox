package http_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

// generateValidTelegramHash рахує валідний HMAC-SHA256 хеш за специфікацією Telegram
func generateValidTelegramHash(botToken string, id int64, firstName, lastName, username, photoURL string, authDate int64) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("auth_date=%d", authDate))
	if firstName != "" {
		parts = append(parts, fmt.Sprintf("first_name=%s", firstName))
	}
	parts = append(parts, fmt.Sprintf("id=%d", id))
	if lastName != "" {
		parts = append(parts, fmt.Sprintf("last_name=%s", lastName))
	}
	if photoURL != "" {
		parts = append(parts, fmt.Sprintf("photo_url=%s", photoURL))
	}
	if username != "" {
		parts = append(parts, fmt.Sprintf("username=%s", username))
	}
	sort.Strings(parts)
	dataCheckString := strings.Join(parts, "\n")

	sha := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, sha[:])
	mac.Write([]byte(dataCheckString))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestTelegramAuthValidation(t *testing.T) {
	botToken := "123456789:ABCdefGhIjkLmNoPqRsTuVwXyZ"
	h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "")
	h.SetOAuth(botToken, "testbot", "discord-id", "discord-sec", "https://redirect.com", "https://app.com")

	now := time.Now().Unix()
	validHash := generateValidTelegramHash(botToken, 987654321, "Taras", "Shevchenko", "kobzar", "https://t.me/photo.jpg", now)

	t.Run("invalid json payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/telegram", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		h.TelegramAuth(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("invalid hash rejected", func(t *testing.T) {
		body := fmt.Sprintf(`{"id":987654321,"first_name":"Taras","username":"kobzar","auth_date":%d,"hash":"wrong_hash"}`, now)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/telegram", strings.NewReader(body))
		rr := httptest.NewRecorder()
		h.TelegramAuth(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for invalid hash, got %d", rr.Code)
		}
	})

	t.Run("expired auth_date rejected", func(t *testing.T) {
		oldDate := now - 90000 // > 24 hours
		oldHash := generateValidTelegramHash(botToken, 987654321, "Taras", "", "kobzar", "", oldDate)
		body := fmt.Sprintf(`{"id":987654321,"first_name":"Taras","username":"kobzar","auth_date":%d,"hash":%q}`, oldDate, oldHash)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/telegram", strings.NewReader(body))
		rr := httptest.NewRecorder()
		h.TelegramAuth(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized for expired date, got %d", rr.Code)
		}
	})

	t.Run("valid hash signature recognized", func(t *testing.T) {
		// Valid hash passes verifyTelegramAuth; then will fail on nil userRepo (500)
		body := fmt.Sprintf(`{"id":987654321,"first_name":"Taras","last_name":"Shevchenko","username":"kobzar","photo_url":"https://t.me/photo.jpg","auth_date":%d,"hash":%q}`, now, validHash)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/telegram", strings.NewReader(body))
		rr := httptest.NewRecorder()
		defer func() {
			_ = recover() // nil userRepo panic is expected if signature was accepted!
		}()
		h.TelegramAuth(rr, req)
		// If signature was rejected, it would return 401 before touching userRepo!
		if rr.Code == http.StatusUnauthorized {
			t.Errorf("valid signature should not return 401")
		}
	})
}

func TestTelegramLoginWeb(t *testing.T) {
	h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "")
	h.SetOAuth("token", "oxidefilmbot", "", "", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/telegram/login", nil)
	rr := httptest.NewRecorder()
	h.TelegramLoginWeb(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "data-telegram-login=\"oxidefilmbot\"") {
		t.Errorf("expected telegram widget with bot username, got: %s", body)
	}
	if !strings.Contains(body, "telegram-widget.js") {
		t.Errorf("expected telegram widget script")
	}
}

func TestDiscordLoginRedirect(t *testing.T) {
	t.Run("redirect when configured", func(t *testing.T) {
		h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "")
		h.SetOAuth("", "", "my-client-id", "my-secret", "https://film.oxideteam.pp.ua/api/v1/auth/discord/callback", "")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login", nil)
		rr := httptest.NewRecorder()
		h.DiscordLogin(rr, req)

		if rr.Code != http.StatusTemporaryRedirect {
			t.Fatalf("expected 307 redirect, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "discord.com/api/oauth2/authorize") {
			t.Errorf("unexpected redirect location: %s", loc)
		}
		if !strings.Contains(loc, "client_id=my-client-id") {
			t.Errorf("client_id missing from redirect URL: %s", loc)
		}
		if !strings.Contains(loc, "scope=identify+email") && !strings.Contains(loc, "scope=identify%20email") {
			t.Errorf("scopes missing from redirect URL: %s", loc)
		}
	})

	t.Run("error when client_id is not configured", func(t *testing.T) {
		h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login", nil)
		rr := httptest.NewRecorder()
		h.DiscordLogin(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", rr.Code)
		}
	})
}

func TestDiscordCallbackErrorHandling(t *testing.T) {
	h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "")
	h.SetOAuth("", "", "my-client-id", "my-secret", "https://redirect.com", "")

	t.Run("missing code", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/callback", nil)
		rr := httptest.NewRecorder()
		h.DiscordCallback(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 when code is missing, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Помилка Discord") {
			t.Errorf("expected error page in response, got: %s", rr.Body.String())
		}
	})
}

func TestDiscordAuthAPIErrors(t *testing.T) {
	h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "")

	t.Run("invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/discord", strings.NewReader("{bad"))
		rr := httptest.NewRecorder()
		h.DiscordAuthAPI(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("empty code", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/discord", strings.NewReader(`{"code":""}`))
		rr := httptest.NewRecorder()
		h.DiscordAuthAPI(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})
}

func TestGoogleOAuth(t *testing.T) {
	t.Run("error when client_id is not configured", func(t *testing.T) {
		h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/login", nil)
		rr := httptest.NewRecorder()
		h.GoogleLogin(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", rr.Code)
		}
	})

	t.Run("redirects to google auth when configured", func(t *testing.T) {
		h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "google-client-id")
		h.SetGoogleOAuth("google-client-id", "google-secret", "https://film.oxideteam.pp.ua/api/v1/auth/google/callback")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/login?redirect_to=http://127.0.0.1:9999/callback", nil)
		rr := httptest.NewRecorder()
		h.GoogleLogin(rr, req)

		if rr.Code != http.StatusTemporaryRedirect {
			t.Errorf("expected 307, got %d", rr.Code)
		}

		loc := rr.Header().Get("Location")
		if !strings.HasPrefix(loc, "https://accounts.google.com/o/oauth2/v2/auth") {
			t.Errorf("expected redirect to accounts.google.com, got: %s", loc)
		}
		if !strings.Contains(loc, "client_id=google-client-id") {
			t.Errorf("client_id missing from redirect URL: %s", loc)
		}
	})

	t.Run("callback handles missing code", func(t *testing.T) {
		h := transporthttp.NewAuthHandler(nil, nil, nil, "jwtsecret", "google-client-id")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback", nil)
		rr := httptest.NewRecorder()
		h.GoogleCallback(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400 when code is missing, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Помилка Google") {
			t.Errorf("expected error page in response, got: %s", rr.Body.String())
		}
	})
}
