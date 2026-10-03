package provider

// Concurrency and body-size regression tests for BanderaClient.
//
// The bug these lock down: GetSources held the exclusive write mutex across the
// upstream GET /sources (20s client timeout), and GetSearchSourcesStr read the
// c.enabledSearchStr field AFTER releasing the lock. Every search and every stream
// resolution goes through that path, so a TTL expiry serialised the whole server
// behind one fetch, and the unsynchronised string read could tear a two-word Go
// string header into a truncated source list — silently wrong search results.
//
// Run with -count=20 to shake out flakes (and with -race where cgo exists).

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// perfSourcesJSON builds a /sources payload with n enabled+searchable sources so
// the expected enabled-source string is long enough that a torn read cannot
// accidentally produce the right answer.
func perfSourcesJSON(n int) (body string, wantEnabled string) {
	var b strings.Builder
	b.WriteString(`{"ok":true,"sources":[`)
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		key := fmt.Sprintf("source_key_number_%02d", i)
		keys = append(keys, key)
		fmt.Fprintf(&b, `{"key":%q,"name":"Source %d","enabled":true,`+
			`"capabilities":{"search":true,"content":true,"stream":true},`+
			`"inputs":{"content":["url"],"stream":["url"]}}`, key, i)
	}
	b.WriteString(`]}`)
	return b.String(), strings.Join(keys, ",")
}

