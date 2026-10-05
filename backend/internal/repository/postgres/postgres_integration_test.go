package postgres_test

// Інтеграційні тести репозиторіїв проти справжнього PostgreSQL 16.
// Запуск локально: підняти postgres:16 і задати TEST_POSTGRES_DSN,
// інакше тести пропускаються (t.Skip). У CI їх виконує job із services.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/auth"
	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/edhases/kadrbox-server/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var covPgCounter atomic.Int64

func covTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.InitDB(ctx, dsn)
	if err != nil {
		t.Fatalf("InitDB (migrations included) failed: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func covUniqueEmail(t *testing.T) string {
	t.Helper()
	n := covPgCounter.Add(1)
	safe := strings.ReplaceAll(t.Name(), "/", "_")
	return fmt.Sprintf("cov_%d_%d_%s@x.com", time.Now().UnixNano(), n, safe)
}

func covCreateUser(t *testing.T, repo *postgres.UserRepository) *domain.User {
	t.Helper()
	hash, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	user, err := repo.CreateUser(context.Background(), covUniqueEmail(t), hash, "covuser")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	return user
}

func TestCovPgInitDBInvalidDSN(t *testing.T) {
	// Без БД: битий DSN дає помилку ще на парсингу/підключенні.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := postgres.InitDB(ctx, "postgres://127.0.0.1:1/nope?sslmode=disable"); err == nil {
		t.Error("expected InitDB error for unreachable host, got nil")
	}
}

func TestCovPgUserCreateGet(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := covCreateUser(t, repo)
	if user.ID == uuidNil() {
		t.Error("expected non-zero user id")
	}
	if user.Role != "user" {
		t.Errorf("expected role user, got %q", user.Role)
	}
	if user.IsVerified {
		t.Error("expected IsVerified=false for fresh user")
	}

	byEmail, err := repo.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if byEmail.ID != user.ID || byEmail.Username != user.Username {
		t.Errorf("mismatch: %+v vs %+v", byEmail, user)
	}

	byID, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if byID.Email != user.Email {
		t.Errorf("mismatch: %+v vs %+v", byID, user)
	}
}

func TestCovPgUserDuplicate(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewUserRepository(pool)

	user := covCreateUser(t, repo)
	_, err := repo.CreateUser(context.Background(), user.Email, "hash", "other")
	if !errors.Is(err, postgres.ErrUserAlreadyExists) {
		t.Errorf("expected ErrUserAlreadyExists, got %v", err)
	}
}

