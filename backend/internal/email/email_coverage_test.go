package email

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// covRoundTripper підміняє мережу: перехоплює запит до Resend API
// і повертає запрограмовану відповідь без виходу в інтернет.
type covRoundTripper struct {
	fn func(*http.Request) (*http.Response, error)
}

func (c covRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return c.fn(r)
}

func covHTTPResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestCovEmailNewServiceDefaults(t *testing.T) {
	t.Setenv("RESEND_API", "")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("APP_URL", "")

	s := NewService()
	if s.IsConfigured() {
		t.Error("expected unconfigured service without RESEND_API")
	}
	if s.from != "Oxide Film <noreply@oxideteam.pp.ua>" {
		t.Errorf("unexpected default from: %q", s.from)
	}
	if s.appURL != "https://film.oxideteam.pp.ua" {
		t.Errorf("unexpected default appURL: %q", s.appURL)
	}
	if s.client == nil {
		t.Error("expected non-nil http client")
	}
}

func TestCovEmailNewServiceCustomEnv(t *testing.T) {
	t.Setenv("RESEND_API", "re_test_key")
	t.Setenv("SMTP_FROM", "Test <t@example.com>")
	t.Setenv("APP_URL", "https://app.example")

	s := NewService()
	if !s.IsConfigured() {
		t.Error("expected configured service with RESEND_API set")
	}
	if s.from != "Test <t@example.com>" {
		t.Errorf("unexpected from: %q", s.from)
	}
	if s.appURL != "https://app.example" {
		t.Errorf("unexpected appURL: %q", s.appURL)
	}
}

func TestCovEmailSendUnconfigured(t *testing.T) {
	t.Setenv("RESEND_API", "")
	s := NewService()

	// Жодного виходу в мережу: send повертається до HTTP.
	// Заодно покриває побудову text/html шаблонів обох листів.
	if err := s.SendVerificationEmail("u@example.com", "Іван", "tok123"); err == nil ||
		!strings.Contains(err.Error(), "RESEND_API") {
		t.Errorf("expected RESEND_API error, got %v", err)
	}
	if err := s.SendPasswordResetEmail("u@example.com", "Іван", "tok456"); err == nil ||
		!strings.Contains(err.Error(), "RESEND_API") {
		t.Errorf("expected RESEND_API error, got %v", err)
	}
}

func TestCovEmailSendVerificationSuccess(t *testing.T) {
	t.Setenv("RESEND_API", "re_test_key")
	s := NewService()
	s.appURL = "https://app.example"

	var gotReq *http.Request
	var gotPayload resendPayload
	s.client = &http.Client{Transport: covRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			gotReq = r
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &gotPayload); err != nil {
				t.Errorf("payload is not valid JSON: %v", err)
			}
			return covHTTPResp(http.StatusOK, `{"id":"msg_1"}`), nil
		},
	}}

	if err := s.SendVerificationEmail("u@example.com", "Іван", "tok123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotReq.URL.String() != resendAPI {
		t.Errorf("expected POST to %s, got %s", resendAPI, gotReq.URL.String())
	}
	if got := gotReq.Header.Get("Authorization"); got != "Bearer re_test_key" {
		t.Errorf("unexpected Authorization header: %q", got)
	}
	if got := gotReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("unexpected Content-Type: %q", got)
	}
	if len(gotPayload.To) != 1 || gotPayload.To[0] != "u@example.com" {
		t.Errorf("unexpected To: %v", gotPayload.To)
	}
	if !strings.Contains(gotPayload.Subject, "Підтвердження") {
		t.Errorf("unexpected subject: %q", gotPayload.Subject)
	}
	wantURL := "https://app.example/verify-email?token=tok123"
	if !strings.Contains(gotPayload.Text, "Іван") || !strings.Contains(gotPayload.Text, wantURL) {
		t.Errorf("text body misses username or verify URL: %q", gotPayload.Text)
	}
	if !strings.Contains(gotPayload.HTML, "Іван") || !strings.Contains(gotPayload.HTML, wantURL) {
		t.Errorf("html body misses username or verify URL")
	}
}

func TestCovEmailSendPasswordResetSuccess(t *testing.T) {
	t.Setenv("RESEND_API", "re_test_key")
	s := NewService()
	s.appURL = "https://app.example"

	var gotPayload resendPayload
	s.client = &http.Client{Transport: covRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotPayload)
			return covHTTPResp(http.StatusOK, `{"id":"msg_2"}`), nil
		},
	}}

	if err := s.SendPasswordResetEmail("u@example.com", "Олена", "tok999"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantURL := "https://app.example/reset-password?token=tok999"
	if !strings.Contains(gotPayload.Subject, "Скидання") {
		t.Errorf("unexpected subject: %q", gotPayload.Subject)
	}
	if !strings.Contains(gotPayload.Text, wantURL) || !strings.Contains(gotPayload.HTML, wantURL) {
		t.Errorf("bodies miss reset URL")
	}
}

func TestCovEmailSendAPIError(t *testing.T) {
	t.Setenv("RESEND_API", "re_bad_key")
	s := NewService()
	s.client = &http.Client{Transport: covRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			return covHTTPResp(http.StatusUnauthorized, `{"message":"invalid api key"}`), nil
		},
	}}

	err := s.SendVerificationEmail("u@example.com", "Іван", "tok")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected API error with status 401, got %v", err)
	}
}

func TestCovEmailSendTransportError(t *testing.T) {
	t.Setenv("RESEND_API", "re_test_key")
	s := NewService()
	s.client = &http.Client{Transport: covRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		},
	}}

	err := s.SendVerificationEmail("u@example.com", "Іван", "tok")
	if err == nil || !strings.Contains(err.Error(), "send request") {
		t.Errorf("expected transport error, got %v", err)
	}
}
