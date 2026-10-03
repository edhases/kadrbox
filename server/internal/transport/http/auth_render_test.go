package http

// The OAuth/email status pages are served as text/html from the production API
// origin and interpolate provider-supplied text. These tests pin the escaping.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const xssPayload = `<img src=x onerror="alert(document.cookie)">`

// assertNoRawPayload fails when the payload survived into the output as markup.
func assertNoRawPayload(t *testing.T, body, where string) {
	t.Helper()
	if strings.Contains(body, "<img") {
		t.Errorf("%s: raw markup survived escaping:\n%s", where, body)
	}
	if strings.Contains(body, `onerror="alert`) {
		t.Errorf("%s: raw event handler survived escaping:\n%s", where, body)
	}
}

// assertEscapedMarkup additionally requires the escaped form to be present.
func assertEscapedMarkup(t *testing.T, body, where string) {
	t.Helper()
	assertNoRawPayload(t, body, where)
	if !strings.Contains(body, "&lt;img") && !strings.Contains(body, `\u003cimg`) {
		t.Errorf("%s: expected the payload to appear escaped, got:\n%s", where, body)
	}
}

func TestRenderOAuthStatusHTMLEscapesTitleAndMessage(t *testing.T) {
	body := renderOAuthStatusHTML(false, xssPayload, xssPayload, "", "", "")
	assertEscapedMarkup(t, body, "oauth status page")
}

func TestRenderEmailStatusHTMLEscapesTitleAndMessage(t *testing.T) {
	body := renderEmailStatusHTML(false, xssPayload, xssPayload)
	assertEscapedMarkup(t, body, "email status page")
}

// sanitizeProviderMessage must strip control characters and cap the length, so
// a hostile provider cannot use the status page as arbitrary content.
func TestSanitizeProviderMessage(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		want  string
		check func(t *testing.T, got string)
	}{
		{
			name: "control characters become spaces",
			in:   "line one\nline\ttwo\r\nthree",
			want: "line one line two three",
		},
		{
			name: "NUL is removed",
			in:   "a\x00b",
			want: "a b",
		},
		{
			name: "length is capped",
			in:   strings.Repeat("x", 5000),
			check: func(t *testing.T, got string) {
				if len([]rune(got)) > maxProviderMessageLen+3 {
					t.Errorf("message length = %d runes, want <= %d", len([]rune(got)), maxProviderMessageLen+1)
				}
				if !strings.HasSuffix(got, "...") {
					t.Errorf("a truncated message should be marked: %q", got)
				}
			},
		},
		{
			name: "markup is kept but not executed",
			in:   xssPayload,
			want: xssPayload,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeProviderMessage(tc.in)
			if tc.want != "" && got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// End to end: a provider error carrying an XSS payload must reach the browser
// escaped, whichever branch it lands in.
func TestErrorDescriptionFromProviderIsEscaped(t *testing.T) {
	h := newStateHandler(t)

	seedState := func(record oauthStateRecord) string {
		t.Helper()
		record.CreatedAt = time.Now().Unix()
		payload, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		state, err := newOAuthStateNonce()
		if err != nil {
			t.Fatalf("nonce: %v", err)
		}
		if err := h.stateStore.SetOAuthState(context.Background(), state, payload, oauthStateTTL); err != nil {
			t.Fatalf("SetOAuthState: %v", err)
		}
		return state
	}

	t.Run("rendered page when there is no destination", func(t *testing.T) {
		state := seedState(oauthStateRecord{Provider: "google"})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/auth/google/callback?state="+url.QueryEscape(state)+
				"&error=access_denied&error_description="+url.QueryEscape(xssPayload), nil)

		h.GoogleCallback(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("Content-Type = %q, want text/html", ct)
		}
		assertEscapedMarkup(t, rr.Body.String(), "callback page")
	})

	t.Run("redirect branch encodes the payload", func(t *testing.T) {
		state := seedState(oauthStateRecord{Provider: "google", RedirectTo: "http://127.0.0.1:53123/callback"})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/auth/google/callback?state="+url.QueryEscape(state)+
				"&error=access_denied&error_description="+url.QueryEscape(xssPayload), nil)

		h.GoogleCallback(rr, req)

		location := rr.Header().Get("Location")
		if location == "" {
			t.Fatal("expected a redirect to the validated destination")
		}
		if strings.Contains(location, "<img") {
			t.Errorf("raw markup in the redirect target: %s", location)
		}
		parsed, err := url.Parse(location)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if parsed.Query().Get("error") == "" {
			t.Errorf("no error parameter reached the client: %s", location)
		}
		if parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.Port() != "53123" {
			t.Errorf("redirect left the validated origin: %s", location)
		}
		// The response leads to a token URL, so it must not be cached or
		// referenced.
		if cc := rr.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("Cache-Control = %q, want it to include no-store", cc)
		}
		if rp := rr.Header().Get("Referrer-Policy"); rp != "no-referrer" {
			t.Errorf("Referrer-Policy = %q, want no-referrer", rp)
		}
	})
}

// Every page that carries a token must be uncacheable and must not leak its URL
// through a Referer header.
func TestTokenPagesCarryNoStoreHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	writeHTMLStatus(rr, http.StatusOK, renderOAuthStatusHTML(true, "ok", "done", "google", "access", "refresh"))

	want := map[string]string{
		"Referrer-Policy": "no-referrer",
		"Pragma":          "no-cache",
	}
	for header, value := range want {
		if got := rr.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	if cc := rr.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want it to include no-store", cc)
	}
}

// The deep link is assembled from server-generated values only, and the
// success page must keep working for the Dart client.
func TestRenderOAuthStatusHTMLBuildsDeepLink(t *testing.T) {
	body := renderOAuthStatusHTML(true, "Вхід успішний", "Повертаємося", "google", "AT-123", "RT-456")
	// & is written as &amp; inside the href attribute; that is correct HTML and
	// the browser decodes it back before following the link.
	if !strings.Contains(body, "oxide://auth/google?access_token=AT-123&amp;refresh_token=RT-456") {
		t.Errorf("deep link missing or altered:\n%s", body)
	}
	if !strings.Contains(body, `window.location.href = "oxide://auth/google?access_token=AT-123`) {
		t.Errorf("auto deep-link redirect missing:\n%s", body)
	}
	if strings.Contains(body, "#ZgotmplZ") {
		t.Error("html/template scrubbed the deep link as an unsafe URL")
	}

	// A provider name outside the fixed set must not reach the URL.
	body = renderOAuthStatusHTML(true, "Вхід успішний", "Повертаємося", "evil\"><script>", "AT", "RT")
	assertNoRawPayload(t, body, "deep link page with a hostile provider name")
}

// The reset form embeds the token from the query string in a <script> body, so
// the token must be escaped for that context.
func TestResetPasswordFormEscapesTheToken(t *testing.T) {
	// A quote in the token must not terminate the JS string literal.
	body := renderResetPasswordForm(`abc"; alert(1); //`)
	if strings.Contains(body, `token: "abc"; alert(1)`) {
		t.Errorf("the JS string literal was broken out of:\n%s", body)
	}
	if !strings.Contains(body, `abc\"; alert(1); //`) {
		t.Errorf("expected the quote to be escaped:\n%s", body)
	}

	body = renderResetPasswordForm(`</script><img src=x onerror=alert(1)>`)
	assertEscapedMarkup(t, body, "reset form")
}
