package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FavoritesRepository struct {
	pool *pgxpool.Pool
}

func NewFavoritesRepository(pool *pgxpool.Pool) *FavoritesRepository {
	return &FavoritesRepository{pool: pool}
}

// AddFavorite додає тайтл в обране
func (r *FavoritesRepository) AddFavorite(ctx context.Context, f *domain.Favorite) error {
	if f == nil {
		return errors.New("add favorite: nil favorite")
	}
	f.Year = sanitiseYear(f.Year)
	f.Rating = sanitiseRating(f.Rating)
	f.MediaType = sanitiseMediaType(f.MediaType)
	query := `
		INSERT INTO favorites (user_id, media_id, provider_id, title, poster_url, year, media_type, rating, rating_source, added_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (user_id, media_id, provider_id) DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, f.UserID, f.MediaID, f.ProviderID, f.Title, f.PosterURL, f.Year, f.MediaType, f.Rating, f.RatingSource)
	if err != nil {
		return fmt.Errorf("add favorite: %w", err)
	}
	return nil
}

// RemoveFavorite видаляє з обраного
func (r *FavoritesRepository) RemoveFavorite(ctx context.Context, userID uuid.UUID, mediaID, providerID string) error {
	query := `
		DELETE FROM favorites
		WHERE user_id = $1 AND media_id = $2 AND provider_id = $3
	`
	_, err := r.pool.Exec(ctx, query, userID, mediaID, providerID)
	if err != nil {
		return fmt.Errorf("remove favorite: %w", err)
	}
	return nil
}

// GetUserFavorites повертає сторінку закладок користувача.
//
// Pagination was missing entirely, so one heavy account made the endpoint
// return its whole table. limit <= 0 keeps the previous unlimited behaviour for
// callers that have not been updated yet. `id DESC` breaks added_at ties (all
// rows of one sync share a timestamp), otherwise the same row appears on two
// pages while another is skipped.
func (r *FavoritesRepository) GetUserFavorites(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Favorite, error) {
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, user_id, media_id, provider_id, title, poster_url, year, media_type, rating, rating_source, added_at
		FROM favorites
		WHERE user_id = $1
		ORDER BY added_at DESC, id DESC
	`
	args := []any{userID}
	if limit > 0 {
		query += ` LIMIT $2 OFFSET $3`
		args = append(args, limit, offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get user favorites: %w", err)
	}
	defer rows.Close()

	var list []domain.Favorite
	for rows.Next() {
		var f domain.Favorite
		var poster, rSource *string
		var yr *int
		var rtg *float64

		if err := rows.Scan(&f.ID, &f.UserID, &f.MediaID, &f.ProviderID, &f.Title, &poster, &yr, &f.MediaType, &rtg, &rSource, &f.AddedAt); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		if poster != nil {
			f.PosterURL = *poster
		}
		f.Year = yr
		f.Rating = rtg
		f.RatingSource = rSource
		list = append(list, f)
	}
	// Without this a connection lost mid-iteration is indistinguishable from a
	// complete result, and the client caches the short list as authoritative.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get user favorites: %w", err)
	}
	return list, nil
}

// CountUserFavorites повертає загальну кількість закладок, щоб клієнт міг
// визначити, чи є ще сторінки, без завантаження всіх рядків.
func (r *FavoritesRepository) CountUserFavorites(ctx context.Context, userID uuid.UUID) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM favorites WHERE user_id = $1`, userID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count user favorites: %w", err)
	}
	return total, nil
}

// IsFavorite перевіряє чи тайтл у збережених
func (r *FavoritesRepository) IsFavorite(ctx context.Context, userID uuid.UUID, mediaID, providerID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM favorites WHERE user_id = $1 AND media_id = $2 AND provider_id = $3
		)
	`
	var exists bool
	err := r.pool.QueryRow(ctx, query, userID, mediaID, providerID).Scan(&exists)
	return exists, err
}
