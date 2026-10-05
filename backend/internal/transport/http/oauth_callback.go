package http

// Completion of a provider callback: the three outcomes every callback needs.
//
// The destination used here is always rec.RedirectTo — the value validated and
// stored when the flow started — never a value taken from the callback request.
// That is the whole point of moving the destination server-side: previously the
// handler re-read it out of the `state` query parameter on every branch, so a
// crafted callback could point the redirect (and the tokens appended to it)
// anywhere the old allow-list check happened to accept.

import (
	"log"
	"net/http"
)

// rejectOAuthCallback answers a callback that failed state validation.
//
// It is always 400 with a fixed message: telling the caller *which* check failed
// (unknown state, replayed state, wrong session, wrong link token) would turn
// the endpoint into an oracle for guessing valid state values.
func (h *AuthHandler) rejectOAuthCallback(w http.ResponseWriter, title, message string) {
	log.Printf("[Auth] rejected OAuth callback for %s (state validation failed)", title)
	writeHTMLStatus(w, http.StatusBadRequest, renderOAuthStatusHTML(false, title, message, "", "", ""))
}

// failOAuthAttempt reports a failed attempt. When the flow had a validated
// destination the error is handed to the client through that destination;
// otherwise a status page is rendered.
//
// The redirect itself carries no token, but the headers are still set: the
// response is a 307 to a third party and must not be cached.
func (h *AuthHandler) failOAuthAttempt(w http.ResponseWriter, r *http.Request, rec oauthStateRecord, message string) {
	message = sanitizeProviderMessage(message)
	log.Printf("[Auth] OAuth attempt failed (provider=%s): %s", rec.Provider, message)

	if rec.RedirectTo != "" {
		setNoTokenCacheHeaders(w)
		http.Redirect(w, r, appendQueryParams(rec.RedirectTo, map[string]string{"error": message}), http.StatusTemporaryRedirect)
		return
	}
	writeHTMLStatus(w, http.StatusBadRequest,
		renderOAuthStatusHTML(false, "Помилка авторизації", message, rec.Provider, "", ""))
}

// completeOAuthAttempt finishes a successful attempt.
//
// Tokens in a URL are a compromise forced by the shipped Dart client, which
// reads access_token/refresh_token from the loopback redirect's query string.
// What can be fixed here is everything around it: the destination is exact and
// server-side, and the response is marked no-store / no-referrer so the URL is
// neither cached nor leaked through a Referer header to the next hop.
func (h *AuthHandler) completeOAuthAttempt(
	w http.ResponseWriter,
	r *http.Request,
	rec oauthStateRecord,
	provider string,
	successTitle string,
	accessToken string,
	refreshToken string,
) {
	if rec.RedirectTo != "" {
		destination := appendQueryParams(rec.RedirectTo, map[string]string{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
		})
		setNoTokenCacheHeaders(w)
		http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
		return
	}
	writeHTMLStatus(w, http.StatusOK,
		renderOAuthStatusHTML(true, successTitle, "Повертаємося у додаток Oxide Film...", provider, accessToken, refreshToken))
}
