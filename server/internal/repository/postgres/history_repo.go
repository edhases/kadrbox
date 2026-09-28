package postgres

import (
	"context"
	"fmt"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
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
	query := `
		INSERT INTO watch_history (
			user_id, media_id, provider_id, title, poster_url, year, media_type,
			season, episode, episode_title, position_ms, duration_ms, last_stream_url, voiceover, rating, rating_source, watched_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW()
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
			watched_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query,
		h.UserID, h.MediaID, h.ProviderID, h.Title, h.PosterURL, h.Year, h.MediaType,
		h.Season, h.Episode, h.EpisodeTitle, h.PositionMs, h.DurationMs, h.LastStreamURL, h.Voiceover,
		h.Rating, h.RatingSource,
	)
	if err != nil {
		return fmt.Errorf("upsert watch history: %w", err)
	}
	return nil
}

// GetUserHistory повертає історію переглядів користувача
func (r *HistoryRepository) GetUserHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.WatchHistory, error) {
	query := `
		SELECT id, user_id, media_id, provider_id, title, poster_url, year, media_type,
		       season, episode, episode_title, position_ms, duration_ms, last_stream_url, voiceover, rating, rating_source, watched_at
		FROM watch_history
		WHERE user_id = $1
		ORDER BY watched_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get user history: %w", err)
	}
	defer rows.Close()

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

	return list, nil
}

// GetContinueWatching формує список «Продовжити перегляд» (прогрес 5%..95%)
func (r *HistoryRepository) GetContinueWatching(ctx context.Context, userID uuid.UUID, limit int) ([]domain.WatchHistory, error) {
	query := `
		SELECT id, user_id, media_id, provider_id, title, poster_url, year, media_type,
		       season, episode, episode_title, position_ms, duration_ms, last_stream_url, voiceover, rating, rating_source, watched_at
		FROM watch_history
		WHERE user_id = $1
		  AND duration_ms > 0
		  AND position_ms > 0
		  AND (CAST(position_ms AS FLOAT) / CAST(duration_ms AS FLOAT)) BETWEEN 0.05 AND 0.95
		ORDER BY watched_at DESC
		LIMIT $2
	`
	rows, err := r.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get continue watching: %w", err)
	}
	defer rows.Close()

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
			return nil, fmt.Errorf("scan continue watching: %w", err)
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

	return list, nil
}
