package postgres

// Coverage for the pool/migration plumbing that is pure configuration.
//
// Everything asserted here runs without a database: applyPoolLimits only
// mutates a pgxpool.Config value, the defaults are pure functions of the
// process, envInt/envDuration are pure functions of the environment, and
// loadMigrations walks an fs.FS that the test supplies.

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// offlineDSN parses without connecting: pgxpool.ParseConfig only fills in the
// struct, so no socket is opened.
const offlineDSN = "postgres://user:pass@127.0.0.1:5432/oxide?sslmode=disable"

func TestCovApplyPoolLimitsTakesEveryKnobFromTheEnvironment(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "17")
	t.Setenv("DB_MIN_CONNS", "3")
	t.Setenv("DB_MAX_CONN_LIFETIME", "90m")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "45s")
	t.Setenv("DB_HEALTH_CHECK_PERIOD", "2m")
	t.Setenv("DB_CONNECT_TIMEOUT", "7s")

	cfg, err := pgxpool.ParseConfig(offlineDSN)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	applyPoolLimits(cfg)

	if cfg.MaxConns != 17 {
		t.Errorf("MaxConns = %d, want 17", cfg.MaxConns)
	}
	if cfg.MinConns != 3 {
		t.Errorf("MinConns = %d, want 3", cfg.MinConns)
	}
	if cfg.MaxConnLifetime != 90*time.Minute {
		t.Errorf("MaxConnLifetime = %s, want 90m", cfg.MaxConnLifetime)
	}
	if cfg.MaxConnIdleTime != 45*time.Second {
		t.Errorf("MaxConnIdleTime = %s, want 45s", cfg.MaxConnIdleTime)
	}
	if cfg.HealthCheckPeriod != 2*time.Minute {
		t.Errorf("HealthCheckPeriod = %s, want 2m", cfg.HealthCheckPeriod)
	}
	if cfg.ConnConfig.ConnectTimeout != 7*time.Second {
		t.Errorf("ConnectTimeout = %s, want 7s", cfg.ConnConfig.ConnectTimeout)
	}
	// The statement timeout is applied per physical connection, not as a
	// connection parameter, so a connection opened after a reaper still gets it.
	if cfg.AfterConnect == nil {
		t.Fatal("AfterConnect is nil: a statement timeout would not be applied to every connection")
	}
	if got := cfg.ConnConfig.RuntimeParams["application_name"]; got != "kadrbox-server" {
		t.Errorf("application_name = %q, want %q", got, "kadrbox-server")
	}
}

func TestCovApplyPoolLimitsClampsMinConnsToMaxConns(t *testing.T) {
	// A min larger than the max is a hard pgx error ("min_conns must be <=
	// max_conns"), so the clamp is load-bearing, not cosmetic.
	t.Setenv("DB_MAX_CONNS", "4")
	t.Setenv("DB_MIN_CONNS", "9")

	cfg, err := pgxpool.ParseConfig(offlineDSN)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	applyPoolLimits(cfg)

	if cfg.MinConns != cfg.MaxConns {
		t.Errorf("MinConns = %d, MaxConns = %d; MinConns must be clamped down to MaxConns", cfg.MinConns, cfg.MaxConns)
	}
}

func TestCovApplyPoolLimitsFallsBackToDefaultsWhenEnvIsUnset(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "")
	t.Setenv("DB_MIN_CONNS", "")

	cfg, err := pgxpool.ParseConfig(offlineDSN)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	applyPoolLimits(cfg)

	if want := int32(defaultMaxConns()); cfg.MaxConns != want {
		t.Errorf("MaxConns = %d, want the machine-derived default %d", cfg.MaxConns, want)
	}
	if want := int32(defaultMinConns()); cfg.MinConns != want {
		t.Errorf("MinConns = %d, want the machine-derived default %d", cfg.MinConns, want)
	}
	if cfg.ConnConfig.RuntimeParams["application_name"] == "" {
		t.Error("application_name must be set even on the all-defaults path")
	}
}

func TestCovApplyPoolLimitsAllocatesRuntimeParamsWhenAbsent(t *testing.T) {
	cfg, err := pgxpool.ParseConfig(offlineDSN)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	// A DSN can arrive with RuntimeParams already populated; the code must not
	// panic or drop them.
	cfg.ConnConfig.RuntimeParams = nil
	applyPoolLimits(cfg)
	if cfg.ConnConfig.RuntimeParams == nil {
		t.Fatal("RuntimeParams left nil; the assignment below would panic")
	}
	if got := cfg.ConnConfig.RuntimeParams["application_name"]; got != "kadrbox-server" {
		t.Errorf("application_name = %q, want %q", got, "kadrbox-server")
	}
}

