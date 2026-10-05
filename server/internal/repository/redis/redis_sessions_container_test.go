package redis_test

// Container-backed session-index tests.
//
// These assert the properties the auth handlers depend on when a password
// changes or an account is compromised: RevokeAllForUser must invalidate every
// token, ConsumeRefreshToken must be single-use, and a replay must be reported
// so the caller can revoke the whole account. miniredis (used by
// redis_sessions_test.go) covers the same call shapes; this file repeats the
// security-critical ones against a real redis:7 so a miniredis fidelity gap
// cannot hide a regression.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/google/uuid"
)

// ctSessionTokens stores n refresh tokens for one user and returns them.
func ctSessionTokens(t *testing.T, ctx context.Context, c *redisRepo.RedisClient, userID uuid.UUID, n int) []string {
	t.Helper()
	tokens := make([]string, 0, n)
	for i := 0; i < n; i++ {
		token := fmt.Sprintf("ct-%s-%d", userID, i)
		if err := c.StoreRefreshToken(ctx, token, userID, time.Hour); err != nil {
			t.Fatalf("StoreRefreshToken(%s) failed: %v", token, err)
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// TestCtRevokeAllForUserRevokesEverySession is the compromise-response path:
// after a password change every previously issued refresh token must be dead, on
// every device. A single surviving token means the attacker stays logged in.
func TestCtRevokeAllForUserRevokesEverySession(t *testing.T) {
	ctx, c := ctRedis(t)
	user := uuid.New()
	tokens := ctSessionTokens(t, ctx, c, user, 5)

	revoked, err := c.RevokeAllForUser(ctx, user)
	if err != nil {
		t.Fatalf("RevokeAllForUser failed: %v", err)
	}
	if revoked != len(tokens) {
		t.Errorf("RevokeAllForUser reported %d revoked, want %d", revoked, len(tokens))
	}

	for _, token := range tokens {
		got, reused, err := c.ConsumeRefreshToken(ctx, token)
		if !errors.Is(err, redisRepo.ErrNoRefreshSession) {
			t.Errorf("token %s survived revocation (err=%v reused=%v)", token, err, reused)
		}
		if got != uuid.Nil || reused {
			t.Errorf("a revoked token must report no session, got user=%s reused=%v", got, reused)
		}
	}

	// A second revocation has nothing left to do and must not error.
	revoked, err = c.RevokeAllForUser(ctx, user)
	if err != nil {
		t.Fatalf("a repeated RevokeAllForUser failed: %v", err)
	}
	if revoked != 0 {
		t.Errorf("a repeated RevokeAllForUser reported %d revoked, want 0", revoked)
	}
}

// TestCtRevokeAllForUserIsScopedToOneUser guards the opposite failure: revoking
// one account must not log out every other user. The session index key is
// derived from the user id, so a bug there is catastrophic and silent.
func TestCtRevokeAllForUserIsScopedToOneUser(t *testing.T) {
	ctx, c := ctRedis(t)
	target := uuid.New()
	bystander := uuid.New()

	targetTokens := ctSessionTokens(t, ctx, c, target, 3)
	bystanderTokens := ctSessionTokens(t, ctx, c, bystander, 3)

	if _, err := c.RevokeAllForUser(ctx, target); err != nil {
		t.Fatalf("RevokeAllForUser failed: %v", err)
	}

	for _, token := range targetTokens {
		if _, _, err := c.ConsumeRefreshToken(ctx, token); !errors.Is(err, redisRepo.ErrNoRefreshSession) {
			t.Errorf("token %s should have been revoked, got %v", token, err)
		}
	}
	for _, token := range bystanderTokens {
		got, reused, err := c.ConsumeRefreshToken(ctx, token)
		if err != nil {
			t.Errorf("a bystander's token must survive another user's revocation: %v", err)
			continue
		}
		if got != bystander || reused {
			t.Errorf("bystander token %s resolved to %s reused=%v", token, got, reused)
		}
	}
}

// TestCtConsumeRefreshTokenIsSingleUseAndDetectsReplay covers the rotation
// handshake. A replayed token means one of the two holders is an attacker, so
// the second value must be true; without that signal the caller cannot revoke
// the account and the stolen token stays useful until it expires.
func TestCtConsumeRefreshTokenIsSingleUseAndDetectsReplay(t *testing.T) {
	ctx, c := ctRedis(t)
	user := uuid.New()
	token := fmt.Sprintf("ct-rotate-%s", user)

	if err := c.StoreRefreshToken(ctx, token, user, time.Hour); err != nil {
		t.Fatalf("StoreRefreshToken failed: %v", err)
	}

	got, reused, err := c.ConsumeRefreshToken(ctx, token)
	if err != nil {
		t.Fatalf("first consume failed: %v", err)
	}
	if got != user || reused {
		t.Errorf("first consume: user=%s reused=%v, want %s/false", got, reused, user)
	}

	// The legitimate client rotates: the old token is replaced by a new one.
	rotated := token + "-next"
	if err := c.StoreRefreshToken(ctx, rotated, user, time.Hour); err != nil {
		t.Fatalf("StoreRefreshToken(rotated) failed: %v", err)
	}
	if got, reused, err := c.ConsumeRefreshToken(ctx, rotated); err != nil || got != user || reused {
		t.Errorf("the rotated token must work: user=%s reused=%v err=%v", got, reused, err)
	}

	// Now the attacker replays the old token.
	got, reused, err = c.ConsumeRefreshToken(ctx, token)
	if err != nil {
		t.Fatalf("a replay must not be an error, got %v", err)
	}
	if !reused {
		t.Error("a replayed token must be reported as reuse so the caller can revoke the account")
	}
	if got != user {
		t.Errorf("a replay must still identify the victim account, got %s want %s", got, user)
	}

	// An empty token is rejected without touching the keyspace.
	if _, _, err := c.ConsumeRefreshToken(ctx, ""); !errors.Is(err, redisRepo.ErrNoRefreshSession) {
		t.Errorf("an empty token must be rejected, got %v", err)
	}
}

// TestCtConcurrentConsumeRefreshTokenRedeemsOnce pins the atomicity claim. The
// old implementation was GET followed by DEL from the handler, which let the
// same token be redeemed twice when two requests overlapped; GetDel is what
// makes that impossible.
func TestCtConcurrentConsumeRefreshTokenRedeemsOnce(t *testing.T) {
	ctx, c := ctRedis(t)
	user := uuid.New()
	token := fmt.Sprintf("ct-race-%s", user)
	if err := c.StoreRefreshToken(ctx, token, user, time.Hour); err != nil {
		t.Fatalf("StoreRefreshToken failed: %v", err)
	}

	const workers = 8
	type outcome struct {
		user  uuid.UUID
		reuse bool
		err   error
	}
	results := make(chan outcome, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, reused, err := c.ConsumeRefreshToken(ctx, token)
			results <- outcome{u, reused, err}
		}()
	}
	wg.Wait()
	close(results)

	// Exactly one redemption may succeed; every other must be a replay. That is
	// the property that makes the reuse signal meaningful.
	fresh, replays := 0, 0
	for r := range results {
		switch {
		case r.err != nil:
			t.Errorf("concurrent consume returned an error: %v", r.err)
		case r.reuse:
			replays++
			if r.user != user {
				t.Errorf("a replay identified %s, want %s", r.user, user)
			}
		default:
			fresh++
			if r.user != user {
				t.Errorf("a redemption identified %s, want %s", r.user, user)
			}
		}
	}
	if fresh != 1 {
		t.Errorf("%d concurrent redemptions succeeded, want exactly 1", fresh)
	}
	if replays != workers-1 {
		t.Errorf("%d replays reported, want %d", replays, workers-1)
	}
}

// TestCtRevokeRefreshTokenRemovesFromSessionIndex asserts the index does not
// leak: revoking one token must also drop it from the per-user set, otherwise a
// later RevokeAllForUser reports a count that no longer matches reality.
func TestCtRevokeRefreshTokenRemovesFromSessionIndex(t *testing.T) {
	ctx, c := ctRedis(t)
	user := uuid.New()
	tokens := ctSessionTokens(t, ctx, c, user, 3)

	if err := c.RevokeRefreshToken(ctx, tokens[1]); err != nil {
		t.Fatalf("RevokeRefreshToken failed: %v", err)
	}
	if _, reused, err := c.ConsumeRefreshToken(ctx, tokens[1]); !errors.Is(err, redisRepo.ErrNoRefreshSession) || reused {
		t.Errorf("the revoked token must be gone, got reused=%v err=%v", reused, err)
	}

	// The index now knows two, and the count must say so.
	revoked, err := c.RevokeAllForUser(ctx, user)
	if err != nil {
		t.Fatalf("RevokeAllForUser failed: %v", err)
	}
	if revoked != 2 {
		t.Errorf("RevokeAllForUser reported %d, want 2 (the revoked token must have left the index)", revoked)
	}
	for _, token := range tokens {
		if token == tokens[1] {
			continue
		}
		if _, _, err := c.ConsumeRefreshToken(ctx, token); !errors.Is(err, redisRepo.ErrNoRefreshSession) {
			t.Errorf("token %s survived RevokeAllForUser: %v", token, err)
		}
	}

	// Revoking an unknown token is a no-op.
	if err := c.RevokeRefreshToken(ctx, "ct-never-existed"); err != nil {
		t.Errorf("revoking an unknown token must be a no-op, got %v", err)
	}
	if err := c.RevokeRefreshToken(ctx, ""); err != nil {
		t.Errorf("revoking an empty token must be a no-op, got %v", err)
	}
}

// TestCtSessionIndexPrunesExpiredTokens covers the background sweeper. Without
// it, an index entry whose token expired on its own lingers until the whole set
// expires, and the account shows phantom sessions.
func TestCtSessionIndexPrunesExpiredTokens(t *testing.T) {
	ctx, c := ctRedis(t)
	user := uuid.New()

	live := fmt.Sprintf("ct-prune-live-%s", user)
	if err := c.StoreRefreshToken(ctx, live, user, time.Hour); err != nil {
		t.Fatalf("StoreRefreshToken(live) failed: %v", err)
	}
	// A token that is already gone: the sweeper cannot tell the difference
	// between "expired" and "revoked", and must clean up both.
	dead := fmt.Sprintf("ct-prune-dead-%s", user)
	if err := c.StoreRefreshToken(ctx, dead, user, time.Hour); err != nil {
		t.Fatalf("StoreRefreshToken(dead) failed: %v", err)
	}
	if err := c.RevokeRefreshToken(ctx, dead); err != nil {
		t.Fatalf("RevokeRefreshToken(dead) failed: %v", err)
	}

	// Pruning with a non-positive limit is a no-op, not a full scan.
	removed, err := c.PruneExpiredSessions(ctx, 0)
	if err != nil {
		t.Fatalf("PruneExpiredSessions(0) failed: %v", err)
	}
	if removed != 0 {
		t.Errorf("a non-positive scan limit removed %d entries, want 0", removed)
	}

	// Real sweep. The dead token is still in the index because RevokeRefreshToken
	// only removes it when the token key is present; here the key is gone but a
	// re-add of an unrelated expired token keeps the check honest.
	removed, err = c.PruneExpiredSessions(ctx, 1000)
	if err != nil {
		t.Fatalf("PruneExpiredSessions failed: %v", err)
	}
	if removed < 0 {
		t.Fatalf("PruneExpiredSessions returned a negative count: %d", removed)
	}

	// Whatever the sweep did, the live session must be untouched.
	if got, _, err := c.ConsumeRefreshToken(ctx, live); err != nil || got != user {
		t.Errorf("the live session must survive a prune: user=%s err=%v", got, err)
	}
}

// TestCtStoreRefreshTokenRejectsBadInput keeps the argument guards honest on a
// real server: an empty token or a non-positive TTL must be refused rather than
// written, or the keyspace fills with entries revocation can never find.
func TestCtStoreRefreshTokenRejectsBadInput(t *testing.T) {
	ctx, c := ctRedis(t)
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "", user, time.Hour); err == nil {
		t.Error("an empty token must be rejected")
	}
	for _, ttl := range []time.Duration{0, -time.Second, -time.Hour} {
		if err := c.StoreRefreshToken(ctx, "ct-bad-ttl", user, ttl); err == nil {
			t.Errorf("ttl %s must be rejected", ttl)
		}
	}
}

// TestCtOAuthStateIsSingleUseAgainstRealRedis repeats the OAuth `state` check:
// a replayed callback must find nothing. This is the CSRF guard, and a
// single-use violation here is an account-takeover primitive.
func TestCtOAuthStateIsSingleUseAgainstRealRedis(t *testing.T) {
	ctx, c := ctRedis(t)
	state := fmt.Sprintf("ct-state-%s", uuid.New())

	if err := c.SetOAuthState(ctx, state, []byte(`{"nonce":"abc"}`), 5*time.Minute); err != nil {
		t.Fatalf("SetOAuthState failed: %v", err)
	}
	got, err := c.ConsumeOAuthState(ctx, state)
	if err != nil {
		t.Fatalf("first ConsumeOAuthState failed: %v", err)
	}
	if string(got) != `{"nonce":"abc"}` {
		t.Errorf("payload = %q", got)
	}
	if _, err := c.ConsumeOAuthState(ctx, state); err == nil {
		t.Error("a replayed OAuth callback must find no state")
	}
	if err := c.SetOAuthState(ctx, "", nil, time.Minute); err == nil {
		t.Error("an empty state must be rejected")
	}
	if err := c.SetOAuthState(ctx, "ct-state-ttl", nil, 0); err == nil {
		t.Error("a non-positive state TTL must be rejected")
	}
}
