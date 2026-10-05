package postgres

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---- migration loading ------------------------------------------------------

func TestPgLoadEmbeddedMigrations(t *testing.T) {
	migrations, err := loadMigrations(MigrationsFS)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected at least the five original migrations")
	}

	seen := map[string]bool{}
	for i, m := range migrations {
		if !migrationBasePattern.MatchString(m.Version) {
			t.Errorf("version %q does not match the naming pattern", m.Version)
		}
		if seen[m.Version] {
			t.Errorf("duplicate version %q", m.Version)
		}
		seen[m.Version] = true
		if i > 0 && migrations[i-1].Version >= m.Version {
			t.Errorf("migrations not sorted: %q before %q", migrations[i-1].Version, m.Version)
		}
		if len(m.SQL) == 0 {
			t.Errorf("migration %s has no SQL", m.Name)
		}
	}

	// The five shipped before the version ledger existed must still be present,
	// otherwise a fresh database would be built on a partial schema.
	for _, want := range []string{
		"000001_init", "000002_email_verification", "000003_password_resets",
		"000004_oauth_providers", "000005_remove_removed_source_rows",
	} {
		if !seen[want] {
			t.Errorf("expected original migration %q to still be embedded", want)
		}
	}
}

func TestPgEmbeddedMigrationsHaveDownCounterparts(t *testing.T) {
	entries, err := fs.ReadDir(MigrationsFS, migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	up, down := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.Name(), upSuffix):
			up[strings.TrimSuffix(e.Name(), upSuffix)] = true
		case strings.HasSuffix(e.Name(), downSuffix):
			down[strings.TrimSuffix(e.Name(), downSuffix)] = true
		default:
			t.Errorf("unexpected file in migrations dir: %s", e.Name())
		}
	}

	for version := range up {
		if !down[version] {
			t.Errorf("migration %s has no %s*%s counterpart", version, downSuffix, "")
		}
	}
	for version := range down {
		if !up[version] {
			t.Errorf("found down script for unknown migration %s", version)
		}
	}
}

func TestPgLoadMigrationsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		fsys fstest.MapFS
	}{
		{
			name: "badFileName",
			fsys: fstest.MapFS{
				"migrations/notes.up.sql":       &fstest.MapFile{Data: []byte("SELECT 1")},
				"migrations/000001_init.up.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
			},
		},
		{
			name: "duplicateNumber",
			fsys: fstest.MapFS{
				"migrations/000001_init.up.sql":  &fstest.MapFile{Data: []byte("SELECT 1")},
				"migrations/000001_other.up.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
			},
		},
		{
			name: "emptyMigration",
			fsys: fstest.MapFS{
				"migrations/000001_init.up.sql": &fstest.MapFile{Data: []byte("   \n\t")},
			},
		},
		{
			name: "noMigrations",
			fsys: fstest.MapFS{
				"migrations/000001_init.down.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
			},
		},
		{
			name: "missingDir",
			fsys: fstest.MapFS{
				"other/000001_init.up.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := loadMigrations(tc.fsys); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestPgLoadMigrationsSkipsDownScripts(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/000002_second.up.sql":   &fstest.MapFile{Data: []byte("SELECT 2")},
		"migrations/000002_second.down.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
		"migrations/000001_first.up.sql":    &fstest.MapFile{Data: []byte("SELECT 1")},
		"migrations/000001_first.down.sql":  &fstest.MapFile{Data: []byte("SELECT 1")},
	}
	migrations, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("expected 2 up migrations, got %d", len(migrations))
	}
	if migrations[0].Version != "000001_first" || migrations[1].Version != "000002_second" {
		t.Errorf("unexpected order: %q, %q", migrations[0].Version, migrations[1].Version)
	}
}

