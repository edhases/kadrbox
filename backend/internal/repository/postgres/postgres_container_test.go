package postgres_test

// Self-contained Postgres rig for the repository layer.
//
// Before this file the entire persistence layer was covered by tests that
// skipped unless TEST_POSTGRES_DSN was exported, so `go test ./...` printed
// "ok" on a developer machine while every repository method sat at 0.0%. The
// guarantee existed in exactly one place (the CI job with `services:`) and
// failed open everywhere else.
//
// TestMain closes that hole: unless the caller already provided a DSN, it
// starts a postgres:16-alpine container and exports TEST_POSTGRES_DSN for the
// whole package, so every env-gated test in this package (including the ones
// this file did not write) runs with `go test ./internal/repository/postgres/...`
// and no external setup. The container is terminated before the process exits
// so repeated CI runs do not leak it.
//
// Precedence:
//  1. TEST_POSTGRES_DSN already set  -> use it, start nothing (CI services, a
//     developer's own instance, or an external database in a soak test).
//  2. -short                        -> start nothing and leave the variable
//     unset, so the env-gated tests skip. `go test -short` stays hermetic.
//  3. otherwise                     -> start a container.
//
// Docker being unavailable is reported loudly rather than swallowed: see
// TestPostgresContainerAvailability.

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// containerPostgresImage is pinned to the same major version the CI service
	// and docker-compose.yml use, so a test that passes locally is not exercising
	// a different server than production.
	containerPostgresImage = "postgres:16-alpine"

	// stablePostgresContainerName is the base name of the throwaway container.
	// The PID suffix keeps two concurrent `go test` runs (a leftover editor test
	// plus CI, or two packages racing on the same runner) from colliding on
	// Docker's globally unique container names; within one process -count=2
	// reuses the same instance.
	stablePostgresContainerName = "oxide-test-postgres"
)

// postgresDSN is the DSN the whole package runs against. Empty means "no
// database in this run", which every consumer must treat as skip, never as
// pass.
var postgresDSN string

var ctSeq atomic.Int64

// TestMain is the single entry point that decides where this package's database
// comes from. It is defined once for the whole test binary, which also covers
// the internal `package postgres` tests compiled into it.
func TestMain(m *testing.M) {
	// testing.Short() reads a flag and m.Run() is what parses flags, so parse
	// first to keep -short observable here.
	flag.Parse()

	container, err := startPostgresForPackage()
	switch {
	case err != nil:
		// Deliberately not a hard failure: a contributor without Docker must
		// still be able to run `go test ./...`. The banner, plus
		// TestPostgresContainerAvailability, make the loss of coverage
		// impossible to miss.
		fmt.Fprintf(os.Stderr,
			"\n*** WARNING: no TEST_POSTGRES_DSN and no usable Docker daemon: %v\n"+
				"*** The repository tests in this package are being SKIPPED and the\n"+
				"*** persistence layer is UNTESTED in this run. Install/start Docker,\n"+
				"*** or export TEST_POSTGRES_DSN to get real coverage.\n\n", err)
	case container != nil:
		// Terminating on the way out is what keeps repeated CI runs from leaking
		// a container (and a published port) per run.
		defer func() {
			terminateCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if terr := container.Terminate(terminateCtx); terr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to terminate the test postgres container: %v\n", terr)
			}
		}()
	}

	os.Exit(m.Run())
}

// withContainerName pins the container's Docker name. Docker container names
// are global, so two concurrent `go test` runs that both asked for "postgres"
// make the loser fail with 409 Conflict and leak whichever container lost the
// race; the caller supplies a run-unique suffix. A named container is also what
// makes `docker ps -a` during a hung run attributable.
func withContainerName(name string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		req.Name = name
		return nil
	}
}

// startPostgresForPackage resolves the DSN the whole package runs against and,
// when it had to start a container, returns it so the caller can terminate it.
func startPostgresForPackage() (testcontainers.Container, error) {
	if dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN")); dsn != "" {
		postgresDSN = dsn
		return nil, nil
	}
	if testing.Short() {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage(containerPostgresImage),
		withContainerName(fmt.Sprintf("%s-%d", stablePostgresContainerName, os.Getpid())),
		tcpostgres.WithDatabase("oxide_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", containerPostgresImage, err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("resolve the container connection string: %w", err)
	}
	postgresDSN = dsn
	if err := os.Setenv("TEST_POSTGRES_DSN", dsn); err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("export TEST_POSTGRES_DSN for the rest of the package: %w", err)
	}
	return container, nil
}

