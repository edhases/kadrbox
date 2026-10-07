package middleware

import (
	"net/http"
)

// Response headers applied to every response, not just the HTML pages.
//
// These were missing entirely, which the audit found. The per-page helper in
// transport/http covers the token pages; this covers everything else, including
// the JSON API and the WebSocket rejection responses, where a browser will
// actually read the headers.
//
// Each one is here for a concrete failure it prevents:
//
//   - nosniff: stops a browser reinterpreting a JSON error body as HTML. Without
//     it an attacker-controlled string in an error message can execute.
//   - DENY plus CSP frame-ancestors: these pages carry one-time tokens in their
//     URLs. Framed, a hostile page reads the token out of the address bar. DENY
//     covers legacy browsers, frame-ancestors covers modern ones, and neither
//     one is sufficient alone.
//   - HSTS: the whole point of this service is that a session token must never
//     travel in the clear. One visit over plain HTTP would otherwise be enough to
//     keep the downgrade available.
//   - referrer-policy: the same token-in-URL problem, one navigation later. A
//     reset link that lands on a page loading any external resource would
//     otherwise hand `?token=` to that resource's server in the Referer.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")

		// frame-ancestors is the CSP spelling of the same rule. Kept as a
		// separate header rather than folded into a full CSP so it applies to
		// responses that have no CSP of their own.
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")

		// Two years, with subdomains. max-age alone is the part that matters for
		// a token leak; the includeSubDomains is here because a forgotten
		// subdomain is exactly where a downgrade gets reintroduced.
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")

		next.ServeHTTP(w, r)
	})
}
