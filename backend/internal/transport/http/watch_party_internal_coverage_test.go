package http

// Internal coverage for the pieces of the watch-party and auth rate-limit
// plumbing that have no exported seam: the room-code pattern, the ticket
// handler's dispatch, and the limiter's client key.
//
// The Hub is constructed without a bus and without starting its loop: issuing a
// ticket is a pure signing operation, so this stays hermetic.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
	"github.com/edhases/kadrbox-server/internal/transport/ws"
	"github.com/google/uuid"
)

// ticketCtx builds a request that already carries a user id, the way
// middleware.AuthMiddleware would.
func ticketCtx(userID uuid.UUID) context.Context {
	// middleware.UserIDKey is what AuthMiddleware populates; reaching it
	// directly keeps the test off a signed JWT.
	return context.WithValue(context.Background(), middleware.UserIDKey, userID)
}

func TestCovWatchPartyIssueRejectsUnauthenticatedCallers(t *testing.T) {
	// The route sits behind AuthMiddleware, but the handler must not trust
	// that: a router change would otherwise let anyone mint a ticket.
	h := NewWatchPartyHandler(ws.NewHub(nil, ws.Options{JWTSecret: "0123456789abcdef0123456789abcdef"}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/watch-party/tickets",
		strings.NewReader(`{"roomCode":"ROOM1"}`))

	h.Issue(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestCovWatchPartyIssueValidatesTheRoomCode(t *testing.T) {
	// The code mirrors the hub's own check: issuing a ticket for a room the hub
	// would reject at handshake wastes a round trip and misleads the client.
	bad := []struct {
		name string
		room string
	}{
		{name: "tooShort", room: "ABC"},
		{name: "tooLong", room: "ABCDEFGHIJKLMNOPQ"},
		{name: "illegalCharacter", room: "ROOM 1"},
		{name: "punctuation", room: "ROOM.1"},
		{name: "empty", room: ""},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			h := NewWatchPartyHandler(ws.NewHub(nil, ws.Options{JWTSecret: "0123456789abcdef0123456789abcdef"}))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/watch-party/tickets",
				strings.NewReader(`{"roomCode":`+quote(tc.room)+`}`))
			req = req.WithContext(ticketCtx(uuid.New()))

			h.Issue(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			var env map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("error body is not JSON: %v", err)
			}
			if env["error"] != "invalid room code" {
				t.Errorf("error = %q, want %q", env["error"], "invalid room code")
			}
		})
	}
}

