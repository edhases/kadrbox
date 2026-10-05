package http_test

// Session-store half of the in-memory RefreshStore fake.
//
// memRefreshStore itself lives in memstores_test.go, which this change does not
// own, so the two methods AuthHandler now depends on are attached here rather
// than by editing that file. They are real implementations over the same token
// map the production Redis client uses — not stubs — so rotation, logout,
// "password change kills every session" and replay detection are asserted
// against genuine state transitions.
//
// The extra state a Redis client keeps in sibling keys (the short-lived
// "already consumed" markers) cannot live on memRefreshStore, because adding a
// field would mean editing a file this change does not own. It is kept in a
// side table keyed by the store pointer instead.

import (
	"context"
	"sync"
	"time"

	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/google/uuid"
)

// NewAuthHandler picks its OAuth state store by asserting that whatever store it
// was given implements transporthttp.OAuthStateStore. If the concrete Redis
// client ever stops satisfying it, production silently falls back to the
// in-process store — so it is asserted at compile time here.
var (
	_ transporthttp.RefreshStore    = (*redisRepo.RedisClient)(nil)
	_ transporthttp.OAuthStateStore = (*redisRepo.RedisClient)(nil)
)

// memUsedState is the per-store side table.
type memUsedState struct {
	mu sync.Mutex
	// used mirrors the "refresh:used:<token>" markers.
	used map[string]uuid.UUID
	// oauthStates mirrors the "oauth:state:<nonce>" records, with their expiry.
	oauthStates map[string]memOAuthStateEntry
	// consumes and revokeAlls let a test assert that rotation went through the
	// atomic path rather than the old GET-then-DEL pair.
	consumes   int
	revokeAlls int
}

type memOAuthStateEntry struct {
	payload   []byte
	expiresAt time.Time
}

var memUsedStates = struct {
	mu sync.Mutex
	m  map[*memRefreshStore]*memUsedState
}{m: map[*memRefreshStore]*memUsedState{}}

func usedStateFor(s *memRefreshStore) *memUsedState {
	memUsedStates.mu.Lock()
	defer memUsedStates.mu.Unlock()
	state, ok := memUsedStates.m[s]
	if !ok {
		state = &memUsedState{used: map[string]uuid.UUID{}, oauthStates: map[string]memOAuthStateEntry{}}
		memUsedStates.m[s] = state
	}
	return state
}

// SetOAuthState stores a login attempt with a TTL.
func (s *memRefreshStore) SetOAuthState(ctx context.Context, state string, payload []byte, ttl time.Duration) error {
	stateStore := usedStateFor(s)
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()
	stateStore.oauthStates[state] = memOAuthStateEntry{payload: payload, expiresAt: time.Now().Add(ttl)}
	return nil
}

// ConsumeOAuthState atomically reads and deletes a login attempt.
func (s *memRefreshStore) ConsumeOAuthState(ctx context.Context, state string) ([]byte, error) {
	stateStore := usedStateFor(s)
	stateStore.mu.Lock()
	defer stateStore.mu.Unlock()
	entry, ok := stateStore.oauthStates[state]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(stateStore.oauthStates, state)
		return nil, ErrNotFound
	}
	delete(stateStore.oauthStates, state)
	return entry.payload, nil
}

// ConsumeRefreshToken atomically reads and deletes the token. A token that is
// absent but carries a used marker is reported as (userID, true, nil): a replay,
// which the handler treats as a compromise.
func (s *memRefreshStore) ConsumeRefreshToken(ctx context.Context, token string) (uuid.UUID, bool, error) {
	state := usedStateFor(s)

	state.mu.Lock()
	state.consumes++
	state.mu.Unlock()

	s.mu.Lock()
	entry, ok := s.tokens[token]
	if ok && time.Now().After(entry.expiresAt) {
		// An expired token behaves as absent, exactly as Redis would.
		delete(s.tokens, token)
		ok = false
	}
	if ok {
		delete(s.tokens, token)
		userID := entry.userID
		s.mu.Unlock()
		state.mu.Lock()
		state.used[token] = userID
		state.mu.Unlock()
		return userID, false, nil
	}
	s.mu.Unlock()

	state.mu.Lock()
	owner, replayed := state.used[token]
	state.mu.Unlock()
	if replayed {
		return owner, true, nil
	}
	return uuid.Nil, false, ErrNotFound
}

// RevokeAllForUser drops every refresh token of a user and reports how many the
// store knew about.
func (s *memRefreshStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) (int, error) {
	state := usedStateFor(s)
	state.mu.Lock()
	state.revokeAlls++
	state.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	revoked := 0
	for token, entry := range s.tokens {
		if entry.userID == userID {
			delete(s.tokens, token)
			revoked++
		}
	}
	state.mu.Lock()
	for token, owner := range state.used {
		if owner == userID {
			// A marker only matters while the token it describes could still be
			// replayed; every session of this user is gone now.
			delete(state.used, token)
		}
	}
	state.mu.Unlock()
	return revoked, nil
}

// consumeCount reports how many atomic consumes this store served.
func (s *memRefreshStore) consumeCount() int {
	state := usedStateFor(s)
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.consumes
}

// liveTokensForUser counts the tokens currently stored for one user.
func (s *memRefreshStore) liveTokensForUser(userID uuid.UUID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, entry := range s.tokens {
		if entry.userID == userID {
			count++
		}
	}
	return count
}

// tokenIsLive reports whether a specific token is still redeemable.
func (s *memRefreshStore) tokenIsLive(token string) bool {
	_, err := s.GetUserIDByRefreshToken(context.Background(), token)
	return err == nil
}
