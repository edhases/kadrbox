package ws

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edhases/oxide-server/internal/domain"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// recordingBus wraps the real Redis client so a test can observe what was
// published, and can hook the publish to assert on the hub's lock state. It
// embeds the concrete client so it still satisfies EventSubscriber and
// StateStore as *redisRepo.RedisClient does.
type recordingBus struct {
	*redisRepo.RedisClient
	mu        sync.Mutex
	events    []domain.WatchPartyEvent
	onPublish func()
}

func (b *recordingBus) setHook(fn func()) {
	b.mu.Lock()
	b.onPublish = fn
	b.mu.Unlock()
}

func (b *recordingBus) PublishWatchPartyEvent(ctx context.Context, roomCode string, event *domain.WatchPartyEvent) error {
	b.mu.Lock()
	hook := b.onPublish
	b.mu.Unlock()
	if hook != nil {
		hook()
	}
	b.mu.Lock()
	b.events = append(b.events, *event)
	b.mu.Unlock()
	return b.RedisClient.PublishWatchPartyEvent(ctx, roomCode, event)
}

func (b *recordingBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

func (b *recordingBus) actions() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.events))
	for _, e := range b.events {
		out = append(out, e.Action)
	}
	return out
}

func startRedisHub(t *testing.T, mutate ...func(*Options)) (*Hub, *recordingBus) {
	t.Helper()
	m, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(m.Close)

	client, err := redisRepo.NewRedisClient(m.Addr(), "")
	if err != nil {
		t.Fatalf("redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	bus := &recordingBus{RedisClient: client}
	cfg := Options{JWTSecret: testSecret}
	for _, fn := range mutate {
		fn(&cfg)
	}
	h := NewHub(bus, cfg)
	go h.Run()
	t.Cleanup(func() {
		h.GracefulStop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.WaitPumps(ctx)
	})
	return h, bus
}

// ---------------------------------------------------------------------------
// publish behaviour
// ---------------------------------------------------------------------------

func TestJoinAndLeaveArePublished(t *testing.T) {
	h, bus := startRedisHub(t)
	srv := hubServer(t, h)

	host := mustDial(t, srv, "/?room=PUB0001&ticket="+ticket(t, h, "u-host", "Host", "PUB0001", time.Minute))
	waitRoomSize(t, h, "PUB0001", 1, 2*time.Second)
	waitCounter(t, func() int64 { return int64(countAction(bus.actions(), ActionUserJoined)) }, 1, "join publish")

	host.Close()
	waitRoomsEmpty(t, h, 3*time.Second)
	waitCounter(t, func() int64 { return int64(countAction(bus.actions(), ActionUserLeft)) }, 1, "userLeft publish")

	actions := bus.actions()
	// Ordering between userLeft and the asynchronous roomInfo publish is not
	// guaranteed, so assert on presence.
	if countAction(actions, ActionUserJoined) == 0 || countAction(actions, ActionUserLeft) == 0 {
		t.Fatalf("published actions = %v, want both userJoined and userLeft", actions)
	}
}

func countAction(actions []string, want string) int {
	n := 0
	for _, a := range actions {
		if a == want {
			n++
		}
	}
	return n
}

// TestPublishHappensOutsideTheHubLock is the regression for publishing under
// h.mu: the bus callback tries to take the same lock the hub loop holds, which
// only completes if the publish runs after the unlock.
func TestPublishHappensOutsideTheHubLock(t *testing.T) {
	h, bus := startRedisHub(t)
	bus.setHook(func() {
		// Deadlocks if the hub still holds h.mu at publish time.
		h.mu.RLock()
		h.mu.RUnlock()
	})

	srv := hubServer(t, h)
	host := mustDial(t, srv, "/?room=PUB0002&ticket="+ticket(t, h, "u-host", "Host", "PUB0002", time.Minute))
	waitRoomSize(t, h, "PUB0002", 1, 2*time.Second)
	awaitAction(t, host, ActionUserJoined, 2*time.Second)

	if err := host.WriteMessage(websocket.TextMessage, []byte(`{"action":"play"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitCounter(t, func() int64 { return int64(bus.count()) }, 2, "publishes including the inbound frame")
}

func TestInboundFrameIsPublishedWithAuthenticatedIdentity(t *testing.T) {
	h, bus := startRedisHub(t)
	srv := hubServer(t, h)

	host := mustDial(t, srv, "/?room=PUB0003&ticket="+ticket(t, h, "u-real", "RealUser", "PUB0003", time.Minute))
	waitRoomSize(t, h, "PUB0003", 1, 2*time.Second)
	awaitAction(t, host, ActionRoomInfo, 2*time.Second)

	// The frame tries to impersonate somebody else; the published copy must carry
	// the ticket's identity, not the body.
	if err := host.WriteMessage(websocket.TextMessage, []byte(
		`{"action":"chat","payload":"hi","senderId":"u-victim","senderName":"Admin","roomCode":"OTHER"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitCounter(t, func() int64 { return int64(bus.count()) }, 2, "publishes including the inbound frame")

	bus.mu.Lock()
	defer bus.mu.Unlock()
	published := bus.events[len(bus.events)-1]
	if published.SenderID != "u-real" || published.SenderName != "RealUser" {
		t.Fatalf("published sender = %q/%q, want u-real/RealUser", published.SenderID, published.SenderName)
	}
	if published.RoomCode != "PUB0003" {
		t.Fatalf("published room = %q, want PUB0003", published.RoomCode)
	}
}

func TestPublishContextIsBounded(t *testing.T) {
	// A bus that only ends when its context does must not wedge the hub loop: the
	// publish runs outside h.mu and under RedisOpTimeout.
	h := NewHub(&hangingBus{hang: 5 * time.Second}, Options{
		JWTSecret:         testSecret,
		RedisOpTimeout:    50 * time.Millisecond,
		MaxRooms:          16,
		MaxClientsPerRoom: 4,
	})
	go h.Run()
	defer h.GracefulStop()

	srv := hubServer(t, h)
	start := time.Now()
	_ = mustDial(t, srv, "/?room=HANG001&ticket="+ticket(t, h, "u-1", "A", "HANG001", time.Minute))
	waitRoomSize(t, h, "HANG001", 1, 3*time.Second)

	// The loop survived the timed-out publish and keeps accepting clients.
	_ = mustDial(t, srv, "/?room=HANG001&ticket="+ticket(t, h, "u-2", "B", "HANG001", time.Minute))
	waitRoomSize(t, h, "HANG001", 2, 3*time.Second)

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("a slow event bus cost %s of hub downtime", elapsed)
	}
}

// hangingBus ignores the deadline on purpose: it models a Redis call that only
// ends when its context does.
type hangingBus struct {
	hang time.Duration
}

func (b *hangingBus) PublishWatchPartyEvent(ctx context.Context, _ string, _ *domain.WatchPartyEvent) error {
	select {
	case <-time.After(b.hang):
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// subscription fan-out
// ---------------------------------------------------------------------------

func TestStartSubscriptionIsIdempotent(t *testing.T) {
	h, _ := startRedisHub(t)
	ctx := context.Background()

	if err := h.StartSubscription(ctx); err != nil {
		t.Fatalf("first StartSubscription: %v", err)
	}
	if err := h.StartSubscription(ctx); err != ErrSubscriptionAlreadyStarted {
		t.Fatalf("second StartSubscription = %v, want ErrSubscriptionAlreadyStarted", err)
	}
}

// TestSubscriptionFansOutRemoteEvents is the regression for the write-only
// pub/sub: an event published to the channel by another party must reach local
// clients, while this instance's own publishes must not be duplicated.
func TestSubscriptionFansOutRemoteEvents(t *testing.T) {
	h, bus := startRedisHub(t)
	srv := hubServer(t, h)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := h.StartSubscription(ctx); err != nil {
		t.Fatalf("StartSubscription: %v", err)
	}

	host := mustDial(t, srv, "/?room=FAN0001&ticket="+ticket(t, h, "u-host", "Host", "FAN0001", time.Minute))
	waitRoomSize(t, h, "FAN0001", 1, 2*time.Second)
	awaitAction(t, host, ActionRoomInfo, 2*time.Second)

	// Wait for the per-room subscription to actually be open.
	waitForSubscription(t, h, "FAN0001")

	// A remote instance publishes: the frame must be fanned out locally. The
	// publish is repeated because a real deployment can deliver the first one
	// while the subscription is still settling, and the point of the assertion is
	// the fan-out, not the race.
	remote := &domain.WatchPartyEvent{
		Action: ActionChat, RoomCode: "FAN0001", SenderID: "u-remote",
		SenderName: "Remote", Payload: "from another instance", Timestamp: time.Now(),
	}
	stopRemote := make(chan struct{})
	remoteDone := make(chan struct{})
	go func() {
		defer close(remoteDone)
		for {
			select {
			case <-stopRemote:
				return
			case <-time.After(50 * time.Millisecond):
				_ = bus.PublishWatchPartyEvent(ctx, "FAN0001", remote)
			}
		}
	}()
	ev := awaitPayload(t, host, "from another instance", 5*time.Second)
	close(stopRemote)
	<-remoteDone
	if ev.SenderID != "u-remote" {
		t.Fatalf("remote event not relayed verbatim: %+v", ev)
	}

	// This instance's own publish comes back over the bus and must be suppressed,
	// otherwise every local client sees each frame twice.
	before := bus.count()
	if err := host.WriteMessage(websocket.TextMessage, []byte(`{"action":"chat","payload":"local"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitCounter(t, func() int64 { return int64(bus.count()) }, int64(before)+1, "local publish")

	// Nothing else should arrive (the local frame was already echo-filtered, and
	// the bus echo must be suppressed too).
	expectSilence(t, host, 500*time.Millisecond)
}

func waitForSubscription(t *testing.T, h *Hub, room string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.subMu.Lock()
		_, ok := h.subs[room]
		h.subMu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no subscription was opened for room %s", room)
}

func TestSubscriptionRejectsForeignRoomFrames(t *testing.T) {
	// A hub whose loop is not started keeps the fan-out queue observable.
	h := NewHub(nil, Options{JWTSecret: testSecret})
	h.handleRemoteEvent("ROOMZ", []byte(`{"action":"chat","roomCode":"OTHER","payload":"x"}`))
	if len(h.broadcast) != 0 {
		t.Fatalf("queue holds %d frames, want 0", len(h.broadcast))
	}
	h.handleRemoteEvent("ROOMZ", []byte(`{"action":"chat","roomCode":"ROOMZ","payload":"x"}`))
	if len(h.broadcast) != 1 {
		t.Fatalf("queue holds %d frames, want 1", len(h.broadcast))
	}
}

// ---------------------------------------------------------------------------
// room state
// ---------------------------------------------------------------------------

func TestRoomStateIsHandedToJoiningClient(t *testing.T) {
	h, _ := startRedisHub(t)
	srv := hubServer(t, h)

	if err := h.SaveRoomState(context.Background(), "STAT001", &domain.WatchPartyState{
		HostID: "u-host", MediaID: "m-1", CurrentPositionMs: 90_000,
		IsPlaying: true, PlaybackSpeed: 1.25,
	}); err != nil {
		t.Fatalf("SaveRoomState: %v", err)
	}

	state, err := h.RoomState(context.Background(), "STAT001")
	if err != nil {
		t.Fatalf("RoomState: %v", err)
	}
	if state.CurrentPositionMs != 90_000 || !state.IsPlaying {
		t.Fatalf("round-tripped state = %+v", state)
	}

	conn := mustDial(t, srv, "/?room=STAT001&ticket="+ticket(t, h, "u-1", "A", "STAT001", time.Minute))
	info := awaitAction(t, conn, ActionRoomInfo, 3*time.Second)
	payload, ok := info.Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("roomInfo payload = %#v", info.Payload)
	}
	carried, ok := payload["state"].(map[string]interface{})
	if !ok || carried["currentPositionMs"] != float64(90_000) {
		t.Fatalf("roomInfo did not carry playback state: %#v", payload)
	}
}

func TestHostFramesPersistPlaybackState(t *testing.T) {
	h, _ := startRedisHub(t)
	srv := hubServer(t, h)

	host := mustDial(t, srv, "/?room=STAT002&ticket="+ticket(t, h, "u-host", "Host", "STAT002", time.Minute))
	waitRoomSize(t, h, "STAT002", 1, 2*time.Second)
	awaitAction(t, host, ActionRoomInfo, 2*time.Second)

	for _, frame := range []string{
		`{"action":"play"}`,
		`{"action":"seek","payload":12000}`,
		`{"action":"speed","payload":1.5}`,
		`{"action":"pause"}`,
	} {
		if err := host.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			t.Fatalf("write %s: %v", frame, err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, err := h.RoomState(context.Background(), "STAT002")
		if err == nil && state.HostID == "u-host" && state.CurrentPositionMs == 12_000 &&
			!state.IsPlaying && state.PlaybackSpeed == 1.5 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	state, _ := h.RoomState(context.Background(), "STAT002")
	t.Fatalf("persisted state never converged: %+v", state)
}

func TestGuestFramesDoNotPersistState(t *testing.T) {
	h, _ := startRedisHub(t)
	srv := hubServer(t, h)

	_ = mustDial(t, srv, "/?room=STAT003&ticket="+ticket(t, h, "u-host", "Host", "STAT003", time.Minute))
	waitRoomSize(t, h, "STAT003", 1, 2*time.Second)

	guest := mustDial(t, srv, "/?room=STAT003&ticket="+ticket(t, h, "u-guest", "Guest", "STAT003", time.Minute))
	waitRoomSize(t, h, "STAT003", 2, 2*time.Second)

	if err := guest.WriteMessage(websocket.TextMessage, []byte(`{"action":"seek","payload":99000}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitCounter(t, func() int64 { return h.Stats().RejectedMessages }, 1, "dropped guest frame")

	if state, err := h.RoomState(context.Background(), "STAT003"); err == nil && state != nil {
		t.Fatalf("a guest's rejected frame still changed playback state: %+v", state)
	}
}

func TestStateStoreUnavailableWithoutBus(t *testing.T) {
	h := NewHub(nil, Options{JWTSecret: testSecret})
	if err := h.SaveRoomState(context.Background(), "ROOM1", &domain.WatchPartyState{}); err != ErrStateUnavailable {
		t.Fatalf("SaveRoomState err = %v, want ErrStateUnavailable", err)
	}
}

func TestEventJSONShapeIsUnchanged(t *testing.T) {
	// The Flutter client parses these exact keys; changing them is a breaking
	// protocol change, so pin the shape.
	event := &domain.WatchPartyEvent{
		Action: ActionSeek, RoomCode: "ROOM1", SenderID: "u-1", SenderName: "A",
		Payload: int64(42000), Timestamp: time.Now(),
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"action"`, `"roomCode"`, `"senderId"`, `"senderName"`, `"payload"`, `"timestamp"`} {
		if !strings.Contains(string(encoded), key) {
			t.Fatalf("event JSON %s is missing %s", encoded, key)
		}
	}
}
