package http

// Per-handler brute-force protection for the credential endpoints.
//
// The router already installs middleware.RateLimitMiddleware(30/min per IP),
// but that budget is shared with every other route and is far too generous for
// /auth/login and /auth/register: 30 password guesses per minute per IP is a
// fast offline-style attack against a 409 conflict enumeration oracle.
//
// This limiter is deliberately in-handler: middleware/ is owned elsewhere. It
// is the defence-in-depth layer, and it is also the only place that can apply
// per-account backoff, which a pure per-IP limiter cannot express.

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// loginAttemptsPerWindow and loginWindow bound guesses per source address.
	loginAttemptsPerWindow = 5
	loginWindow            = time.Minute

	// registerAttemptsPerWindow bounds account creation per source address.
	registerAttemptsPerWindow = 5
	registerWindow            = time.Minute

	// accountFailureThreshold is how many consecutive failures against one
	// account (from any address) before exponential backoff kicks in. Counting
	// per account as well as per IP is what stops a botnet, and it is the only
	// signal that sees a distributed password-spray.
	accountFailureThreshold = 5

	// baseAccountBackoff doubles per failure past the threshold.
	baseAccountBackoff = 2 * time.Second

	// maxAccountBackoff caps the delay so a real user who fat-fingers a password
	// a few times is never locked out for long.
	maxAccountBackoff = 5 * time.Minute

	// limiterCleanupInterval bounds how often the maps are swept.
	limiterCleanupInterval = 10 * time.Minute
)

type attemptWindow struct {
	count     int
	expiresAt time.Time
}

type accountFailures struct {
	count      int
	blockedTil time.Time
	lastSeen   time.Time
}

// authRateLimiter is per-AuthHandler, so it resets with the process and is
// never shared across handlers.
type authRateLimiter struct {
	mu sync.Mutex

	loginByIP    map[string]*attemptWindow
	registerByIP map[string]*attemptWindow
	accounts     map[string]*accountFailures

	lastSweep time.Time
}

func newAuthRateLimiter() *authRateLimiter {
	return &authRateLimiter{
		loginByIP:    map[string]*attemptWindow{},
		registerByIP: map[string]*attemptWindow{},
		accounts:     map[string]*accountFailures{},
		lastSweep:    time.Now(),
	}
}

// clientKey identifies the caller.
//
// r.RemoteAddr is used deliberately and X-Forwarded-For is NOT: that header is
// attacker-controlled unless every proxy in front of the service rewrites it,
// and trusting it here would make the limiter trivially bypassable. A deployment
// behind a proxy therefore shares one bucket per proxy address, which is
// conservative (more throttling) rather than unsafe.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	if host == "" {
		host = "unknown"
	}
	return host
}

// allowLogin reports whether one more login attempt from this caller is allowed.
func (l *authRateLimiter) allowLogin(r *http.Request) (bool, time.Duration) {
	return l.allowWindow(&l.loginByIP, clientKey(r), loginAttemptsPerWindow, loginWindow)
}

// allowRegister reports whether one more registration from this caller is allowed.
func (l *authRateLimiter) allowRegister(r *http.Request) (bool, time.Duration) {
	return l.allowWindow(&l.registerByIP, clientKey(r), registerAttemptsPerWindow, registerWindow)
}

func (l *authRateLimiter) allowWindow(
	bucket *map[string]*attemptWindow,
	key string,
	limit int,
	window time.Duration,
) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked()

	now := time.Now()
	entry, ok := (*bucket)[key]
	if !ok || now.After(entry.expiresAt) {
		(*bucket)[key] = &attemptWindow{count: 1, expiresAt: now.Add(window)}
		return true, 0
	}
	if entry.count >= limit {
		return false, time.Until(entry.expiresAt)
	}
	entry.count++
	return true, 0
}

// allowAccount returns the remaining backoff for a login against a known account
// key. The key should be a normalised email, so the same account cannot be
// sprayed under case variations.
func (l *authRateLimiter) allowAccount(key string) (bool, time.Duration) {
	if key == "" {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.accounts[key]
	if !ok {
		return true, 0
	}
	now := time.Now()
	if now.After(entry.blockedTil) {
		return true, 0
	}
	return false, time.Until(entry.blockedTil)
}

// recordLoginFailure escalates the per-account backoff. Called on any failed
// login, whether or not the account exists: for an unknown email the key is
// still meaningful, because a spray against one address is exactly the pattern
// we want to slow down.
func (l *authRateLimiter) recordLoginFailure(key string) {
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.accounts[key]
	if !ok {
		entry = &accountFailures{}
		l.accounts[key] = entry
	}
	entry.count++
	entry.lastSeen = time.Now()
	if entry.count < accountFailureThreshold {
		return
	}
	backoff := baseAccountBackoff << uint(entry.count-accountFailureThreshold)
	if backoff > maxAccountBackoff || backoff <= 0 {
		backoff = maxAccountBackoff
	}
	entry.blockedTil = time.Now().Add(backoff)
}

// recordLoginSuccess clears the per-account backoff.
func (l *authRateLimiter) recordLoginSuccess(key string) {
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.accounts, key)
}

// sweepLocked drops expired windows and stale account records so the maps cannot
// grow without bound under a spray of unique addresses.
func (l *authRateLimiter) sweepLocked() {
	now := time.Now()
	if now.Sub(l.lastSweep) < limiterCleanupInterval {
		return
	}
	l.lastSweep = now

	for key, entry := range l.loginByIP {
		if now.After(entry.expiresAt) {
			delete(l.loginByIP, key)
		}
	}
	for key, entry := range l.registerByIP {
		if now.After(entry.expiresAt) {
			delete(l.registerByIP, key)
		}
	}
	for key, entry := range l.accounts {
		if now.After(entry.blockedTil) && now.Sub(entry.lastSeen) > limiterCleanupInterval {
			delete(l.accounts, key)
		}
	}
}

// rejectRateLimited writes a 429 with Retry-After.
func rejectRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := int(retryAfter.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	jsonError(w, "too many attempts, retry later", http.StatusTooManyRequests)
}
