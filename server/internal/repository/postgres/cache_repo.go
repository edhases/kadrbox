package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CacheRepository struct {
	pool *pgxpool.Pool
}

func NewCacheRepository(pool *pgxpool.Pool) *CacheRepository {
	return &CacheRepository{pool: pool}
}

// Set зберігає довільні JSON-дані парсингу в таблицю content_cache
func (r *CacheRepository) Set(ctx context.Context, key, providerID, contentType string, data interface{}, ttl time.Duration) error {
	rawJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal cache data: %w", err)
	}

	query := `
		INSERT INTO content_cache (cache_key, provider_id, content_type, data, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (cache_key)
		DO UPDATE SET
			data = EXCLUDED.data,
			expires_at = EXCLUDED.expires_at,
			created_at = NOW()
	`
	expiresAt := time.Now().Add(ttl)
	_, err = r.pool.Exec(ctx, query, key, providerID, contentType, rawJSON, expiresAt)
	if err != nil {
		return fmt.Errorf("insert cache: %w", err)
	}
	return nil
}

// Get отримує закешовані дані, якщо вони ще не прострочені
func (r *CacheRepository) Get(ctx context.Context, key string, target interface{}) (bool, error) {
	query := `
		SELECT data FROM content_cache
		WHERE cache_key = $1 AND expires_at > NOW()
	`
	var rawJSON []byte
	err := r.pool.QueryRow(ctx, query, key).Scan(&rawJSON)
	if err != nil {
		return false, nil // Cache miss
	}

	if err := json.Unmarshal(rawJSON, target); err != nil {
		return false, fmt.Errorf("unmarshal cache data: %w", err)
	}
	return true, nil
}

// DeleteExpired видаляє застарілі кеш-рядки (викликається фоновим воркером)
func (r *CacheRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM content_cache WHERE expires_at < NOW()`
	cmdTag, err := r.pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("delete expired cache: %w", err)
	}
	return cmdTag.RowsAffected(), nil
}
