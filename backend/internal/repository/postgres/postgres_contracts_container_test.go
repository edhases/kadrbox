package postgres_test

// Container-backed contract tests for the behaviours the repository layer
// promises but that nothing asserted before: the UNIQUE NULLS NOT DISTINCT
// upsert path, monotonicity, pagination edges, single-use tokens, NULL
// tolerance in scanUser, migration idempotency and the CHECK constraints.
//
// Requires a Postgres: postgres_container_test.go provides one (container or
// TEST_POSTGRES_DSN) and ctPool skips when neither is available. -short skips
// all of it.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/edhases/kadrbox-server/internal/repository/postgres"
	"github.com/google/uuid"
)

// ---- watch history ----------------------------------------------------------

// TestCtUpsertWatchHistoryIsIdempotentForMovies pins the UNIQUE NULLS NOT
// DISTINCT path, which had never executed before. A movie has season = episode =
// NULL, so a plain UNIQUE constraint treats every NULL as distinct and the
// ON CONFLICT target never matches: the "same" movie would be inserted once per
// sync, and the sync endpoint would keep growing the table with rows the user
// can never distinguish.
func TestCtUpsertWatchHistoryIsIdempotentForMovies(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
	})

	base := time.Now().Add(-time.Hour)
	// Two syncs of the same movie: identical (user, media, provider, NULL
	// season, NULL episode), different position and a later watched_at.
	first := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-movie", ProviderID: "uakino", Title: "Матриця",
		PositionMs: 50, DurationMs: 100, WatchedAt: base,
	}
	if err := repo.UpsertWatchHistory(ctx, first); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	second := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-movie", ProviderID: "uakino", Title: "Матриця",
		PositionMs: 90, DurationMs: 100, WatchedAt: base.Add(time.Minute),
	}
	if err := repo.UpsertWatchHistory(ctx, second); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM watch_history WHERE user_id = $1 AND media_id = 'ct-movie'`,
		userID).Scan(&count); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected NULL season/episode to collapse to exactly 1 row, got %d", count)
	}

	// ...and the newest write wins.
	list, err := repo.GetUserHistory(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("GetUserHistory failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
	if list[0].PositionMs != 90 {
		t.Errorf("position_ms = %d, want the newer 90", list[0].PositionMs)
	}
	if list[0].Season != nil || list[0].Episode != nil {
		t.Errorf("season/episode must stay NULL for a movie, got %v/%v", list[0].Season, list[0].Episode)
	}
}

// TestCtUpsertWatchHistoryDistinctEpisodesDoNotCollapse is the other half of the
// constraint: NULLS NOT DISTINCT must not over-collapse. Two episodes of the
// same series share (user, media, provider) but differ in (season, episode) and
// must remain separate rows.
func TestCtUpsertWatchHistoryDistinctEpisodesDoNotCollapse(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
	})

	for _, ep := range []int{1, 2, 3} {
		season := 1
		h := &domain.WatchHistory{
			UserID: userID, MediaID: "ct-series", ProviderID: "uakino", Title: "Серіал",
			Season: &season, Episode: &ep,
			PositionMs: 10, DurationMs: 100, WatchedAt: time.Now(),
		}
		if err := repo.UpsertWatchHistory(ctx, h); err != nil {
			t.Fatalf("upsert episode %d failed: %v", ep, err)
		}
	}

	// Re-syncing episode 2 must update it, not add a fourth row.
	season, ep := 1, 2
	h := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-series", ProviderID: "uakino", Title: "Серіал",
		Season: &season, Episode: &ep,
		PositionMs: 42, DurationMs: 100, WatchedAt: time.Now(),
	}
	if err := repo.UpsertWatchHistory(ctx, h); err != nil {
		t.Fatalf("re-upsert episode 2 failed: %v", err)
	}

	list, err := repo.GetUserHistory(ctx, userID, 50, 0)
	if err != nil {
		t.Fatalf("GetUserHistory failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 episode rows, got %d: %+v", len(list), list)
	}
	for _, row := range list {
		if row.Episode != nil && *row.Episode == 2 && row.PositionMs != 42 {
			t.Errorf("episode 2 position_ms = %d, want the updated 42", row.PositionMs)
		}
	}
}

// TestCtUpsertWatchHistoryIsMonotonic pins the cross-device regression. Device A
// watched to 90% at T1. Device B was offline, watched to 10% at T0 < T1, and
// reconnects after A. If B's stale row wins, A rolls the user's own progress
// backwards on the next sync — the exact symptom this WHERE clause exists to
// prevent.
func TestCtUpsertWatchHistoryIsMonotonic(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
	})

	newer := time.Now().Add(-time.Hour)
	older := newer.Add(-2 * time.Hour)

	a := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-mono", ProviderID: "uakino", Title: "t",
		PositionMs: 90, DurationMs: 100, WatchedAt: newer,
	}
	if err := repo.UpsertWatchHistory(ctx, a); err != nil {
		t.Fatalf("device A upsert failed: %v", err)
	}

	// Device B arrives late in wall-clock time but carries an older watched_at.
	b := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-mono", ProviderID: "uakino", Title: "t",
		PositionMs: 10, DurationMs: 100, WatchedAt: older,
	}
	if err := repo.UpsertWatchHistory(ctx, b); err != nil {
		t.Fatalf("stale upsert failed: %v", err)
	}

	list, err := repo.GetUserHistory(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("GetUserHistory failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
	if list[0].PositionMs != 90 {
		t.Errorf("stale sync regressed position to %d, want 90", list[0].PositionMs)
	}
	if !list[0].WatchedAt.Equal(newer.Truncate(time.Microsecond)) {
		t.Errorf("watched_at = %s, want the newer %s", list[0].WatchedAt, newer)
	}
}

// TestCtUpsertWatchHistoryZeroWatchedAtUsesServerClock covers the other
// monotonicity edge: a client with no clock sends the zero time, which must be
// replaced by now() rather than stored as year 1 (which would then beat nothing
// and freeze the row).
func TestCtUpsertWatchHistoryZeroWatchedAtUsesServerClock(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
	})

	before := time.Now().Add(-time.Second)
	h := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-clock", ProviderID: "uakino", Title: "t",
		PositionMs: 10, DurationMs: 100, // WatchedAt deliberately zero
	}
	if err := repo.UpsertWatchHistory(ctx, h); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if h.WatchedAt.Before(before) {
		t.Errorf("a zero WatchedAt must be replaced by the server clock, got %s", h.WatchedAt)
	}

	// A future client clock must not be stored verbatim either, or that row
	// would win every later comparison.
	future := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-future", ProviderID: "uakino", Title: "t",
		PositionMs: 10, DurationMs: 100, WatchedAt: time.Now().Add(72 * time.Hour),
	}
	if err := repo.UpsertWatchHistory(ctx, future); err != nil {
		t.Fatalf("future upsert failed: %v", err)
	}
	if future.WatchedAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("a future WatchedAt must be clamped to now, got %s", future.WatchedAt)
	}
}

// ---- pagination -------------------------------------------------------------

// TestCtGetUserHistoryPaginationBoundaries walks every boundary the sync client
// can send. The bug these guard against is a page that silently repeats or skips
// a row: watched_at ties are common (one sync stamps every row at once), so the
// ORDER BY needs the id tiebreak.
func TestCtGetUserHistoryPaginationBoundaries(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
	})

	const total = 5
	seed := time.Now().Add(-time.Hour)
	for i := 0; i < total; i++ {
		h := &domain.WatchHistory{
			UserID: userID, MediaID: fmt.Sprintf("ct-hist-%d", i), ProviderID: "uakino",
			Title: fmt.Sprintf("h%d", i), PositionMs: 10, DurationMs: 100,
			// Deliberately identical timestamps for the first three rows so the
			// id tiebreak in ORDER BY is what keeps the pages disjoint.
			WatchedAt: seed.Add(time.Duration(i/3) * time.Minute),
		}
		if err := repo.UpsertWatchHistory(ctx, h); err != nil {
			t.Fatalf("seed %d failed: %v", i, err)
		}
	}

	if got, err := repo.GetUserHistory(ctx, userID, 0, 0); err != nil {
		t.Errorf("limit=0 must fall back to the default page, got %v", err)
	} else if len(got) != total {
		t.Errorf("limit=0 returned %d rows, want all %d", len(got), total)
	}

	if got, err := repo.GetUserHistory(ctx, userID, -1, 0); err != nil {
		t.Errorf("limit=-1 must fall back to the default page, got %v", err)
	} else if len(got) != total {
		t.Errorf("limit=-1 returned %d rows, want all %d", len(got), total)
	}

	if got, err := repo.GetUserHistory(ctx, userID, 10, total); err != nil {
		t.Errorf("offset=count must be a valid empty page, got %v", err)
	} else if len(got) != 0 {
		t.Errorf("offset=count returned %d rows, want 0", len(got))
	}

	if got, err := repo.GetUserHistory(ctx, userID, 10, total+1); err != nil {
		t.Errorf("offset=count+1 must not error, got %v", err)
	} else if len(got) != 0 {
		t.Errorf("offset=count+1 returned %d rows, want 0", len(got))
	}

	if got, err := repo.GetUserHistory(ctx, userID, 10, -1); err != nil {
		t.Errorf("a negative offset must be clamped, got %v", err)
	} else if len(got) != total {
		t.Errorf("a negative offset returned %d rows, want %d", len(got), total)
	}

	// Two-page walk: no duplicates, no skips, union equals the single-page read.
	var walked []string
	for offset := 0; ; offset += 2 {
		page, err := repo.GetUserHistory(ctx, userID, 2, offset)
		if err != nil {
			t.Fatalf("page at offset %d failed: %v", offset, err)
		}
		if len(page) == 0 {
			break
		}
		for _, h := range page {
			walked = append(walked, h.MediaID)
		}
		if len(page) < 2 {
			break
		}
	}
	if len(walked) != total {
		t.Fatalf("two-page walk saw %d of %d rows: %v", len(walked), total, walked)
	}
	seen := map[string]bool{}
	for _, id := range walked {
		if seen[id] {
			t.Errorf("row %s returned on two pages — the ORDER BY is not total", id)
		}
		seen[id] = true
	}
}

// TestCtGetUserFavoritesPaginationBoundaries does the same for favorites, whose
// repository has different limit semantics from history: limit <= 0 means
// unlimited there, not "use the default".
func TestCtGetUserFavoritesPaginationBoundaries(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewFavoritesRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM favorites WHERE user_id = $1`, userID)
	})

	const total = 5
	for i := 0; i < total; i++ {
		if err := repo.AddFavorite(ctx, &domain.Favorite{
			UserID: userID, MediaID: fmt.Sprintf("ct-fav-%d", i), ProviderID: "uakino",
			Title: fmt.Sprintf("f%d", i), MediaType: "movie",
		}); err != nil {
			t.Fatalf("AddFavorite %d failed: %v", i, err)
		}
	}

	count, err := repo.CountUserFavorites(ctx, userID)
	if err != nil {
		t.Fatalf("CountUserFavorites failed: %v", err)
	}
	if count != total {
		t.Fatalf("CountUserFavorites = %d, want %d", count, total)
	}

	if got, err := repo.GetUserFavorites(ctx, userID, 0, 0); err != nil {
		t.Errorf("limit=0 must mean unlimited, got %v", err)
	} else if len(got) != total {
		t.Errorf("limit=0 returned %d rows, want all %d", len(got), total)
	}

	if got, err := repo.GetUserFavorites(ctx, userID, -1, 0); err != nil {
		t.Errorf("limit=-1 must mean unlimited, got %v", err)
	} else if len(got) != total {
		t.Errorf("limit=-1 returned %d rows, want all %d", len(got), total)
	}

	if got, err := repo.GetUserFavorites(ctx, userID, 10, total); err != nil {
		t.Errorf("offset=count must be a valid empty page, got %v", err)
	} else if len(got) != 0 {
		t.Errorf("offset=count returned %d rows, want 0", len(got))
	}

	if got, err := repo.GetUserFavorites(ctx, userID, 10, total+1); err != nil {
		t.Errorf("offset=count+1 must not error, got %v", err)
	} else if len(got) != 0 {
		t.Errorf("offset=count+1 returned %d rows, want 0", len(got))
	}

	if got, err := repo.GetUserFavorites(ctx, userID, 10, -1); err != nil {
		t.Errorf("a negative offset must be clamped, got %v", err)
	} else if len(got) != total {
		t.Errorf("a negative offset returned %d rows, want %d", len(got), total)
	}

	var walked []string
	for offset := 0; ; offset += 2 {
		page, err := repo.GetUserFavorites(ctx, userID, 2, offset)
		if err != nil {
			t.Fatalf("page at offset %d failed: %v", offset, err)
		}
		if len(page) == 0 {
			break
		}
		for _, f := range page {
			walked = append(walked, f.MediaID)
		}
		if len(page) < 2 {
			break
		}
	}
	if len(walked) != total {
		t.Fatalf("two-page walk saw %d of %d rows: %v", len(walked), total, walked)
	}
	seen := map[string]bool{}
	for _, id := range walked {
		if seen[id] {
			t.Errorf("row %s returned on two pages — the ORDER BY is not total", id)
		}
		seen[id] = true
	}
}

