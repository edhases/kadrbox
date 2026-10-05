package redis_test

// Contract tests for the parts of the Redis client the auth handlers depend on
// that the existing suites do not pin down: argument validation that must be
// rejected before any command is sent, the reuse-detection marker window, the
// session-index pruning rules, and the readiness Ping.
//
// Everything runs against miniredis, an in-process RESP server on a loopback
// port, so there is no external Redis and no network dependency.

import (
	"context"
	"errors"
	"testing"
	"time"

	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

func TestRedisPingIsWiredForTheReadinessProbe(t *testing.T) {
	_, c := covMiniRedis(t)
	if err := c.Ping(context.Background()); err != nil {
		t.Errorf("Ping on a live server = %v, want nil", err)
	}
}

func TestRedisPingFailsAfterTheServerIsGone(t *testing.T) {
	m, c := covMiniRedis(t)
	// /readyz must go red when Redis disappears, so Ping has to actually
	// reach the server rather than answer from a cached connection state.
	m.Close()
	if err := c.Ping(context.Background()); err == nil {
		t.Error("Ping succeeded against a closed server, want an error so /readyz reports unhealthy")
	}
}

func TestRedisStoreRefreshTokenRejectsUnusableArguments(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "", user, time.Hour); err == nil {
		t.Error("an empty token was accepted, want an error")
	}
	// A zero/negative TTL would make the key immortal in Redis: SET with no
	// expiry. That has to be refused at the boundary.
	if err := c.StoreRefreshToken(ctx, "tok", user, 0); err == nil {
		t.Error("a zero TTL was accepted, want an error")
	}
	if err := c.StoreRefreshToken(ctx, "tok", user, -time.Hour); err == nil {
		t.Error("a negative TTL was accepted, want an error")
	}
}

func TestRedisStoreRefreshTokenIndexesTheSessionUnderTheSameTTL(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "tok-idx", user, 90*time.Second); err != nil {
		t.Fatalf("store: %v", err)
	}

	indexKey := "sessions:" + user.String()
	if members, err := m.SMembers(indexKey); err != nil || len(members) != 1 || members[0] != "tok-idx" {
		t.Errorf("session index members = %v (err %v), want exactly [tok-idx]", members, err)
	}
	// The index must not outlive the sessions it references, or revocation
	// would keep working against an expired account's empty index.
	ttl := m.TTL(indexKey)
	if ttl <= 0 || ttl > 90*time.Second {
		t.Errorf("session index TTL = %s, want a positive value no larger than the token TTL", ttl)
	}
}

func TestRedisConsumeRefreshTokenRejectsEmptyAndLeavesNoMarker(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()

	got, reused, err := c.ConsumeRefreshToken(ctx, "")
	if !errors.Is(err, redisRepo.ErrNoRefreshSession) {
		t.Errorf("error = %v, want %v", err, redisRepo.ErrNoRefreshSession)
	}
	if got != uuid.Nil || reused {
		t.Errorf("empty token returned (%s, %v), want (nil, false)", got, reused)
	}
	// An empty token must not be burned into the used-marker keyspace: the
	// next legitimate use of any token is unaffected, and no key is created.
	if m.Exists("refresh:used:") {
		t.Error("an empty token left a reuse marker behind")
	}
}

func TestRedisConsumeRefreshTokenDropsAPoisonedValueWithoutLeakingIt(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()

	// A non-UUID value can only come from another writer or a schema change.
	// It must read as "no session", not as a parse error that tells the client
	// the keyspace is damaged.
	if err := m.Set("refresh:poison", "not-a-uuid"); err != nil {
		t.Fatalf("miniredis set: %v", err)
	}
	got, reused, err := c.ConsumeRefreshToken(ctx, "poison")
	if !errors.Is(err, redisRepo.ErrNoRefreshSession) {
		t.Errorf("error = %v, want %v so a poisoned value is indistinguishable from an unknown token", err, redisRepo.ErrNoRefreshSession)
	}
	if got != uuid.Nil || reused {
		t.Errorf("poisoned token returned (%s, %v), want (nil, false)", got, reused)
	}
	// The rotation itself consumed the key, so it must not be left behind.
	if m.Exists("refresh:poison") {
		t.Error("the poisoned token key survived a consume")
	}
}

