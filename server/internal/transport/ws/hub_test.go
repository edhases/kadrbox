package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/gorilla/websocket"
)

const testSecret = "unit-test-secret"

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

// startHub builds a hub with a ticket secret and a running loop, and registers
// a cleanup that stops it and waits for the pumps.
func startHub(t *testing.T, mutate ...func(*Options)) *Hub {
	t.Helper()
	cfg := Options{JWTSecret: testSecret}
	for _, fn := range mutate {
		fn(&cfg)
	}
	h := NewHub(nil, cfg)
	go h.Run()
	t.Cleanup(func() {
		h.GracefulStop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.WaitPumps(ctx)
		select {
		case <-h.Done():
		case <-ctx.Done():
			t.Error("hub loop did not return")
		}
	})
	return h
}

// syntheticClient registers a client without a socket. Only Run touches conn
// (and only to close it on shutdown, which it does not do), so a nil conn is safe
// here and lets the queue-eviction paths be driven deterministically.
func syntheticClient(h *Hub, room, id, name string, sendBuf int) *Client {
	return &Client{hub: h, send: make(chan []byte, sendBuf), roomCode: room, userID: id, userName: name}
}

func ticket(t *testing.T, h *Hub, userID, userName, room string, ttl time.Duration) string {
	t.Helper()
	token, err := h.IssueWatchPartyTicket(userID, userName, room, ttl)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	return token
}

// hubServer exposes a running hub over HTTP.
func hubServer(t *testing.T, h *Hub) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h.HandleWebSocket))
	t.Cleanup(srv.Close)
	return srv
}

func dialWS(srv *httptest.Server, query string, header http.Header) (*websocket.Conn, *http.Response, error) {
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + query
	return websocket.DefaultDialer.Dial(url, header)
}

func mustDial(t *testing.T, srv *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	conn, _, err := dialWS(srv, query, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// awaitAction reads until the wanted action arrives, discarding the server's
// other traffic (roomInfo is emitted from its own goroutine, so its position in
// the stream relative to userJoined is not deterministic).
func awaitAction(t *testing.T, conn *websocket.Conn, want string, timeout time.Duration) domain.WatchPartyEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		_ = conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %q failed: %v", want, err)
		}
		var ev domain.WatchPartyEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			t.Fatalf("not a WatchPartyEvent: %v (%s)", err, data)
		}
		if ev.Action == want {
			return ev
		}
	}
}

// expectSilence asserts nothing arrives within d.
//
// IMPORTANT: gorilla's read deadline is not resumable — once a read on a
// connection times out, that connection must not be read again (the next read
// reports a timeout even when a frame is waiting). So every expectSilence call
// must be the LAST read performed on that connection.
func expectSilence(t *testing.T, conn *websocket.Conn, d time.Duration) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(d))
	if _, data, err := conn.ReadMessage(); err == nil {
		t.Fatalf("unexpected frame: %s", data)
	}
}

// awaitPayload reads chat frames until one carries the wanted payload.
func awaitPayload(t *testing.T, conn *websocket.Conn, want string, timeout time.Duration) domain.WatchPartyEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ev := awaitAction(t, conn, ActionChat, time.Until(deadline))
		if ev.Payload == want {
			return ev
		}
		if time.Now().After(deadline) {
			t.Fatalf("chat %q never arrived", want)
		}
	}
}

// waitCounter polls a counter until it reaches want.
func waitCounter(t *testing.T, get func() int64, want int64, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if get() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s = %d, want >= %d", what, get(), want)
}

func roomSize(h *Hub, room string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[room])
}

func totalRooms(h *Hub) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms)
}

func waitRoomSize(t *testing.T, h *Hub, room string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if roomSize(h, room) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("room %s: want %d clients, have %d", room, want, roomSize(h, room))
}

func waitRoomsEmpty(t *testing.T, h *Hub, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if totalRooms(h) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("hub.rooms still holds %d entries", totalRooms(h))
}

func drain(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a queued frame")
		return nil
	}
}

func parseEvent(t *testing.T, msg []byte) domain.WatchPartyEvent {
	t.Helper()
	var ev domain.WatchPartyEvent
	if err := json.Unmarshal(msg, &ev); err != nil {
		t.Fatalf("not a WatchPartyEvent: %v (%s)", err, msg)
	}
	return ev
}

