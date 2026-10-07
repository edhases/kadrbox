package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The global response headers.
//
// They were absent everywhere, which the audit found. The per-page helper in
// transport/http already covered the token pages, so these matter for everything
// else -- the JSON API, the uploads, the WebSocket rejections -- where a browser
// does read the headers and nothing was there to read.
func TestSecurityHeadersAppliedToEveryResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

	for name, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Content-Security-Policy":   "frame-ancestors 'none'",
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestSecurityHeadersDoNotTouchTheBody(t *testing.T) {
	// The middleware must be transparent: it sets headers and nothing else.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"a@example.com"}`))

	SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if got := rec.Body.String(); got != `{"ok":true}` {
		t.Errorf("body = %q, want the handler's", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want the handler's", got)
	}
}

func TestSecurityHeadersRunBeforeTheHandlerOverwritesACSP(t *testing.T) {
	// A page that needs its own policy replaces the header, which is why
	// frame-ancestors has to be repeated in the page policy rather than relied on
	// from here. This pins that behaviour so the repetition is not mistaken for
	// redundancy.
	rec := httptest.NewRecorder()
	SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'none'; frame-ancestors 'none'")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reset-password", nil))

	got := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(got, "frame-ancestors 'none'") {
		t.Errorf("a page policy without frame-ancestors drops the framing defence: %q", got)
	}
}

func TestSecurityHeadersApplyToErrorResponsesToo(t *testing.T) {
	// The paths that matter most are the ones that fail: an error body is exactly
	// where a browser decides how to interpret a content type.
	for _, code := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
	} {
		rec := httptest.NewRecorder()
		SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/whatever", nil))

		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("status %d did not carry nosniff", code)
		}
		if rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("status %d did not carry X-Frame-Options", code)
		}
	}
}
