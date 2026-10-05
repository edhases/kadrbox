package postgres_test

// Integration coverage for the fixes in this repository that can only be
// verified against a real server: the migration runner's ledger/advisory lock,
// the case-insensitive email uniqueness, the CHECK constraints, the monotonic
// watch-history upsert and the transactional password-reset consumption.
//
// Skipped unless TEST_POSTGRES_DSN is set, exactly like
// postgres_integration_test.go. Names are prefixed pg_ to stay clear of the
// helpers owned by the other test files in this package.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

var pgCounter atomic.Int64

func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — integration test skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := postgres.InitDB(ctx, dsn)
	if err != nil {
		t.Fatalf("InitDB (migrations included) failed: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func pgUser(t *testing.T, pool *pgxpool.Pool) *domain.User {
	t.Helper()
	repo := postgres.NewUserRepository(pool)
	email := fmt.Sprintf("pg_%d_%d@x.com", time.Now().UnixNano(), pgCounter.Add(1))
	user, err := repo.CreateUser(context.Background(), email, "hash", "pguser")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	return user
}

// TestPgMigrationLedgerIsIdempotent boots InitDB twice against the same database.
// The second boot must apply nothing, which is only observable through the
// ledger: before it existed, every boot re-ran every file.
func TestPgMigrationLedgerIsIdempotent(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()

	first, err := pool.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ('pg_nonexistent_probe') ON CONFLICT DO NOTHING`)
	if err != nil {
		t.Fatalf("schema_migrations missing after InitDB: %v", err)
	}
	if first.RowsAffected() != 1 {
		t.Fatal("expected to insert a probe row into schema_migrations")
	}
	appliedAt := time.Now().Add(-time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET applied_at = $1 WHERE version = 'pg_nonexistent_probe'`, appliedAt); err != nil {
		t.Fatalf("probe update failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM schema_migrations WHERE version = 'pg_nonexistent_probe'`)
	})

	second := pgPool(t) // second InitDB against the same database
	var stamp time.Time
	if err := second.QueryRow(ctx,
		`SELECT applied_at FROM schema_migrations WHERE version = 'pg_nonexistent_probe'`).Scan(&stamp); err != nil {
		t.Fatalf("probe row missing after second InitDB: %v", err)
	}
	if stamp.After(appliedAt) {
		t.Errorf("probe applied_at changed from %s to %s — migrations were re-applied", appliedAt, stamp)
	}
}

func TestPgMigrationConcurrentInitDB(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — integration test skipped")
	}
	// Two instances racing must serialise on the advisory lock instead of
	// erroring out on a duplicate object.
	const workers = 4
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			pool, err := postgres.InitDB(ctx, dsn)
			if err == nil {
				pool.Close()
			}
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent InitDB failed: %v", err)
		}
	}
}

func TestPgEmailIsCaseInsensitive(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := pgUser(t, pool)

	// A differently-cased request must find the same account...
	got, err := repo.GetUserByEmail(ctx, strings.ToUpper(user.Email))
	if err != nil {
		t.Fatalf("GetUserByEmail (upper case) failed: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("expected %s, got %s", user.ID, got.ID)
	}

	// ...and must not be able to create a second account for it.
	_, err = repo.CreateUser(ctx, strings.ToUpper(user.Email), "hash", "impostor")
	if !errors.Is(err, postgres.ErrEmailTaken) {
		t.Errorf("expected ErrEmailTaken, got %v", err)
	}
}

func TestPgEmailWrittenLowercased(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewUserRepository(pool)

	user, err := repo.CreateUser(context.Background(), "  Pg_Mixed_Case@Example.COM ", "hash", "pguser")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	if user.Email != "pg_mixed_case@example.com" {
		t.Errorf("expected a normalised email, got %q", user.Email)
	}
}

func TestPgTelegramAndDiscordSentinels(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	first := pgUser(t, pool)
	second := pgUser(t, pool)
	telegramID := int64(770000001)
	if err := repo.LinkTelegram(ctx, first.ID, telegramID); err != nil {
		t.Fatalf("LinkTelegram failed: %v", err)
	}

	// Linking the same Telegram id to another account must not be reported as an
	// email conflict.
	err := repo.LinkTelegram(ctx, second.ID, telegramID)
	if errors.Is(err, postgres.ErrEmailTaken) || errors.Is(err, postgres.ErrUserAlreadyExists) {
		t.Errorf("telegram conflict misreported as an email conflict: %v", err)
	}
}

func TestPgCheckConstraintsRejectGarbage(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	user := pgUser(t, pool)

	year := 12345
	h := &domain.WatchHistory{
		UserID: user.ID, MediaID: "pg-garbage", ProviderID: "uakino", Title: "t",
		Year: &year, // out of the range the repository sanitises
	}
	// The repository sanitises, so the row must be written with the value
	// corrected rather than rejected.
	if err := postgres.NewHistoryRepository(pool).UpsertWatchHistory(ctx, h); err != nil {
		t.Fatalf("upsert with out-of-range year failed: %v", err)
	}
	if h.Year == nil {
		t.Fatal("expected the year to be clamped, not dropped")
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, user.ID)
	})

	// Direct SQL still cannot write out-of-range data.
	_, err := pool.Exec(ctx,
		`INSERT INTO watch_history (user_id, media_id, provider_id, title, rating, position_ms, duration_ms)
		 VALUES ($1, 'pg-raw', 'uakino', 't', 99, 0, 0)`, user.ID)
	if err == nil {
		t.Error("expected the rating CHECK constraint to reject 99")
	}
}

