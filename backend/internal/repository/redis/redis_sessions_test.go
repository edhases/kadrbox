package redis_test

// Session-store behaviour against a real Redis protocol implementation
// (miniredis), covering the guarantees the auth handlers depend on:
// single-use rotation with replay detection, bulk revocation scoped to one user,
// an index that cannot outlive its tokens, and single-use OAuth state.

import (
	"context"
	"errors"
	"testing"
	"time"

	redisRepo "github.com/edhases/kadrbox-server/internal/repository/redis"
	"github.com/google/uuid"
)

func newSessionRig(t *testing.T) (context.Context, *redisRepo.RedisClient) {
	t.Helper()
	_, c := covMiniRedis(t)
	return context.Background(), c
}

func TestRedisConsumeRefreshTokenIsSingleUse(t *testing.T) {
	ctx, c := newSessionRig(t)
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "tok-a", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}

	got, reused, err := c.ConsumeRefreshToken(ctx, "tok-a")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if reused {
		t.Error("a first redemption must not be reported as a replay")
	}
	if got != user {
		t.Errorf("consume returned %s, want %s", got, user)
	}

	// The key is gone, so a second consume reports the replay rather than
	// redeeming the token again.
	got, reused, err = c.ConsumeRefreshToken(ctx, "tok-a")
	if err != nil {
		t.Fatalf("second consume: %v", err)
	}
	if !reused {
		t.Error("a second consume must be reported as a replay")
	}
	if got != user {
		t.Errorf("replay returned user %s, want %s", got, user)
	}
}

func TestRedisConsumeRefreshTokenDetectsReplay(t *testing.T) {
	ctx, c := newSessionRig(t)
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "tok-b", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, _, err := c.ConsumeRefreshToken(ctx, "tok-b"); err != nil {
		t.Fatalf("first consume: %v", err)
	}

	// A replay must be distinguishable from a token that never existed: it is
	// the signal to cut off every session of that user.
	got, reused, err := c.ConsumeRefreshToken(ctx, "tok-b")
	if err != nil {
		t.Fatalf("replay consume: %v", err)
	}
	if !reused {
		t.Fatal("a replayed token must be reported as reused")
	}
	if got != user {
		t.Errorf("replay returned user %s, want %s so the handler can revoke its sessions", got, user)
	}
}

func TestRedisConsumeRefreshTokenUnknownToken(t *testing.T) {
	ctx, c := newSessionRig(t)
	_, reused, err := c.ConsumeRefreshToken(ctx, "never-issued")
	if err == nil {
		t.Fatal("expected an error for an unknown token")
	}
	if !errors.Is(err, redisRepo.ErrNoRefreshSession) {
		t.Errorf("err = %v, want ErrNoRefreshSession", err)
	}
	if reused {
		t.Error("an unknown token is not a replay")
	}
}

func TestRedisRevokeAllForUser(t *testing.T) {
	ctx, c := newSessionRig(t)
	victim, bystander := uuid.New(), uuid.New()

	for _, token := range []string{"v1", "v2", "v3"} {
		if err := c.StoreRefreshToken(ctx, token, victim, time.Hour); err != nil {
			t.Fatalf("store: %v", err)
		}
	}
	if err := c.StoreRefreshToken(ctx, "b1", bystander, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}

	revoked, err := c.RevokeAllForUser(ctx, victim)
	if err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if revoked != 3 {
		t.Errorf("RevokeAllForUser = %d, want 3", revoked)
	}

	for _, token := range []string{"v1", "v2", "v3"} {
		if _, err := c.GetUserIDByRefreshToken(ctx, token); err == nil {
			t.Errorf("victim token %s survived", token)
		}
	}
	if _, err := c.GetUserIDByRefreshToken(ctx, "b1"); err != nil {
		t.Errorf("bystander token was revoked: %v", err)
	}
}

func TestRedisRevokeAllForUserWithNoSessions(t *testing.T) {
	ctx, c := newSessionRig(t)
	revoked, err := c.RevokeAllForUser(ctx, uuid.New())
	if err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if revoked != 0 {
		t.Errorf("RevokeAllForUser = %d, want 0", revoked)
	}
}

