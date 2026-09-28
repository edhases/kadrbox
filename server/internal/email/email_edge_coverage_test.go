package email

import (
	"net/http"
	"strings"
	"testing"
)

func TestCovEmailSendNonJSONErrorBody(t *testing.T) {
	t.Setenv("RESEND_API", "re_test_key")
	s := NewService()
	s.client = &http.Client{Transport: covRoundTripper{
		fn: func(r *http.Request) (*http.Response, error) {
			// Тіло не JSON: Decode мовчки лишає nil, статус все одно в помилці.
			return covHTTPResp(http.StatusInternalServerError, `<html>oops</html>`), nil
		},
	}}

	err := s.SendPasswordResetEmail("u@example.com", "Олена", "tok")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected API error with status 500, got %v", err)
	}
}
