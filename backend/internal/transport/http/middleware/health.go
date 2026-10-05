package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/edhases/kadrbox-server/internal/logging"
)

// ReadinessTimeout bounds the total duration of the dependency probes.
const ReadinessTimeout = 2 * time.Second

// errProbeUnconfigured means no probe was wired for a dependency. Readiness is
// fail-closed: without a real check the service must not report ready.
var errProbeUnconfigured = errors.New("health probe not configured")

// Pinger is the narrow dependency surface readiness needs. It is satisfied by
// *pgxpool.Pool (func (p *Pool) Ping(ctx context.Context) error) but NOT by
// go-redis, whose Client.Ping returns *redis.StatusCmd — hence Redis is passed
// as a func(ctx) error closure instead of this interface.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler serves liveness (/healthz) and readiness (/readyz). It also
// implements http.Handler, dispatching on the request path.
type HealthHandler struct {
	service   string
	pgPing    func(context.Context) error
	redisPing func(context.Context) error
}

// New builds the health handler. redisPing may be nil (fail-closed readiness).
// The Postgres probe is taken from SetPostgresPing so that main.go can wire
// both dependencies without widening NewRouter's signature.
func New(redisPing func(ctx context.Context) error, service string) *HealthHandler {
	if service == "" {
		service = "kadrbox-server"
	}
	return &HealthHandler{service: service, redisPing: redisPing, pgPing: PostgresPing()}
}

// WithPostgresPing overrides the Postgres probe and returns the handler.
func (h *HealthHandler) WithPostgresPing(ping func(context.Context) error) *HealthHandler {
	h.pgPing = ping
	return h
}

// WithRedisPing overrides the Redis probe and returns the handler.
func (h *HealthHandler) WithRedisPing(ping func(context.Context) error) *HealthHandler {
	h.redisPing = ping
	return h
}

var (
	probeMu       sync.RWMutex
	postgresProbe func(context.Context) error
	redisProbe    func(context.Context) error
)

// SetPostgresPing registers the process-wide Postgres readiness probe.
func SetPostgresPing(ping func(context.Context) error) {
	probeMu.Lock()
	postgresProbe = ping
	probeMu.Unlock()
}

// SetRedisPing registers the process-wide Redis readiness probe.
func SetRedisPing(ping func(context.Context) error) {
	probeMu.Lock()
	redisProbe = ping
	probeMu.Unlock()
}

// PostgresPing returns the registered Postgres probe (nil when unset).
func PostgresPing() func(context.Context) error {
	probeMu.RLock()
	defer probeMu.RUnlock()
	return postgresProbe
}

// RedisPing returns the registered Redis probe (nil when unset).
func RedisPing() func(context.Context) error {
	probeMu.RLock()
	defer probeMu.RUnlock()
	return redisProbe
}

// Livez reports that the process is alive. It must never touch a dependency:
// a failing database must not make the orchestrator restart the container.
func (h *HealthHandler) Livez(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": h.service,
	})
}

// Readyz reports whether the service can serve traffic. Every dependency is
// probed with ReadinessTimeout; a nil probe or an error yields 503.
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), ReadinessTimeout)
	defer cancel()

	for _, dep := range []struct {
		name string
		ping func(context.Context) error
	}{
		{"postgres", h.pgPing},
		{"redis", h.redisPing},
	} {
		if err := runProbe(ctx, dep.ping); err != nil {
			logging.L().WarnContext(ctx, "readiness check failed",
				"dependency", dep.name, "service", h.service, "error", err.Error())
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status":     "unavailable",
				"dependency": dep.name,
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func runProbe(ctx context.Context, ping func(context.Context) error) error {
	if ping == nil {
		return errProbeUnconfigured
	}
	return ping(ctx)
}

// ServeHTTP dispatches on the request path so the handler can also be mounted
// as a subtree handler (e.g. r.Mount("/", health)).
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz", "/health":
		h.Livez(w, r)
	case "/readyz":
		h.Readyz(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]string) {
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