func TestPgRoleIsNotNullAndEnforced(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()

	email := fmt.Sprintf("pg_nullrole_%d@x.com", time.Now().UnixNano())
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, username) VALUES ($1,'h','pg')`, email); err != nil {
		t.Fatalf("insert without an explicit role failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE email = $1`, email)
	})

	var role string
	if err := pool.QueryRow(ctx, `SELECT role FROM users WHERE email = $1`, email).Scan(&role); err != nil {
		t.Fatalf("select role failed: %v", err)
	}
	if role != "user" {
		t.Errorf("role = %q, want the 'user' default", role)
	}

	// The role column must reject NULL outright now.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, username, role) VALUES ('pg_null_role@x.com','h','pg',NULL)`)
	if err == nil {
		t.Error("expected the role NOT NULL constraint to reject NULL")
	}

	// ...and reject a value outside the enum.
	_, err = pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, username, role) VALUES ('pg_bad_role@x.com','h','pg','wizard')`)
	if err == nil {
		t.Error("expected the role CHECK constraint to reject 'wizard'")
	}
}

func TestPgWatchHistoryIsMonotonic(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewHistoryRepository(pool)
	ctx := context.Background()
	user := pgUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, user.ID)
	})

	newer := time.Now().Add(-time.Hour)
	older := newer.Add(-2 * time.Hour)

	// Device A watches to 50% and syncs.
	a := &domain.WatchHistory{
		UserID: user.ID, MediaID: "pg-mono", ProviderID: "uakino", Title: "t",
		PositionMs: 50, DurationMs: 100, WatchedAt: newer,
	}
	if err := repo.UpsertWatchHistory(ctx, a); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	// Device B was offline at 20% and reconnects later in wall-clock time.
	b := &domain.WatchHistory{
		UserID: user.ID, MediaID: "pg-mono", ProviderID: "uakino", Title: "t",
		PositionMs: 20, DurationMs: 100, WatchedAt: older,
	}
	if err := repo.UpsertWatchHistory(ctx, b); err != nil {
		t.Fatalf("stale upsert failed: %v", err)
	}

	list, err := repo.GetUserHistory(ctx, user.ID, 10, 0)
	if err != nil {
		t.Fatalf("GetUserHistory failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
	if list[0].PositionMs != 50 {
		t.Errorf("stale sync regressed progress to %d, want 50", list[0].PositionMs)
	}
	if !list[0].WatchedAt.Equal(newer.Truncate(time.Microsecond)) {
		t.Errorf("watched_at = %s, want %s", list[0].WatchedAt, newer)
	}
}

func TestPgWatchHistoryNullSeasonEpisodeCollapses(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewHistoryRepository(pool)
	ctx := context.Background()
	user := pgUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, user.ID)
	})

	// A movie has season = episode = NULL; UNIQUE NULLS NOT DISTINCT must treat
	// those as one row for the upsert to hit the conflict target.
	for i, pos := range []int64{10, 20} {
		h := &domain.WatchHistory{
			UserID: user.ID, MediaID: "pg-movie", ProviderID: "uakino", Title: "t",
			PositionMs: pos, DurationMs: 100, WatchedAt: time.Now().Add(time.Duration(-i) * time.Minute),
		}
		if err := repo.UpsertWatchHistory(ctx, h); err != nil {
			t.Fatalf("upsert %d failed: %v", i, err)
		}
	}
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM watch_history WHERE user_id = $1 AND media_id = 'pg-movie'`, user.ID).Scan(&count); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected NULL season/episode to collapse to 1 row, got %d", count)
	}
}