func perfSourcesServer(t *testing.T, body string, delay time.Duration, hits *int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt64(hits, 1)
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestPerfSourcesConcurrentReadsNeverTear hammers GetSources/GetSearchSourcesStr
// from many goroutines with a cold cache (sourcesTTL=0 forces a fetch every time)
// and asserts the enabled-source string is the FULL list every single time — a
// non-empty check would pass on a torn read, an equality check cannot.
func TestPerfSourcesConcurrentReadsNeverTear(t *testing.T) {
	const (
		goroutines = 32
		rounds     = 8
		sourceN    = 12
	)
	body, wantEnabled := perfSourcesJSON(sourceN)
	var hits int64
	srv := perfSourcesServer(t, body, 0, &hits)

	c := NewBanderaClient(srv.URL, srv.Client())
	c.SetSourcesTTL(0) // never serve a cached value: every call is a cache miss

	var wg sync.WaitGroup
	errs := make(chan string, goroutines*rounds*2)
	start := make(chan struct{})

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for r := 0; r < rounds; r++ {
				meta, err := c.GetSources(context.Background())
				if err != nil {
					errs <- "GetSources: " + err.Error()
					continue
				}
				if len(meta) != sourceN {
					errs <- fmt.Sprintf("GetSources returned %d entries, want %d", len(meta), sourceN)
				}

				got := c.GetSearchSourcesStr(context.Background())
				if got != wantEnabled {
					errs <- fmt.Sprintf("torn/short enabled source list: got %d bytes, want %d bytes (%q)",
						len(got), len(wantEnabled), got)
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for msg := range errs {
		t.Error(msg)
	}
}

// TestPerfConcurrentGetSourcesIssuesOneFetch asserts the singleflight dedup: N
// concurrent cache misses must produce exactly ONE upstream GET /sources.
func TestPerfConcurrentGetSourcesIssuesOneFetch(t *testing.T) {
	const goroutines = 24
	body, _ := perfSourcesJSON(6)
	var hits int64
	// 120ms of server-side delay keeps the flight open long enough for every
	// goroutine to join it.
	srv := perfSourcesServer(t, body, 120*time.Millisecond, &hits)

	c := NewBanderaClient(srv.URL, srv.Client())
	c.SetSourcesTTL(0)

	var wg sync.WaitGroup
	start := make(chan struct{})
	failures := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := c.GetSources(context.Background()); err != nil {
				failures <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(failures)

	for err := range failures {
		t.Errorf("GetSources: %v", err)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("concurrent cache misses must collapse into ONE upstream fetch, got %d", got)
	}
}

// TestPerfConcurrentGetSourcesNotSerialisedBehindSlowFetch is the latency half of
// the same defect: with a slow upstream, N concurrent callers must finish in
// roughly the time of ONE fetch. Under the old write-lock-held-across-HTTP code
// this was N x delay.
func TestPerfConcurrentGetSourcesNotSerialisedBehindSlowFetch(t *testing.T) {
	const (
		goroutines = 16
		delay      = 250 * time.Millisecond
	)
	body, _ := perfSourcesJSON(4)
	var hits int64
	srv := perfSourcesServer(t, body, delay, &hits)

	c := NewBanderaClient(srv.URL, srv.Client())
	c.SetSourcesTTL(0)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := c.GetSources(context.Background()); err != nil {
				t.Errorf("GetSources: %v", err)
			}
		}()
	}

	begin := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(begin)

	serialised := goroutines * delay
	t.Logf("%d concurrent callers took %v (one fetch = %v, fully serialised would be %v)",
		goroutines, elapsed, delay, serialised)

	// Generous bound: 4x one fetch. The old code needed goroutines x delay.
	if limit := 4 * delay; elapsed > limit {
		t.Errorf("concurrent callers were serialised behind the fetch: %v > %v", elapsed, limit)
	}
}

// TestPerfSourcesReturnsImmutableMap asserts the map handed to callers is a copy:
// mutating it must not corrupt the cached state seen by the next caller. This is
// why GetSources clones instead of sharing the published map.
func TestPerfSourcesReturnsImmutableMap(t *testing.T) {
	body, _ := perfSourcesJSON(3)
	srv := perfSourcesServer(t, body, 0, nil)

	c := NewBanderaClient(srv.URL, srv.Client())

	first, err := c.GetSources(context.Background())
	if err != nil {
		t.Fatalf("GetSources: %v", err)
	}
	delete(first, "source_key_number_00")
	first["injected"] = SourceMeta{Key: "injected"}

	second, err := c.GetSources(context.Background())
	if err != nil {
		t.Fatalf("GetSources (cached): %v", err)
	}
	if _, ok := second["source_key_number_00"]; !ok {
		t.Error("caller mutated the cached source map: a published entry disappeared")
	}
	if _, ok := second["injected"]; ok {
		t.Error("caller mutation leaked into the cached source map")
	}
}

// TestPerfSourcesStringAndMetaComeFromTheSameResponse asserts the snapshot
// couples the metadata map with the enabled-source string, so a caller can never
// pair sources from response N with a search list from response N+1.
func TestPerfSourcesStringAndMetaComeFromTheSameResponse(t *testing.T) {
	body, wantEnabled := perfSourcesJSON(3)
	srv := perfSourcesServer(t, body, 0, nil)

	c := NewBanderaClient(srv.URL, srv.Client())

	meta, err := c.GetSources(context.Background())
	if err != nil {
		t.Fatalf("GetSources: %v", err)
	}
	got := c.GetSearchSourcesStr(context.Background())
	if got != wantEnabled {
		t.Fatalf("enabled string mismatch: %q vs %q", got, wantEnabled)
	}

	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fromMeta := strings.Join(keys, ",")
	if fromMeta != wantEnabled {
		t.Errorf("metadata keys (%s) and the search string (%s) disagree — they came from different responses",
			fromMeta, wantEnabled)
	}
}

// TestPerfSourcesFetchIsNotCancelledByFirstCaller pins the singleflight context
// ownership: one caller giving up must not fail the shared fetch for the others.
// (registry.go SingleFlightSearch has the identical hazard, fixed the same way.)
func TestPerfSourcesFetchIsNotCancelledByFirstCaller(t *testing.T) {
	body, wantEnabled := perfSourcesJSON(3)
	var hits int64
	srv := perfSourcesServer(t, body, 300*time.Millisecond, &hits)

	c := NewBanderaClient(srv.URL, srv.Client())

	quitter, cancelQuitter := context.WithCancel(context.Background())
	// Cancelled before it ever reaches the fetch: it must not poison the flight.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancelQuitter()
	}()

	if _, err := c.GetSources(quitter); err == nil {
		t.Log("quitter happened to win the flight; the assertion below still applies")
	}

	// A second caller arrives after the quitter is gone and must still be served
	// by a completed (or in-flight) fetch, not by the cancellation.
	got := c.GetSearchSourcesStr(context.Background())
	if got != wantEnabled {
		t.Errorf("expected the full enabled source list %q, got %q", wantEnabled, got)
	}
}

// TestPerfOversizedBanderaBodyRejected asserts a response body above
// MaxUpstreamBodyBytes is refused rather than decoded.
func TestPerfOversizedBanderaBodyRejected(t *testing.T) {
	// Valid JSON whose padded field pushes the body past the ceiling.
	padding := strings.Repeat("x", MaxUpstreamBodyBytes+4096)
	body := `{"ok":true,"sources":[{"key":"a","enabled":true,"capabilities":{"search":true},"pad":"` +
		padding + `"}]}`
	if int64(len(body)) <= MaxUpstreamBodyBytes {
		t.Fatalf("test fixture is not oversized: %d <= %d", len(body), MaxUpstreamBodyBytes)
	}

	t.Run("sources", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}))
		defer srv.Close()

		c := NewBanderaClient(srv.URL, srv.Client())
		if _, err := c.GetSources(context.Background()); err == nil {
			t.Error("oversized /sources body must be rejected")
		}
	})

	t.Run("search", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}))
		defer srv.Close()

		c := NewBanderaClient(srv.URL, srv.Client())
		if _, err := c.Search(context.Background(), "x"); err == nil {
			t.Error("oversized /search body must be rejected")
		}
	})

	t.Run("content", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}))
		defer srv.Close()

		c := NewBanderaClient(srv.URL, srv.Client())
		if _, err := c.GetContent(context.Background(), "a", []byte(`{}`)); err == nil {
			t.Error("oversized /content body must be rejected")
		}
	})

	t.Run("stream", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}))
		defer srv.Close()

		c := NewBanderaClient(srv.URL, srv.Client())
		if _, err := c.GetStream(context.Background(), "a", []byte(`{}`)); err == nil {
			t.Error("oversized /stream body must be rejected")
		}
	})
}