// ctPool returns a migrated pool for the package-wide database, skipping when no
// database is available. It is the entry point for every test in this file:
// `go test -short`, and a Docker-less run, degrade to "not run here" instead of
// a false pass.
func ctPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if postgresDSN == "" {
		t.Skip("no Postgres available (TEST_POSTGRES_DSN unset, -short passed, or Docker unavailable)")
	}
	return ctPoolForDSN(t, postgresDSN)
}

func ctPoolForDSN(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := postgres.InitDB(ctx, dsn)
	if err != nil {
		t.Fatalf("InitDB (migrations included) failed: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestPostgresContainerAvailability turns the silent skip into a visible one.
// Without a database the other tests in this file skip, which on its own looks
// identical to a green run; this one reports, in the test output, exactly which
// of the two paths was taken and proves the connection works.
func TestPostgresContainerAvailability(t *testing.T) {
	switch {
	case postgresDSN == "":
		t.Skip("Postgres unavailable: set TEST_POSTGRES_DSN or install Docker to cover internal/repository/postgres")
	case testing.Short():
		t.Log("using the caller-provided TEST_POSTGRES_DSN")
	default:
		t.Logf("using a throwaway %s container", containerPostgresImage)
	}

	pool := ctPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var one int
	if err := pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("the resolved database is not usable: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 returned %d", one)
	}
}

// ctUser creates a throwaway account and removes it afterwards.
func ctUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	n := ctSeq.Add(1)
	email := fmt.Sprintf("ct_%d_%d_%s@x.com", time.Now().UnixNano(), n, sanitizeTestName(t.Name()))
	user, err := postgres.NewUserRepository(pool).CreateUser(context.Background(), email, "hash", "ctuser")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	t.Cleanup(func() {
		// Cascades clear watch_history/favorites/token rows. The tests below
		// clean up explicitly too, but a failure mid-test must not leave rows
		// behind for the next run to trip over.
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	return user.ID
}

func sanitizeTestName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ctFreshDatabase creates an empty database next to the one under test, returns
// its DSN, and registers the DROP. This is the only way to observe
// runMigrations against a genuinely empty schema: the package-wide database has
// already been migrated by the first InitDB.
//
// The returned DSN is *not* migrated — the caller decides when to run InitDB
// against it, which is the whole point.
func ctFreshDatabase(t *testing.T, prefix string) string {
	t.Helper()
	if postgresDSN == "" {
		t.Skip("no Postgres available")
	}
	// The name is derived from a fixed prefix and integers only, so no user
	// input can reach the identifier; Sanitize quotes it anyway.
	name := fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), ctSeq.Add(1))

	admin, err := pgx.Connect(context.Background(), dsnWithDatabase(postgresDSN, "postgres"))
	if err != nil {
		t.Skipf("cannot reach the server's maintenance database to create a fresh one: %v", err)
	}
	if _, err := admin.Exec(context.Background(), `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		_ = admin.Close(context.Background())
		t.Fatalf("CREATE DATABASE %s failed: %v", name, err)
	}
	_ = admin.Close(context.Background())

	t.Cleanup(func() {
		cleanup, err := pgx.Connect(context.Background(), dsnWithDatabase(postgresDSN, "postgres"))
		if err != nil {
			return
		}
		defer func() { _ = cleanup.Close(context.Background()) }()
		// Terminate stragglers first: a pool leaked by a failed test would
		// otherwise keep the DROP blocked and poison every later run.
		_, _ = cleanup.Exec(context.Background(),
			`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
			 WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
		_, _ = cleanup.Exec(context.Background(), `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize())
	})
	return dsnWithDatabase(postgresDSN, name)
}

// ctMigrationLedger reads schema_migrations as version -> applied_at. It is the
// only observable proof that a migration was not re-executed: re-running an
// idempotent DDL script leaves no trace except a moved timestamp.
func ctMigrationLedger(t *testing.T, pool *pgxpool.Pool) map[string]time.Time {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT version, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("read schema_migrations failed: %v", err)
	}
	defer rows.Close()

	ledger := map[string]time.Time{}
	for rows.Next() {
		var version string
		var appliedAt time.Time
		if err := rows.Scan(&version, &appliedAt); err != nil {
			t.Fatalf("scan schema_migrations failed: %v", err)
		}
		ledger[version] = appliedAt
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema_migrations failed: %v", err)
	}
	return ledger
}

// dsnWithDatabase points a URL-style DSN at a different database, falling back
// to appending dbname= when the input is in key/value form.
func dsnWithDatabase(dsn, database string) string {
	trimmed := strings.TrimSpace(dsn)
	if u, err := url.Parse(trimmed); err == nil && u.Scheme != "" && strings.Contains(trimmed, "://") {
		u.Path = "/" + database
		return u.String()
	}
	sep := "?"
	if strings.Contains(trimmed, "?") {
		sep = "&"
	}
	return trimmed + sep + "dbname=" + database
}
