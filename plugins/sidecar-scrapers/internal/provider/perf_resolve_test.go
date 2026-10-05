package provider

// Latency regression tests for the player iframe fan-out in resolve.go.
//
// The bug these lock down: resolveStreamsFromItemPage looped over up to
// MaxPlayerIframes candidates one at a time, so latency was the SUM of the
// round-trips, and because every candidate shared one PlayerResolveTimeout a
// single hung player host consumed 15s of the 20s budget and the resolve gave up
// on a title a later candidate would have resolved.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// perfPlayer is one fake player host: a URL plus the tag that identifies the
// streams it produces (cdn.example/<tag>/master.m3u8).
type perfPlayer struct {
	tag string
	url string
}

// perfPlayerServer serves a player page exposing exactly one media URL after an
// optional delay. A delay of 0 with hang=true blocks until the request context
// dies, i.e. it models a player host that accepts the connection and never
// answers.
func perfPlayerServer(t *testing.T, tag string, delay time.Duration, hang bool) perfPlayer {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hang {
			<-r.Context().Done()
			return
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<html><body><script>
		var player = new Playerjs({file: "https://cdn.example/`+tag+`/master.m3u8"});
		</script></body></html>`)
	}))
	t.Cleanup(srv.Close)
	return perfPlayer{tag: tag, url: srv.URL}
}

func perfItemPage(players []perfPlayer) string {
	var b strings.Builder
	b.WriteString("<html><body><div class=\"tabs-box\">")
	for _, p := range players {
		fmt.Fprintf(&b, `<iframe src="%s/player/%s/embed/1" title="%s"></iframe>`, p.url, p.tag, p.tag)
	}
	b.WriteString("</div></body></html>")
	return b.String()
}

// tagOfPlayerURL extracts the perfPlayer tag from its iframe URL.
func tagOfPlayerURL(playerURL string) string {
	const marker = "/player/"
	idx := strings.Index(playerURL, marker)
	if idx < 0 {
		return playerURL
	}
	rest := playerURL[idx+len(marker):]
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// TestPerfPlayerFanoutIsMaxNotSum asserts N player endpoints of equal latency are
// probed concurrently: total time must track the SLOWEST one, not their sum.
func TestPerfPlayerFanoutIsMaxNotSum(t *testing.T) {
	const (
		players = 6
		delay   = 300 * time.Millisecond
	)

	pls := make([]perfPlayer, players)
	for i := range pls {
		pls[i] = perfPlayerServer(t, fmt.Sprintf("p%d", i), delay, false)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	begin := time.Now()
	resp, err := resolveStreamsFromItemPage(ctx, covTLS(t), "lavakino",
		"https://lavakino.net/filmys/1-test.html", perfItemPage(pls), 0, 0, "")
	elapsed := time.Since(begin)

	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(resp.Streams) != players {
		t.Errorf("expected %d streams from %d players, got %d", players, players, len(resp.Streams))
	}

	summed := players * delay
	t.Logf("%d players x %v resolved in %v (sum would be %v)", players, delay, elapsed, summed)

	// A single round-trip plus slack. Sequential fan-out needed ~1.8s here.
	if limit := 3 * delay; elapsed > limit {
		t.Errorf("fan-out looks sequential: %v > %v (one round-trip is %v)", elapsed, limit, delay)
	}
}

// TestPerfHungPlayerDoesNotConsumeWholeBudget is the budget half: one player host
// that never answers must not eat the shared PlayerResolveTimeout and cause the
// resolvable candidates to be reported as unresolvable.
func TestPerfHungPlayerDoesNotConsumeWholeBudget(t *testing.T) {
	const good = 3

	// The hung candidate is listed FIRST in the item page, which is where the old
	// sequential loop would have started and blocked.
	pls := []perfPlayer{perfPlayerServer(t, "hung", 0, true)}
	for i := 0; i < good; i++ {
		pls = append(pls, perfPlayerServer(t, fmt.Sprintf("good%d", i), 50*time.Millisecond, false))
	}

	// A caller budget larger than playerCandidateTimeout but far smaller than the
	// old behaviour needed: sequential, this would blow through it after the hung
	// candidate alone.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	begin := time.Now()
	resp, err := resolveStreamsFromItemPage(ctx, covTLS(t), "uakino",
		"https://uakino.biz/filmy/1-test.html", perfItemPage(pls), 0, 0, "")
	elapsed := time.Since(begin)

	if err != nil {
		t.Fatalf("a hung candidate must not fail the whole resolve: %v", err)
	}
	if len(resp.Streams) != good {
		t.Errorf("expected %d streams from the healthy players, got %d", good, len(resp.Streams))
	}

	t.Logf("resolve with one hung player took %v (per-candidate sub-budget %v, shared budget %v)",
		elapsed, playerCandidateTimeout, PlayerResolveTimeout)

	// The hung candidate must be cut off by its own sub-budget, not by the whole
	// call, so the caller sees the healthy results rather than an error.
	if elapsed > playerCandidateTimeout+3*time.Second {
		t.Errorf("resolve took %v; the hung candidate was not bounded by its %v sub-budget",
			elapsed, playerCandidateTimeout)
	}
}

// TestPerfHungCandidatesStillYieldErrUnresolvable keeps the failure path honest:
// when EVERY candidate hangs, the resolve still reports ErrUnresolvablePlayer.
func TestPerfHungCandidatesStillYieldErrUnresolvable(t *testing.T) {
	pls := []perfPlayer{
		perfPlayerServer(t, "h0", 0, true),
		perfPlayerServer(t, "h1", 0, true),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := resolveStreamsFromItemPage(ctx, covTLS(t), "eneyida",
		"https://eneyida.tv/films/1-test.html", perfItemPage(pls), 0, 0, "")
	if err == nil {
		t.Fatal("expected an error when every player candidate hangs")
	}
	if !strings.Contains(err.Error(), "eneyida") {
		t.Errorf("error must name the provider, got %v", err)
	}
}

// TestPerfPlayerFanoutPreservesRankOrder asserts the parallel fan-out did not
// change the MERGE order: streams must still follow the candidate ranking, not
// completion order (the fastest server is deliberately NOT first).
func TestPerfPlayerFanoutPreservesRankOrder(t *testing.T) {
	slow := perfPlayerServer(t, "slow", 400*time.Millisecond, false)
	fast := perfPlayerServer(t, "fast", 10*time.Millisecond, false)

	// Both are cross-host player iframes; ranking is score-based, so pin the
	// expectation to the ranking itself rather than to a hand-written order.
	itemPage := perfItemPage([]perfPlayer{slow, fast})
	candidates := rankPlayerCandidates(itemPage, "https://lavakino.net/filmys/1-test.html")
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d (%+v)", len(candidates), candidates)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := resolveStreamsFromItemPage(ctx, covTLS(t), "lavakino",
		"https://lavakino.net/filmys/1-test.html", itemPage, 0, 0, "")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(resp.Streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(resp.Streams))
	}

	// The media path of each candidate's own stream ends in /<tag>/master.m3u8,
	// which identifies the candidate that produced it. The stream merged first
	// must come from the rank-0 candidate, even though the rank-0 candidate is the
	// SLOWER one — a completion-ordered merge would invert this.
	const prefix = "https://cdn.example/"
	if !strings.HasPrefix(resp.Streams[0].URL, prefix) || !strings.HasPrefix(resp.Streams[1].URL, prefix) {
		t.Fatalf("unexpected streams: %q, %q", resp.Streams[0].URL, resp.Streams[1].URL)
	}
	gotFirst := strings.TrimSuffix(strings.TrimPrefix(resp.Streams[0].URL, prefix), "/master.m3u8")
	gotSecond := strings.TrimSuffix(strings.TrimPrefix(resp.Streams[1].URL, prefix), "/master.m3u8")
	wantFirst := tagOfPlayerURL(candidates[0].URL)
	wantSecond := tagOfPlayerURL(candidates[1].URL)

	if gotFirst != wantFirst || gotSecond != wantSecond {
		t.Errorf("merged order must follow candidate rank: got (%s, %s), want (%s, %s)",
			gotFirst, gotSecond, wantFirst, wantSecond)
	}
}

// TestPerfPlayerFanoutAllFetchesIssued guards against a fan-out that quietly
// probes only the first candidate (which would still satisfy the timing tests
// above when the pool is not saturated).
func TestPerfPlayerFanoutAllFetchesIssued(t *testing.T) {
	const players = 6
	var issued int64
	pls := make([]perfPlayer, players)
	for i := range pls {
		tag := fmt.Sprintf("m%d", i)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&issued, 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, `<html><body><script>
			var p = {sources: [{src: "https://cdn.example/`+tag+`/master.m3u8"}]};
			</script></body></html>`)
		}))
		t.Cleanup(srv.Close)
		pls[i] = perfPlayer{tag: tag, url: srv.URL}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := resolveStreamsFromItemPage(ctx, covTLS(t), "lavakino",
		"https://lavakino.net/filmys/1-test.html", perfItemPage(pls), 0, 0, "")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if got := atomic.LoadInt64(&issued); got != players {
		t.Errorf("expected every candidate to be probed exactly once, got %d fetches for %d candidates",
			got, players)
	}
	if len(resp.Streams) != players {
		t.Errorf("expected %d merged streams, got %d", players, len(resp.Streams))
	}
}
