package http

// Exact-origin allow-list for OAuth post-login redirects.
//
// The old check accepted any URL whose *hostname* was 127.0.0.1 or localhost on
// any port, plus every *.oxideteam.pp.ua subdomain, plus any oxide:// scheme.
// Live access and refresh tokens were then appended to that URL, so a hostile
// page could point the flow at a listener it controls and harvest a session.
//
// The rule here is an exact (scheme, host, port) triple. Hostnames are compared
// after normalisation, never by substring or suffix: "localhost.evil.com",
// "evil.com/?x=localhost" and "https://oxideteam.pp.ua@evil.com" (userinfo) all
// fail, as does any port not named here.

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

// allowedRedirectOrigin is one exact (scheme, host, port) triple.
type allowedRedirectOrigin struct {
	scheme string
	host   string
	port   string
}

func (o allowedRedirectOrigin) String() string {
	return o.scheme + "://" + o.host + ":" + o.port
}

// loopbackPortAllowListEnv names the operator override for the loopback ports a
// client may be redirected to.
//
// The Dart client binds an ephemeral port chosen by the OS, so the port cannot
// be pinned to a constant. What *is* pinned is (a) loopback only, (b) an
// explicit port that was actually written in the URL, and (c) membership in the
// list below, which an operator can narrow.
//
// Default: the IANA dynamic/private range Windows and Linux hand out for
// ephemeral binds (49152-65535). Ports below 1024 and the rest of the ephemeral
// space are refused unless configured.
const loopbackPortAllowListEnv = "OAUTH_ALLOWED_LOOPBACK_PORTS"

const defaultLoopbackPortRange = "49152-65535"

// The parsed list is cached against the raw env value so the (rare) reparse
// happens only when the configuration actually changes.
var loopbackPorts = struct {
	mu    sync.Mutex
	raw   string
	ports map[int]bool
}{}

func allowedLoopbackPorts() map[int]bool {
	raw := strings.TrimSpace(os.Getenv(loopbackPortAllowListEnv))
	if raw == "" {
		raw = defaultLoopbackPortRange
	}

	loopbackPorts.mu.Lock()
	defer loopbackPorts.mu.Unlock()
	if loopbackPorts.ports != nil && loopbackPorts.raw == raw {
		return loopbackPorts.ports
	}
	loopbackPorts.raw = raw
	loopbackPorts.ports = parsePortList(raw)
	return loopbackPorts.ports
}

func parsePortList(raw string) map[int]bool {
	ports := map[int]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, found := strings.Cut(part, "-"); found {
			lowPort, lowErr := strconv.Atoi(strings.TrimSpace(lo))
			highPort, highErr := strconv.Atoi(strings.TrimSpace(hi))
			if lowErr != nil || highErr != nil || lowPort <= 0 || highPort < lowPort {
				continue
			}
			for port := lowPort; port <= highPort; port++ {
				ports[port] = true
			}
			continue
		}
		if single, err := strconv.Atoi(part); err == nil && single > 0 {
			ports[single] = true
		}
	}
	return ports
}

var allowedOAuthRedirectOrigins = []allowedRedirectOrigin{
	// Production web client.
	{scheme: "https", host: "film.oxideteam.pp.ua", port: "443"},
	{scheme: "https", host: "oxideteam.pp.ua", port: "443"},
}

// loopbackHosts is an exact set: no suffix matching, so "localhost.evil.com"
// and "127.0.0.1.nip.io" are not loopback.
var loopbackHosts = map[string]bool{
	"127.0.0.1": true,
	"::1":       true,
	"localhost": true,
}

// isSafeRedirectURL reports whether rawURL is an exact match for one of the
// allow-listed origins.
func isSafeRedirectURL(rawURL string) bool {
	if rawURL == "" || len(rawURL) > 2048 {
		return false
	}
	// Control characters can truncate or split a URL in a downstream parser.
	if strings.ContainsAny(rawURL, "\x00\n\r\t ") {
		return false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() {
		return false
	}
	// Userinfo is the classic way to smuggle a trusted-looking host past a
	// hostname check: https://oxideteam.pp.ua@evil.com has Hostname() == evil.com,
	// but "oxideteam.pp.ua" appears in the raw string.
	if parsed.User != nil {
		return false
	}
	// A scheme-relative URL (//evil.com) has no scheme of its own; reject it
	// rather than letting a caller's base URL decide the scheme.
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		return false
	}

	host, port, ok := canonicalHostPort(parsed)
	if !ok {
		return false
	}

	if loopbackHosts[host] {
		// Loopback is only ever allowed over plain http on an explicitly
		// written, allow-listed port. A missing port means 80 and is rejected.
		if scheme != "http" || parsed.Port() == "" {
			return false
		}
		parsedPort, convErr := strconv.Atoi(parsed.Port())
		if convErr != nil {
			return false
		}
		return allowedLoopbackPorts()[parsedPort]
	}

	for _, allowed := range allowedOAuthRedirectOrigins {
		if scheme == allowed.scheme && host == allowed.host && port == allowed.port {
			return true
		}
	}
	return false
}

// canonicalHostPort normalises the host and resolves the effective port,
// including the scheme default when the URL omits it.
func canonicalHostPort(parsed *url.URL) (host string, port string, ok bool) {
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", "", false
	}
	host = strings.ToLower(strings.TrimSuffix(hostname, "."))

	if explicit := parsed.Port(); explicit != "" {
		if _, err := strconv.Atoi(explicit); err != nil {
			return "", "", false
		}
		port = explicit
	} else {
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return "", "", false
		}
	}
	return host, port, true
}

// appendQueryParams builds the post-login redirect by appending params to the
// destination that was validated at flow start. Callers must pass exactly the
// destination stored in the OAuth state record — never a value from the
// callback's query string.
func appendQueryParams(destination string, params map[string]string) string {
	base, err := parseAbsoluteURL(destination)
	if err != nil {
		return destination
	}
	query := base.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	base.RawQuery = query.Encode()
	return base.String()
}