func TestRedisConsumeRefreshTokenTreatsAPoisonedReuseMarkerAsUnknown(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()

	// Token absent, marker present but unparsable: without the guard this
	// would return a parse error and 500 the refresh endpoint.
	if err := m.Set("refresh:used:tok-bad", "not-a-uuid"); err != nil {
		t.Fatalf("miniredis set: %v", err)
	}
	got, reused, err := c.ConsumeRefreshToken(ctx, "tok-bad")
	if !errors.Is(err, redisRepo.ErrNoRefreshSession) {
		t.Errorf("error = %v, want %v", err, redisRepo.ErrNoRefreshSession)
	}
	if reused || got != uuid.Nil {
		t.Errorf("unparsable marker returned (%s, %v), want (nil, false)", got, reused)
	}
}

func TestRedisRevokeRefreshTokenIsANoOpForUnknownTokens(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx := context.Background()

	// Logout is idempotent from the client's point of view, and an empty token
	// must not turn into a Redis command against a malformed key.
	if err := c.RevokeRefreshToken(ctx, ""); err != nil {
		t.Errorf("revoking an empty token = %v, want nil", err)
	}
	if err := c.RevokeRefreshToken(ctx, "never-existed"); err != nil {
		t.Errorf("revoking an unknown token = %v, want nil", err)
	}
}

