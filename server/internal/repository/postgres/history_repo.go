package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HistoryRepository struct {
	pool *pgxpool.Pool
}

func NewHistoryRepository(pool *pgxpool.Pool) *HistoryRepository {
	return &HistoryRepository{pool: pool}
}

// UpsertWatchHistory зберігає або оновлює прогрес відтворення (ON CONFLICT DO UPDATE)
func (r *HistoryRepository) UpsertWatchHistory(ctx context.Context, h *domain.WatchHistory) error {
	if h == nil {
		return errors.New("upsert watch history: nil entry")
	}
	sanitiseHistory(h)
	h.WatchedAt = resolveWatchedAt(h.WatchedAt, time.Now())

	_, err := r.pool.Exec(ctx, watchHistoryUpsertQuery,
		h.UserID, h.MediaID, h.ProviderID, h.Title, h.PosterURL, h.Year, h.MediaType,
		h.Season, h.Episode, h.EpisodeTitle, h.PositionMs, h.DurationMs, h.LastStreamURL, h.Voiceover,
		h.Rating, h.RatingSource, h.WatchedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert watch history: %w", err)
	}
	return nil
}

// watchHistoryUpsertQuery writes the client's watched_at instead of the server
// clock, and makes the update monotonic.
//
// The conflict target matches `uq_user_history UNIQUE NULLS NOT DISTINCT
// (user_id, media_id, provider_id, season, episode)` from 000001: with NULLS NOT
// DISTINCT the (NULL, NULL) pair used for movies collapses to one row, which is
// what a plain UNIQUE would fail to do.
//
// Stamping NOW() and overwriting unconditionally is what let an offline device
// resurrect old progress: device B synced its 20% at T0, the server stored it as
// T2, and device A — whose row was newer — saw a "newer" cloudWatchedAt and rolled
// itself back to 20%. The WHERE clause makes a late-arriving stale row a no-op,
// so watched_at only ever moves forward.
const watchHistoryUpsertQuery = `
	INSERT INTO watch_history (
		user_id, media_id, provider_id, title, poster_url, year, media_type,
		season, episode, episode_title, position_ms, duration_ms, last_stream_url, voiceover, rating, rating_source, watched_at
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
	)
	ON CONFLICT (user_id, media_id, provider_id, season, episode)
	DO UPDATE SET
		title = EXCLUDED.title,
		poster_url = COALESCE(EXCLUDED.poster_url, watch_history.poster_url),
		episode_title = COALESCE(EXCLUDED.episode_title, watch_history.episode_title),
		position_ms = EXCLUDED.position_ms,
		duration_ms = EXCLUDED.duration_ms,
		last_stream_url = COALESCE(EXCLUDED.last_stream_url, watch_history.last_stream_url),
		voiceover = COALESCE(EXCLUDED.voiceover, watch_history.voiceover),
		rating = COALESCE(EXCLUDED.rating, watch_history.rating),
		rating_source = COALESCE(EXCLUDED.rating_source, watch_history.rating_source),
		watched_at = GREATEST(watch_history.watched_at, EXCLUDED.watched_at)
	WHERE EXCLUDED.watched_at >= watch_history.watched_at
`

// GetUserHistory повертає історію переглядів користувача
func (r *HistoryRepository) GetUserHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.WatchHistory, error) {
	limit, offset = normalisePage(limit, offset, defaultHistoryPageLimit)
	query := `
		SELECT id, user_id, media_id, provider_id, title, poster_url, year, media_type,
		       season, episode, episode_title, position_ms, duration_ms, last_stream_url, voiceover, rating, rating_source, watched_at
		FROM watch_history
		WHERE user_id = $1
		ORDER BY watched_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get user history: %w", err)
	}
	defer rows.Close()

	list, err := scanHistoryRows(rows)
	if err != nil {
		return nil, fmt.Errorf("get user history: %w", err)
	}
	return list, nil
}

// GetContinueWatching формує список «Продовжити перегляд» (прогрес 5%..95%)
func (r *HistoryRepository) GetContinueWatching(ctx context.Context, userID uuid.UUID, limit int) ([]domain.WatchHistory, error) {
	if limit <= 0 {
		limit = defaultHistoryPageLimit
	}
	// Integer arithmetic instead of CAST(...AS FLOAT)/CAST(...AS FLOAT): the
	// division made the predicate non-sargable (an index on position_ms cannot
	// be used across a per-row division) and cost two casts per row. The bounds
	// are inclusive and exactly equivalent to ratio BETWEEN 0.05 AND 0.95.
	query := `
		SELECT id, user_id, media_id, provider_id, title, poster_url, year, media_type,
		       season, episode, episode_title, position_ms, duration_ms, last_stream_url, voiceover, rating, rating_source, watched_at
		FROM watch_history
		WHERE user_id = $1
		  AND duration_ms > 0
		  AND position_ms > 0
		  AND position_ms * 100 >= duration_ms * 5
		  AND position_ms * 100 <= duration_ms * 95
		ORDER BY watched_at DESC, id DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get continue watching: %w", err)
	}
	defer rows.Close()

	list, err := scanHistoryRows(rows)
	if err != nil {
		return nil, fmt.Errorf("get continue watching: %w", err)
	}
	return list, nil
}

