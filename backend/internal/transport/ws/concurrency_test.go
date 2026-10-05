package ws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/gorilla/websocket"
)

// These tests exist for the -race detector: every one of them drives the paths
// that previously deadlocked, panicked, or raced.

// TestConcurrentUpgradesBroadcastAndShutdown is the regression for the two
// blocking points that used to hang forever: `h.broadcast <- &event` in readPump
// had no shutdown escape, and GracefulStop closed stopChan unconditionally. Many
// clients join while others flood the room, all of it racing a shutdown.
func TestConcurrentUpgradesBroadcastAndShutdown(t *testing.T) {
	h := NewHub(nil, Options{
		JWTSecret:            testSecret,
		MaxRooms:             32,
		MaxClientsPerRoom:    16,
		BroadcastSendTimeout: 200 * time.Millisecond,
		RedisOpTimeout:       100 * time.Millisecond,
		BroadcastBuffer:      8,
		SendBuffer:           4,
		JoinLimiter:          JoinLimiterConfig{Disabled: true},
	})
	go h.Run()
	srv := hubServer(t, h)

	const (
		rooms  = 3
		peers  = 5
		rounds = 10
	)

	var wg sync.WaitGroup
	conns := make(chan *websocket.Conn, rooms*peers)

	// Joiners.
	for i := 0; i < rooms*peers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			room := fmt.Sprintf("RACE%02d", i%rooms)
			conn, _, err := dialWS(srv, "/?room="+room+"&ticket="+ticket(t, h, fmt.Sprintf("u-%d", i), "P", room, time.Minute), nil)
			if err != nil {
				return // rate/capacity rejections are a legitimate outcome here
			}
			conns <- conn
		}(i)
	}

	// Flooders: every room's first client is its host, so use its own ticket.
	for i := 0; i < peers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			room := fmt.Sprintf("RACE%02d", i%rooms)
			conn, _, err := dialWS(srv, "/?room="+room+"&ticket="+ticket(t, h, fmt.Sprintf("u-%d", i), "P", room, time.Minute), nil)
			if err != nil {
				return
			}
			defer conn.Close()
			for round := 0; round < rounds; round++ {
				frames := []string{
					`{"action":"play"}`,
					`{"action":"pause"}`,
					`{"action":"seek","payload":1000}`,
					`{"action":"chat","payload":"flood"}`,
					`{"action":"speed","payload":1.25}`,
				}
				for _, frame := range frames {
					if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
						return
					}
				}
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Shut down underneath all of it.
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(25 * time.Millisecond)
		h.GracefulStop()
		h.GracefulStop() // idempotent
		h.GracefulStop()
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent join/broadcast/shutdown did not finish")
	}

	// Reap the admitted clients.
	close(conns)
	for conn := range conns {
		_ = conn.Close()
	}

	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("hub loop did not return")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.WaitPumps(ctx); err != nil {
		t.Fatalf("WaitPumps: %v", err)
	}

	h.mu.RLock()
	remaining := len(h.rooms)
	h.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("hub.rooms still holds %d entries after shutdown", remaining)
	}
}