func TestRedisRevokeRefreshTokenDropsTheIndexMember(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "tok-r1", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := c.StoreRefreshToken(ctx, "tok-r2", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}

	if err := c.RevokeRefreshToken(ctx, "tok-r1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Leaving the member in the index is what made PruneExpiredSessions
	// necessary in the first place: a live index entry for a dead token.
	members, err := m.SMembers("sessions:" + user.String())
	if err != nil {
		t.Fatalf("smembers: %v", err)
	}
	if len(members) != 1 || members[0] != "tok-r2" {
		t.Errorf("index members = %v, want only [tok-r2]", members)
	}
}

func TestRedisRevokeRefreshTokenIgnoresAnUnparsableOwner(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()

	if err := m.Set("refresh:orphan", "not-a-uuid"); err != nil {
		t.Fatalf("miniredis set: %v", err)
	}
	// The token is still revoked (the key is gone); the only thing lost is the
	// index cleanup, and that must not become an error the client sees.
	if err := c.RevokeRefreshToken(ctx, "orphan"); err != nil {
		t.Errorf("revoke = %v, want nil", err)
	}
	if m.Exists("refresh:orphan") {
		t.Error("the token key survived a revoke")
	}
}

func TestRedisRevokeAllForUserReportsTheCountAndDeletesTheIndex(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()
	other := uuid.New()

	for _, tok := range []string{"a", "b", "c"} {
		if err := c.StoreRefreshToken(ctx, tok, user, time.Hour); err != nil {
			t.Fatalf("store %s: %v", tok, err)
		}
	}
	if err := c.StoreRefreshToken(ctx, "keep", other, time.Hour); err != nil {
		t.Fatalf("store keep: %v", err)
	}

	revoked, err := c.RevokeAllForUser(ctx, user)
	if err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if revoked != 3 {
		t.Errorf("revoked = %d, want 3", revoked)
	}
	for _, tok := range []string{"a", "b", "c"} {
		if m.Exists("refresh:" + tok) {
			t.Errorf("token %q survived RevokeAllForUser", tok)
		}
	}
	if !m.Exists("refresh:keep") {
		t.Error("another user's token was revoked")
	}
	// The index itself must go, or the next Store re-populates a set that
	// still claims revoked sessions.
	if m.Exists("sessions:" + user.String()) {
		t.Error("the session index key survived RevokeAllForUser")
	}
}

func TestRedisRevokeAllForUserOnAnUnknownAccountIsHarmless(t *testing.T) {
	_, c := covMiniRedis(t)

	revoked, err := c.RevokeAllForUser(context.Background(), uuid.New())
	if err != nil {
		t.Errorf("RevokeAllForUser on an unknown account = %v, want (0, nil)", err)
	}
	if revoked != 0 {
		t.Errorf("revoked = %d, want 0", revoked)
	}
}

func TestRedisPruneExpiredSessionsIgnoresNonPositiveScanLimits(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx := context.Background()

	for _, limit := range []int{0, -1} {
		removed, err := c.PruneExpiredSessions(ctx, limit)
		if err != nil || removed != 0 {
			t.Errorf("PruneExpiredSessions(%d) = (%d, %v), want (0, nil)", limit, removed, err)
		}
	}
}

func TestRedisPruneExpiredSessionsRemovesDeadIndexMembers(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()

	// One live session and one member whose token expired on its own: the
	// index entry TTL matches the token TTL, but the token can still be gone
	// (rotated, or evicted) while the index survives.
	if err := c.StoreRefreshToken(ctx, "live", user, time.Hour); err != nil {
		t.Fatalf("store live: %v", err)
	}
	indexKey := "sessions:" + user.String()
	if _, err := m.SAdd(indexKey, "dead"); err != nil {
		t.Fatalf("sadd dead: %v", err)
	}

	removed, err := c.PruneExpiredSessions(ctx, 10)
	if err != nil {
		t.Fatalf("PruneExpiredSessions: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	members, err := m.SMembers(indexKey)
	if err != nil {
		t.Fatalf("smembers: %v", err)
	}
	if len(members) != 1 || members[0] != "live" {
		t.Errorf("index members = %v, want only [live]", members)
	}
}

func TestRedisPruneExpiredSessionsDeletesAnEmptiedIndex(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()

	indexKey := "sessions:" + user.String()
	if _, err := m.SAdd(indexKey, "dead-1", "dead-2"); err != nil {
		t.Fatalf("sadd: %v", err)
	}

	removed, err := c.PruneExpiredSessions(ctx, 10)
	if err != nil {
		t.Fatalf("PruneExpiredSessions: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}
	// An index holding nothing is pure overhead scanned on every prune tick.
	if m.Exists(indexKey) {
		t.Error("an emptied session index was left behind")
	}
}

func TestRedisPruneExpiredSessionsKeepsAnIndexThatStillHasLiveTokens(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()
	user := uuid.New()

	if err := c.StoreRefreshToken(ctx, "live", user, time.Hour); err != nil {
		t.Fatalf("store: %v", err)
	}
	indexKey := "sessions:" + user.String()
	if _, err := m.SAdd(indexKey, "dead"); err != nil {
		t.Fatalf("sadd: %v", err)
	}

	if _, err := c.PruneExpiredSessions(ctx, 10); err != nil {
		t.Fatalf("PruneExpiredSessions: %v", err)
	}
	if !m.Exists(indexKey) {
		t.Error("the index was deleted even though a live session still references it")
	}
	if !m.Exists("refresh:live") {
		t.Error("the live token was pruned")
	}
}

func TestRedisPruneExpiredSessionsHonoursTheScanLimit(t *testing.T) {
	m, c := covMiniRedis(t)
	ctx := context.Background()

	// Two indexes, each with one dead member. A limit of 1 must stop after the
	// first key: this runs on a background ticker and must not stall Redis.
	for range 2 {
		user := uuid.New()
		if _, err := m.SAdd("sessions:"+user.String(), "dead"); err != nil {
			t.Fatalf("sadd: %v", err)
		}
	}

	removed, err := c.PruneExpiredSessions(ctx, 1)
	if err != nil {
		t.Fatalf("PruneExpiredSessions: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d with a scan limit of 1, want 1", removed)
	}
}

func TestRedisOAuthStateValidationAndSingleUse(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx := context.Background()

	if err := c.SetOAuthState(ctx, "", []byte("{}"), time.Minute); err == nil {
		t.Error("an empty state key was accepted, want an error")
	}
	if err := c.SetOAuthState(ctx, "st", nil, 0); err == nil {
		t.Error("a zero state TTL was accepted, want an error")
	}
	if err := c.SetOAuthState(ctx, "st", nil, -time.Minute); err == nil {
		t.Error("a negative state TTL was accepted, want an error")
	}

	if _, err := c.ConsumeOAuthState(ctx, ""); !errors.Is(err, goredis.Nil) {
		t.Errorf("consume with an empty state = %v, want redis.Nil", err)
	}
	if err := c.SetOAuthState(ctx, "st", []byte(`{"provider":"google"}`), time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	payload, err := c.ConsumeOAuthState(ctx, "st")
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if string(payload) != `{"provider":"google"}` {
		t.Errorf("payload = %q, want the stored bytes", payload)
	}
	// Single-use: the record is gone, so a replayed callback finds nothing.
	if _, err := c.ConsumeOAuthState(ctx, "st"); err == nil {
		t.Error("an OAuth state was redeemable twice")
	}
}
