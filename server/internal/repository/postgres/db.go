package postgres

import (
	"context"
	"embed"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

// InitDB створює пул з'єднань pgx та автоматично виконує міграції з пам'яті
func InitDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database DSN: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Автоматичний накат вбудованих міграцій
	if err := runMigrations(ctx, pool); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return pool, nil
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			log.Printf("[Postgres] Applying embedded migration: %s", entry.Name())
			sqlBytes, err := MigrationsFS.ReadFile("migrations/" + entry.Name())
			if err != nil {
				return fmt.Errorf("read migration file %s: %w", entry.Name(), err)
			}

			if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
				return fmt.Errorf("execute migration %s: %w", entry.Name(), err)
			}
		}
	}

	log.Println("[Postgres] All embedded migrations applied successfully")
	return nil
}