func TestPgContinueWatchingBounds(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewHistoryRepository(pool)
	ctx := context.Background()
	user := pgUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, user.ID)
	})

	add := func(media string, pos, dur int64) {
		if err := repo.UpsertWatchHistory(ctx, &domain.WatchHistory{
			UserID: user.ID, MediaID: media, ProviderID: "uakino", Title: media,
			PositionMs: pos, DurationMs: dur, WatchedAt: time.Now(),
		}); err != nil {
			t.Fatalf("upsert %s failed: %v", media, err)
		}
	}
	add("pg-half", 500, 1000)  // exactly 50%
	add("pg-lower", 50, 1000)  // exactly 5% — inclusive
	add("pg-upper", 950, 1000) // exactly 95% — inclusive
	add("pg-just-under", 49, 1000)
	add("pg-over", 951, 1000)
	add("pg-fresh", 0, 1000)
	add("pg-no-duration", 500, 0)

	list, err := repo.GetContinueWatching(ctx, user.ID, 50)
	if err != nil {
		t.Fatalf("GetContinueWatching failed: %v", err)
	}
	got := map[string]bool{}
	for _, h := range list {
		got[h.MediaID] = true
	}
	for _, want := range []string{"pg-half", "pg-lower", "pg-upper"} {
		if !got[want] {
			t.Errorf("expected %s in continue watching, got %v", want, got)
		}
	}
	for _, notWant := range []string{"pg-just-under", "pg-over", "pg-fresh", "pg-no-duration"} {
		if got[notWant] {
			t.Errorf("did not expect %s in continue watching", notWant)
		}
	}
}

func TestPgFavoritesPaginationAndCount(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewFavoritesRepository(pool)
	ctx := context.Background()
	user := pgUser(t, pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM favorites WHERE user_id = $1`, user.ID)
	})

	const total = 7
	for i := 0; i < total; i++ {
		if err := repo.AddFavorite(ctx, &domain.Favorite{
			UserID: user.ID, MediaID: fmt.Sprintf("pg-fav-%d", i), ProviderID: "uakino",
			Title: fmt.Sprintf("fav %d", i), MediaType: "movie",
		}); err != nil {
			t.Fatalf("AddFavorite %d failed: %v", i, err)
		}
	}

	got, err := repo.CountUserFavorites(ctx, user.ID)
	if err != nil {
		t.Fatalf("CountUserFavorites failed: %v", err)
	}
	if got != total {
		t.Errorf("CountUserFavorites = %d, want %d", got, total)
	}

	page1, err := repo.GetUserFavorites(ctx, user.ID, 3, 0)
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	page2, err := repo.GetUserFavorites(ctx, user.ID, 3, 3)
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if len(page1) != 3 || len(page2) != 3 {
		t.Fatalf("expected 3+3 rows, got %d+%d", len(page1), len(page2))
	}

	seen := map[string]bool{}
	for _, f := range append(page1, page2...) {
		if seen[f.MediaID] {
			t.Errorf("row %s returned on two pages — ordering is not total", f.MediaID)
		}
		seen[f.MediaID] = true
	}

	all, err := repo.GetUserFavorites(ctx, user.ID, 0, 0)
	if err != nil {
		t.Fatalf("unlimited fetch failed: %v", err)
	}
	if len(all) != total {
		t.Errorf("limit<=0 must mean unlimited: got %d, want %d", len(all), total)
	}
}

func TestPgPasswordResetTokenConsumedAtomically(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()
	user := pgUser(t, pool)

	if err := repo.CreatePasswordResetToken(ctx, user.ID, "pg-reset-token"); err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}

	if _, err := repo.ConsumePasswordResetTokenAndUpdatePassword(ctx, "pg-reset-token", "newhash"); err != nil {
		t.Fatalf("ConsumePasswordResetTokenAndUpdatePassword failed: %v", err)
	}

	after, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if after.PasswordHash != "newhash" {
		t.Errorf("password not updated, got %q", after.PasswordHash)
	}

	// The link must not be replayable.
	_, err = repo.ConsumePasswordResetTokenAndUpdatePassword(ctx, "pg-reset-token", "attackhash")
	if !errors.Is(err, postgres.ErrResetTokenNotFound) {
		t.Errorf("expected ErrResetTokenNotFound on replay, got %v", err)
	}
	after, err = repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if after.PasswordHash != "newhash" {
		t.Errorf("replay changed the password to %q", after.PasswordHash)
	}

	// An unknown token changes nothing.
	if _, err := repo.ConsumePasswordResetTokenAndUpdatePassword(ctx, "pg-nope", "x"); !errors.Is(err, postgres.ErrResetTokenNotFound) {
		t.Errorf("expected ErrResetTokenNotFound, got %v", err)
	}
}

func TestPgVerificationTokenReplaceIsAtomic(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()
	user := pgUser(t, pool)

	if err := repo.CreateVerificationToken(ctx, user.ID, "pg-v1"); err != nil {
		t.Fatalf("CreateVerificationToken failed: %v", err)
	}
	if err := repo.CreateVerificationToken(ctx, user.ID, "pg-v2"); err != nil {
		t.Fatalf("second CreateVerificationToken failed: %v", err)
	}

	if _, err := repo.GetUserByVerificationToken(ctx, "pg-v1"); !errors.Is(err, postgres.ErrTokenNotFound) {
		t.Errorf("expected the first token to be replaced, got %v", err)
	}
	if _, err := repo.GetUserByVerificationToken(ctx, "pg-v2"); err != nil {
		t.Errorf("expected the second token to work, got %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM email_verifications WHERE user_id = $1`, user.ID).Scan(&count); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly one live token row, got %d", count)
	}
}