func TestCovWatchPartyIssueNormalisesTheRoomCodeAndMintsTickets(t *testing.T) {
	h := NewWatchPartyHandler(ws.NewHub(nil, ws.Options{JWTSecret: "0123456789abcdef0123456789abcdef"}))
	userID := uuid.New()

	cases := []struct {
		name   string
		body   string
		room   string
		isHost bool
	}{
		{name: "guest", body: `{"roomCode":" room-1 "}`, room: "ROOM-1"},
		{name: "host", body: `{"roomCode":"room1","isHost":true}`, room: "ROOM1", isHost: true},
		{name: "underscoresAndDigitsAreAccepted", body: `{"roomCode":"ab_12"}`, room: "AB_12"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/watch-party/tickets", strings.NewReader(tc.body))
			req = req.WithContext(ticketCtx(userID))

			h.Issue(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}

			var env struct {
				Ticket   string    `json:"ticket"`
				RoomCode string    `json:"roomCode"`
				Expires  time.Time `json:"expiresAt"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
			}
			if env.Ticket == "" {
				t.Error("no ticket in the response; the client could not connect")
			}
			// The normalised form is what the hub will see, so returning the
			// raw form would leave the client joining a different room.
			if env.RoomCode != tc.room {
				t.Errorf("roomCode = %q, want the normalised %q", env.RoomCode, tc.room)
			}
			if !env.Expires.After(time.Now()) {
				t.Errorf("expiresAt = %s, want a future timestamp", env.Expires)
			}
		})
	}
}

func TestCovWatchPartyIssueRejectsAMalformedOrOversizedBody(t *testing.T) {
	h := NewWatchPartyHandler(ws.NewHub(nil, ws.Options{JWTSecret: "0123456789abcdef0123456789abcdef"}))
	userID := uuid.New()

	cases := []struct {
		name string
		body string
	}{
		{name: "invalidJSON", body: `{not json`},
		{name: "emptyBody", body: ``},
		{name: "wrongFieldType", body: `{"roomCode":123}`},
		{name: "oversized", body: `{"roomCode":"ROOM1","pad":"` + strings.Repeat("x", 8<<10) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/watch-party/tickets", strings.NewReader(tc.body))
			req = req.WithContext(ticketCtx(userID))

			h.Issue(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCovWatchPartyIssueReportsAnUnconfiguredIssuerAsAServerError(t *testing.T) {
	// With no JWTSecret the hub cannot sign a ticket. That is a server
	// misconfiguration, not a client error, so it must not be a 400.
	h := NewWatchPartyHandler(ws.NewHub(nil, ws.Options{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/watch-party/tickets",
		strings.NewReader(`{"roomCode":"ROOM1"}`))
	req = req.WithContext(ticketCtx(uuid.New()))

	h.Issue(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500 when tickets cannot be signed (body %s)", rec.Code, rec.Body.String())
	}
}

// ---- auth rate limiter -----------------------------------------------------

func TestCovClientKeyUsesThePeerAddressNotForwardedHeaders(t *testing.T) {
	// X-Forwarded-For is attacker-controlled, so using it here would hand out
	// a fresh bucket per request.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.7:41234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Real-IP", "5.6.7.8")

	if got := clientKey(req); got != "203.0.113.7" {
		t.Errorf("clientKey = %q, want the peer address 203.0.113.7", got)
	}
}

func TestCovClientKeySurvivesAMalformedOrAbsentRemoteAddr(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "hostOnly", remoteAddr: "203.0.113.7", want: "203.0.113.7"},
		{name: "ipv6WithPort", remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "empty", remoteAddr: "", want: "unknown"},
		{name: "whitespaceOnly", remoteAddr: "   ", want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			req.RemoteAddr = tc.remoteAddr
			if got := clientKey(req); got != tc.want {
				t.Errorf("clientKey(RemoteAddr=%q) = %q, want %q", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

func TestCovAllowWindowOpensAFreshBucketAfterTheWindowExpires(t *testing.T) {
	l := newAuthRateLimiter()
	mk := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		return req
	}

	// Exhaust the budget.
	for range loginAttemptsPerWindow {
		if allowed, _ := l.allowLogin(mk()); !allowed {
			t.Fatalf("blocked before the budget of %d was spent", loginAttemptsPerWindow)
		}
	}
	if allowed, retryAfter := l.allowLogin(mk()); allowed {
		t.Error("a sixth attempt was allowed; the per-IP budget is not enforced")
	} else if retryAfter <= 0 {
		t.Errorf("Retry-After = %s, want a positive wait", retryAfter)
	}

	// Expire the window by rewinding it, then confirm the bucket reopens
	// rather than latching the caller out.
	l.mu.Lock()
	for k, w := range l.loginByIP {
		w.expiresAt = time.Now().Add(-time.Second)
		_ = k
	}
	l.mu.Unlock()

	if allowed, _ := l.allowLogin(mk()); !allowed {
		t.Error("still blocked after the window expired; a user can never log in again")
	}
}

func TestCovAllowWindowSeparatesRegistersFromLogins(t *testing.T) {
	// The two budgets are independent: exhausting logins must not block a
	// legitimate registration from the same address.
	l := newAuthRateLimiter()
	mk := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		return req
	}

	for range loginAttemptsPerWindow {
		l.allowLogin(mk())
	}
	if allowed, _ := l.allowRegister(mk()); !allowed {
		t.Error("the login budget blocked registration; the two windows are not independent")
	}
}

func TestCovAllowAccountBackoffGrowsAndClears(t *testing.T) {
	l := newAuthRateLimiter()

	// Below the threshold there is no backoff, so a real user who mistypes a
	// password a few times is not locked out.
	for i := 1; i < accountFailureThreshold; i++ {
		l.recordLoginFailure("user@example.com")
		if allowed, _ := l.allowAccount("user@example.com"); !allowed {
			t.Fatalf("blocked after only %d failures, threshold is %d", i, accountFailureThreshold)
		}
	}

	l.recordLoginFailure("user@example.com") // reaches the threshold
	allowed, retryAfter := l.allowAccount("user@example.com")
	if allowed {
		t.Error("still allowed at the failure threshold; the backoff never engages")
	}
	if retryAfter <= 0 {
		t.Errorf("backoff = %s, want a positive wait", retryAfter)
	}

	// A successful sign-in clears it.
	l.recordLoginSuccess("user@example.com")
	if allowed, _ := l.allowAccount("user@example.com"); !allowed {
		t.Error("still blocked after a successful login")
	}
}

func TestCovAccountBackoffIsCappedSoNobodyIsLockedOutForever(t *testing.T) {
	l := newAuthRateLimiter()
	// Many more failures than the cap implies: without a ceiling the shift
	// would overflow into a negative duration and unblock immediately.
	for range accountFailureThreshold + 40 {
		l.recordLoginFailure("sprayed@example.com")
	}

	allowed, retryAfter := l.allowAccount("sprayed@example.com")
	if allowed {
		t.Error("the account was not blocked at all")
	}
	if retryAfter > maxAccountBackoff+time.Second {
		t.Errorf("backoff = %s, want it capped near %s", retryAfter, maxAccountBackoff)
	}
	if retryAfter <= 0 {
		t.Errorf("backoff = %s; a non-positive wait means the cap overflowed", retryAfter)
	}
}

func TestCovAccountBackoffIgnoresAnEmptyKey(t *testing.T) {
	// An empty email must not become one shared bucket: that would let an
	// unauthenticated caller lock out every anonymous attempt at once.
	l := newAuthRateLimiter()
	for range accountFailureThreshold + 5 {
		l.recordLoginFailure("")
	}
	if allowed, _ := l.allowAccount(""); !allowed {
		t.Error("the empty key was throttled")
	}
	if len(l.accounts) != 0 {
		t.Errorf("the empty key allocated %d account records", len(l.accounts))
	}
}

func TestCovSweepDropsExpiredWindowsAndStaleAccounts(t *testing.T) {
	// Unbounded maps are a memory leak under a spray of unique addresses, so
	// the sweep has to actually delete.
	l := newAuthRateLimiter()

	mk := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "198.51.100.4:9999"
		return req
	}
	l.allowLogin(mk())
	l.allowRegister(mk())
	for range accountFailureThreshold + 2 {
		l.recordLoginFailure("stale@example.com")
	}

	if len(l.loginByIP) == 0 || len(l.registerByIP) == 0 || len(l.accounts) == 0 {
		t.Fatal("nothing was recorded; the test setup did not run")
	}

	// Age every record past both the window and the cleanup interval, then
	// rewind lastSweep so the next limiter call actually sweeps.
	l.mu.Lock()
	past := time.Now().Add(-2 * limiterCleanupInterval)
	for _, w := range l.loginByIP {
		w.expiresAt = past
	}
	for _, w := range l.registerByIP {
		w.expiresAt = past
	}
	for _, a := range l.accounts {
		a.blockedTil = past
		a.lastSeen = past
	}
	l.lastSweep = past
	l.mu.Unlock()

	if allowed, _ := l.allowLogin(mk()); !allowed {
		t.Error("the caller is still blocked after its window expired and was swept")
	}
	if allowed, _ := l.allowRegister(mk()); !allowed {
		t.Error("registration is still blocked after its window expired and was swept")
	}

	l.mu.Lock()
	loginLeft, registerLeft, accountsLeft := len(l.loginByIP), len(l.registerByIP), len(l.accounts)
	l.mu.Unlock()
	if loginLeft != 1 {
		t.Errorf("loginByIP holds %d entries, want only the fresh one", loginLeft)
	}
	if registerLeft != 1 {
		t.Errorf("registerByIP holds %d entries, want only the fresh one", registerLeft)
	}
	if accountsLeft != 0 {
		t.Errorf("accounts holds %d entries, want the stale one swept", accountsLeft)
	}
}

func TestCovRejectRateLimitedNeverAdvertisesZeroSeconds(t *testing.T) {
	// Retry-After: 0 tells a client to retry immediately, which is exactly the
	// opposite of the point.
	for _, retryAfter := range []time.Duration{0, -time.Second, 100 * time.Millisecond} {
		rec := httptest.NewRecorder()
		rejectRateLimited(rec, retryAfter)

		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("got %d, want 429", rec.Code)
		}
		if got := rec.Header().Get("Retry-After"); got != "1" {
			t.Errorf("Retry-After for %s = %q, want \"1\"", retryAfter, got)
		}
	}

	rec := httptest.NewRecorder()
	rejectRateLimited(rec, 90*time.Second)
	if got := rec.Header().Get("Retry-After"); got != "90" {
		t.Errorf("Retry-After = %q, want \"90\" (whole seconds)", got)
	}
}

// ---- redirects -------------------------------------------------------------

func TestCovAllowedRedirectOriginString(t *testing.T) {
	o := allowedRedirectOrigin{scheme: "https", host: "oxideteam.pp.ua", port: "443"}
	if got := o.String(); got != "https://oxideteam.pp.ua:443" {
		t.Errorf("String() = %q, want %q", got, "https://oxideteam.pp.ua:443")
	}
}

func TestCovParsePortList(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []int
	}{
		{name: "single", raw: "8080", want: []int{8080}},
		{name: "range", raw: "100-103", want: []int{100, 101, 102, 103}},
		{name: "mixed", raw: " 80 , 9000-9001 ,", want: []int{80, 9000, 9001}},
		{name: "garbageIsSkipped", raw: "abc,80,1-0,-5,0", want: []int{80}},
		{name: "empty", raw: "   ", want: nil},
		{name: "nothingUsable", raw: ",,", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePortList(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("parsePortList(%q) = %v, want %v", tc.raw, got, tc.want)
			}
			for _, p := range tc.want {
				if !got[p] {
					t.Errorf("parsePortList(%q) is missing %d (got %v)", tc.raw, p, got)
				}
			}
		})
	}
}

func TestCovAppendQueryParamsPassesThroughUnparsableInput(t *testing.T) {
	// A destination that does not parse is returned verbatim rather than
	// rewritten, so the caller never silently loses the original target.
	got := appendQueryParams("://broken", map[string]string{"error": "x"})
	if got != "://broken" {
		t.Errorf("appendQueryParams = %q, want the input verbatim", got)
	}
}

// quote wraps a string as a JSON literal.
func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
