package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Rate limits on the endpoints the audit found unprotected.
//
// The router installs a shared 30/min per IP budget, which is too generous for
// every endpoint below: each either sends mail, burns a secret, or runs the KDF.
//
// The important case is the per-recipient one. A per-IP limit does not stop email
// bombing -- the abuse is many source addresses and one victim inbox -- so the
// budget that matters is keyed by recipient.
func limiterReq(remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", nil)
	req.RemoteAddr = remoteAddr
	return req
}

func TestRateLimitMailRequestPerCaller(t *testing.T) {
	l := newAuthRateLimiter()

	for i := range mailAttemptsPerWindow {
		if allowed, _ := l.allowMailRequest(limiterReq("198.51.100.1:1000")); !allowed {
			t.Fatalf("attempt %d was refused inside the budget", i+1)
		}
	}
	if allowed, _ := l.allowMailRequest(limiterReq("198.51.100.1:1000")); allowed {
		t.Errorf("attempt %d was allowed past the per-caller budget", mailAttemptsPerWindow+1)
	}

	// A different caller has its own budget: this is a per-caller limit, not a
	// global one, and must not lock out a neighbour behind the same NAT.
	if allowed, _ := l.allowMailRequest(limiterReq("198.51.100.2:1000")); !allowed {
		t.Error("a second caller was refused on the first's budget")
	}
}

func TestRateLimitVerifyAndResetAreBounded(t *testing.T) {
	l := newAuthRateLimiter()

	for range verifyAttemptsPerWindow {
		if allowed, _ := l.allowVerifyEmail(limiterReq("203.0.113.7:1")); !allowed {
			t.Fatal("a verification attempt inside the budget was refused")
		}
	}
	if allowed, _ := l.allowVerifyEmail(limiterReq("203.0.113.7:1")); allowed {
		t.Error("verification was allowed past its budget")
	}

	for range resetAttemptsPerWindow {
		if allowed, _ := l.allowResetPassword(limiterReq("203.0.113.8:1")); !allowed {
			t.Fatal("a reset attempt inside the budget was refused")
		}
	}
	if allowed, _ := l.allowResetPassword(limiterReq("203.0.113.8:1")); allowed {
		t.Error("reset was allowed past its budget")
	}
}

func TestRateLimitRefreshIsGenerousButNotUnbounded(t *testing.T) {
	// Refresh sits on the happy path of every signed-in client, so the budget has
	// to survive several devices on one address and still stop a hammered
	// endpoint. A limit of 5 here would log real users out.
	l := newAuthRateLimiter()

	for range refreshAttemptsPerWindow {
		if allowed, _ := l.allowRefresh(limiterReq("198.51.100.30:1")); !allowed {
			t.Fatal("a refresh inside the budget was refused")
		}
	}
	if allowed, _ := l.allowRefresh(limiterReq("198.51.100.30:1")); allowed {
		t.Error("refresh was allowed past its budget")
	}
}

func TestRateLimitMailBudgetIsPerRecipientNotPerCaller(t *testing.T) {
	// The abuse this exists for: one attacker, many source addresses, one
	// victim's inbox.
	l := newAuthRateLimiter()

	for i := range mailAttemptsPerRecipient {
		allowed, _ := l.allowMailTo("victim@example.com")
		if !allowed {
			t.Fatalf("recipient budget refused attempt %d from a fresh caller", i+1)
		}
		// Each attempt comes from a different address, so the per-caller budget is
		// never touched and cannot be what stopped it.
		l.allowMailRequest(limiterReq("203.0.113.100:1"))
	}

	if allowed, _ := l.allowMailTo("victim@example.com"); allowed {
		t.Error("one inbox was filled from many source addresses")
	}

	// Somebody else's inbox is untouched by that.
	if allowed, _ := l.allowMailTo("bystander@example.com"); !allowed {
		t.Error("an unrelated recipient was refused on the victim's budget")
	}
}

func TestRateLimitMailBudgetIgnoresCaseAndPadding(t *testing.T) {
	// Otherwise the budget is sidestepped by varying case, which costs nothing.
	l := newAuthRateLimiter()

	for range mailAttemptsPerRecipient {
		l.allowMailTo(normalizeEmail("Victim@Example.COM"))
	}

	if allowed, _ := l.allowMailTo("  VICTIM@example.com  "); allowed {
		t.Error("a differently-cased address got its own budget")
	}
}

