package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// TestIPRateLimiterMapIsBounded — білий ящик: мапа бакетів не може вирости
// понад maxTrackedClients навіть якщо кожен запит приходить з унікального
// адреса (найгірший сценарій "ботнет з одним запитом на IP").
func TestIPRateLimiterMapIsBounded(t *testing.T) {
	if err := SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies(nil): %v", err)
	}

	limiter := NewIPRateLimiter(1000, 1000)
	const attempts = maxTrackedClients + 25_000

	for i := 0; i < attempts; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = uniqueAddr(i) + ":1"
		limiterKey(req)
		if !limiter.allow(limiterKey(req)) {
			t.Fatalf("request %d was denied: a new client must never be permanently blocked", i)
		}
	}

	if got := len(limiter.clients); got > maxTrackedClients {
		t.Fatalf("client map grew to %d entries, cap is %d", got, maxTrackedClients)
	}
	if len(limiter.order) > 2*maxTrackedClients {
		t.Fatalf("eviction FIFO grew to %d entries, expected compaction around %d", len(limiter.order), maxTrackedClients)
	}
}

// TestIPRateLimiterEvictsOldestFirst — при переповненні першим зникає
// найдавніше створений бакет (FIFO), а не випадковий.
func TestIPRateLimiterEvictsOldestFirst(t *testing.T) {
	limiter := NewIPRateLimiter(1000, 1000)
	for i := 0; i < maxTrackedClients; i++ {
		limiter.allow(uniqueAddr(i))
	}
	if _, ok := limiter.clients[uniqueAddr(0)]; !ok {
		t.Fatal("the first bucket should still exist before overflow")
	}

	limiter.allow(uniqueAddr(maxTrackedClients))

	if _, ok := limiter.clients[uniqueAddr(0)]; ok {
		t.Error("expected the oldest bucket to be evicted once the cap is hit")
	}
	if _, ok := limiter.clients[uniqueAddr(maxTrackedClients)]; !ok {
		t.Error("expected the newly inserted bucket to be tracked")
	}
	if got := len(limiter.clients); got > maxTrackedClients {
		t.Errorf("map holds %d buckets, cap is %d", got, maxTrackedClients)
	}
}

// TestIPRateLimiterPurgeDropsIdleBuckets — purge не допускає накопичення
// клюнів від «повернувшихся» клієнтів.
func TestIPRateLimiterPurgeDropsIdleBuckets(t *testing.T) {
	limiter := NewIPRateLimiter(1000, 1000)

	for i := 0; i < 100; i++ {
		limiter.allow(uniqueAddr(i))
	}
	if len(limiter.clients) == 0 {
		t.Fatal("expected buckets to be created")
	}

	// Робимо всі бакети «старими» і запускаємо purge.
	for _, b := range limiter.clients {
		b.lastUpdate = b.lastUpdate.Add(-2 * bucketIdleTTL)
	}
	limiter.lastPurge = time.Now().Add(-time.Hour)
	limiter.allow(uniqueAddr(999_999))
	if got := len(limiter.clients); got > 2 {
		t.Fatalf("idle buckets were not purged: %d left", got)
	}
}

func TestLimiterKeyCollapsesRewrittenPeer(t *testing.T) {
	if err := SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies(nil): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	want := limiterKey(req)

	req.Header.Set("X-Forwarded-For", "198.51.100.2")
	if got := limiterKey(req); got != want {
		t.Fatalf("spoofed XFF changed the limiter key: %q -> %q", want, got)
	}
	if want != "203.0.113.9" {
		t.Fatalf("expected the socket peer to be the key, got %q", want)
	}
}

func TestExtractIPStaysUsefulForLogs(t *testing.T) {
	if err := SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.7")
	if got := extractIP(req); got != "203.0.113.9" {
		t.Fatalf("extractIP should report the client behind a trusted proxy, got %q", got)
	}

	// Для недовіреного peer'а логи показують адресу сокета, а не заголовок.
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2.RemoteAddr = "198.51.100.4:1234"
	req2.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := extractIP(req2); got != "198.51.100.4" {
		t.Fatalf("extractIP must not echo an untrusted header, got %q", got)
	}
}

func uniqueAddr(i int) string {
	return "10." + strconv.Itoa((i/65536)%256) + "." + strconv.Itoa((i/256)%256) + "." + strconv.Itoa(i%256)
}