func TestCovDefaultConnBoundsAreSaneAndOrdered(t *testing.T) {
	maxConns := defaultMaxConns()
	if maxConns < 8 || maxConns > 50 {
		t.Errorf("defaultMaxConns() = %d, want a value inside the documented 8..50 band", maxConns)
	}
	minConns := defaultMinConns()
	if minConns < 2 {
		t.Errorf("defaultMinConns() = %d, want at least 2", minConns)
	}
	if minConns > maxConns {
		t.Errorf("defaultMinConns() = %d > defaultMaxConns() = %d", minConns, maxConns)
	}
	// The documented relationship is max/8, floored at 2.
	if want := maxConns / 8; minConns > max(2, want) {
		t.Errorf("defaultMinConns() = %d, want at most %d", minConns, max(2, want))
	}
}

func TestCovEnvInt(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		set   bool
		want  int
		unset bool
	}{
		{name: "unsetUsesFallback", unset: true, want: 42},
		{name: "blankUsesFallback", raw: "   ", set: true, want: 42},
		{name: "paddedValueIsTrimmed", raw: "  13  ", set: true, want: 13},
		{name: "zeroIsHonoured", raw: "0", set: true, want: 0},
		{name: "garbageUsesFallback", raw: "many", set: true, want: 42},
		{name: "negativeUsesFallback", raw: "-5", set: true, want: 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unset {
				t.Setenv("COV_DB_INT", "")
			} else {
				t.Setenv("COV_DB_INT", tc.raw)
			}
			if got := envInt("COV_DB_INT", 42); got != tc.want {
				t.Errorf("envInt(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestCovEnvDuration(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		set  bool
		want time.Duration
	}{
		{name: "unsetUsesFallback", want: 5 * time.Second},
		{name: "blankUsesFallback", raw: "   ", set: true, want: 5 * time.Second},
		{name: "plainDuration", raw: "1h30m", set: true, want: 90 * time.Minute},
		{name: "zeroIsRefused", raw: "0s", set: true, want: 5 * time.Second},
		{name: "negativeIsRefused", raw: "-1m", set: true, want: 5 * time.Second},
		{name: "unitlessIsRefused", raw: "30", set: true, want: 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("COV_DB_DUR", tc.raw)
			} else {
				t.Setenv("COV_DB_DUR", "")
			}
			if got := envDuration("COV_DB_DUR", 5*time.Second); got != tc.want {
				t.Errorf("envDuration(%q) = %s, want %s", tc.raw, got, tc.want)
			}
		})
	}
}

func TestCovLoadMigrationsSortsUpScriptsAndIgnoresTheRest(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/000003_third.up.sql":    {Data: []byte("SELECT 3")},
		"migrations/000001_first.up.sql":    {Data: []byte("SELECT 1")},
		"migrations/000002_second.up.sql":   {Data: []byte("SELECT 2")},
		"migrations/000002_second.down.sql": {Data: []byte("DROP 2")},
		"migrations/README.md":              {Data: []byte("not a migration")},
		"migrations/000004_notes.txt":       {Data: []byte("not a migration")},
	}

	got, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}

	wantVersions := []string{"000001_first", "000002_second", "000003_third"}
	if len(got) != len(wantVersions) {
		t.Fatalf("loaded %d migrations (%v), want %d", len(got), versionsOf(got), len(wantVersions))
	}
	for i, want := range wantVersions {
		if got[i].Version != want {
			t.Errorf("migrations[%d].Version = %q, want %q (full order %v)", i, got[i].Version, want, versionsOf(got))
		}
	}
	// Version and Name differ only by the direction suffix, and the body is the
	// file content verbatim: the runner executes got[i].SQL as-is.
	if got[0].Name != "000001_first.up.sql" {
		t.Errorf("Name = %q, want %q", got[0].Name, "000001_first.up.sql")
	}
	if string(got[0].SQL) != "SELECT 1" {
		t.Errorf("SQL = %q, want %q", got[0].SQL, "SELECT 1")
	}
}

func TestCovLoadMigrationsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		fsys    fstest.MapFS
		wantSub string
	}{
		{
			name:    "missingDirectory",
			fsys:    fstest.MapFS{"other/000001_x.up.sql": {Data: []byte("SELECT 1")}},
			wantSub: "open migrations",
		},
		{
			name:    "malformedName",
			fsys:    fstest.MapFS{"migrations/1_bad.up.sql": {Data: []byte("SELECT 1")}},
			wantSub: "does not match",
		},
		{
			name:    "uppercaseName",
			fsys:    fstest.MapFS{"migrations/000001_Mixed.up.sql": {Data: []byte("SELECT 1")}},
			wantSub: "does not match",
		},
		{
			name: "twoFilesShareANumber",
			fsys: fstest.MapFS{
				"migrations/000001_first.up.sql":  {Data: []byte("SELECT 1")},
				"migrations/000001_second.up.sql": {Data: []byte("SELECT 2")},
			},
			wantSub: "duplicate migration number",
		},
		{
			name:    "emptyBody",
			fsys:    fstest.MapFS{"migrations/000001_blank.up.sql": {Data: []byte("   \n\t\n")}},
			wantSub: "is empty",
		},
		{
			name:    "noUpScripts",
			fsys:    fstest.MapFS{"migrations/000001_only.down.sql": {Data: []byte("DROP 1")}},
			wantSub: "no migrations.up.sql migrations found",
		},
		{
			name:    "onlyDirectories",
			fsys:    fstest.MapFS{"migrations/sub/000001_x.up.sql": {Data: []byte("SELECT 1")}},
			wantSub: "no migrations.up.sql migrations found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadMigrations(tc.fsys)
			if err == nil {
				t.Fatalf("loadMigrations accepted invalid input, returned %v", versionsOf(got))
			}
			if !contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantSub)
			}
			if got != nil {
				t.Errorf("migrations = %v, want nil alongside the error", versionsOf(got))
			}
		})
	}
}

func TestCovEmbeddedMigrationsAreLoadableAndChronological(t *testing.T) {
	got, err := loadMigrations(MigrationsFS)
	if err != nil {
		t.Fatalf("the shipped migrations must load: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("MigrationsFS embedded no migrations at all")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Version >= got[i].Version {
			t.Errorf("not sorted: migrations[%d]=%q before migrations[%d]=%q",
				i-1, got[i-1].Version, i, got[i].Version)
		}
	}
}

func TestCovPendingMigrationsIsEmptyForAFullyAppliedLedger(t *testing.T) {
	all, err := loadMigrations(MigrationsFS)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	applied := map[string]bool{}
	for _, m := range all {
		applied[m.Version] = true
	}
	if pending := pendingMigrations(all, applied); len(pending) != 0 {
		t.Errorf("pendingMigrations with a complete ledger returned %d entries: a restart would re-run migrations", len(pending))
	}

	// Dropping one version must make exactly that migration pending again, and
	// nothing else: this is the idempotency property the runner relies on.
	delete(applied, all[0].Version)
	pending := pendingMigrations(all, applied)
	if len(pending) != 1 || pending[0].Version != all[0].Version {
		t.Errorf("pendingMigrations returned %v, want exactly [%s]", versionsOf(pending), all[0].Version)
	}
}

// unreadableFS lists a file it then refuses to read, which is the shape a
// truncated deploy artefact or a permissions problem produces. fstest.MapFS
// cannot express it: every listed entry is readable by construction.
type unreadableFS struct {
	inner fstest.MapFS
	deny  string
}

func (f unreadableFS) Open(name string) (fs.File, error) {
	if name == f.deny {
		return nil, fs.ErrPermission
	}
	return f.inner.Open(name)
}

func TestCovLoadMigrationsReportsAnUnreadableScript(t *testing.T) {
	fsys := unreadableFS{
		inner: fstest.MapFS{
			"migrations/000001_first.up.sql":  {Data: []byte("SELECT 1")},
			"migrations/000002_second.up.sql": {Data: []byte("SELECT 2")},
		},
		deny: "migrations/000002_second.up.sql",
	}

	got, err := loadMigrations(fsys)
	if err == nil {
		t.Fatalf("loadMigrations accepted an unreadable script, returned %v", versionsOf(got))
	}
	if !contains(err.Error(), "read migration 000002_second.up.sql") {
		t.Errorf("error = %q, want it to name the script it could not read", err)
	}
}

func versionsOf(ms []migration) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Version)
	}
	return out
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
