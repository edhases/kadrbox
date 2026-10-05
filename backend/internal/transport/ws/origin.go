package ws

import (
	"net/http"
	"net/url"
	"strings"
)

// originRule is one entry of the browser-origin allow-list.
type originRule struct {
	// anyScheme is true for a scheme-less entry such as "localhost:3000", which
	// then matches http and https. It exists so the same list can be pasted in
	// from the CORS middleware, which is written in terms of hostnames.
	anyScheme bool
	scheme    string
	host      string
}

func parseOriginRule(entry string) (originRule, bool) {
	entry = strings.TrimSpace(entry)
	if entry == "" || entry == "*" {
		return originRule{}, false
	}
	if !strings.Contains(entry, "://") {
		return originRule{anyScheme: true, host: strings.ToLower(entry)}, true
	}
	u, err := url.Parse(entry)
	if err != nil || u.Host == "" {
		return originRule{}, false
	}
	return originRule{scheme: strings.ToLower(u.Scheme), host: strings.ToLower(u.Host)}, true
}

func (o originRule) matches(origin *url.URL) bool {
	if !strings.EqualFold(origin.Host, o.host) {
		return false
	}
	return o.anyScheme || strings.EqualFold(origin.Scheme, o.scheme)
}

// checkOrigin replaces the old unconditional `return true`.
//
// With an empty allow-list (the default) only requests carrying NO Origin
// header are upgraded. Browsers always send Origin on a WebSocket handshake,
// so this denies every browser cross-origin attempt while leaving native and
// desktop clients — which send none — working. main.go injects the same list
// the CORS middleware uses; a same-host origin is additionally accepted because
// that is not a cross-origin request at all.
func (h *Hub) checkOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if strings.EqualFold(origin, "null") {
		// Opaque origin: sandboxed iframe, file://, some privacy configs.
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if r.Host != "" && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, rule := range h.originRules {
		if rule.matches(u) {
			return true
		}
	}
	return false
}