// TestReadPumpBroadcastSendsAreAbortable drives the specific line that used to
// park forever: `c.hub.broadcast <- &event` had no shutdown escape. The hub loop
// is wedged here by holding h.mu, which is exactly the situation that used to
// park a readPump forever holding an open socket. The pump must give up after
// BroadcastSendTimeout and stay usable.
func TestReadPumpBroadcastSendsAreAbortable(t *testing.T) {
	h := NewHub(nil, Options{
		JWTSecret:            testSecret,
		BroadcastSendTimeout: 100 * time.Millisecond,
		BroadcastBuffer:      1,
		SendBuffer:           16,
		JoinLimiter:          JoinLimiterConfig{Disabled: true},
	})
	go h.Run()
	defer h.GracefulStop()
	srv := hubServer(t, h)

	host := mustDial(t, srv, "/?room=WEDGE1&ticket="+ticket(t, h, "u-1", "Host", "WEDGE1", time.Minute))
	guest := mustDial(t, srv, "/?room=WEDGE1&ticket="+ticket(t, h, "u-2", "Guest", "WEDGE1", time.Minute))
	waitRoomSize(t, h, "WEDGE1", 2, 2*time.Second)
	// Unordered: roomInfo is built in a goroutine (Redis read) and races userJoined.
	awaitActions(t, host, []string{ActionRoomInfo, ActionUserJoined}, 5*time.Second)
	awaitActions(t, guest, []string{ActionUserJoined}, 5*time.Second)

	// Wedge the hub loop so the fan-out queue fills up and producers have to wait.
	h.mu.Lock()
	for i := 0; i < 6; i++ {
		if err := host.WriteMessage(websocket.TextMessage, []byte(`{"action":"chat","payload":"wedged"}`)); err != nil {
			h.mu.Unlock()
			t.Fatalf("write: %v", err)
		}
	}
	time.Sleep(400 * time.Millisecond) // 4x BroadcastSendTimeout
	h.mu.Unlock()

	if h.Stats().DroppedMessages == 0 {
		t.Fatal("no message was dropped even though the queue was wedged")
	}

	// The readPump survived the abort and the hub recovered: this frame must reach
	// the guest (frames queued before the wedge are drained first).
	if err := host.WriteMessage(websocket.TextMessage, []byte(`{"action":"chat","payload":"after"}`)); err != nil {
		t.Fatalf("write after the wedge: %v", err)
	}
	awaitPayload(t, guest, "after", 5*time.Second)
}

// TestEnqueueIsAbortableOnStop covers the shutdown arm of the same select.
func TestEnqueueIsAbortableOnStop(t *testing.T) {
	h := NewHub(nil, Options{
		JWTSecret:            testSecret,
		BroadcastSendTimeout: 5 * time.Second, // long enough that only stopChan can help
	})
	// Loop deliberately not started, so the queue fills and stays full.
	for i := 0; i < h.opts.BroadcastBuffer; i++ {
		h.broadcast <- &domain.WatchPartyEvent{Action: ActionChat, RoomCode: "FULL001"}
	}

	blocked := make(chan bool, 1)
	go func() { blocked <- h.enqueue(&domain.WatchPartyEvent{Action: ActionChat, RoomCode: "FULL001"}) }()

	select {
	case ok := <-blocked:
		t.Fatalf("enqueue returned %v while the queue was full and the hub was live", ok)
	case <-time.After(100 * time.Millisecond):
	}

	h.GracefulStop()
	select {
	case ok := <-blocked:
		if ok {
			t.Fatal("enqueue reported success after shutdown")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("enqueue stayed parked after GracefulStop")
	}
}

// TestShutdownWhileClientsAreConnected hammers the upgrade path against a hub
// that is stopping, which used to leave half-upgraded sockets with pumps that
// could never be reaped.
func TestShutdownWhileClientsAreConnected(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		h := NewHub(nil, Options{JWTSecret: testSecret, JoinLimiter: JoinLimiterConfig{Disabled: true}})
		go h.Run()
		srv := hubServer(t, h)

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				room := "STOP001"
				conn, _, err := dialWS(srv, "/?room="+room+"&ticket="+ticket(t, h, fmt.Sprintf("u-%d", i), "P", room, time.Minute), nil)
				if err != nil {
					return
				}
				defer conn.Close()
				// Read until the socket closes, which is what a real client does.
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				for {
					if _, _, err := conn.ReadMessage(); err != nil {
						return
					}
				}
			}(i)
		}

		time.Sleep(time.Duration(attempt) * 3 * time.Millisecond)
		h.GracefulStop()

		finished := make(chan struct{})
		go func() { wg.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(15 * time.Second):
			t.Fatalf("attempt %d: clients did not finish after shutdown", attempt)
		}

		select {
		case <-h.Done():
		case <-time.After(5 * time.Second):
			t.Fatalf("attempt %d: hub loop did not return", attempt)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := h.WaitPumps(ctx); err != nil {
			cancel()
			t.Fatalf("attempt %d: WaitPumps: %v", attempt, err)
		}
		cancel()
		srv.Close()
	}
}