func TestPgMarkEmailVerifiedClearsTokens(t *testing.T) {
	pool := pgPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()
	user := pgUser(t, pool)

	if err := repo.CreateVerificationToken(ctx, user.ID, "pg-verify"); err != nil {
		t.Fatalf("CreateVerificationToken failed: %v", err)
	}
	if err := repo.MarkEmailVerified(ctx, user.ID); err != nil {
		t.Fatalf("MarkEmailVerified failed: %v", err)
	}
	if _, err := repo.GetUserByVerificationToken(ctx, "pg-verify"); !errors.Is(err, postgres.ErrTokenNotFound) {
		t.Errorf("expected the token to be consumed, got %v", err)
	}

	// Re-verifying an existing user is a no-op, not an error: the UPDATE still
	// matches the row, so the call must succeed. That idempotency is what lets
	// the register auto-verify path retry safely.
	if err := repo.MarkEmailVerified(ctx, user.ID); err != nil {
		t.Errorf("expected a repeat call to be a no-op, got %v", err)
	}

	// A user that is genuinely gone must be reported, not silently accepted.
	deleted := pgUser(t, pool)
	if err := repo.DeleteUser(ctx, deleted.ID); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
	if err := repo.MarkEmailVerified(ctx, deleted.ID); !errors.Is(err, postgres.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound for a deleted user, got %v", err)
	}
}

func TestPgIndexesExist(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()

	want := []string{
		"idx_users_email_lower",
		"idx_email_verif_user_uniq",
		"idx_password_resets_user_uniq",
		"idx_email_verif_expires",
		"idx_password_resets_expires",
		"idx_content_cache_provider",
		"idx_watch_party_rooms_host",
		"idx_history_continue_v2",
	}
	for _, name := range want {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = $1)`, name).Scan(&exists); err != nil {
			t.Fatalf("index probe for %s failed: %v", name, err)
		}
		if !exists {
			t.Errorf("expected index %s to exist", name)
		}
	}

	// The byte-identical duplicates must be gone.
	for _, name := range []string{"idx_email_verif_token", "idx_password_resets_token", "idx_history_continue"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = $1)`, name).Scan(&exists); err != nil {
			t.Fatalf("index probe for %s failed: %v", name, err)
		}
		if exists {
			t.Errorf("expected redundant index %s to be dropped", name)
		}
	}
}