// drainAction waits for the next queued frame carrying the wanted action,
// discarding others (roomInfo is emitted from its own goroutine, so its position
// in a client's queue is not deterministic).
func drainAction(t *testing.T, ch <-chan []byte, want string) domain.WatchPartyEvent {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			ev := parseEvent(t, msg)
			if ev.Action == want {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a %q frame", want)
			return domain.WatchPartyEvent{}
		}
	}
}

// expectNoAction asserts the unwanted action never appears within d.
func expectNoAction(t *testing.T, ch <-chan []byte, unwanted string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case msg := <-ch:
			if ev := parseEvent(t, msg); ev.Action == unwanted {
				t.Fatalf("did not expect a %q frame, got %+v", unwanted, ev)
			}
		case <-deadline:
			return
		}
	}
}

// ---------------------------------------------------------------------------
// room registry / eviction
// ---------------------------------------------------------------------------

// TestUnregisterRemovesEmptyRoomEntry is the direct regression for the leak that
// the slow-consumer branch used to leave behind: the last client going away must
// take its room entry with it.
func TestUnregisterRemovesEmptyRoomEntry(t *testing.T) {
	h := startHub(t)

	a := syntheticClient(h, "ROOM1", "u-a", "A", 8)
	b := syntheticClient(h, "ROOM1", "u-b", "B", 8)
	h.register <- a
	drainAction(t, a.send, ActionUserJoined)
	h.register <- b
	drainAction(t, b.send, ActionUserJoined)
	waitRoomSize(t, h, "ROOM1", 2, time.Second)

	h.unregister <- a
	waitRoomSize(t, h, "ROOM1", 1, time.Second)

	h.unregister <- b
	waitRoomsEmpty(t, h, 2*time.Second)
	if totalRooms(h) != 0 {
		t.Fatalf("hub.rooms = %d entries, want 0", totalRooms(h))
	}
}