// TestConcurrentGracefulStopCalls asserts stopOnce really serialises the close,
// since main.go and a signal handler can both reach it.
func TestConcurrentGracefulStopCalls(t *testing.T) {
	h := NewHub(nil, Options{JWTSecret: testSecret})
	go h.Run()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.GracefulStop()
		}()
	}
	wg.Wait()

	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("hub loop did not return")
	}
}

// TestConcurrentRegistrationAndEviction mixes registration, eviction of
// slow consumers, host re-election and shutdown on the same rooms, which is where
// the shared dropLocked helper has to stay correct.
func TestConcurrentRegistrationAndEviction(t *testing.T) {
	h := NewHub(nil, Options{
		JWTSecret:         testSecret,
		MaxRooms:          8,
		MaxClientsPerRoom: 4,
		BroadcastBuffer:   4,
		SendBuffer:        1,
		JoinLimiter:       JoinLimiterConfig{Disabled: true},
	})
	go h.Run()

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			room := fmt.Sprintf("EVICT%02d", i%4)
			client := syntheticClient(h, room, fmt.Sprintf("u-%d", i), "P", 1)
			h.register <- client
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				h.broadcast <- &domain.WatchPartyEvent{
					Action: ActionChat, RoomCode: fmt.Sprintf("EVICT%02d", j%4),
					SenderID: "u-x", Payload: "flood", Timestamp: time.Now(),
				}
			}
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("registration/eviction storm did not settle")
	}

	h.GracefulStop()
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("hub loop did not return")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.WaitPumps(ctx); err != nil {
		t.Fatalf("WaitPumps: %v", err)
	}

	h.mu.RLock()
	remaining := len(h.rooms)
	h.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("hub.rooms = %d after shutdown, want 0", remaining)
	}
}

// TestRejectedUpgradesDoNotLeakReservations hammers the admission path with bad
// tickets and checks the capacity bookkeeping stays balanced.
func TestRejectedUpgradesDoNotLeakReservations(t *testing.T) {
	h := NewHub(nil, Options{
		JWTSecret:         testSecret,
		MaxClientsPerRoom: 2,
		JoinLimiter:       JoinLimiterConfig{Disabled: true},
	})
	go h.Run()
	defer h.GracefulStop()
	srv := hubServer(t, h)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Half are admitted, half fail admission after reserving.
			token := ticket(t, h, fmt.Sprintf("u-%d", i), "P", "LEAK001", time.Minute)
			if i%2 == 0 {
				token = "garbage"
			}
			conn, _, err := dialWS(srv, "/?room=LEAK001&ticket="+token, nil)
			if err == nil {
				_ = conn.Close()
			}
		}(i)
	}
	wg.Wait()

	h.mu.RLock()
	pending := h.reserved["LEAK001"]
	clients := len(h.rooms["LEAK001"])
	h.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("reservations leaked: %d pending", pending)
	}
	if clients > 2 {
		t.Fatalf("room holds %d clients, want at most 2", clients)
	}
}

// TestUpgradesAfterStopAreRejected checks a client that handshakes during
// shutdown gets a clean HTTP error instead of a hung socket.
func TestUpgradesAfterStopAreRejected(t *testing.T) {
	h := NewHub(nil, Options{JWTSecret: testSecret, JoinLimiter: JoinLimiterConfig{Disabled: true}})
	go h.Run()
	hubServer(t, h)
	h.GracefulStop()
	<-h.Done()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/watch-party?room=AFTER1&ticket="+ticket(t, h, "u-1", "A", "AFTER1", time.Minute), nil)
	h.HandleWebSocket(rec, req)
	if rec.Code == http.StatusSwitchingProtocols {
		t.Fatal("an upgrade was admitted after shutdown")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
