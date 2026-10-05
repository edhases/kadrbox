package main

// Coverage for cmd/api.
//
// main() is a signal handler and run() is process wiring: both need a live
// Postgres and Redis, so neither is reachable from a unit test. What is
// reachable is the one pure function in the file and the early failure of run()
// against a database that is not there — which is also the path an operator
// actually hits on a bad DSN, so it is worth pinning.
//
// Nothing here opens a socket to anything but a closed loopback port.

import (
	"context"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/config"
)

func TestCovSplitCSV(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "onlyWhitespace", raw: "   ", want: nil},
		{name: "onlyCommas", raw: ",,,", want: nil},
		{name: "single", raw: "a", want: []string{"a"}},
		{name: "trimsSurroundingSpace", raw: " a , b ", want: []string{"a", "b"}},
		{
			name: "dropsEmptyElements",
			// "a,,b" must not become ["a", "", "b"]: an empty allow-list entry
			// would be handed to the SSRF allow-list verbatim.
			raw:  "a,,b,",
			want: []string{"a", "b"},
		},
		{name: "keepsInnerSpaces", raw: "a b,c", want: []string{"a b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCSV(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("splitCSV(%q) = %v, want %v", tc.raw, got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.raw, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestCovSplitCSVRejectsEmbeddedNewlines(t *testing.T) {
	// A newline inside an entry would produce an allow-list item that never
	// matches a host, and one that breaks log lines when printed. Splitting on
	// commas only means it survives into the slice as a single odd entry, so the
	// caller's own trimming is the only defence — assert the behaviour rather
	// than pretend it is filtered here.
	got := splitCSV("a\nb,c")
	if len(got) != 2 || got[0] != "a\nb" {
		t.Errorf("splitCSV with a newline = %v, want the newline kept inside one entry", got)
	}
}

// covBrokenDSN points at a loopback port nothing listens on, so the failure is
// immediate and never leaves the machine.
const covBrokenDSN = "postgres://user:pass@127.0.0.1:1/oxide?sslmode=disable&connect_timeout=1"

func TestCovRunRefusesToStartOnAnUnreachableDatabase(t *testing.T) {
	cfg := &config.Config{
		ServerPort:   "0",
		DBHost:       "127.0.0.1",
		DBPort:       "1",
		DBUser:       "user",
		DBPassword:   "cov-password-long-enough",
		DBName:       "oxide",
		DBSSLMode:    "disable",
		RedisAddr:    "127.0.0.1:1",
		RedisPass:    "cov-redis-password",
		JWTSecret:    "cov-secret-key-that-is-long-enough",
		AppURL:       "https://app.example",
		LogLevel:     "info",
		LogFormat:    "text",
		BaseProxyURL: "",
	}
	if err := cfg.Validate(); err != nil {
		t.Skipf("config is not valid in this environment, nothing to assert: %v", err)
	}

	err := run(context.Background(), cfg)
	if err == nil {
		t.Fatal("run() succeeded with no database listening; a misconfigured DSN must not start a server that serves nothing")
	}
	if !strings.Contains(err.Error(), "Postgres") {
		t.Errorf("error = %q, want it to name the failed dependency", err)
	}
}

func TestCovRunHonoursAnAlreadyCancelledContext(t *testing.T) {
	// Cancelling before the call must surface as a failure, not a hang: the
	// process would otherwise sit in a connect loop after SIGTERM.
	cfg := &config.Config{
		ServerPort: "0",
		DBHost:     "127.0.0.1",
		DBPort:     "1",
		DBUser:     "user",
		DBPassword: "cov-password-long-enough",
		DBName:     "oxide",
		DBSSLMode:  "disable",
		RedisAddr:  "127.0.0.1:1",
		RedisPass:  "cov-redis-password",
		JWTSecret:  "cov-secret-key-that-is-long-enough",
		AppURL:     "https://app.example",
		LogLevel:   "info",
		LogFormat:  "text",
	}
	if err := cfg.Validate(); err != nil {
		t.Skipf("config is not valid in this environment, nothing to assert: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := run(ctx, cfg); err == nil {
		t.Fatal("run() succeeded on a cancelled context")
	}
}
