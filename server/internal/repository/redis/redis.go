package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *goredis.Client
}

func NewRedisClient(addr, password string) (*RedisClient, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis at %s: %w", addr, err)
	}

	return &RedisClient{client: client}, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

// StoreRefreshToken зберігає довготривалий refresh-токен на 30 днів
func (r *RedisClient) StoreRefreshToken(ctx context.Context, token string, userID uuid.UUID, ttl time.Duration) error {
	key := fmt.Sprintf("refresh:%s", token)
	return r.client.Set(ctx, key, userID.String(), ttl).Err()
}

// GetUserIDByRefreshToken перевіряє валідність refresh-токена
func (r *RedisClient) GetUserIDByRefreshToken(ctx context.Context, token string) (uuid.UUID, error) {
	key := fmt.Sprintf("refresh:%s", token)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(val)
}

// RevokeRefreshToken інвалідує сесію (наприклад, при Logout)
func (r *RedisClient) RevokeRefreshToken(ctx context.Context, token string) error {
	key := fmt.Sprintf("refresh:%s", token)
	return r.client.Del(ctx, key).Err()
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