// TestPgMigrationRunnerIsIdempotent exercises the pure decision the runner makes
// on every boot: a second pass with the recorded versions must schedule nothing.
// The transactional part of applyMigration needs a live server and is covered by
// the TEST_POSTGRES_DSN integration test instead.
func TestPgMigrationRunnerIsIdempotent(t *testing.T) {
	migrations, err := loadMigrations(MigrationsFS)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}

	// Fresh database: schema_migrations is empty.
	applied := map[string]bool{}
	first := pendingMigrations(migrations, applied)
	if len(first) != len(migrations) {
		t.Fatalf("expected every migration on a fresh database, got %d of %d", len(first), len(migrations))
	}

	for _, m := range first {
		applied[m.Version] = true
	}
	if second := pendingMigrations(migrations, applied); len(second) != 0 {
		t.Fatalf("second pass scheduled migrations again: %v", second)
	}

	// A partially applied state (a crash, or a file added by a later release)
	// resumes exactly at the first unrecorded version.
	dropped := len(migrations) - 2
	for _, m := range migrations[dropped:] {
		delete(applied, m.Version)
	}
	resumed := pendingMigrations(migrations, applied)
	if len(resumed) != 2 {
		t.Fatalf("expected 2 pending after losing the last two versions, got %d", len(resumed))
	}
	if resumed[0].Version != migrations[dropped].Version || resumed[1].Version != migrations[dropped+1].Version {
		t.Errorf("expected resume at %s, %s — got %s, %s",
			migrations[dropped].Version, migrations[dropped+1].Version, resumed[0].Version, resumed[1].Version)
	}

	// An unknown version in the ledger (a downgrade, a renamed file) must not
	// make the runner skip anything.
	unknown := pendingMigrations(migrations, map[string]bool{"000999_gone": true})
	if len(unknown) != len(migrations) {
		t.Errorf("an unknown recorded version must not suppress migrations: got %d of %d", len(unknown), len(migrations))
	}
}

