package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cacheDeleteBatchSize bounds one DELETE statement. The expiry sweep runs on a
// timer against a table that can hold the whole catalogue of every provider,
// so an unbounded DELETE can hold locks long enough to stall the very writes
// that are adding fresh rows.
const cacheDeleteBatchSize = 500

// cacheSweepMaxAttempts bounds the retry of a failed sweep batch. main.go
// ticks this worker on a fixed interval and discards the count on error, so
// without a retry a single transient failure silently defers the purge to the
// next tick and lets the table grow unbounded in the meantime.
const cacheSweepMaxAttempts = 3

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
		return fmt.Errorf("insert cache %q: %w", key, err)
	}
	return nil
}

// Get отримує закешовані дані, якщо вони ще не прострочені.
//
// Only a genuinely absent or expired row is a miss. Every other failure — a
// dead pool, pool exhaustion, a cancelled context, a scan type mismatch — is
// returned as an error, because the caller cannot act on a miss it cannot
// distinguish from an outage: it would turn a one-row cache read into a full
// multi-provider fan-out and amplify a cache-layer failure into a
// scraping-layer failure.
func (r *CacheRepository) Get(ctx context.Context, key string, target interface{}) (bool, error) {
	query := `
		SELECT data FROM content_cache
		WHERE cache_key = $1 AND expires_at > NOW()
	`
	var rawJSON []byte
	err := r.pool.QueryRow(ctx, query, key).Scan(&rawJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("read cache %q: %w", key, err)
	}

	if err := json.Unmarshal(rawJSON, target); err != nil {
		return false, fmt.Errorf("unmarshal cache %q: %w", key, err)
	}
	return true, nil
}

// DeleteExpired видаляє застарілі кеш-рядки (викликається фоновим воркером).
//
// Batched: a single unbounded DELETE over a large table is one long
// transaction holding row locks, and the sweep competes with the very writes
// that are populating the cache. Returns the total number of rows removed
// across all batches.
func (r *CacheRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `
		DELETE FROM content_cache
		WHERE ctid IN (
			SELECT ctid FROM content_cache
			WHERE expires_at < NOW()
			LIMIT $1
		)
	`

	var total int64
	for {
		rows, err := r.execDeleteBatch(ctx, query)
		if err != nil {
			return total, err
		}
		total += rows
		if rows < cacheDeleteBatchSize {
			return total, nil
		}
		if err := ctx.Err(); err != nil {
			return total, fmt.Errorf("cache sweep cancelled after %d rows: %w", total, err)
		}
	}
}

func (r *CacheRepository) execDeleteBatch(ctx context.Context, query string) (int64, error) {
	var lastErr error
	for attempt := 1; attempt <= cacheSweepMaxAttempts; attempt++ {
		cmdTag, err := r.pool.Exec(ctx, query, cacheDeleteBatchSize)
		if err == nil {
			return cmdTag.RowsAffected(), nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return 0, fmt.Errorf("delete expired cache after %d attempts: %w", cacheSweepMaxAttempts, lastErr)
}
