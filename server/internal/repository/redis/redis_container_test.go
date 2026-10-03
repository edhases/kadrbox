package redis_test

// Container-backed rig for the Redis session index.
//
// The existing Redis tests run against miniredis, which is a faithful but
// in-process RESP implementation. It cannot model the two things that make the
// session index hard to get right: real key eviction, and the interleaving of
// several clients' commands on one connection pool. Those are the properties
// this file pins, against a real redis:7-alpine.
//
// Precedence:
//  1. TEST_REDIS_ADDR already set -> use it, start nothing (CI services, a
//     developer's own instance, or an external Redis in a soak test).
//  2. -short                   -> start nothing; the rig reports "not run".
//  3. otherwise                -> start a container.
//
// The container is terminated before the process exits so repeated CI runs do
// not leak it. Docker being unavailable is reported loudly, not swallowed; see
// TestRedisContainerAvailability.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// containerRedisImage is pinned to the same major version the CI service and
	// docker-compose.yml use.
	containerRedisImage = "redis:7-alpine"

	// stableRedisContainerName is the base name of the throwaway container. The
	// PID suffix keeps two concurrent `go test` runs from colliding on Docker's
	// globally unique container names; within one process -count=2 reuses the
	// same instance.
	stableRedisContainerName = "oxide-test-redis"
)

// redisAddr is the host:port the whole package's container-backed tests use.
// Empty means "no container in this run", which every consumer must treat as
// skip, never as pass.
var redisAddr string

// TestMain is the single entry point that decides where this package's Redis
// comes from.
func TestMain(m *testing.M) {
	flag.Parse()

	container, err := startRedisForPackage()
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr,
			"\n*** WARNING: no TEST_REDIS_ADDR and no usable Docker daemon: %v\n"+
				"*** The container-backed Redis session tests are being SKIPPED in this\n"+
				"*** run. Install/start Docker, or export TEST_REDIS_ADDR.\n\n", err)
	case container != nil:
		defer func() {
			terminateCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if terr := container.Terminate(terminateCtx); terr != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to terminate the test redis container: %v\n", terr)
			}
		}()
	}

	os.Exit(m.Run())
}

func startRedisForPackage() (testcontainers.Container, error) {
	if addr := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR")); addr != "" {
		redisAddr = addr
		return nil, nil
	}
	if testing.Short() {
		return nil, nil
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := tcredis.RunContainer(startCtx,
		testcontainers.WithImage(containerRedisImage),
		withContainerName(fmt.Sprintf("%s-%d", stableRedisContainerName, os.Getpid())),
		// miniredis's log line is not emitted by redis itself, so waiting for it
		// would hang; "Ready to accept connections" is the real one.
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").WithStartupTimeout(2*time.Minute),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", containerRedisImage, err)
	}

	endpoint, err := container.Endpoint(startCtx, "")
	if err != nil {
		_ = container.Terminate(startCtx)
		return nil, fmt.Errorf("resolve the redis endpoint: %w", err)
	}
	redisAddr = endpoint
	if err := os.Setenv("TEST_REDIS_ADDR", endpoint); err != nil {
		_ = container.Terminate(startCtx)
		return nil, fmt.Errorf("export TEST_REDIS_ADDR for the rest of the package: %w", err)
	}
	return container, nil
}

// withContainerName pins the container's Docker name. Docker container names
// are global, so two concurrent runs both asking for "redis" make the loser
// fail with 409 Conflict and leak whichever container lost the race.
func withContainerName(name string) testcontainers.CustomizeRequestOption {
	return func(req *testcontainers.GenericContainerRequest) error {
		req.Name = name
		return nil
	}
}

// ctRedis returns a client against the container-backed Redis and skips when
// none is available.
func ctRedis(t *testing.T) (context.Context, *redisRepo.RedisClient) {
	t.Helper()
	if redisAddr == "" {
		t.Skip("no Redis available (TEST_REDIS_ADDR unset, -short passed, or Docker unavailable)")
	}
	client, err := redisRepo.NewRedisClient(redisAddr, "")
	if err != nil {
		t.Fatalf("NewRedisClient(%s) failed: %v", redisAddr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return context.Background(), client
}

// TestRedisContainerAvailability turns the silent skip into a visible one.
func TestRedisContainerAvailability(t *testing.T) {
	switch {
	case redisAddr == "":
		t.Skip("Redis unavailable: set TEST_REDIS_ADDR or install Docker to cover the session index")
	case testing.Short():
		t.Log("using the caller-provided TEST_REDIS_ADDR")
	default:
		t.Logf("using a throwaway %s container", containerRedisImage)
	}

	ctx, client := ctRedis(t)
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("the resolved Redis is not usable: %v", err)
	}
}