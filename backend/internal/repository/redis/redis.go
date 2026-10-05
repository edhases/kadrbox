package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// ErrNoRefreshSession means the presented refresh token is unknown, already
// expired, or was revoked. It is deliberately indistinguishable from "token
// does not exist" so an attacker cannot probe which tokens were ever issued.
var ErrNoRefreshSession = errors.New("refresh token is not active")

type RedisClient struct {
	client *goredis.Client
}

func NewRedisClient(addr, password string) (*RedisClient, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
		// Явні таймаути замість нульових значень за замовчуванням: Watch Party
		// публікує собыття в Redis під час shutdown, і необмежене очікування
		// на мертвому з'єднанні зупинило б весь drain.
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis at %s: %w", addr, err)
	}

	return &RedisClient{client: client}, nil
}

// Ping використовується readiness-проверкою (/readyz). go-redis повертає
// *StatusCmd, а не error, тому метод обгортає його.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Key namespaces. Kept as constants so a typo cannot silently create a second,
// differently-named keyspace that revocation would then miss.
const (
	refreshKeyPrefix     = "refresh:"
	refreshUsedKeyPrefix = "refresh:used:"
	sessionSetKeyPrefix  = "sessions:"
	oauthStateKeyPrefix  = "oauth:state:"
)

// refreshUsedMarkerTTL bounds how long a consumed refresh token stays
// recognisable. Reuse after this window degrades to a plain unknown-token
// rejection, which is the same outcome the client sees for a bogus token.
const refreshUsedMarkerTTL = 24 * time.Hour

// StoreRefreshToken зберігає довготривалий refresh-токен на 30 днів
//
// Besides the token key itself the token is added to the per-user session
// index (sessions:<userID>) so every session of an account can be revoked in
// one call (password change, logout-everywhere, compromise response). The
// index shares the token's TTL, so it cannot outlive the sessions it tracks.
func (r *RedisClient) StoreRefreshToken(ctx context.Context, token string, userID uuid.UUID, ttl time.Duration) error {
	if token == "" {
		return errors.New("empty refresh token")
	}
	if ttl <= 0 {
		return errors.New("refresh token ttl must be positive")
	}

	pipe := r.client.TxPipeline()
	pipe.Set(ctx, refreshKeyPrefix+token, userID.String(), ttl)
	pipe.SAdd(ctx, sessionSetKey(userID), token)
	// Guard against a leaked index: it must not outlive the longest-lived
	// session it references.
	pipe.Expire(ctx, sessionSetKey(userID), ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}
	return nil
}

// GetUserIDByRefreshToken перевіряє валідність refresh-токена
//
// Read-only: use ConsumeRefreshToken when the token is about to be rotated, so
// the read-and-delete is a single atomic step.
func (r *RedisClient) GetUserIDByRefreshToken(ctx context.Context, token string) (uuid.UUID, error) {
	key := refreshKeyPrefix + token
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(val)
}

// ConsumeRefreshToken atomically reads and deletes a refresh token.
//
// The second return value reports *reuse*: the token is absent but a
// "already consumed" marker exists, which means the token was replayed. A
// replay means one of the two holders is an attacker, so the caller is
// expected to revoke every session of the returned user.
//
// The old implementation was GET followed by DEL from the handler: two round
// trips, a window in which the same token could be redeemed twice, and a
// discarded error that silently left the token alive.
func (r *RedisClient) ConsumeRefreshToken(ctx context.Context, token string) (uuid.UUID, bool, error) {
	if token == "" {
		return uuid.Nil, false, ErrNoRefreshSession
	}

	val, err := r.client.GetDel(ctx, refreshKeyPrefix+token).Result()
	if err == nil {
		userID, parseErr := uuid.Parse(val)
		if parseErr != nil {
			// Poisoned value in the keyspace: drop it and treat the token as
			// unknown rather than propagating a parse error to the client.
			return uuid.Nil, false, ErrNoRefreshSession
		}
		if setErr := r.client.Set(ctx, refreshUsedKeyPrefix+token, val, refreshUsedMarkerTTL).Err(); setErr != nil {
			// The rotation itself succeeded; losing the marker only costs us
			// reuse detection, so report success and let the caller proceed.
			fmt.Printf("[Redis] failed to mark refresh token as used: %v\n", setErr)
		}
		return userID, false, nil
	}
	if !errors.Is(err, goredis.Nil) {
		return uuid.Nil, false, err
	}

	used, usedErr := r.client.Get(ctx, refreshUsedKeyPrefix+token).Result()
	if usedErr != nil {
		if errors.Is(usedErr, goredis.Nil) {
			return uuid.Nil, false, ErrNoRefreshSession
		}
		return uuid.Nil, false, usedErr
	}
	userID, parseErr := uuid.Parse(used)
	if parseErr != nil {
		return uuid.Nil, false, ErrNoRefreshSession
	}
	return userID, true, nil
}

// RevokeRefreshToken інвалідує сесію (наприклад, при Logout)
func (r *RedisClient) RevokeRefreshToken(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	// GETDEL rather than DEL so the owning user can be located and the token
	// removed from that user's session index too.
	val, err := r.client.GetDel(ctx, refreshKeyPrefix+token).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil // revoking an unknown token is a no-op, as before
		}
		return err
	}
	userID, parseErr := uuid.Parse(val)
	if parseErr != nil {
		return nil
	}
	return r.client.SRem(ctx, sessionSetKey(userID), token).Err()
}

