package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const resendAPI = "https://api.resend.com/emails"

// Service відправляє листи через Resend.com API
type Service struct {
	apiKey string
	from   string // напр. "Oxide Film <noreply@oxideteam.pp.ua>"
	appURL string
	client *http.Client
}

// NewService читає конфіг з env:
//
//	RESEND_API   — API ключ з resend.com (обов'язково)
//	SMTP_FROM    — адреса відправника, напр. "Oxide Film <noreply@oxideteam.pp.ua>"
//	APP_URL      — базова URL застосунку для посилань у листах
func NewService() *Service {
	from := getEnv("SMTP_FROM", "Oxide Film <noreply@oxideteam.pp.ua>")
	appURL := getEnv("APP_URL", "https://film.oxideteam.pp.ua")
	return &Service{
		apiKey: getEnv("RESEND_API", ""),
		from:   from,
		appURL: appURL,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// IsConfigured перевіряє наявність API ключа
func (s *Service) IsConfigured() bool {
	return s.apiKey != ""
}

// SendVerificationEmail надсилає лист підтвердження реєстрації
func (s *Service) SendVerificationEmail(toEmail, username, token string) error {
	verifyURL := fmt.Sprintf("%s/verify-email?token=%s", s.appURL, token)

	subject := "Підтвердження реєстрації — Oxide Film"
	text := fmt.Sprintf(`Привіт, %s!

Дякуємо за реєстрацію в Oxide Film.
Щоб підтвердити вашу електронну адресу, натисніть посилання:

%s

Посилання дійсне 24 години.

Якщо ви не реєструвалися — просто проігноруйте цей лист.

З повагою,
Команда Oxide Film`, username, verifyURL)

	html := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:480px;margin:auto;padding:32px">
  <h2 style="color:#6366f1">🎬 Oxide Film</h2>
  <p>Привіт, <strong>%s</strong>!</p>
  <p>Дякуємо за реєстрацію. Натисніть кнопку, щоб підтвердити email:</p>
  <a href="%s" style="display:inline-block;margin:16px 0;padding:12px 24px;background:#6366f1;color:#fff;text-decoration:none;border-radius:8px;font-weight:bold">
    Підтвердити email
  </a>
  <p style="color:#888;font-size:13px">Або скопіюйте посилання: %s</p>
  <p style="color:#888;font-size:13px">Посилання дійсне 24 години.</p>
</div>`, username, verifyURL, verifyURL)

	return s.send(toEmail, subject, text, html)
}

// SendPasswordResetEmail надсилає лист скидання пароля
func (s *Service) SendPasswordResetEmail(toEmail, username, token string) error {
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.appURL, token)

	subject := "Скидання пароля — Oxide Film"
	text := fmt.Sprintf(`Привіт, %s!

Ми отримали запит на скидання пароля для вашого акаунту Oxide Film.

%s

Посилання дійсне 1 годину. Якщо ви не запитували — проігноруйте цей лист.

З повагою,
Команда Oxide Film`, username, resetURL)

	html := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:480px;margin:auto;padding:32px">
  <h2 style="color:#6366f1">🎬 Oxide Film</h2>
  <p>Привіт, <strong>%s</strong>!</p>
  <p>Ми отримали запит на скидання пароля. Натисніть кнопку:</p>
  <a href="%s" style="display:inline-block;margin:16px 0;padding:12px 24px;background:#6366f1;color:#fff;text-decoration:none;border-radius:8px;font-weight:bold">
    Скинути пароль
  </a>
  <p style="color:#888;font-size:13px">Посилання дійсне 1 годину.</p>
</div>`, username, resetURL)

	return s.send(toEmail, subject, text, html)
}

// ---- internal ---------------------------------------------------------------

type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

func (s *Service) send(to, subject, text, html string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("email: RESEND_API env not set")
	}

	payload := resendPayload{
		From:    s.from,
		To:      []string{to},
		Subject: subject,
		Text:    text,
		HTML:    html,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("email: marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, resendAPI, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("email: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("email: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("email: resend API error %d: %v", resp.StatusCode, errBody)
	}

	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