// TestSlowConsumerEvictionRemovesEmptyRoom covers the same leak through the
// eviction path: a client whose queue is full is dropped AND its room entry is
// deleted. Before the fix the client was deleted but the empty room survived, so
// every attacker-chosen room code leaked a map entry.
func TestSlowConsumerEvictionRemovesEmptyRoom(t *testing.T) {
	h := startHub(t)

	slow := syntheticClient(h, "ROOM2", "u-slow", "Slow", 2)
	h.register <- slow
	drainAction(t, slow.send, ActionUserJoined)
	drainAction(t, slow.send, ActionRoomInfo)

	// Fill both slots so the next fan-out has to evict.
	slow.send <- []byte(`{"filler":1}`)
	slow.send <- []byte(`{"filler":2}`)

	h.broadcast <- &domain.WatchPartyEvent{
		Action: ActionPlay, RoomCode: "ROOM2", SenderID: "u-other", Timestamp: time.Now(),
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && totalRooms(h) != 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := totalRooms(h); got != 0 {
		t.Fatalf("hub.rooms = %d entries after the last client was evicted, want 0", got)
	}
	if stats := h.Stats(); stats.DroppedSlowConsumers == 0 {
		t.Fatal("slow-consumer eviction was not counted")
	}
}

func TestBroadcastIsolatedByRoom(t *testing.T) {
	h := startHub(t)

	a := syntheticClient(h, "ROOM3", "u-a", "A", 8)
	b := syntheticClient(h, "OTHER1", "u-b", "B", 8)
	h.register <- a
	drainAction(t, a.send, ActionUserJoined)
	h.register <- b
	drainAction(t, b.send, ActionUserJoined)

	h.broadcast <- &domain.WatchPartyEvent{
		Action: ActionPlay, RoomCode: "ROOM3", SenderID: "u-x", Timestamp: time.Now(),
	}
	if ev := drainAction(t, a.send, ActionPlay); ev.Action != ActionPlay {
		t.Fatalf("room member got %q", ev.Action)
	}
	expectNoAction(t, b.send, ActionPlay, 150*time.Millisecond)
}

func TestEchoFilterHidesOwnPlaybackFrames(t *testing.T) {
	h := startHub(t)

	a := syntheticClient(h, "ROOM4", "u-a", "A", 8)
	b := syntheticClient(h, "ROOM4", "u-b", "B", 8)
	h.register <- a
	drainAction(t, a.send, ActionUserJoined)
	h.register <- b
	drainAction(t, b.send, ActionUserJoined)

	h.broadcast <- &domain.WatchPartyEvent{
		Action: ActionPlay, RoomCode: "ROOM4", SenderID: "u-a", Timestamp: time.Now(),
	}
	ev := drainAction(t, b.send, ActionPlay)
	if ev.SenderID != "u-a" {
		t.Fatalf("SenderID = %q, want u-a", ev.SenderID)
	}
	expectNoAction(t, a.send, ActionPlay, 200*time.Millisecond)
}

// TestDropLockedIsIdempotent guards the invariant the shared eviction helper has
// to hold: a send channel is closed exactly once, no matter how many paths try
// to evict the same client.
func TestDropLockedIsIdempotent(t *testing.T) {
	h := startHub(t)
	c := syntheticClient(h, "ROOM5", "u-a", "A", 8)
	h.register <- c
	drain(t, c.send)

	h.mu.Lock()
	if !h.dropLocked(c) {
		h.mu.Unlock()
		t.Fatal("first dropLocked returned false for a registered client")
	}
	if h.dropLocked(c) {
		h.mu.Unlock()
		t.Fatal("second dropLocked returned true; send channel would be closed twice")
	}
	h.mu.Unlock()

	if totalRooms(h) != 0 {
		t.Fatalf("hub.rooms = %d, want 0", totalRooms(h))
	}
}

// ---------------------------------------------------------------------------
// capacity
// ---------------------------------------------------------------------------

func TestRoomCapacityRejectsOverLimitUpgrades(t *testing.T) {
	h := startHub(t, func(o *Options) {
		o.MaxClientsPerRoom = 2
		o.MaxRooms = 4
	})
	srv := hubServer(t, h)

	conn1 := mustDial(t, srv, "/?room=CAP001&ticket="+ticket(t, h, "u-1", "A", "CAP001", time.Minute))
	waitRoomSize(t, h, "CAP001", 1, time.Second)
	_ = conn1

	conn2 := mustDial(t, srv, "/?room=CAP001&ticket="+ticket(t, h, "u-2", "B", "CAP001", time.Minute))
	waitRoomSize(t, h, "CAP001", 2, time.Second)
	_ = conn2

	conn, resp, err := dialWS(srv, "/?room=CAP001&ticket="+ticket(t, h, "u-3", "C", "CAP001", time.Minute), nil)
	if err == nil {
		conn.Close()
		t.Fatal("third client was admitted to a full room")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %v, want 503", resp)
	}
}

func TestRoomCountCapacityRejectsNewRooms(t *testing.T) {
	h := startHub(t, func(o *Options) {
		o.MaxRooms = 1
		o.MaxClientsPerRoom = 8
	})
	srv := hubServer(t, h)

	_ = mustDial(t, srv, "/?room=ROOMAA&ticket="+ticket(t, h, "u-1", "A", "ROOMAA", time.Minute))
	waitRoomSize(t, h, "ROOMAA", 1, time.Second)

	_, resp, err := dialWS(srv, "/?room=ROOMBB&ticket="+ticket(t, h, "u-2", "B", "ROOMBB", time.Minute), nil)
	if err == nil {
		t.Fatal("a second room was admitted with MaxRooms=1")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %v, want 503", resp)
	}
}

// TestReservationReleasedOnFailedUpgrade makes sure a failed handshake does not
// leak a capacity slot.
func TestReservationReleasedOnFailedUpgrade(t *testing.T) {
	h := startHub(t, func(o *Options) { o.MaxClientsPerRoom = 1 })
	srv := hubServer(t, h)

	// A plain GET is not a WebSocket handshake: the upgrade fails after the
	// reservation was taken.
	resp, err := http.Get(srv.URL + "/?room=RESV01&ticket=" + ticket(t, h, "u-1", "A", "RESV01", time.Minute))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()

	h.mu.RLock()
	pending := h.reserved["RESV01"]
	h.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("reservation leaked: reserved[RESV01] = %d", pending)
	}

	// The slot is still usable.
	_ = mustDial(t, srv, "/?room=RESV01&ticket="+ticket(t, h, "u-1", "A", "RESV01", time.Minute))
	waitRoomSize(t, h, "RESV01", 1, time.Second)
}

