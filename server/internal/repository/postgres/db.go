package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

const (
	migrationsDir = "migrations"

	upSuffix   = ".up.sql"
	downSuffix = ".down.sql"

	// migrationsLockID is an arbitrary but stable key for the session-level
	// advisory lock that serialises the migration run between instances. It is
	// scoped to this database, so unrelated databases on the same cluster are
	// unaffected.
	migrationsLockID int64 = 7_243_118_506_517

	// A statement that runs longer than this is almost always a bad plan or a
	// stuck lock rather than real work, and it would otherwise hold a pooled
	// connection (and, inside a migration, the advisory lock) indefinitely.
	defaultStatementTimeout = 15 * time.Second

	// Migrations get a much larger budget: creating an index or a constraint on
	// a large production table legitimately takes minutes, and the statement
	// timeout above must not abort the schema upgrade it exists to protect.
	defaultMigrationTimeout = 10 * time.Minute

	defaultMaxConnLifetime   = 30 * time.Minute
	defaultMaxConnIdleTime   = 5 * time.Minute
	defaultHealthCheckPeriod = time.Minute
	defaultConnectTimeout    = 10 * time.Second
)

var (
	// migrationBasePattern constrains embedded file names to NNNNNN_snake_case so
	// that sorting by version is chronological and a stray file cannot be applied
	// or silently skipped.
	migrationBasePattern = regexp.MustCompile(`^[0-9]{6}_[a-z0-9_]+$`)
)

// schemaMigrationsDDL is the version ledger. Without it every process start
// re-executed every migration file — including the destructive one-shot in
// 000005, which made a re-synced row disappear again on the next restart.
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

// migration is a single embedded "up" script. Version is the base name without
// the direction suffix ("000001_init"), which is what lands in schema_migrations.
type migration struct {
	Version string
	Name    string
	SQL     []byte
}

// InitDB створює пул з'єднань pgx та автоматично виконує міграції з пам'яті
func InitDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database DSN: %w", err)
	}

	applyPoolLimits(config)

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Автоматичний накат вбудованих міграцій
	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return pool, nil
}

