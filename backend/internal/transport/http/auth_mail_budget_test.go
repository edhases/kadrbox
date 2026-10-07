package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/edhases/kadrbox-server/internal/email"
	transporthttp "github.com/edhases/kadrbox-server/internal/transport/http"
)

// The per-INBOX mail budget, proven where mail actually goes out.
//
// A per-IP limit does not stop email bombing: the abuse is many source addresses
// aimed at one victim's inbox, so every bot gets its own allowance and the victim
// pays for all of them. The budget that matters is therefore keyed by recipient.
//
// This needs a working mail service. With RESEND_API unset the handler answers
// 503 before doing anything, so a test written against that rig would pass
// without ever reaching the code it claims to cover.

// countingTransport records every outbound request and answers 200, so the number
// of messages that actually left is observable.
type countingTransport struct {
	mu sync.Mutex
	n  int
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       http.NoBody,
		Request:    r,
	}, nil
}

func (c *countingTransport) sends() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// mailRig builds an AuthHandler with a configured mail service and a transport
// that counts what is sent.
func mailRig(t *testing.T, seedEmails ...string) (*authHandlerForMail, *countingTransport) {
	t.Helper()

	t.Setenv("RESEND_API", "test-resend-key")
	t.Setenv("APP_URL", "https://app.example")

	tr := &countingTransport{}
	swapTransport(t, tr)

	users := newMemUserStore()
	sessions := newMemRefreshStore()
	for _, emailAddr := range seedEmails {
		covNewUser(t, users, emailAddr, "u")
	}

	return &authHandlerForMail{
		h:     covAuth(users, sessions, email.NewService()),
		users: users,
	}, tr
}

type authHandlerForMail struct {
	h     *transporthttp.AuthHandler
	users *memUserStore
}

// post drives one of the two mail endpoints from a given source address.
func (a *authHandlerForMail) post(
	t *testing.T, endpoint, emailAddr, remoteAddr string,
) *httptest.ResponseRecorder {
	t.Helper()

	var handler http.HandlerFunc
	switch endpoint {
	case "forgot":
		handler = a.h.ForgotPassword
	case "resend":
		handler = a.h.ResendVerification
	default:
		t.Fatalf("unknown endpoint %q", endpoint)
	}

	return postFromAddr(t, handler, "/api/v1/auth/"+endpoint, context.Background(),
		map[string]string{"email": emailAddr}, remoteAddr)
}

func TestMailEndpointsShareOneBudgetPerInbox(t *testing.T) {
	// Alternate the two endpoints against one inbox, each from a fresh source
	// address so the per-caller budget can never be what stops it.
	rig, tr := mailRig(t, "victim@example.com")

	const rounds = 8
	for i := range rounds {
		endpoint := "forgot"
		if i%2 == 1 {
			endpoint = "resend"
		}
		rig.post(t, endpoint, "victim@example.com",
			fmt.Sprintf("198.51.100.%d:1", i+1))
	}

	// Without a shared per-recipient budget this would be eight messages to one
	// stranger's inbox. Alternating endpoints must not double the allowance.
	if got := tr.sends(); got == 0 {
		t.Fatal("no message was ever sent, so the test proves nothing")
	}
	if got := tr.sends(); got > 4 {
		t.Errorf("%d messages reached one inbox from %d alternating requests; the "+
			"per-recipient budget is not holding", got, rounds)
	}
}

func TestMailBudgetIsPerInboxNotGlobal(t *testing.T) {
	// Exhaust one inbox and check a second is still reachable: a global budget
	// would let one user lock everybody else out.
	rig, tr := mailRig(t, "noisy@example.com", "quiet@example.com")

	for i := range 8 {
		rig.post(t, "forgot", "noisy@example.com", fmt.Sprintf("198.51.100.%d:1", i+1))
	}
	afterNoisy := tr.sends()

	rig.post(t, "forgot", "quiet@example.com", "203.0.113.30:1")

	if tr.sends() == afterNoisy {
		t.Error("one inbox's exhausted budget stopped a different inbox from being served")
	}
}

func TestSpentMailBudgetDoesNotLeakThroughItsStatusOrBody(t *testing.T) {
	// The limiter refuses the inbox with the SAME generic 200 as a fresh one.
	// A 429 here would say "this address has been asked three times already",
	// which is a smaller statement than existence and still a statement.
	rig, _ := mailRig(t, "victim@example.com", "bystander@example.com")

	for i := range 8 {
		rig.post(t, "forgot", "victim@example.com", fmt.Sprintf("198.51.100.%d:1", i+1))
	}

	spent := rig.post(t, "forgot", "victim@example.com", "203.0.113.40:1")
	fresh := rig.post(t, "forgot", "bystander@example.com", "203.0.113.41:1")

	if spent.Code != fresh.Code {
		t.Errorf("status differs on a spent budget: spent %d, fresh %d",
			spent.Code, fresh.Code)
	}
	if spent.Body.String() != fresh.Body.String() {
		t.Errorf("a spent budget answers differently, so it leaks:\n  spent: %s\n  fresh: %s",
			spent.Body.String(), fresh.Body.String())
	}
}

func TestMailBudgetIgnoresCaseOnTheAddress(t *testing.T) {
	// Otherwise the budget is free: the same inbox under different casing is
	// still the same inbox.
	rig, tr := mailRig(t, "victim@example.com")

	for i := range 8 {
		casing := []string{"victim@example.com", "Victim@Example.COM", "VICTIM@example.com"}[i%3]
		rig.post(t, "forgot", casing, fmt.Sprintf("198.51.100.%d:1", i+1))
	}

	if got := tr.sends(); got > 4 {
		t.Errorf("%d messages went out for one inbox asked under three spellings", got)
	}
}

func TestForgotPasswordSendsNothingForAnUnknownAddress(t *testing.T) {
	// The other half of the promise: the budget is charged per address whether or
	// not the address exists, so a spray of unknown addresses costs nothing extra
	// but also mails nobody.
	rig, tr := mailRig(t)

	for i := range 5 {
		rig.post(t, "forgot", "nobody@example.com", fmt.Sprintf("198.51.100.%d:1", i+1))
	}

	if got := tr.sends(); got != 0 {
		t.Errorf("%d messages were sent for addresses that do not exist", got)
	}
}

func TestMailRigSendsForAKnownAddress(t *testing.T) {
	// Guard on the guard: without this, every assertion above would also pass if
	// the transport silently stopped working.
	rig, tr := mailRig(t, "victim@example.com")

	rec := rig.post(t, "forgot", "victim@example.com", "198.51.100.200:1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if tr.sends() != 1 {
		t.Fatalf("%d messages were sent for one known address, want 1", tr.sends())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	// Must not confirm the address exists.
	for _, leak := range []string{"sent", "reset link has been sent to"} {
		if v, ok := payload["message"].(string); ok && strings.Contains(v, leak) &&
			!strings.Contains(v, "if the email exists") {
			t.Errorf("the response confirms delivery: %q", v)
		}
	}
}