// ---- helpers ----------------------------------------------------------------

const defaultHistoryPageLimit = 50

// scanHistoryRows reads a full result set. rows.Err() must be checked: without
// it a connection that dies mid-iteration looks like a short history and the
// client caches the truncated list as complete.
func scanHistoryRows(rows pgx.Rows) ([]domain.WatchHistory, error) {
	var list []domain.WatchHistory
	for rows.Next() {
		var h domain.WatchHistory
		var poster, epTitle, streamURL, voice, rSource *string
		var yr *int
		var rtg *float64
		var s, e *int

		if err := rows.Scan(
			&h.ID, &h.UserID, &h.MediaID, &h.ProviderID, &h.Title, &poster, &yr, &h.MediaType,
			&s, &e, &epTitle, &h.PositionMs, &h.DurationMs, &streamURL, &voice, &rtg, &rSource, &h.WatchedAt,
		); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}

		if poster != nil {
			h.PosterURL = *poster
		}
		if epTitle != nil {
			h.EpisodeTitle = *epTitle
		}
		if streamURL != nil {
			h.LastStreamURL = *streamURL
		}
		if voice != nil {
			h.Voiceover = *voice
		}
		h.Year = yr
		h.Rating = rtg
		h.RatingSource = rSource
		h.Season = s
		h.Episode = e

		list = append(list, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// resolveWatchedAt keeps the client's timestamp but refuses to let a device with
// a wrong clock poison the monotonic update rule: a future watched_at would win
// every later comparison and freeze the row until wall-clock time caught up.
func resolveWatchedAt(watchedAt, now time.Time) time.Time {
	if watchedAt.IsZero() {
		return now
	}
	if watchedAt.After(now) {
		return now
	}
	return watchedAt
}

// sanitiseHistory clamps client-supplied values into the range the CHECK
// constraints in migration 000009 allow, so a buggy device produces a slightly
// wrong row instead of a rejected sync request.
func sanitiseHistory(h *domain.WatchHistory) {
	if h == nil {
		return
	}
	if h.PositionMs < 0 {
		h.PositionMs = 0
	}
	if h.DurationMs < 0 {
		h.DurationMs = 0
	}
	if h.DurationMs > 0 && h.PositionMs > h.DurationMs {
		h.PositionMs = h.DurationMs
	}
	h.Year = sanitiseYear(h.Year)
	h.Season = sanitiseEpisodeNumber(h.Season)
	h.Episode = sanitiseEpisodeNumber(h.Episode)
	h.Rating = sanitiseRating(h.Rating)
	h.MediaType = sanitiseMediaType(h.MediaType)
}

// sanitiseRating drops values outside 0..10 (and NaN/Inf, which PostgreSQL
// accepts as floats but which would poison every aggregate).
func sanitiseRating(rating *float64) *float64 {
	if rating == nil {
		return nil
	}
	if math.IsNaN(*rating) || math.IsInf(*rating, 0) || *rating < 0 || *rating > 10 {
		return nil
	}
	return rating
}

// sanitiseYear allows 0: it is this codebase's "year unknown" sentinel
// (ParseFlexibleYear returns 0), so forbidding it would break real inserts.
func sanitiseYear(year *int) *int {
	if year == nil {
		return nil
	}
	if *year < 0 || *year > 9999 {
		return nil
	}
	return year
}

// sanitiseEpisodeNumber drops non-positive season/episode numbers: episode 0 is
// a special ("prequel") value upstream but not a real row the UI can address,
// and the CHECK constraints require >= 1.
func sanitiseEpisodeNumber(n *int) *int {
	if n == nil {
		return nil
	}
	if *n < 1 {
		return nil
	}
	return n
}

func sanitiseMediaType(mediaType string) string {
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func normalisePage(limit, offset, fallback int) (int, int) {
	if limit <= 0 {
		limit = fallback
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