func TestRateLimitMailAllowsAnEmptyRecipient(t *testing.T) {
	// The handler rejects an empty address before this is consulted, but an empty
	// key must never become a shared bucket: every malformed request would then
	// share one budget.
	l := newAuthRateLimiter()

	for range 100 {
		if allowed, _ := l.allowMailTo(""); !allowed {
			t.Fatal("an empty recipient was refused")
		}
	}
	if len(l.mailByRecipient) != 0 {
		t.Errorf("the empty key allocated %d buckets", len(l.mailByRecipient))
	}
}

func TestRateLimitWindowsExpire(t *testing.T) {
	// A limiter that never lets go is a denial of service by the limiter.
	l := newAuthRateLimiter()

	for range mailAttemptsPerWindow {
		l.allowMailRequest(limiterReq("198.51.100.50:1"))
	}
	if allowed, _ := l.allowMailRequest(limiterReq("198.51.100.50:1")); allowed {
		t.Fatal("the per-caller budget did not fill")
	}

	l.mu.Lock()
	past := time.Now().Add(-2 * mailWindow)
	for _, w := range l.mailByIP {
		w.expiresAt = past
	}
	for _, w := range l.mailByRecipient {
		w.expiresAt = past
	}
	l.lastSweep = time.Now().Add(-2 * limiterCleanupInterval)
	l.mu.Unlock()

	if allowed, _ := l.allowMailRequest(limiterReq("198.51.100.50:1")); !allowed {
		t.Error("the caller is still blocked after its window expired")
	}
	if allowed, _ := l.allowMailTo("victim@example.com"); !allowed {
		t.Error("the recipient is still blocked after its window expired")
	}
}

func TestResendVerificationAnswersIdenticallyForEveryOutcome(t *testing.T) {
	// The oracle this closes: ResendVerification used to answer "email already
	// verified" for a registered-and-verified address while an unknown one got a
	// generic sentence. Two bodies for two states is an enumeration oracle behind
	// a 200, which is the hardest kind to notice in a log.
	//
	// Asserted on the single response helper rather than through the handler,
	// because with no RESEND_API configured the handler short-circuits with 503
	// before reaching any of the three states -- so a handler-level test would
	// pass here without ever reaching the code it claims to cover.
	want := httptest.NewRecorder()
	respondGenericResend(want)

	if want.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", want.Code)
	}

	// Nothing in the body may vary by outcome, and it must not name one.
	body := want.Body.String()
	for _, leak := range []string{
		"already verified",
		"no such",
		"not found",
		"does not exist",
	} {
		if strings.Contains(strings.ToLower(body), leak) {
			t.Errorf("the shared response mentions %q, which states an outcome: %s", leak, body)
		}
	}
}

func TestRateLimitNewBucketsAreSwept(t *testing.T) {
	// Unbounded maps are a memory leak under a spray of unique addresses, and
	// the new buckets are the ones an attacker can fill freely.
	l := newAuthRateLimiter()

	l.allowMailRequest(limiterReq("198.51.100.60:1"))
	l.allowVerifyEmail(limiterReq("198.51.100.61:1"))
	l.allowResetPassword(limiterReq("198.51.100.62:1"))
	l.allowRefresh(limiterReq("198.51.100.63:1"))
	l.allowMailTo("victim@example.com")

	l.mu.Lock()
	past := time.Now().Add(-2 * limiterCleanupInterval)
	for _, bucket := range []map[string]*attemptWindow{
		l.mailByIP, l.verifyByIP, l.resetByIP, l.refreshByIP, l.mailByRecipient,
	} {
		for _, w := range bucket {
			w.expiresAt = past
		}
	}
	l.lastSweep = past
	l.mu.Unlock()

	// Any limiter call sweeps.
	l.allowRefresh(limiterReq("198.51.100.64:1"))

	l.mu.Lock()
	left := len(l.mailByIP) + len(l.verifyByIP) + len(l.resetByIP) +
		len(l.refreshByIP) + len(l.mailByRecipient)
	l.mu.Unlock()

	// Only the one fresh refresh record remains.
	if left != 1 {
		t.Errorf("%d records survived the sweep, want 1", left)
	}
}
