package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The Content-Security-Policy on the HTML pages.
//
// The bug this pins is a self-inflicted one: the Telegram widget page loads a
// script from telegram.org, and it was being served the strict policy whose
// script-src is 'unsafe-inline' with no external origin in it. Our own policy
// blocked the widget, the button never rendered, and the page's own timeout
// script then displayed "the Telegram button did not appear" -- which reads like
// a domain-not-registered problem and sends the user to /setdomain.
//
// So the policy for that page has to allow exactly telegram.org, and every other
// page has to stay locked down.
func TestStrictCSPAllowsNoExternalOrigin(t *testing.T) {
	if strings.Contains(strictCSP, "https://") {
		t.Errorf("the default page policy names an external origin: %s", strictCSP)
	}
	for _, directive := range []string{"default-src 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(strictCSP, directive) {
			t.Errorf("the default page policy is missing %q: %s", directive, strictCSP)
		}
	}
}

func TestTelegramWidgetCSPAllowsTelegramAndNothingElse(t *testing.T) {
	if !strings.Contains(telegramWidgetCSP, "script-src 'unsafe-inline' https://telegram.org") {
		t.Errorf("the widget policy still blocks the widget script: %s", telegramWidgetCSP)
	}
	// The widget injects an iframe from telegram.org, so frame-src has to name it
	// too or the button renders but never completes.
	if !strings.Contains(telegramWidgetCSP, "frame-src https://telegram.org") {
		t.Errorf("the widget policy blocks the iframe: %s", telegramWidgetCSP)
	}
	// Still no images, fonts, connections or forms from anywhere.
	if !strings.Contains(telegramWidgetCSP, "default-src 'none'") {
		t.Errorf("the widget policy is not closed by default: %s", telegramWidgetCSP)
	}
	if !strings.Contains(telegramWidgetCSP, "frame-ancestors 'none'") {
		t.Errorf("the widget policy allows framing: %s", telegramWidgetCSP)
	}

	// Exactly one external origin, spelled out. Anything else would be a hole.
	if got := strings.Count(telegramWidgetCSP, "https://"); got != 2 {
		t.Errorf("the widget policy names %d https origins, want 2 (script and frame): %s",
			got, telegramWidgetCSP)
	}
}

func TestWriteHTMLStatusWithCSPReplacesThePolicyNotTheCacheHeaders(t *testing.T) {
	// The cache headers must survive a page-specific policy: a page holding a
	// token is never cacheable, whatever it is allowed to load.
	rec := httptest.NewRecorder()
	writeHTMLStatusWithCSP(rec, http.StatusOK, "<html></html>", telegramWidgetCSP)

	if got := rec.Header().Get("Content-Security-Policy"); got != telegramWidgetCSP {
		t.Errorf("CSP = %q, want the page policy", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store to survive", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestWriteHTMLStatusUsesTheStrictPolicy(t *testing.T) {
	rec := httptest.NewRecorder()
	writeHTMLStatus(rec, http.StatusBadRequest, "<html></html>")

	if got := rec.Header().Get("Content-Security-Policy"); got != strictCSP {
		t.Errorf("CSP = %q, want the strict default", got)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// cspDirective extracts one directive's value list from a policy string. Lives
// here for the same-package tests; oauth_test.go is in the external test package
// and has its own.
func cspDirective(policy, name string) (string, bool) {
	for _, part := range strings.Split(policy, ";") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) > 0 && fields[0] == name {
			return strings.Join(fields[1:], " "), true
		}
	}
	return "", false
}

// Token-in-URL pages must never be framed and never leak the URL onward. Both
// clauses are in the strict policy, and the audit claimed they were missing from
// /reset-password; they were present, applied through writeHTMLStatus.
func TestTokenPagesAreUncacheableAndUnframable(t *testing.T) {
	for _, name := range []string{"reset-password", "verify-email"} {
		rec := httptest.NewRecorder()
		writeHTMLStatus(rec, http.StatusOK, "<html></html>")

		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s can be framed: %s", name, csp)
		}
		if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
			t.Errorf("%s is cacheable", name)
		}
		if rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s can leak its URL through Referer", name)
		}
	}
}
