package redis_test

// Тести Redis-клієнта без зовнішнього сервера: miniredis (in-process)
// слухає реальний localhost-порт, тож NewRedisClient проходить Ping
// і всі команди йдуть справжнім RESP-протоколом через go-redis.

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edhases/oxide-server/internal/domain"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

func covMiniRedis(t *testing.T) (*miniredis.Miniredis, *redisRepo.RedisClient) {
	t.Helper()
	m, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(m.Close)
	c, err := redisRepo.NewRedisClient(m.Addr(), "")
	if err != nil {
		t.Fatalf("failed to connect to miniredis: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return m, c
}

func TestCovRedisRefreshTokenRoundTrip(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx := context.Background()
	id := uuid.New()

	if err := c.StoreRefreshToken(ctx, "tok-1", id, time.Hour); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	got, err := c.GetUserIDByRefreshToken(ctx, "tok-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got != id {
		t.Errorf("expected %s, got %s", id, got)
	}
}

func TestCovRedisRefreshTokenMissing(t *testing.T) {
	_, c := covMiniRedis(t)
	if _, err := c.GetUserIDByRefreshToken(context.Background(), "nope"); err == nil {
		t.Error("expected error for unknown token, got nil")
	}
}

func TestCovRedisRefreshTokenMalformed(t *testing.T) {
	m, c := covMiniRedis(t)
	// Сміття напряму в сховище: uuid.Parse має впасти.
	if err := m.Set("refresh:bad", "not-a-uuid"); err != nil {
		t.Fatalf("miniredis set failed: %v", err)
	}
	if _, err := c.GetUserIDByRefreshToken(context.Background(), "bad"); err == nil {
		t.Error("expected uuid parse error, got nil")
	}
}

func TestCovRedisRevoke(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx := context.Background()
	id := uuid.New()

	if err := c.StoreRefreshToken(ctx, "tok-r", id, time.Hour); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	if err := c.RevokeRefreshToken(ctx, "tok-r"); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if _, err := c.GetUserIDByRefreshToken(ctx, "tok-r"); err == nil {
		t.Error("expected error after revoke, got nil")
	}
	// Revoke неіснуючого — не помилка (DEL).
	if err := c.RevokeRefreshToken(ctx, "tok-r"); err != nil {
		t.Errorf("revoke of missing key failed: %v", err)
	}
}

func TestCovRedisWatchPartyStateRoundTrip(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx := context.Background()

	want := &domain.WatchPartyState{
		HostID: "u1", MediaID: "m1", StreamURL: "https://cdn/x.m3u8",
		CurrentPositionMs: 12345, IsPlaying: true, PlaybackSpeed: 1.5,
		UpdatedAtEpoch: 1700000000,
	}
	if err := c.SetWatchPartyState(ctx, "ROOM1", want); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	got, err := c.GetWatchPartyState(ctx, "ROOM1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("state mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestCovRedisWatchPartyStateMissing(t *testing.T) {
	_, c := covMiniRedis(t)
	if _, err := c.GetWatchPartyState(context.Background(), "NOPE"); err == nil {
		t.Error("expected error for unknown room, got nil")
	}
}

func TestCovRedisPublish(t *testing.T) {
	_, c := covMiniRedis(t)
	ev := &domain.WatchPartyEvent{Action: "play", RoomCode: "R", SenderID: "u1"}
	if err := c.PublishWatchPartyEvent(context.Background(), "R", ev); err != nil {
		t.Errorf("publish failed: %v", err)
	}
}

func TestCovRedisPubSub(t *testing.T) {
	_, c := covMiniRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub := c.SubscribeWatchPartyEvents(ctx, "R2")
	defer func() { _ = sub.Close() }()

	// Підписка асинхронна: даємо їй встигнути до публікації.
	time.Sleep(200 * time.Millisecond)

	sent := &domain.WatchPartyEvent{Action: "chat", RoomCode: "R2", SenderID: "u9"}
	if err := c.PublishWatchPartyEvent(ctx, "R2", sent); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("no chat message received in time")
		}
		msg, err := sub.ReceiveTimeout(ctx, 500*time.Millisecond)
		if err != nil {
			continue // таймаут вікна — чекаємо далі до дедлайну
		}
		rm, ok := msg.(*goredis.Message)
		if !ok {
			continue // *redis.Subscription (підтвердження підписки) — пропускаємо
		}
		if rm.Channel != "room:R2:events" {
			t.Errorf("unexpected channel: %q", rm.Channel)
		}
		if !strings.Contains(rm.Payload, `"action":"chat"`) {
			t.Errorf("unexpected payload: %s", rm.Payload)
		}
		return
	}
}

func TestCovRedisConnectFailure(t *testing.T) {
	// Порт 1 відхиляє з'єднання: Ping падає, конструктор повертає помилку.
	if _, err := redisRepo.NewRedisClient("127.0.0.1:1", ""); err == nil {
		t.Error("expected connection error, got nil")
	}
}
