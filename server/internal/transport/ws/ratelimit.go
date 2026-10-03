package ws

import (
	"sync"
	"time"
)

// JoinLimiterConfig tunes the self-contained join limiter. The zero value means
// "enabled with the package defaults"; disabling it entirely takes a deliberate
// Disabled: true because the endpoint is otherwise trivially abuseable.
type JoinLimiterConfig struct {
	Disabled   bool
	Window     time.Duration
	MaxPerKey  int
	MaxBuckets int
}

func (c JoinLimiterConfig) withDefaults() JoinLimiterConfig {
	if c.Window <= 0 {
		c.Window = DefaultJoinLimiterWindow
	}
	if c.MaxPerKey <= 0 {
		c.MaxPerKey = DefaultJoinsPerIPPerWindow
	}
	if c.MaxBuckets <= 0 {
		c.MaxBuckets = DefaultJoinLimiterMaxKeys
	}
	return c
}

type joinBucket struct {
	count   int
	resetAt time.Time
}

// JoinLimiter is a fixed-window counter with lazy TTL cleanup and a hard key
// cap. It is intentionally dependency-free: a token bucket or sliding window
// would need nothing more than this, and the router's own middleware lives in a
// package this one must not depend on.
//
// Memory is bounded two ways: buckets expire after Window, and once MaxBuckets
// distinct keys are live the limiter refuses NEW keys rather than growing. An
// attacker rotating source addresses therefore cannot force unbounded growth;
// the cost is that such a burst is rejected until buckets age out.
type JoinLimiter struct {
	cfg       JoinLimiterConfig
	mu        sync.Mutex
	buckets   map[string]*joinBucket
	lastSweep time.Time
}

func NewJoinLimiter(cfg JoinLimiterConfig) *JoinLimiter {
	return &JoinLimiter{
		cfg:     cfg.withDefaults(),
		buckets: make(map[string]*joinBucket),
	}
}

// Allow consumes one unit for key. It reports whether the join may proceed.
// A nil limiter allows everything, which is what Disabled: true produces.
func (l *JoinLimiter) Allow(key string) bool {
	if l == nil || l.cfg.Disabled {
		return true
	}
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= l.cfg.Window/4 {
		l.sweepLocked(now)
		l.lastSweep = now
	}

	bucket, ok := l.buckets[key]
	if !ok || !now.Before(bucket.resetAt) {
		if len(l.buckets) >= l.cfg.MaxBuckets {
			return false
		}
		bucket = &joinBucket{resetAt: now.Add(l.cfg.Window)}
		l.buckets[key] = bucket
	}
	bucket.count++
	return bucket.count <= l.cfg.MaxPerKey
}

// sweepLocked drops every bucket whose window has elapsed. Called at most once
// per Window/4, so the scan cost is amortised to O(liveKeys) per that interval.
func (l *JoinLimiter) sweepLocked(now time.Time) {
	for key, bucket := range l.buckets {
		if !now.Before(bucket.resetAt) {
			delete(l.buckets, key)
		}
	}
}

// tracked reports how many live buckets exist. Test-only.
func (l *JoinLimiter) tracked() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
