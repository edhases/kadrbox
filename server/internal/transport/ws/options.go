package ws

import "time"

// Defaults for every knob in Options. Every field left at its zero value in a
// caller-supplied Options is replaced by the value here, so `Options{}` is a
// valid, secure configuration.
const (
	// DefaultTicketTTL bounds how long a Watch Party admission ticket is
	// replayable. The ticket only has to survive the single WebSocket handshake
	// that follows its issuance, so 60s is generous; anything longer widens the
	// window in which a leaked ticket (proxy logs, referrer headers) can be
	// replayed to join a room as the victim.
	DefaultTicketTTL = 60 * time.Second

	// DefaultBroadcastBuffer is the depth of the fan-out queue. 256 events is far
	// more than a room can consume inside one tick and small enough that
	// saturation means the hub loop is wedged, not merely busy.
	DefaultBroadcastBuffer = 256

	// DefaultSendBuffer is the per-client queue depth.
	DefaultSendBuffer = 256

	// DefaultBroadcastSendTimeout bounds how long a producer may park on the
	// fan-out queue. Run() drains that queue in a bare select with no blocking
	// work left in the loop (every Redis call was moved out of the lock and off
	// this goroutine), so a 5s wait means the loop has been stuck for at least
	// five seconds. A connection stuck that long has also blown through the
	// 20s ping/pong cycle used to detect dead peers, so dropping the event and
	// counting it bounds the damage; parking forever would leak the goroutine
	// and keep the socket open after shutdown.
	DefaultBroadcastSendTimeout = 5 * time.Second

	// DefaultRedisOpTimeout bounds every Redis round-trip the hub makes. It is
	// short on purpose: no publish happens under h.mu any more, so a slow Redis
	// costs us fan-out latency, never hub-wide liveness.
	DefaultRedisOpTimeout = 2 * time.Second

	// DefaultMaxRooms caps simultaneously occupied rooms. Room codes are
	// attacker-chosen, so this bounds the rooms map, the per-room Redis
	// subscriptions and the memory held per idle client.
	DefaultMaxRooms = 512

	// DefaultMaxClientsPerRoom caps one room's fan-out cost.
	DefaultMaxClientsPerRoom = 32

	// DefaultJoinLimiterWindow / DefaultJoinsPerIPPerWindow bound reconnect
	// storms and room-code guessing from a single source.
	DefaultJoinLimiterWindow     = time.Minute
	DefaultJoinsPerIPPerWindow   = 20
	DefaultJoinLimiterMaxKeys    = 8192
	DefaultJoinsPerUserPerWindow = 10
)

// Options configures a Hub. The zero value is valid and safe: tickets are
// required (no secret means no upgrades at all), no browser origin is trusted,
// and the capacity/rate limits fall back to the defaults above.
type Options struct {
	// JWTSecret signs and verifies Watch Party admission tickets. Empty means the
	// hub refuses every upgrade with 503 — it never falls back to query-param
	// identity.
	JWTSecret string

	// TicketTTL is the default lifetime passed by callers that omit it.
	TicketTTL time.Duration

	// AllowedOrigins is the browser-origin allow-list, normally the same list the
	// CORS middleware uses. When empty, only requests with NO Origin header are
	// upgraded: native/desktop clients (the Flutter app over dart:io) send none,
	// and every cross-origin browser request is rejected.
	AllowedOrigins []string

	// MaxRooms and MaxClientsPerRoom bound hub growth. Zero means the default.
	MaxRooms          int
	MaxClientsPerRoom int

	// BroadcastBuffer and SendBuffer size the two queues. Zero means default.
	BroadcastBuffer int
	SendBuffer      int

	// BroadcastSendTimeout and RedisOpTimeout bound the two blocking points.
	// Zero means default.
	BroadcastSendTimeout time.Duration
	RedisOpTimeout       time.Duration

	// JoinLimiter tunes the per-source join limiter. The zero value uses the
	// defaults above.
	JoinLimiter JoinLimiterConfig
}

// withDefaults fills every unset field, so callers can pass Options{} or a
// partial Options without accidentally disabling a limit.
func (o Options) withDefaults() Options {
	if o.TicketTTL <= 0 {
		o.TicketTTL = DefaultTicketTTL
	}
	if o.MaxRooms <= 0 {
		o.MaxRooms = DefaultMaxRooms
	}
	if o.MaxClientsPerRoom <= 0 {
		o.MaxClientsPerRoom = DefaultMaxClientsPerRoom
	}
	if o.BroadcastBuffer <= 0 {
		o.BroadcastBuffer = DefaultBroadcastBuffer
	}
	if o.SendBuffer <= 0 {
		o.SendBuffer = DefaultSendBuffer
	}
	if o.BroadcastSendTimeout <= 0 {
		o.BroadcastSendTimeout = DefaultBroadcastSendTimeout
	}
	if o.RedisOpTimeout <= 0 {
		o.RedisOpTimeout = DefaultRedisOpTimeout
	}
	o.JoinLimiter = o.JoinLimiter.withDefaults()
	return o
}