func TestCovPgUserNotFound(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	if _, err := repo.GetUserByEmail(ctx, "definitely-missing@x.com"); !errors.Is(err, postgres.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
	if _, err := repo.GetUserByID(ctx, newUUID()); !errors.Is(err, postgres.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestCovPgVerificationFlow(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := covCreateUser(t, repo)

	if err := repo.CreateVerificationToken(ctx, user.ID, "tok-abc"); err != nil {
		t.Fatalf("CreateVerificationToken failed: %v", err)
	}
	gotID, err := repo.GetUserByVerificationToken(ctx, "tok-abc")
	if err != nil {
		t.Fatalf("GetUserByVerificationToken failed: %v", err)
	}
	if gotID != user.ID {
		t.Errorf("expected %s, got %s", user.ID, gotID)
	}

	// Заміна токена видаляє попередній.
	if err := repo.CreateVerificationToken(ctx, user.ID, "tok-def"); err != nil {
		t.Fatalf("second CreateVerificationToken failed: %v", err)
	}
	if _, err := repo.GetUserByVerificationToken(ctx, "tok-abc"); !errors.Is(err, postgres.ErrTokenNotFound) {
		t.Errorf("expected old token gone, got %v", err)
	}

	if _, err := repo.GetUserByVerificationToken(ctx, "missing"); !errors.Is(err, postgres.ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}

	if err := repo.MarkEmailVerified(ctx, user.ID); err != nil {
		t.Fatalf("MarkEmailVerified failed: %v", err)
	}
	after, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if !after.IsVerified {
		t.Error("expected IsVerified=true after MarkEmailVerified")
	}
	// Верифікація чистить токени юзера.
	if _, err := repo.GetUserByVerificationToken(ctx, "tok-def"); !errors.Is(err, postgres.ErrTokenNotFound) {
		t.Errorf("expected tokens cleaned, got %v", err)
	}
}

func TestCovPgHistoryUpsertGet(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userRepo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := covCreateUser(t, userRepo)
	yr := 1999
	h := &domain.WatchHistory{
		UserID: user.ID, MediaID: "m1", ProviderID: "uakino", Title: "Матриця",
		Year: &yr, MediaType: "movie", PositionMs: 50, DurationMs: 100,
	}

	if err := repo.UpsertWatchHistory(ctx, h); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	// Повторний upsert за тим самим ключем — оновлення, не новий рядок.
	h.PositionMs = 70
	if err := repo.UpsertWatchHistory(ctx, h); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	list, err := repo.GetUserHistory(ctx, user.ID, 50, 0)
	if err != nil {
		t.Fatalf("GetUserHistory failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row after upserts, got %d", len(list))
	}
	got := list[0]
	if got.PositionMs != 70 || got.Title != "Матриця" {
		t.Errorf("unexpected row: %+v", got)
	}
	if got.Year == nil || *got.Year != 1999 || got.Season != nil || got.Episode != nil {
		t.Errorf("unexpected nullable fields: %+v", got)
	}

	// Серія серіалу: season/episode заповнені.
	s, e := 2, 5
	ep := &domain.WatchHistory{
		UserID: user.ID, MediaID: "s1", ProviderID: "uakino", Title: "Серіал",
		MediaType: "series", Season: &s, Episode: &e,
		PositionMs: 10, DurationMs: 100,
	}
	if err := repo.UpsertWatchHistory(ctx, ep); err != nil {
		t.Fatalf("episode upsert failed: %v", err)
	}
	list, err = repo.GetUserHistory(ctx, user.ID, 50, 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("expected 2 rows, got %d (%v)", len(list), err)
	}
}

func TestCovPgContinueWatching(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userRepo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := covCreateUser(t, userRepo)
	mk := func(media string, pos, dur int64) {
		if err := repo.UpsertWatchHistory(ctx, &domain.WatchHistory{
			UserID: user.ID, MediaID: media, ProviderID: "uakino", Title: media,
			PositionMs: pos, DurationMs: dur,
		}); err != nil {
			t.Fatalf("upsert %s failed: %v", media, err)
		}
	}
	mk("half", 50, 100) // 50% — всередині 5%..95%
	mk("fresh", 0, 100) // 0% — поза межами
	mk("done", 99, 100) // 99% — поза межами
	mk("nodur", 50, 0)  // duration 0 — поза межами

	list, err := repo.GetContinueWatching(ctx, user.ID, 20)
	if err != nil {
		t.Fatalf("GetContinueWatching failed: %v", err)
	}
	if len(list) != 1 || list[0].MediaID != "half" {
		t.Errorf("expected only [half], got %+v", list)
	}
}

func TestCovPgFavorites(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewFavoritesRepository(pool)
	userRepo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := covCreateUser(t, userRepo)
	fav := &domain.Favorite{
		UserID: user.ID, MediaID: "m1", ProviderID: "uakino", Title: "Матриця",
		MediaType: "movie",
	}

	isFav, err := repo.IsFavorite(ctx, user.ID, "m1", "uakino")
	if err != nil || isFav {
		t.Fatalf("expected not favorite initially (%v, %v)", isFav, err)
	}
	if err := repo.AddFavorite(ctx, fav); err != nil {
		t.Fatalf("AddFavorite failed: %v", err)
	}
	// Повторне додавання — DO NOTHING, без помилки і без дубліката.
	if err := repo.AddFavorite(ctx, fav); err != nil {
		t.Fatalf("second AddFavorite failed: %v", err)
	}
	if isFav, err := repo.IsFavorite(ctx, user.ID, "m1", "uakino"); err != nil || !isFav {
		t.Fatalf("expected favorite (%v, %v)", isFav, err)
	}
	list, err := repo.GetUserFavorites(ctx, user.ID, 50, 0)
	if err != nil || len(list) != 1 || list[0].Title != "Матриця" {
		t.Fatalf("unexpected favorites: %+v (%v)", list, err)
	}
	if err := repo.RemoveFavorite(ctx, user.ID, "m1", "uakino"); err != nil {
		t.Fatalf("RemoveFavorite failed: %v", err)
	}
	if isFav, err := repo.IsFavorite(ctx, user.ID, "m1", "uakino"); err != nil || isFav {
		t.Fatalf("expected removed (%v, %v)", isFav, err)
	}
}

func TestCovPgCacheSetGet(t *testing.T) {
	pool := covTestPool(t)
	repo := postgres.NewCacheRepository(pool)
	ctx := context.Background()

	type payload struct {
		Title string `json:"title"`
		Year  int    `json:"year"`
	}
	key := fmt.Sprintf("cov_%d", time.Now().UnixNano())
	want := payload{Title: "Дюна", Year: 2021}

	if err := repo.Set(ctx, key, "uakino", "search", want, time.Hour); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	var got payload
	hit, err := repo.Get(ctx, key, &got)
	if err != nil || !hit {
		t.Fatalf("expected hit (%v, %v)", hit, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mismatch: %+v vs %+v", got, want)
	}

	// Перезапис того самого ключа.
	want.Year = 2024
	if err := repo.Set(ctx, key, "uakino", "search", want, time.Hour); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}
	var got2 payload
	if hit, err := repo.Get(ctx, key, &got2); err != nil || !hit || got2.Year != 2024 {
		t.Errorf("overwrite not visible: %+v (%v, %v)", got2, hit, err)
	}

	// Прострочений запис — cache miss.
	expKey := key + "_exp"
	if err := repo.Set(ctx, expKey, "uakino", "search", want, -time.Hour); err != nil {
		t.Fatalf("Set expired failed: %v", err)
	}
	var dummy payload
	if hit, err := repo.Get(ctx, expKey, &dummy); err != nil || hit {
		t.Errorf("expected miss for expired (%v, %v)", hit, err)
	}

	if hit, err := repo.Get(ctx, "cov_missing_key", &dummy); err != nil || hit {
		t.Errorf("expected miss for unknown key (%v, %v)", hit, err)
	}

	n, err := repo.DeleteExpired(ctx)
	if err != nil || n < 1 {
		t.Errorf("expected ≥1 purged row (%d, %v)", n, err)
	}
}