// ---- favorites idempotency --------------------------------------------------

// TestCtAddFavoriteTwiceIsIdempotentNotConflict documents the actual contract.
//
// NOTE: the remediation brief asked for `AddFavorite` twice to return
// ErrConflict. There is no such sentinel in package postgres, and the
// implementation is `ON CONFLICT (user_id, media_id, provider_id) DO NOTHING` —
// i.e. adding a favourite the user already has is a silent no-op, not an error.
// That is the correct behaviour for the client's retry path (a duplicated
// request after a lost response must not surface as a failure), so this test
// pins what the code does rather than what the brief assumed. Reported as a
// brief/code discrepancy; see the final report.
func TestCtAddFavoriteTwiceIsIdempotentNotConflict(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewFavoritesRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM favorites WHERE user_id = $1`, userID)
	})

	fav := &domain.Favorite{
		UserID: userID, MediaID: "ct-dup", ProviderID: "uakino", Title: "Дюна", MediaType: "movie",
	}
	if err := repo.AddFavorite(ctx, fav); err != nil {
		t.Fatalf("first AddFavorite failed: %v", err)
	}
	// A changed title on the replay must not overwrite the stored one either.
	replay := &domain.Favorite{
		UserID: userID, MediaID: "ct-dup", ProviderID: "uakino", Title: "Дюна: Частина друга", MediaType: "movie",
	}
	if err := repo.AddFavorite(ctx, replay); err != nil {
		t.Fatalf("a repeated AddFavorite must not error, got %v", err)
	}

	count, err := repo.CountUserFavorites(ctx, userID)
	if err != nil {
		t.Fatalf("CountUserFavorites failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after two adds, got %d", count)
	}
	got, err := repo.GetUserFavorites(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("GetUserFavorites failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].Title != "Дюна" {
		t.Errorf("DO NOTHING must not overwrite the stored title; got %q", got[0].Title)
	}
}

// TestCtRemoveFavoriteTwiceIsIdempotent asserts the other half: removing
// something that is not there is a no-op, not an error. Logout and
// "remove everything" flows call this unconditionally.
func TestCtRemoveFavoriteTwiceIsIdempotent(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewFavoritesRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()

	if err := repo.AddFavorite(ctx, &domain.Favorite{
		UserID: userID, MediaID: "ct-rm", ProviderID: "uakino", Title: "t", MediaType: "movie",
	}); err != nil {
		t.Fatalf("AddFavorite failed: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := repo.RemoveFavorite(ctx, userID, "ct-rm", "uakino"); err != nil {
			t.Fatalf("RemoveFavorite attempt %d failed: %v", attempt, err)
		}
	}
	isFav, err := repo.IsFavorite(ctx, userID, "ct-rm", "uakino")
	if err != nil {
		t.Fatalf("IsFavorite failed: %v", err)
	}
	if isFav {
		t.Error("expected the favourite to be gone after two removals")
	}

	// Removing a row that never existed is also a no-op.
	if err := repo.RemoveFavorite(ctx, userID, "ct-never-existed", "uakino"); err != nil {
		t.Errorf("removing an unknown favourite must be a no-op, got %v", err)
	}
}

// ---- password reset tokens --------------------------------------------------

// TestCtPasswordResetTokenIsSingleUse covers both halves of the token contract
// in one flow: expiry rejects the read, and consumption destroys the row so a
// replayed reset link cannot be used twice. The second half is the security
// property — a leaked reset email must be useless after the owner uses it.
func TestCtPasswordResetTokenIsSingleUse(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewUserRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM password_resets WHERE user_id = $1`, userID)
	})

	// --- expiry -------------------------------------------------------------
	if err := repo.CreatePasswordResetToken(ctx, userID, "ct-expired"); err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}
	got, err := repo.GetUserByPasswordResetToken(ctx, "ct-expired")
	if err != nil {
		t.Fatalf("a live token must resolve, got %v", err)
	}
	if got != userID {
		t.Errorf("token resolved to %s, want %s", got, userID)
	}

	// Age the row past its 1h TTL; NOW() is used rather than a literal so the
	// row is unambiguously in the past regardless of session time zone.
	if _, err := pool.Exec(ctx,
		`UPDATE password_resets SET expires_at = NOW() - INTERVAL '1 minute' WHERE token = 'ct-expired'`); err != nil {
		t.Fatalf("age the token failed: %v", err)
	}
	if _, err := repo.GetUserByPasswordResetToken(ctx, "ct-expired"); !errors.Is(err, postgres.ErrResetTokenNotFound) {
		t.Errorf("an expired token must not resolve, got %v", err)
	}
	if _, err := repo.ConsumePasswordResetTokenAndUpdatePassword(ctx, "ct-expired", "attackerhash"); !errors.Is(err, postgres.ErrResetTokenNotFound) {
		t.Errorf("an expired token must not be redeemable, got %v", err)
	}
	after, err := repo.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if after.PasswordHash != "hash" {
		t.Errorf("a failed redemption changed the password to %q", after.PasswordHash)
	}

	// --- single use ---------------------------------------------------------
	if err := repo.CreatePasswordResetToken(ctx, userID, "ct-live"); err != nil {
		t.Fatalf("CreatePasswordResetToken failed: %v", err)
	}
	if _, err := repo.ConsumePasswordResetTokenAndUpdatePassword(ctx, "ct-live", "newhash"); err != nil {
		t.Fatalf("first redemption failed: %v", err)
	}
	after, err = repo.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if after.PasswordHash != "newhash" {
		t.Errorf("password not updated, got %q", after.PasswordHash)
	}

	// The replay must be rejected *and* must not write anything.
	if _, err := repo.ConsumePasswordResetTokenAndUpdatePassword(ctx, "ct-live", "attackhash"); !errors.Is(err, postgres.ErrResetTokenNotFound) {
		t.Errorf("a replayed reset link must be rejected, got %v", err)
	}
	if _, err := repo.GetUserByPasswordResetToken(ctx, "ct-live"); !errors.Is(err, postgres.ErrResetTokenNotFound) {
		t.Errorf("a consumed token must not resolve afterwards, got %v", err)
	}
	after, err = repo.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if after.PasswordHash != "newhash" {
		t.Errorf("the replay changed the password to %q", after.PasswordHash)
	}

	for _, token := range []string{"", "   "} {
		if _, err := repo.ConsumePasswordResetTokenAndUpdatePassword(ctx, token, "x"); !errors.Is(err, postgres.ErrResetTokenNotFound) {
			t.Errorf("token %q must be rejected, got %v", token, err)
		}
	}
}