// applyPoolLimits turns the pool from hardcoded literals into bounded,
// self-healing configuration. Every knob is read straight from the environment
// (no cross-package config change needed); a missing or unparsable value falls
// back to a default derived from the machine size.
func applyPoolLimits(config *pgxpool.Config) {
	config.MaxConns = int32(envInt("DB_MAX_CONNS", defaultMaxConns()))
	config.MinConns = int32(envInt("DB_MIN_CONNS", defaultMinConns()))
	if config.MinConns > config.MaxConns {
		config.MinConns = config.MaxConns
	}

	// Without a finite lifetime the pool keeps handing out connections the far
	// side has already closed or that sit behind a broken NAT/firewall.
	config.MaxConnLifetime = envDuration("DB_MAX_CONN_LIFETIME", defaultMaxConnLifetime)
	config.MaxConnIdleTime = envDuration("DB_MAX_CONN_IDLE_TIME", defaultMaxConnIdleTime)
	config.HealthCheckPeriod = envDuration("DB_HEALTH_CHECK_PERIOD", defaultHealthCheckPeriod)
	config.ConnConfig.ConnectTimeout = envDuration("DB_CONNECT_TIMEOUT", defaultConnectTimeout)
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = map[string]string{}
	}
	config.ConnConfig.RuntimeParams["application_name"] = "oxide-server"

	timeout := envDuration("DB_STATEMENT_TIMEOUT", defaultStatementTimeout)
	// Applied via AfterConnect (not RuntimeParams) so the limit is set on every
	// physical connection, including ones opened after a reaper or a failover.
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SELECT set_config('statement_timeout', $1, false)`, strconv.FormatInt(timeout.Milliseconds(), 10))
		return err
	}
}

func defaultMaxConns() int {
	n := runtime.NumCPU() * 4
	if n < 8 {
		n = 8
	}
	if n > 50 {
		n = 50
	}
	return n
}

func defaultMinConns() int {
	n := defaultMaxConns() / 8
	if n < 2 {
		n = 2
	}
	return n
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		log.Printf("[Postgres] ignoring invalid %s=%q, using %d", key, raw, fallback)
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("[Postgres] ignoring invalid %s=%q, using %s", key, raw, fallback)
		return fallback
	}
	return d
}

// runMigrations applies every embedded "up" migration that is not already
// recorded in schema_migrations. Each one runs in its own transaction together
// with its version insert, so a failure leaves neither a half-applied migration
// nor a ledger row claiming it was applied.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := loadMigrations(MigrationsFS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	// Session-level advisory lock: a second instance booting at the same time
	// blocks here instead of racing on CREATE/DELETE. A session lock (rather than
	// a transaction one) is deliberate — it also covers the window before the
	// first transaction starts — and Postgres releases it when the session ends,
	// so a killed process cannot wedge the lock forever.
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationsLockID); err != nil {
		return fmt.Errorf("acquire migrations advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, migrationsLockID); err != nil {
			log.Printf("[Postgres] failed to release migrations advisory lock: %v", err)
		}
	}()

	if _, err := conn.Exec(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}

	pending := pendingMigrations(migrations, applied)
	for _, m := range pending {
		log.Printf("[Postgres] applying migration %s", m.Name)
		if err := applyMigration(ctx, conn, m); err != nil {
			return err
		}
	}

	if len(pending) == 0 {
		log.Printf("[Postgres] schema up to date (%d migrations)", len(migrations))
	} else {
		log.Printf("[Postgres] applied %d of %d migrations", len(pending), len(migrations))
	}
	return nil
}

// applyMigration runs one script and its ledger insert in a single transaction.
// It deliberately uses the already-acquired lock-holding connection: the
// advisory lock must stay valid for the whole run, and taking a second pooled
// connection would deadlock on a pool configured with MaxConns = 1.
func applyMigration(ctx context.Context, conn *pgxpool.Conn, m migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction for %s: %w", m.Name, err)
	}
	// No-op once the transaction is committed or rolled back.
	defer func() {
		if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			log.Printf("[Postgres] rollback %s: %v", m.Name, rbErr)
		}
	}()

	// The connection-wide statement timeout would abort legitimate DDL on a big
	// table, so the migration transaction raises it for itself only.
	if _, err := tx.Exec(ctx, `SELECT set_config('statement_timeout', $1, true)`, strconv.FormatInt(envDuration("DB_MIGRATION_TIMEOUT", defaultMigrationTimeout).Milliseconds(), 10)); err != nil {
		return fmt.Errorf("raise statement timeout for %s: %w", m.Name, err)
	}

	if _, err := tx.Exec(ctx, string(m.SQL)); err != nil {
		return fmt.Errorf("execute migration %s: %w", m.Name, err)
	}

	// Recorded in the same transaction as the DDL/DML: the version can only be
	// marked applied if the migration really committed.
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING`,
		m.Version); err != nil {
		return fmt.Errorf("record migration %s: %w", m.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", m.Name, err)
	}
	return nil
}

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// pendingMigrations is the pure core of the runner's idempotency: feeding it the
// versions that were already recorded must always yield the empty list.
func pendingMigrations(all []migration, applied map[string]bool) []migration {
	pending := make([]migration, 0, len(all))
	for _, m := range all {
		if !applied[m.Version] {
			pending = append(pending, m)
		}
	}
	return pending
}

// loadMigrations reads the embedded FS and returns the "up" scripts sorted by
// version. Down scripts are embedded (they are part of the release artefact and
// hand-run by an operator) but never executed here.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, migrationsDir)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]string, len(entries))
	numbers := make(map[string]string, len(entries))
	out := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), downSuffix) || !strings.HasSuffix(entry.Name(), upSuffix) {
			continue
		}
		name := entry.Name()
		version := strings.TrimSuffix(name, upSuffix)
		if !migrationBasePattern.MatchString(version) {
			return nil, fmt.Errorf("migration %q does not match %s", name, migrationBasePattern.String())
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("duplicate migration version %q: both %s and %s exist", version, prev, name)
		}
		seen[version] = name
		// Two files sharing a number (000001_init / 000001_other) would make the
		// chronological order depend on their names rather than on the version.
		number := version[:6]
		if prev, dup := numbers[number]; dup {
			return nil, fmt.Errorf("duplicate migration number %q: both %s and %s exist", number, prev, name)
		}
		numbers[number] = name

		body, err := fs.ReadFile(fsys, migrationsDir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			return nil, fmt.Errorf("migration %s is empty", name)
		}
		out = append(out, migration{Version: version, Name: name, SQL: body})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no %s%s migrations found", migrationsDir, upSuffix)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}