func TestPgPendingMigrationsEmptySet(t *testing.T) {
	if got := pendingMigrations(nil, nil); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

// ---- pool configuration -----------------------------------------------------

func TestPgEnvIntFallbacks(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  int
	}{
		{"unset", "", 25},
		{"valid", "40", 40},
		{"zeroIsHonoured", "0", 0},
		{"garbage", "not-a-number", 25},
		{"negative", "-5", 25},
		{"padded", "  12  ", 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DB_MAX_CONNS", tc.value)
			if got := envInt("DB_MAX_CONNS", 25); got != tc.want {
				t.Errorf("envInt = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPgEnvDurationFallbacks(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"unset", "", 15 * time.Second},
		{"valid", "2m", 2 * time.Minute},
		{"garbage", "soon", 15 * time.Second},
		{"zero", "0s", 15 * time.Second},
		{"negative", "-1h", 15 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DB_STATEMENT_TIMEOUT", tc.value)
			if got := envDuration("DB_STATEMENT_TIMEOUT", 15*time.Second); got != tc.want {
				t.Errorf("envDuration = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPgDefaultPoolLimits(t *testing.T) {
	maxConns := defaultMaxConns()
	minConns := defaultMinConns()
	if maxConns < 8 || maxConns > 50 {
		t.Errorf("defaultMaxConns = %d, want within [8,50]", maxConns)
	}
	if minConns < 2 {
		t.Errorf("defaultMinConns = %d, want >= 2", minConns)
	}
	if minConns > maxConns {
		t.Errorf("defaultMinConns (%d) > defaultMaxConns (%d)", minConns, maxConns)
	}
}

func TestPgApplyPoolLimits(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "11")
	t.Setenv("DB_MIN_CONNS", "30") // must be clamped down to MaxConns
	t.Setenv("DB_MAX_CONN_LIFETIME", "10m")
	t.Setenv("DB_STATEMENT_TIMEOUT", "3s")

	config, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	applyPoolLimits(config)

	if config.MaxConns != 11 {
		t.Errorf("MaxConns = %d, want 11", config.MaxConns)
	}
	if config.MinConns != 11 {
		t.Errorf("MinConns = %d, want it clamped to 11", config.MinConns)
	}
	if config.MaxConnLifetime != 10*time.Minute {
		t.Errorf("MaxConnLifetime = %s, want 10m", config.MaxConnLifetime)
	}
	if config.MaxConnIdleTime != defaultMaxConnIdleTime {
		t.Errorf("MaxConnIdleTime = %s, want %s", config.MaxConnIdleTime, defaultMaxConnIdleTime)
	}
	if config.AfterConnect == nil {
		t.Fatal("AfterConnect must be set so statement_timeout applies to every connection")
	}
	if got := config.ConnConfig.RuntimeParams["application_name"]; got != "kadrbox-server" {
		t.Errorf("application_name = %q, want oxide-server", got)
	}
}

// ---- unique-violation classification ---------------------------------------

func TestPgClassifyUserConstraintTypedErrors(t *testing.T) {
	cases := []struct {
		name       string
		constraint string
		want       error
	}{
		{"email", "users_email_key", ErrEmailTaken},
		{"emailCiIndex", "idx_users_email_lower", ErrEmailTaken},
		{"telegram", "users_telegram_id_key", ErrTelegramTaken},
		{"discord", "users_discord_id_key", ErrDiscordTaken},
		{"unrecognisedNameFallsBackToEmail", "users_something_else_key", ErrUserAlreadyExists},
		{"nameSubstringEmail", "uq_lower_email", ErrEmailTaken},
		{"nameSubstringTelegram", "custom_telegram_unique", ErrTelegramTaken},
		{"nameSubstringDiscord", "custom_discord_unique", ErrDiscordTaken},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Wrapped, the way the repository wraps it, so errors.As has to
			// unwrap rather than type-assert.
			err := fmt.Errorf("scan user: %w", &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: tc.constraint})
			got, ok := classifyUserConstraint(err)
			if !ok {
				t.Fatal("expected a classification")
			}
			if !errors.Is(got, tc.want) {
				t.Errorf("classify = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPgErrEmailTakenIsErrUserAlreadyExists(t *testing.T) {
	// auth_handler and the integration test compare against ErrUserAlreadyExists;
	// the rename must not break them.
	if !errors.Is(ErrEmailTaken, ErrUserAlreadyExists) {
		t.Error("ErrEmailTaken must keep matching ErrUserAlreadyExists")
	}
}

func TestPgClassifyUserConstraintNonUniqueErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"otherPgCode", &pgconn.PgError{Code: "23503", ConstraintName: "users_email_key"}, false},
		{"checkViolation", &pgconn.PgError{Code: "23514", ConstraintName: "chk_users_role"}, false},
		{"plainError", errors.New("connection refused"), false},
		{"stringifiedDupKey", errors.New(`ERROR: duplicate key value violates unique constraint "users_email_key"`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := classifyUserConstraint(tc.err); ok != tc.want {
				t.Errorf("classify ok = %v, want %v", ok, tc.want)
			}
		})
	}
}

// TestPgIsDuplicateKeyErrorTypedPath documents the intended fix for defect #6:
// a *pgconn.PgError is judged by its SQLSTATE, not by substring matching, so a
// message that merely happens to contain "23505" is no longer a false positive.
func TestPgIsDuplicateKeyErrorTypedPath(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"typedUnique", &pgconn.PgError{Code: pgUniqueViolation}, true},
		{"typedUniqueWrapped", fmt.Errorf("x: %w", &pgconn.PgError{Code: pgUniqueViolation}), true},
		{"typedOtherCode", &pgconn.PgError{Code: "23503"}, false},
		{"typedOtherCodeWithCodeInMessage", &pgconn.PgError{Code: "23503", Message: "contains 23505 by accident"}, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDuplicateKeyError(tc.err); got != tc.want {
				t.Errorf("isDuplicateKeyError = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPgNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"  Alice@Example.COM ": "alice@example.com",
		"ALICE@EXAMPLE.COM":    "alice@example.com",
		"alice@example.com":    "alice@example.com",
		"":                     "",
		"   ":                  "",
	}
	for in, want := range cases {
		if got := normalizeEmail(in); got != want {
			t.Errorf("normalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- value sanitisers -------------------------------------------------------

func TestPgSanitiseRating(t *testing.T) {
	inRange := 7.5
	negative := -0.5
	tooBig := 11.0
	nan := math.NaN()
	inf := math.Inf(1)
	max := 10.0
	zero := 0.0

	cases := []struct {
		name string
		in   *float64
		want *float64
	}{
		{"nil", nil, nil},
		{"inRange", &inRange, &inRange},
		{"zeroAllowed", &zero, &zero},
		{"maxAllowed", &max, &max},
		{"negativeDropped", &negative, nil},
		{"aboveTenDropped", &tooBig, nil},
		{"nanDropped", &nan, nil},
		{"infDropped", &inf, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitiseRating(tc.in)
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("expected nil, got %v", *got)
			case tc.want != nil && got == nil:
				t.Errorf("expected %v, got nil", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("got %v, want %v", *got, *tc.want)
			}
		})
	}
}

func TestPgSanitiseYear(t *testing.T) {
	ok, zero, negative, absurd := 1999, 0, -3, 99999
	clampLow, clampHigh := 0, 9999
	cases := []struct {
		name string
		in   *int
		want *int
	}{
		{"nil", nil, nil},
		{"normal", &ok, &ok},
		{"zeroIsUnknownYearSentinel", &zero, &zero},
		// Out-of-range values are clamped, not dropped: migration 000009 also
		// clamps ("no score or year is lost"), and a device with a wrong clock
		// should not silently erase the year from a user's history.
		{"negativeClamped", &negative, &clampLow},
		{"absurdClamped", &absurd, &clampHigh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitiseYear(tc.in)
			if tc.want == nil && got != nil {
				t.Errorf("expected nil, got %d", *got)
			}
			if tc.want != nil && (got == nil || *got != *tc.want) {
				t.Errorf("expected %d, got %v", *tc.want, got)
			}
		})
	}
}

func TestPgSanitiseEpisodeNumber(t *testing.T) {
	first, zero, negative := 1, 0, -2
	if got := sanitiseEpisodeNumber(nil); got != nil {
		t.Errorf("expected nil, got %d", *got)
	}
	if got := sanitiseEpisodeNumber(&first); got == nil || *got != 1 {
		t.Errorf("expected 1, got %v", got)
	}
	if got := sanitiseEpisodeNumber(&zero); got != nil {
		t.Errorf("expected 0 to be dropped, got %d", *got)
	}
	if got := sanitiseEpisodeNumber(&negative); got != nil {
		t.Errorf("expected negative to be dropped, got %d", *got)
	}
}

func TestPgSanitiseHistory(t *testing.T) {
	year := 0
	season := 0
	rating := 42.0

	h := &domain.WatchHistory{
		PositionMs: -5,
		DurationMs: 100,
		Year:       &year,
		Season:     &season,
		Rating:     &rating,
		MediaType:  "  Movie ",
	}
	sanitiseHistory(h)

	if h.PositionMs != 0 {
		t.Errorf("negative position should clamp to 0, got %d", h.PositionMs)
	}
	if h.Rating != nil {
		t.Errorf("out-of-range rating should be dropped, got %v", *h.Rating)
	}
	if h.Season != nil {
		t.Errorf("season 0 should be dropped, got %v", *h.Season)
	}
	if h.Year == nil || *h.Year != 0 {
		t.Errorf("year 0 is the unknown-year sentinel and must survive")
	}
	if h.MediaType != "movie" {
		t.Errorf("media type should be trimmed and lowercased, got %q", h.MediaType)
	}
}

func TestPgSanitiseHistoryPositionBeyondDuration(t *testing.T) {
	h := &domain.WatchHistory{PositionMs: 5_000, DurationMs: 100}
	sanitiseHistory(h)
	if h.PositionMs != 100 {
		t.Errorf("position beyond duration should clamp to duration, got %d", h.PositionMs)
	}
}

func TestPgSanitiseHistoryZeroDurationKeepsPosition(t *testing.T) {
	// duration_ms = 0 means "unknown", and the CHECK explicitly allows it, so the
	// position must not be clamped to 0.
	h := &domain.WatchHistory{PositionMs: 5_000, DurationMs: 0}
	sanitiseHistory(h)
	if h.PositionMs != 5_000 {
		t.Errorf("position must be untouched when duration is unknown, got %d", h.PositionMs)
	}
}

func TestPgSanitiseHistoryNilIsSafe(t *testing.T) {
	sanitiseHistory(nil) // must not panic
}

// ---- watched_at -------------------------------------------------------------

func TestPgResolveWatchedAt(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("keepsClientTimestamp", func(t *testing.T) {
		clientTime := now.Add(-2 * time.Hour)
		if got := resolveWatchedAt(clientTime, now); !got.Equal(clientTime) {
			t.Errorf("got %s, want %s", got, clientTime)
		}
	})

	t.Run("zeroBecomesNow", func(t *testing.T) {
		if got := resolveWatchedAt(time.Time{}, now); !got.Equal(now) {
			t.Errorf("got %s, want %s", got, now)
		}
	})

	t.Run("futureIsClampedToNow", func(t *testing.T) {
		// A device with a wrong clock must not be able to freeze the monotonic
		// update rule for every other device.
		skewed := now.Add(72 * time.Hour)
		if got := resolveWatchedAt(skewed, now); !got.Equal(now) {
			t.Errorf("got %s, want %s", got, now)
		}
	})
}

func TestPgNormalisePage(t *testing.T) {
	cases := []struct {
		limit, offset, fallback int
		wantLimit, wantOffset   int
	}{
		{20, 40, 50, 20, 40},
		{0, 0, 50, 50, 0},
		{-1, 0, 50, 50, 0},
		{20, -5, 50, 20, 0},
	}
	for _, tc := range cases {
		limit, offset := normalisePage(tc.limit, tc.offset, tc.fallback)
		if limit != tc.wantLimit || offset != tc.wantOffset {
			t.Errorf("normalisePage(%d,%d) = (%d,%d), want (%d,%d)",
				tc.limit, tc.offset, limit, offset, tc.wantLimit, tc.wantOffset)
		}
	}
}

// ---- embedded SQL invariants ------------------------------------------------

// TestPgEmbeddedMigrationsAreDocumented keeps the schema files self-describing:
// every migration must say what it does, because the down scripts are read by
// humans during an incident, not by the runner.
func TestPgEmbeddedMigrationsAreDocumented(t *testing.T) {
	migrations, err := loadMigrations(MigrationsFS)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range migrations {
		if !strings.Contains(string(m.SQL), "--") {
			t.Errorf("migration %s has no comment header", m.Name)
		}
	}
}

// TestPgRemovedSourceRowsMigrationIsMarkedOneShot guards the reason the version
// ledger exists: 000005 was a one-time data migration and must stay
// recognisable as such, because that history is exactly why the ledger was
// introduced. Its three DELETEs are gone (the script is a documented no-op), so
// this test pins the documentation rather than any row count.
func TestPgRemovedSourceRowsMigrationIsMarkedOneShot(t *testing.T) {
	body, err := fs.ReadFile(MigrationsFS, path.Join(migrationsDir, "000005_remove_removed_source_rows"+upSuffix))
	if err != nil {
		t.Fatalf("read 000005: %v", err)
	}
	text := string(body)
	if !strings.Contains(strings.ToUpper(text), "ONE-TIME") {
		t.Error("000005 must document that it is a one-time data migration")
	}
	if !strings.Contains(text, "schema_migrations") {
		t.Error("000005 must explain that the version ledger is what makes it run once")
	}
}

// TestPgWatchHistoryUpsertSQL pins the monotonicity of the upsert: the client's
// timestamp has to reach the row and a stale row must not overwrite a newer one.
// Both are pure text assertions because the statement cannot be executed without
// a server; the semantics are additionally covered by the integration test.
func TestPgWatchHistoryUpsertSQL(t *testing.T) {
	body := watchHistoryUpsertQuery
	checks := map[string]string{
		"insert uses the client timestamp":    "$17",
		"conflict target is the unique key":   "ON CONFLICT (user_id, media_id, provider_id, season, episode)",
		"update is conditional":               "WHERE EXCLUDED.watched_at >= watch_history.watched_at",
		"watched_at never moves backwards":    "GREATEST(watch_history.watched_at, EXCLUDED.watched_at)",
		"no server-side NOW() for watched_at": "watched_at = NOW()",
	}
	for name, want := range checks {
		found := strings.Contains(body, want)
		if name == "no server-side NOW() for watched_at" {
			if found {
				t.Error("watched_at must not be stamped with NOW()")
			}
			continue
		}
		if !found {
			t.Errorf("upsert SQL missing %q (%s)", want, name)
		}
	}
}