// ---- NULL tolerance ---------------------------------------------------------

// TestCtScanUserToleratesEveryNullableColumn exercises scanUser against a row
// where avatar_url, bio, telegram_id and discord_id are all NULL. Every
// authenticated read goes through this scan, and a nil-pointer dereference here
// is a 500 on the most common request in the API.
func TestCtScanUserToleratesEveryNullableColumn(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	email := fmt.Sprintf("ct_nulls_%d@x.com", time.Now().UnixNano())
	// Written with raw SQL so the NULLs are guaranteed rather than the result of
	// an omitempty-style omission in Go.
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, username, avatar_url, bio, telegram_id, discord_id, role, is_verified)
		 VALUES ($1, 'hash', 'nulls', NULL, NULL, NULL, NULL, 'user', FALSE)
		 RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert with NULLs failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})

	user, err := repo.GetUserByID(ctx, id)
	if err != nil {
		t.Fatalf("GetUserByID on an all-NULL row failed: %v", err)
	}
	if user.AvatarURL != "" || user.Bio != "" {
		t.Errorf("NULL text must scan to the zero value, got avatar=%q bio=%q", user.AvatarURL, user.Bio)
	}
	if user.TelegramID != nil {
		t.Errorf("NULL telegram_id must scan to nil, got %v", *user.TelegramID)
	}
	if user.DiscordID != nil {
		t.Errorf("NULL discord_id must scan to nil, got %v", *user.DiscordID)
	}
	if user.Role != "user" {
		t.Errorf("role = %q, want the 'user' default", user.Role)
	}
	if user.IsVerified {
		t.Error("a fresh row must not be verified")
	}

	// Now populate them and read again: the pointers must be filled in, not
	// silently dropped by a scan that assumed NULLs forever.
	telegram := int64(770000123)
	discord := "discord-ct"
	if _, err := pool.Exec(ctx,
		`UPDATE users SET avatar_url = 'https://cdn/x.png', bio = 'hi', telegram_id = $2, discord_id = $3 WHERE id = $1`,
		id, telegram, discord); err != nil {
		t.Fatalf("populate NULLs failed: %v", err)
	}
	user, err = repo.GetUserByID(ctx, id)
	if err != nil {
		t.Fatalf("GetUserByID after populating failed: %v", err)
	}
	if user.AvatarURL == "" || user.Bio == "" {
		t.Errorf("populated columns were dropped: avatar=%q bio=%q", user.AvatarURL, user.Bio)
	}
	if user.TelegramID == nil || *user.TelegramID != telegram {
		t.Errorf("telegram_id = %v, want %d", user.TelegramID, telegram)
	}
	if user.DiscordID == nil || *user.DiscordID != discord {
		t.Errorf("discord_id = %v, want %q", user.DiscordID, discord)
	}

	byTelegram, err := repo.GetUserByTelegramID(ctx, telegram)
	if err != nil {
		t.Fatalf("GetUserByTelegramID failed: %v", err)
	}
	if byTelegram.ID != id {
		t.Errorf("GetUserByTelegramID returned %s, want %s", byTelegram.ID, id)
	}
	byDiscord, err := repo.GetUserByDiscordID(ctx, discord)
	if err != nil {
		t.Fatalf("GetUserByDiscordID failed: %v", err)
	}
	if byDiscord.ID != id {
		t.Errorf("GetUserByDiscordID returned %s, want %s", byDiscord.ID, id)
	}

	// Unlinking must return the row to the all-NULL state without erroring.
	if err := repo.UnlinkTelegram(ctx, id); err != nil {
		t.Fatalf("UnlinkTelegram failed: %v", err)
	}
	if err := repo.UnlinkDiscord(ctx, id); err != nil {
		t.Fatalf("UnlinkDiscord failed: %v", err)
	}
	user, err = repo.GetUserByID(ctx, id)
	if err != nil {
		t.Fatalf("GetUserByID after unlinking failed: %v", err)
	}
	if user.TelegramID != nil || user.DiscordID != nil {
		t.Errorf("unlink left telegram=%v discord=%v", user.TelegramID, user.DiscordID)
	}
}