// ---------------------------------------------------------------------------
// shutdown
// ---------------------------------------------------------------------------

// TestGracefulStopIsIdempotent is the regression for the unconditional
// close(h.stopChan), which panicked on the second call.
func TestGracefulStopIsIdempotent(t *testing.T) {
	h := NewHub(nil, Options{JWTSecret: testSecret})
	go h.Run()

	for i := 0; i < 5; i++ {
		h.GracefulStop()
	}
	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after GracefulStop")
	}
}

// TestGracefulStopReapsPumpsAndSignalsDone checks the shutdown contract the
// platform layer needs: Done closes when Run returns and WaitPumps returns only
// after both pumps per client are gone.
func TestGracefulStopReapsPumpsAndSignalsDone(t *testing.T) {
	h := NewHub(nil, Options{JWTSecret: testSecret})
	go h.Run()
	srv := hubServer(t, h)

	conns := make([]*websocket.Conn, 0, 3)
	for i := 0; i < 3; i++ {
		id := "u-" + string(rune('a'+i))
		conn := mustDial(t, srv, "/?room=SHUT01&ticket="+ticket(t, h, id, id, "SHUT01", time.Minute))
		conns = append(conns, conn)
	}
	waitRoomSize(t, h, "SHUT01", 3, 2*time.Second)

	h.GracefulStop()
	h.GracefulStop() // idempotent

	select {
	case <-h.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("hub loop did not return")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.WaitPumps(ctx); err != nil {
		t.Fatalf("WaitPumps: %v", err)
	}

	for i, conn := range conns {
		// Frames queued before the Close frame may still be in flight, so read
		// until the socket reports the close.
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var closeErr *websocket.CloseError
		for closeErr == nil {
			_, _, err := conn.ReadMessage()
			if err == nil {
				continue
			}
			var ok bool
			closeErr, ok = err.(*websocket.CloseError)
			if !ok {
				closeErr = &websocket.CloseError{Code: websocket.CloseAbnormalClosure}
			}
		}
		if closeErr.Code != websocket.CloseGoingAway {
			t.Fatalf("client %d: want CloseGoingAway (1001), got %d", i, closeErr.Code)
		}
	}

	if got := totalRooms(h); got != 0 {
		t.Fatalf("hub.rooms = %d entries after shutdown, want 0", got)
	}
}

func TestWaitPumpsHonoursContext(t *testing.T) {
	h := NewHub(nil, Options{JWTSecret: testSecret})
	h.wg.Add(1) // a pump that will never return

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := h.WaitPumps(ctx); err == nil {
		t.Fatal("WaitPumps returned nil while a pump was still running")
	}
	h.wg.Done()
}

// ---------------------------------------------------------------------------
// lifecycle glue
// ---------------------------------------------------------------------------

func TestNewHubDefaultsAreFailClosed(t *testing.T) {
	h := NewHub(nil)
	if h.issuer != nil {
		t.Fatal("a hub with no secret must not be able to issue tickets")
	}
	if _, err := h.IssueWatchPartyTicket("u", "n", "ROOM1", time.Minute); err == nil {
		t.Fatal("issuing a ticket without a secret must fail")
	}
	if _, err := h.RoomState(context.Background(), "ROOM1"); err != ErrStateUnavailable {
		t.Fatalf("RoomState err = %v, want ErrStateUnavailable", err)
	}
	if err := h.StartSubscription(context.Background()); err != ErrSubscriptionUnavailable {
		t.Fatalf("StartSubscription err = %v, want ErrSubscriptionUnavailable", err)
	}
	if h.opts.MaxRooms != DefaultMaxRooms || h.opts.MaxClientsPerRoom != DefaultMaxClientsPerRoom {
		t.Fatalf("zero Options did not fall back to defaults: %+v", h.opts)
	}
	if len(h.originRules) != 0 {
		t.Fatal("zero Options must not trust any browser origin")
	}
}