// RevokeAllForUser invalidates every refresh token of a user and returns how
// many sessions the index knew about. Used by password change, password reset
// and account deletion: after any of those, previously issued refresh tokens
// must not keep working.
func (r *RedisClient) RevokeAllForUser(ctx context.Context, userID uuid.UUID) (int, error) {
	indexKey := sessionSetKey(userID)

	members, err := r.client.SMembers(ctx, indexKey).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return 0, fmt.Errorf("read session index: %w", err)
	}

	revoked := 0
	if len(members) > 0 {
		keys := make([]string, 0, len(members))
		for _, token := range members {
			keys = append(keys, refreshKeyPrefix+token)
		}
		if err := r.client.Del(ctx, keys...).Err(); err != nil {
			return revoked, fmt.Errorf("delete refresh tokens: %w", err)
		}
		revoked = len(members)
	}

	if err := r.client.Del(ctx, indexKey).Err(); err != nil {
		return revoked, fmt.Errorf("delete session index: %w", err)
	}
	return revoked, nil
}

// PruneExpiredSessions removes entries whose refresh token has already expired
// from the per-user session indexes. Without it an index entry whose token
// expired on its own would linger until the whole set expires.
//
// scanLimit bounds the work per call (number of index keys inspected) so this
// can be run from a background ticker without stalling Redis. Returns the
// number of stale entries removed.
func (r *RedisClient) PruneExpiredSessions(ctx context.Context, scanLimit int) (int, error) {
	if scanLimit <= 0 {
		return 0, nil
	}

	removed := 0
	var cursor uint64
	for scanned := 0; scanned < scanLimit; {
		keys, next, err := r.client.Scan(ctx, cursor, sessionSetKeyPrefix+"*", 100).Result()
		if err != nil {
			return removed, fmt.Errorf("scan session indexes: %w", err)
		}
		for _, key := range keys {
			if scanned >= scanLimit {
				break
			}
			scanned++
			n, err := r.pruneSessionIndex(ctx, key)
			if err != nil {
				return removed, err
			}
			removed += n
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return removed, nil
}

func (r *RedisClient) pruneSessionIndex(ctx context.Context, indexKey string) (int, error) {
	members, err := r.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return 0, nil
		}
		return 0, fmt.Errorf("read session index %s: %w", indexKey, err)
	}

	removed := 0
	for _, token := range members {
		exists, err := r.client.Exists(ctx, refreshKeyPrefix+token).Result()
		if err != nil {
			return removed, fmt.Errorf("check refresh token: %w", err)
		}
		if exists == 0 {
			if err := r.client.SRem(ctx, indexKey, token).Err(); err != nil {
				return removed, fmt.Errorf("prune session index: %w", err)
			}
			removed++
		}
	}
	if removed > 0 {
		if card, err := r.client.SCard(ctx, indexKey).Result(); err == nil && card == 0 {
			if err := r.client.Del(ctx, indexKey).Err(); err != nil {
				return removed, fmt.Errorf("delete empty session index: %w", err)
			}
		}
	}
	return removed, nil
}

// SetOAuthState stores an OAuth `state` record with a short TTL.
//
// The record is opaque JSON owned by the HTTP layer; Redis only guarantees
// single-use-on-read semantics through ConsumeOAuthState.
func (r *RedisClient) SetOAuthState(ctx context.Context, state string, payload []byte, ttl time.Duration) error {
	if state == "" {
		return errors.New("empty oauth state")
	}
	if ttl <= 0 {
		return errors.New("oauth state ttl must be positive")
	}
	return r.client.Set(ctx, oauthStateKeyPrefix+state, payload, ttl).Err()
}

// ConsumeOAuthState atomically reads and deletes an OAuth `state` record,
// making it single-use: a replayed callback finds nothing and is rejected.
func (r *RedisClient) ConsumeOAuthState(ctx context.Context, state string) ([]byte, error) {
	if state == "" {
		return nil, goredis.Nil
	}
	val, err := r.client.GetDel(ctx, oauthStateKeyPrefix+state).Result()
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

// SetWatchPartyState зберігає стан кімнати в пам'яті Redis з TTL 2 години
func (r *RedisClient) SetWatchPartyState(ctx context.Context, roomCode string, state *domain.WatchPartyState) error {
	key := fmt.Sprintf("room:%s:state", roomCode)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, 2*time.Hour).Err()
}

// GetWatchPartyState отримує поточний стан кімнати
func (r *RedisClient) GetWatchPartyState(ctx context.Context, roomCode string) (*domain.WatchPartyState, error) {
	key := fmt.Sprintf("room:%s:state", roomCode)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var state domain.WatchPartyState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// PublishWatchPartyEvent транслює подію усім підключеним клієнтам через Redis Pub/Sub
func (r *RedisClient) PublishWatchPartyEvent(ctx context.Context, roomCode string, event *domain.WatchPartyEvent) error {
	channel := fmt.Sprintf("room:%s:events", roomCode)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.client.Publish(ctx, channel, data).Err()
}

// SubscribeWatchPartyEvents підписується на події кімнати
func (r *RedisClient) SubscribeWatchPartyEvents(ctx context.Context, roomCode string) *goredis.PubSub {
	channel := fmt.Sprintf("room:%s:events", roomCode)
	return r.client.Subscribe(ctx, channel)
}

func sessionSetKey(userID uuid.UUID) string {
	return sessionSetKeyPrefix + userID.String()
}