// ---- migrations -------------------------------------------------------------

// TestCtRunMigrationsOnEmptyDatabaseIsIdempotent boots InitDB against a
// genuinely empty database, then boots it a second time. Without the
// schema_migrations ledger the second boot re-executes every file, including
// the destructive one-shot in 000005 — which is how a re-synced row used to
// disappear on the next restart.
func TestCtRunMigrationsOnEmptyDatabaseIsIdempotent(t *testing.T) {
	// A genuinely empty database: every file in the embed FS runs here.
	dsn := ctFreshDatabase(t, "oxide_fresh")
	ctx := context.Background()

	first := ctPoolForDSN(t, dsn)
	before := ctMigrationLedger(t, first)
	if len(before) == 0 {
		t.Fatal("a first InitDB against an empty database must record every migration")
	}

	// Second boot against the same, now-migrated database: nothing must move.
	second := ctPoolForDSN(t, dsn)
	after := ctMigrationLedger(t, second)

	if len(after) != len(before) {
		t.Fatalf("ledger grew from %d to %d versions across a re-run", len(before), len(after))
	}
	for version, first := range before {
		second, ok := after[version]
		if !ok {
			t.Errorf("version %s disappeared from the ledger", version)
			continue
		}
		if !second.Equal(first) {
			t.Errorf("version %s was re-applied: applied_at moved from %s to %s", version, first, second)
		}
	}

	// Every embedded up-script must be recorded exactly once, with no extras.
	if len(after) == 0 {
		t.Fatal("expected a non-empty ledger")
	}
	var duplicates int
	if err := second.QueryRow(ctx,
		`SELECT COUNT(*) FROM (SELECT version FROM schema_migrations GROUP BY version HAVING COUNT(*) > 1) d`).
		Scan(&duplicates); err != nil {
		t.Fatalf("duplicate probe failed: %v", err)
	}
	if duplicates != 0 {
		t.Errorf("expected every version recorded exactly once, found %d duplicated", duplicates)
	}
}