// Rotation deletes the token but leaves its member in the per-user index, so the
// index has to be swept or it accumulates one dead entry per rotation.
func TestRedisPruneExpiredSessions(t *testing.T) {
	_, c := newSessionRig(t)
	ctx := context.Background()
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "live", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := c.StoreRefreshToken(ctx, "rotated", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, _, err := c.ConsumeRefreshToken(ctx, "rotated"); err != nil {
		t.Fatalf("consume: %v", err)
	}

	removed, err := c.PruneExpiredSessions(ctx, 10)
	if err != nil {
		t.Fatalf("PruneExpiredSessions: %v", err)
	}
	if removed != 1 {
		t.Errorf("pruned %d entries, want 1 (the rotated token)", removed)
	}

	// The live session must be untouched.
	if _, err := c.GetUserIDByRefreshToken(ctx, "live"); err != nil {
		t.Errorf("pruning removed a live session: %v", err)
	}
	revoked, err := c.RevokeAllForUser(ctx, user)
	if err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if revoked != 1 {
		t.Errorf("RevokeAllForUser = %d, want 1", revoked)
	}
}

// The index shares the token TTL, so a set of sessions cannot outlive its
// longest-lived member.
func TestRedisSessionIndexExpiresWithItsTokens(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "short", user, time.Minute); err != nil {
		t.Fatalf("store: %v", err)
	}
	m.FastForward(2 * time.Minute)

	if keys := m.Keys(); len(keys) != 0 {
		t.Errorf("keys survived their TTL: %v", keys)
	}
}

func TestRedisPruneExpiredSessionsRespectsTheScanLimit(t *testing.T) {
	_, c := newSessionRig(t)
	for i := 0; i < 5; i++ {
		if err := c.StoreRefreshToken(context.Background(), uuid.New().String(), uuid.New(), time.Hour); err != nil {
			t.Fatalf("store: %v", err)
		}
	}
	// A zero budget must be a no-op rather than a full scan.
	removed, err := c.PruneExpiredSessions(context.Background(), 0)
	if err != nil {
		t.Fatalf("PruneExpiredSessions: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

func TestRedisRevokeRefreshTokenRemovesFromTheSessionIndex(t *testing.T) {
	ctx, c := newSessionRig(t)
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "solo", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := c.RevokeRefreshToken(ctx, "solo"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// The index entry must be gone, or the user would keep a phantom session.
	removed, err := c.PruneExpiredSessions(ctx, 100)
	if err != nil {
		t.Fatalf("PruneExpiredSessions: %v", err)
	}
	if removed != 0 {
		t.Errorf("pruned %d entries, want 0 (the index should already be clean)", removed)
	}

	revoked, err := c.RevokeAllForUser(ctx, user)
	if err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if revoked != 0 {
		t.Errorf("RevokeAllForUser = %d, want 0", revoked)
	}
}

func TestRedisOAuthStateIsSingleUse(t *testing.T) {
	ctx, c := newSessionRig(t)

	if err := c.SetOAuthState(ctx, "nonce-1", []byte(`{"provider":"google"}`), time.Minute); err != nil {
		t.Fatalf("SetOAuthState: %v", err)
	}

	payload, err := c.ConsumeOAuthState(ctx, "nonce-1")
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if string(payload) != `{"provider":"google"}` {
		t.Errorf("payload = %s", payload)
	}

	if _, err := c.ConsumeOAuthState(ctx, "nonce-1"); err == nil {
		t.Error("an OAuth state must be redeemable exactly once")
	}
}

func TestRedisOAuthStateExpires(t *testing.T) {
	m, c := covMiniRedis(t)
	if err := c.SetOAuthState(context.Background(), "nonce-2", []byte(`{}`), time.Minute); err != nil {
		t.Fatalf("SetOAuthState: %v", err)
	}
	m.FastForward(2 * time.Minute)

	if _, err := c.ConsumeOAuthState(context.Background(), "nonce-2"); err == nil {
		t.Error("an expired OAuth state must not be redeemable")
	}
}

func TestRedisOAuthStateRejectsEmptyInput(t *testing.T) {
	ctx, c := newSessionRig(t)
	if err := c.SetOAuthState(ctx, "", []byte(`{}`), time.Minute); err == nil {
		t.Error("an empty state must be rejected")
	}
	if err := c.SetOAuthState(ctx, "x", []byte(`{}`), 0); err == nil {
		t.Error("a zero TTL must be rejected")
	}
	if _, err := c.ConsumeOAuthState(ctx, ""); err == nil {
		t.Error("consuming an empty state must fail")
	}
}

func TestRedisStoreRefreshTokenRejectsBadInput(t *testing.T) {
	ctx, c := newSessionRig(t)
	if err := c.StoreRefreshToken(ctx, "", uuid.New(), time.Hour); err == nil {
		t.Error("an empty token must be rejected")
	}
	if err := c.StoreRefreshToken(ctx, "tok", uuid.New(), 0); err == nil {
		t.Error("a non-positive TTL must be rejected")
	}
}