// TestCtRunMigrationsIsSafeUnderConcurrentInitDB runs several InitDB calls at
// once against a fresh database. The advisory lock in runMigrations is the only
// thing standing between them; without it they race on CREATE TABLE / DROP
// COLUMN and one of them fails with "relation already exists".
func TestCtRunMigrationsIsSafeUnderConcurrentInitDB(t *testing.T) {
	// An unmigrated database, so the workers genuinely race to apply the schema
	// rather than all reading "already up to date".
	dsn := ctFreshDatabase(t, "oxide_race")

	const workers = 4
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			workerCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			pool, err := postgres.InitDB(workerCtx, dsn)
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

	// And the schema must be complete and singly-recorded afterwards.
	pool := ctPoolForDSN(t, dsn)
	ledger := ctMigrationLedger(t, pool)
	if len(ledger) == 0 {
		t.Error("expected a populated ledger after concurrent InitDB")
	}
	var duplicates int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM (SELECT version FROM schema_migrations GROUP BY version HAVING COUNT(*) > 1) d`).
		Scan(&duplicates); err != nil {
		t.Fatalf("duplicate probe failed: %v", err)
	}
	if duplicates != 0 {
		t.Errorf("found %d duplicated ledger rows", duplicates)
	}
}

// TestCtConcurrentUpsertsCollapseToOneRow exercises the unique constraint under
// contention: several goroutines syncing the same movie at once must not create
// duplicates or surface a unique-violation error to the client.
func TestCtConcurrentUpsertsCollapseToOneRow(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
	})

	const workers = 8
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			h := &domain.WatchHistory{
				UserID: userID, MediaID: "ct-race", ProviderID: "uakino", Title: "t",
				PositionMs: int64(10 * i), DurationMs: 100,
				WatchedAt: time.Now().Add(time.Duration(-i) * time.Second),
			}
			errCh <- repo.UpsertWatchHistory(ctx, h)
		}(i)
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent upsert %d failed: %v", i, err)
		}
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM watch_history WHERE user_id = $1 AND media_id = 'ct-race'`, userID).
		Scan(&count); err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after %d concurrent upserts, got %d", workers, count)
	}
}

// ---- email case-insensitivity -----------------------------------------------

// TestCtEmailIsCaseInsensitiveAndUnique asserts the database-level guarantee, not
// just the repository's normalisation. normalizeEmail lowercases on write, but a
// row inserted before that existed (or by raw SQL) is still reachable by a
// differently-cased request, so the unique index has to be on LOWER(email) too.
func TestCtEmailIsCaseInsensitiveAndUnique(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	mixed := fmt.Sprintf("Ct_Mixed_%d@Example.COM", time.Now().UnixNano())
	user, err := repo.CreateUser(ctx, mixed, "hash", "mixed")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	if user.Email != strings.ToLower(mixed) {
		t.Errorf("CreateUser must normalise the email, got %q want %q", user.Email, strings.ToLower(mixed))
	}

	// Every casing must resolve to the same account.
	for _, variant := range []string{
		strings.ToUpper(mixed),
		strings.ToLower(mixed),
		mixed,
		strings.ToUpper(mixed[:4]) + mixed[4:],
	} {
		got, err := repo.GetUserByEmail(ctx, variant)
		if err != nil {
			t.Errorf("GetUserByEmail(%q) failed: %v", variant, err)
			continue
		}
		if got.ID != user.ID {
			t.Errorf("GetUserByEmail(%q) returned %s, want %s", variant, got.ID, user.ID)
		}
	}

	// DOCUMENTED LIMITATION (low severity, not a regression): GetUserByEmail
	// matches with LOWER(email) = LOWER($1) but does not TRIM, whereas
	// CreateUser/normalizeEmail do trim on write. A client that sends a padded
	// address therefore gets ErrUserNotFound. The HTTP layer normalises the
	// account key before the rate limiter (auth_handler.go normalizeEmail), and
	// every current client sends a trimmed value, so this is unreachable in
	// practice; the repository simply is not symmetric. Pinned here so a future
	// change to either side is a deliberate decision rather than an accident.
	byPadded, err := repo.GetUserByEmail(ctx, "  "+strings.ToUpper(mixed)+"  ")
	if err == nil && byPadded.ID != user.ID {
		t.Errorf("a padded lookup must not resolve to a different account, got %s", byPadded.ID)
	}
	t.Logf("padded lookup behaviour (trim is not applied on read): err=%v", err)

	// A differently-cased duplicate must be refused by the repository...
	if _, err := repo.CreateUser(ctx, strings.ToUpper(mixed), "hash", "impostor"); !errors.Is(err, postgres.ErrUserAlreadyExists) {
		t.Errorf("expected ErrUserAlreadyExists for a case-variant email, got %v", err)
	}
	// ...and by the index itself, which is what protects a row inserted by raw
	// SQL or by an older release.
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, username) VALUES ($1, 'hash', 'raw')`,
		strings.ToUpper(mixed)); err == nil {
		t.Error("the unique index must reject a case-variant email inserted by raw SQL")
	}
}

// ---- CHECK constraints ------------------------------------------------------

// TestCtCheckConstraintsRejectOutOfRangeValues writes past the declared ranges
// with raw SQL, bypassing the repository's sanitising. The repository clamps
// these, so the constraints are a backstop; a backstop that never rejects is not
// a backstop.
func TestCtCheckConstraintsRejectOutOfRangeValues(t *testing.T) {
	pool := ctPool(t)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM favorites WHERE user_id = $1`, userID)
	})

	historyInserts := []struct {
		name  string
		query string
	}{
		{"negative position_ms",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms)
			 VALUES ($1, 'ct-ck', 'uakino', 't', -1, 100)`},
		{"negative duration_ms",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 10, -1)`},
		{"position beyond duration",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 200, 100)`},
		{"rating above 10",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, rating)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 0, 0, 99)`},
		{"rating below 0",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, rating)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 0, 0, -1)`},
		{"year above 9999",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, year)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 0, 0, 10000)`},
		{"season below 1",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, season, episode)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 0, 0, 0, 1)`},
		{"episode below 1",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, season, episode)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 0, 0, 1, 0)`},
		{"media_type that is not a slug",
			`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, media_type)
			 VALUES ($1, 'ct-ck', 'uakino', 't', 0, 0, 'Movie With Spaces')`},
	}
	for _, tc := range historyInserts {
		if _, err := pool.Exec(ctx, tc.query, userID); err == nil {
			t.Errorf("watch_history CHECK constraints accepted %s", tc.name)
		}
	}

	favoriteInserts := []struct {
		name  string
		query string
	}{
		{"rating above 10",
			`INSERT INTO favorites (user_id, media_id, provider_id, title, rating) VALUES ($1, 'ct-ck', 'uakino', 't', 11)`},
		{"year above 9999",
			`INSERT INTO favorites (user_id, media_id, provider_id, title, year) VALUES ($1, 'ct-ck', 'uakino', 't', 12345)`},
		{"media_type that is not a slug",
			`INSERT INTO favorites (user_id, media_id, provider_id, title, media_type) VALUES ($1, 'ct-ck', 'uakino', 't', 'TV Show!')`},
	}
	for _, tc := range favoriteInserts {
		if _, err := pool.Exec(ctx, tc.query, userID); err == nil {
			t.Errorf("favorites CHECK constraints accepted %s", tc.name)
		}
	}

	// The boundaries themselves must be accepted, or the constraints are too
	// strict and would reject legitimate client payloads.
	if _, err := pool.Exec(ctx,
		`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, rating, year, season, episode)
		 VALUES ($1, 'ct-ck-edge', 'uakino', 't', 100, 100, 10, 9999, 1, 1)`, userID); err != nil {
		t.Errorf("the maximum legal value must be accepted, got %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO watch_history (user_id, media_id, provider_id, title, position_ms, duration_ms, rating, year)
		 VALUES ($1, 'ct-ck-min', 'uakino', 't', 0, 0, 0, 0)`, userID); err != nil {
		t.Errorf("the minimum legal value (including year 0, the unknown-year sentinel) must be accepted, got %v", err)
	}
}

// TestCtRepositorySanitisesRatherThanRejects is the other side of the
// constraints: a buggy client must produce a slightly wrong row, not a failed
// sync request. A rejected write loses the client's whole sync batch.
func TestCtRepositorySanitisesRatherThanRejects(t *testing.T) {
	pool := ctPool(t)
	repo := postgres.NewHistoryRepository(pool)
	userID := ctUser(t, pool)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM watch_history WHERE user_id = $1`, userID)
	})

	outOfRangeYear := 12345
	outOfRangeRating := 42.0
	zeroSeason, zeroEpisode := 0, 0
	h := &domain.WatchHistory{
		UserID: userID, MediaID: "ct-dirty", ProviderID: "uakino", Title: "t",
		PositionMs: 500, DurationMs: 100, // position beyond duration
		Year: &outOfRangeYear, Rating: &outOfRangeRating,
		Season: &zeroSeason, Episode: &zeroEpisode,
		MediaType: "  MOVIE  ",
	}
	if err := repo.UpsertWatchHistory(ctx, h); err != nil {
		t.Fatalf("a malformed payload must not fail the sync, got %v", err)
	}
	if h.PositionMs != h.DurationMs {
		t.Errorf("position beyond duration must be clamped to %d, got %d", h.DurationMs, h.PositionMs)
	}
	if h.Year == nil || *h.Year != 9999 {
		t.Errorf("an out-of-range year must be clamped to 9999, got %v", h.Year)
	}
	if h.Rating != nil {
		t.Errorf("an out-of-range rating must be dropped, got %v", *h.Rating)
	}
	if h.Season != nil || h.Episode != nil {
		t.Errorf("season/episode 0 must become NULL, got %v/%v", h.Season, h.Episode)
	}
	if h.MediaType != "movie" {
		t.Errorf("media_type must be normalised to a lowercase slug, got %q", h.MediaType)
	}

	// And the row must actually be readable afterwards.
	list, err := repo.GetUserHistory(ctx, userID, 10, 0)
	if err != nil {
		t.Fatalf("GetUserHistory failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
}
