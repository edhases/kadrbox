package ws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/gorilla/websocket"
	goredis "github.com/redis/go-redis/v9"
)

var (
	// ErrHubStopped means the hub is shutting down; no new upgrades.
	ErrHubStopped = errors.New("ws: hub is shutting down")
	// ErrTooManyRooms / ErrRoomFull are the capacity guards.
	ErrTooManyRooms = errors.New("ws: room capacity exhausted")
	ErrRoomFull     = errors.New("ws: room is full")
	// ErrSubscriptionUnavailable / ErrSubscriptionAlreadyStarted bound
	// StartSubscription.
	ErrSubscriptionUnavailable    = errors.New("ws: event bus does not support subscriptions")
	ErrSubscriptionAlreadyStarted = errors.New("ws: subscription already started")
	// ErrStateUnavailable means no StateStore was wired in.
	ErrStateUnavailable = errors.New("ws: room state store not configured")
)

// EventBus is the narrow Redis surface the hub needs. It is declared here rather
// than reusing http.EventPublisher so that this package stays free of any
// dependency on the HTTP layer's shape.
type EventBus interface {
	PublishWatchPartyEvent(ctx context.Context, roomCode string, event *domain.WatchPartyEvent) error
}

// EventSubscriber is the optional half of EventBus that StartSubscription needs.
// *repository/redis.RedisClient satisfies both interfaces.
type EventSubscriber interface {
	SubscribeWatchPartyEvents(ctx context.Context, roomCode string) *goredis.PubSub
}

// StateStore is the narrow interface over the previously-unreachable Redis room
// playback state helpers, used to hand a joining client the current position.
type StateStore interface {
	GetWatchPartyState(ctx context.Context, roomCode string) (*domain.WatchPartyState, error)
	SetWatchPartyState(ctx context.Context, roomCode string, state *domain.WatchPartyState) error
}

// Client is one authenticated WebSocket participant. userID and userName come
// from verified ticket claims and from nowhere else.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	roomCode string
	userID   string
	userName string

	// hostClaim records that the ticket asserted is_host, which makes this client
	// the host on election. The live host is never cached on the Client: readPump
	// asks the hub under h.mu, so host re-election cannot race a pump that is
	// validating a frame.
	hostClaim bool
	joinedAt  time.Time
	remoteIP  string
	ticketID  string
}

// HubStats is a snapshot of the counters that matter operationally.
type HubStats struct {
	DroppedSlowConsumers int64
	DroppedMessages      int64
	RejectedMessages     int64
	RejectedJoins        int64
	RejectedOrigins      int64
}

// roomSubscription is one Redis channel subscription. The id lets the owning
// goroutine confirm on exit that the map slot is still its own (cancel funcs are
// not comparable).
type roomSubscription struct {
	id       uint64
	cancelFn context.CancelFunc
}

type Hub struct {
	bus   EventBus
	state StateStore

	// rooms, hosts and reserved are guarded by mu; mutated only under mu.Lock.
	rooms    map[string]map[*Client]bool
	hosts    map[string]string
	reserved map[string]int

	broadcast  chan *domain.WatchPartyEvent
	register   chan *Client
	unregister chan *Client

	mu       sync.RWMutex
	stopChan chan struct{}
	stopOnce sync.Once
	done     chan struct{}
	wg       sync.WaitGroup

	opts        Options
	upgrader    websocket.Upgrader
	originRules []originRule
	limiter     *JoinLimiter
	issuer      TicketIssuer
	secret      []byte

	subscriber EventSubscriber
	subCtx     context.Context
	subStarted bool
	subMu      sync.Mutex
	subIDSeq   atomic.Uint64
	subs       map[string]*roomSubscription
	recent     *recentFingerprints

	droppedClients atomic.Int64
	droppedMsgs    atomic.Int64
	rejectedMsgs   atomic.Int64
	rejectedJoins  atomic.Int64
	rejectedOrigin atomic.Int64
}

// NewHub builds a hub over the given (possibly nil) event bus.
//
// The variadic Options keep the historical NewHub(redisClient) call site
// compiling while still allowing a fully configured hub. NewHub(nil) is a valid
// in-memory hub that simply refuses every upgrade: with no JWTSecret no ticket
// can be issued or verified, and there is deliberately no fallback to
// query-string identity.
func NewHub(bus EventBus, opts ...Options) *Hub {
	cfg := Options{}
	if len(opts) > 0 {
		cfg = opts[0]
	}
	cfg = cfg.withDefaults()

	h := &Hub{
		bus:        bus,
		rooms:      make(map[string]map[*Client]bool),
		hosts:      make(map[string]string),
		reserved:   make(map[string]int),
		broadcast:  make(chan *domain.WatchPartyEvent, cfg.BroadcastBuffer),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		stopChan:   make(chan struct{}),
		done:       make(chan struct{}),
		opts:       cfg,
		limiter:    NewJoinLimiter(cfg.JoinLimiter),
		recent:     newRecentFingerprints(2 * cfg.RedisOpTimeout),
		subs:       make(map[string]*roomSubscription),
	}
	if bus != nil {
		if sub, ok := bus.(EventSubscriber); ok {
			h.subscriber = sub
		}
		if store, ok := bus.(StateStore); ok {
			h.state = store
		}
	}
	if strings.TrimSpace(cfg.JWTSecret) != "" {
		h.issuer = NewTicketIssuer(cfg.JWTSecret)
		h.secret = []byte(cfg.JWTSecret)
	}
	for _, entry := range cfg.AllowedOrigins {
		if rule, ok := parseOriginRule(entry); ok {
			h.originRules = append(h.originRules, rule)
		}
	}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// Defence in depth: HandleWebSocket already rejects foreign origins with a
		// JSON body, so this only ever fires if that check is ever removed.
		CheckOrigin: h.checkOrigin,
	}
	return h
}

// StartSubscription turns on cross-instance fan-out.
//
// Redis pub/sub was otherwise write-only: nothing called
// SubscribeWatchPartyEvents, so an event published by one instance never reached
// a client connected to another. This subscribes one channel per occupied room
// (started when the first client registers, cancelled when the room empties) and
// feeds what arrives into the local fan-out queue, dropping the echo of events
// this instance published itself.
//
// It is safe to call once; a second call returns ErrSubscriptionAlreadyStarted
// instead of opening duplicate subscriptions. Wiring:
//
//	if err := wsHub.StartSubscription(ctx); err != nil && !errors.Is(err, ws.ErrSubscriptionUnavailable) {
//	    log.Printf("[WS Hub] cross-instance fan-out disabled: %v", err)
//	}
func (h *Hub) StartSubscription(ctx context.Context) error {
	if h.bus == nil || h.subscriber == nil {
		return ErrSubscriptionUnavailable
	}
	h.subMu.Lock()
	if h.subStarted {
		h.subMu.Unlock()
		return ErrSubscriptionAlreadyStarted
	}
	h.subStarted = true
	h.subCtx = ctx
	h.subMu.Unlock()
	log.Printf("[WS Hub] Redis pub/sub subscription enabled")
	return nil
}

// Stats returns a counter snapshot.
func (h *Hub) Stats() HubStats {
	return HubStats{
		DroppedSlowConsumers: h.droppedClients.Load(),
		DroppedMessages:      h.droppedMsgs.Load(),
		RejectedMessages:     h.rejectedMsgs.Load(),
		RejectedJoins:        h.rejectedJoins.Load(),
		RejectedOrigins:      h.rejectedOrigin.Load(),
	}
}

// Done is closed when Run returns. Callers shutting the hub down should wait on
// this rather than sleeping.
func (h *Hub) Done() <-chan struct{} { return h.done }

// WaitPumps blocks until every readPump/writePump has returned, or ctx expires.
// Call it only after the HTTP listener has stopped accepting, because a new
// upgrade would otherwise race wg.Add against w.Wait.
func (h *Hub) WaitPumps(ctx context.Context) error {
	drained := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("ws: client pumps did not settle: %w", ctx.Err())
	}
}

// GracefulStop signals Run to tear the hub down. It is idempotent.
func (h *Hub) GracefulStop() {
	h.stopOnce.Do(func() { close(h.stopChan) })
}

// Run is the hub event loop. It is the only writer of h.rooms/h.hosts (outside
// HandleWebSocket's reservation bookkeeping) and it performs no blocking I/O:
// every Redis call happens after the lock is released, off this goroutine.
func (h *Hub) Run() {
	defer close(h.done)
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] hub loop panic: %v", rec)
		}
	}()

	for {
		select {
		case <-h.stopChan:
			h.shutdown()
			return

		case client := <-h.register:
			h.onRegister(client)

		case client := <-h.unregister:
			h.onClientGone(client)

		case event := <-h.broadcast:
			var evicted []*Client
			h.mu.Lock()
			evicted = h.broadcastToRoomLocked(event)
			h.mu.Unlock()
			for _, c := range evicted {
				h.onClientGone(c)
			}
		}
	}
}

// shutdown closes every client queue and clears the registries. The Close frame
// is written by each writePump, which owns its connection, so no goroutine ever
// writes to a socket another goroutine is writing to.
func (h *Hub) shutdown() {
	h.mu.Lock()
	for code, clients := range h.rooms {
		for c := range clients {
			delete(clients, c)
			close(c.send)
		}
		delete(h.rooms, code)
	}
	h.rooms = make(map[string]map[*Client]bool)
	h.hosts = make(map[string]string)
	h.reserved = make(map[string]int)
	h.mu.Unlock()

	h.stopAllSubscriptions()
}

func (h *Hub) onRegister(c *Client) {
	// A register can still win the select in HandleWebSocket after GracefulStop
	// closed stopChan. Refusing here keeps shutdown()'s cleared registry from
	// being repopulated with a client nobody will ever tear down.
	select {
	case <-h.stopChan:
		h.release(c.roomCode)
		close(c.send)
		if c.conn != nil {
			_ = c.conn.Close()
		}
		return
	default:
	}

	c.joinedAt = time.Now()

	h.mu.Lock()
	if n := h.reserved[c.roomCode]; n > 1 {
		h.reserved[c.roomCode] = n - 1
	} else {
		delete(h.reserved, c.roomCode)
	}
	if h.rooms[c.roomCode] == nil {
		h.rooms[c.roomCode] = make(map[*Client]bool)
	}
	h.rooms[c.roomCode][c] = true
	becameHost := h.assignHostLocked(c)
	joined := &domain.WatchPartyEvent{
		Action:     ActionUserJoined,
		RoomCode:   c.roomCode,
		SenderID:   c.userID,
		SenderName: c.userName,
		Timestamp:  time.Now(),
	}
	h.broadcastToRoomLocked(joined)
	h.mu.Unlock()

	// Redis first, unlocked: publishing under h.mu would let one slow Redis
	// round-trip stall every other room.
	h.publish(joined)
	if becameHost {
		h.emitRoomInfoAsync(c.roomCode)
	}
	h.ensureSubscription(c.roomCode)
}

// assignHostLocked elects the host. Returns true when this client became host.
//
// Precedence: an explicit is_host ticket claim wins; otherwise the first
// authenticated client to register becomes host.
func (h *Hub) assignHostLocked(c *Client) bool {
	host, known := h.hosts[c.roomCode]
	hasHost := known && h.roomHasUserLocked(c.roomCode, host)
	switch {
	case c.hostClaim && (!hasHost || host != c.userID):
		h.hosts[c.roomCode] = c.userID
		return true
	case !hasHost:
		h.hosts[c.roomCode] = c.userID
		return true
	default:
		return false
	}
}

// isRoomHost reports whether c currently holds the host role for its room. The
// read is taken under h.mu because reelectHostLocked mutates h.hosts while
// pumps are running.
func (h *Hub) isRoomHost(c *Client) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.hosts[c.roomCode] == c.userID
}

// reelectHostLocked promotes the longest-present remaining client once the host
// is gone. Returns true when a promotion happened.
func (h *Hub) reelectHostLocked(roomCode string) bool {
	if h.roomHasUserLocked(roomCode, h.hosts[roomCode]) {
		return false
	}
	var best *Client
	for c := range h.rooms[roomCode] {
		if best == nil || c.joinedAt.Before(best.joinedAt) {
			best = c
		}
	}
	if best == nil {
		delete(h.hosts, roomCode)
		return false
	}
	h.hosts[roomCode] = best.userID
	return true
}

func (h *Hub) roomHasUserLocked(roomCode, userID string) bool {
	if userID == "" {
		return false
	}
	for c := range h.rooms[roomCode] {
		if c.userID == userID {
			return true
		}
	}
	return false
}

// dropLocked removes a client and, when it was the last one, its room entry. It
// is the single eviction point shared by unregister and the slow-consumer branch
// of broadcastToRoomLocked — that branch used to delete the client and leave an
// empty room behind, so every attacker-chosen room code leaked a map entry.
//
// It returns false when the client was already gone, which is what makes the
// send channel safe to close exactly once.
func (h *Hub) dropLocked(c *Client) bool {
	clients, ok := h.rooms[c.roomCode]
	if !ok {
		return false
	}
	if _, exists := clients[c]; !exists {
		return false
	}
	delete(clients, c)
	close(c.send)
	if len(clients) == 0 {
		delete(h.rooms, c.roomCode)
	}
	return true
}

// onClientGone runs after dropLocked and entirely outside h.mu: it emits
// userLeft, re-announces a promoted host and releases the room's Redis
// subscription.
func (h *Hub) onClientGone(c *Client) {
	h.mu.Lock()
	removed := h.dropLocked(c)
	empty := len(h.rooms[c.roomCode]) == 0
	hostChanged := false
	if removed {
		hostChanged = h.reelectHostLocked(c.roomCode)
		if empty {
			delete(h.hosts, c.roomCode)
		}
	}
	h.mu.Unlock()

	if !removed {
		return
	}
	h.publish(&domain.WatchPartyEvent{
		Action:     ActionUserLeft,
		RoomCode:   c.roomCode,
		SenderID:   c.userID,
		SenderName: c.userName,
		Timestamp:  time.Now(),
	})
	if hostChanged {
		h.emitRoomInfoAsync(c.roomCode)
	}
	if empty {
		h.stopSubscription(c.roomCode)
	}
}

// broadcastToRoomLocked fans one event out to a room and returns the clients it
// evicted for being too slow. Callers must hold h.mu.Lock (a read lock is not
// enough: it mutates the room map).
func (h *Hub) broadcastToRoomLocked(event *domain.WatchPartyEvent) []*Client {
	clients := h.rooms[event.RoomCode]
	if len(clients) == 0 {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return nil
	}
	var evicted []*Client
	for client := range clients {
		// Echo filter: the originator is not told about its own playback frames.
		// userJoined/userLeft/roomInfo reach everyone, including the originator,
		// because they carry information the originator needs.
		if client.userID == event.SenderID && !isServerAction(event.Action) {
			continue
		}
		select {
		case client.send <- data:
		default:
			h.dropLocked(client)
			h.droppedClients.Add(1)
			evicted = append(evicted, client)
		}
	}
	return evicted
}

func isServerAction(action string) bool {
	return action == ActionUserJoined || action == ActionUserLeft || action == ActionRoomInfo
}

// publish sends an event to Redis with a bounded context and records its
// fingerprint for echo suppression. MUST NOT be called with h.mu held: it is a
// network round-trip, and h.mu serialises the hub loop for every room.
func (h *Hub) publish(event *domain.WatchPartyEvent) {
	if h.bus == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.opts.RedisOpTimeout)
	defer cancel()
	if err := h.bus.PublishWatchPartyEvent(ctx, event.RoomCode, event); err != nil {
		log.Printf("[WS Hub] publish failed for room %s: %v", event.RoomCode, err)
		return
	}
	h.recent.remember(event)
}

// enqueue offers an event to the fan-out queue with an abortable,
// timeout-bounded send, so a producer can never park forever on a channel that
// nobody drains after shutdown.
func (h *Hub) enqueue(event *domain.WatchPartyEvent) bool {
	timer := time.NewTimer(h.opts.BroadcastSendTimeout)
	defer timer.Stop()
	select {
	case h.broadcast <- event:
		return true
	case <-h.stopChan:
		return false
	case <-timer.C:
		h.droppedMsgs.Add(1)
		log.Printf("[WS Hub] fan-out queue saturated for %s; dropping %q for room %s",
			h.opts.BroadcastSendTimeout, event.Action, event.RoomCode)
		return false
	}
}

func (h *Hub) notifyUnregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.stopChan:
	}
}

// HandleWebSocket is the upgrade endpoint.
//
// Admission order, cheapest and most decisive first:
//
//  1. per-source join rate limit            -> 429
//  2. ticket present / signature / purpose / expiry -> 401, 403
//  3. room-code syntax and ticket-room agreement   -> 400, 403
//  4. per-user join rate limit              -> 429
//  5. browser-origin allow-list             -> 403
//  6. room and client capacity              -> 503
//  7. upgrade, register, start pumps
//
// Identity comes from verified ticket claims only. The legacy ?user_id= and
// ?user_name= query parameters are ignored unconditionally: there is no
// compatibility fallback, because honouring them would reinstate the
// impersonation this endpoint had.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	remoteIP := clientIP(r)

	if !h.limiter.Allow("ip:" + remoteIP) {
		h.rejectedJoins.Add(1)
		writeWSError(w, http.StatusTooManyRequests, "rate_limited",
			"too many watch party join attempts from this source")
		return
	}

	claims, err := h.authenticate(r)
	if err != nil {
		h.rejectedJoins.Add(1)
		switch {
		case errors.Is(err, ErrTicketsNotConfigured):
			writeWSError(w, http.StatusServiceUnavailable, "tickets_disabled",
				"watch party ticket verification is not configured")
		case errors.Is(err, ErrMissingTicket):
			writeWSError(w, http.StatusUnauthorized, "missing_ticket",
				"obtain a watch party ticket over HTTP and pass it as ?ticket=")
		case errors.Is(err, ErrTicketRoomMismatch):
			writeWSError(w, http.StatusForbidden, "wrong_room",
				"ticket was issued for a different room")
		default:
			log.Printf("[WS Hub] rejected upgrade from %s: %v", remoteIP, err)
			writeWSError(w, http.StatusUnauthorized, "invalid_ticket",
				"invalid or expired watch party ticket")
		}
		return
	}

	roomCode := strings.TrimSpace(r.URL.Query().Get("room"))
	if !validRoomCode(roomCode) {
		h.rejectedJoins.Add(1)
		writeWSError(w, http.StatusBadRequest, "invalid_room",
			"room must match ^[A-Za-z0-9_-]{4,16}$")
		return
	}
	if claims.RoomCode != roomCode {
		h.rejectedJoins.Add(1)
		writeWSError(w, http.StatusForbidden, "wrong_room",
			"ticket was issued for a different room")
		return
	}
	if !h.limiter.Allow("user:" + claims.UserID) {
		h.rejectedJoins.Add(1)
		writeWSError(w, http.StatusTooManyRequests, "rate_limited",
			"too many watch party join attempts for this user")
		return
	}

	if !h.checkOrigin(r) {
		h.rejectedOrigin.Add(1)
		h.rejectedJoins.Add(1)
		log.Printf("[WS Hub] rejected cross-origin upgrade from %s: Origin=%q", remoteIP, r.Header.Get("Origin"))
		writeWSError(w, http.StatusForbidden, "origin_not_allowed",
			"this origin may not open watch party sockets")
		return
	}

	if err := h.reserve(roomCode); err != nil {
		h.rejectedJoins.Add(1)
		log.Printf("[WS Hub] rejected upgrade for room %s from %s: %v", roomCode, remoteIP, err)
		writeWSError(w, http.StatusServiceUnavailable, "capacity",
			"watch party capacity exhausted, try again later")
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.release(roomCode)
		log.Printf("[WS Hub] upgrade failed for room %s: %v", roomCode, err)
		return
	}

	// The ticket may legitimately omit a display name; fall back rather than
	// broadcast an empty label to the room.
	userName := claims.UserName
	if userName == "" {
		userName = "Guest"
	}

	client := &Client{
		hub:       h,
		conn:      conn,
		send:      make(chan []byte, h.opts.SendBuffer),
		roomCode:  roomCode,
		userID:    claims.UserID,
		userName:  userName,
		hostClaim: claims.IsHost,
		remoteIP:  remoteIP,
		ticketID:  claims.TicketID,
	}

	select {
	case h.register <- client:
		h.wg.Add(2)
		go client.writePump()
		go client.readPump()
	case <-h.stopChan:
		h.release(roomCode)
		_ = conn.Close()
	case <-r.Context().Done():
		h.release(roomCode)
		_ = conn.Close()
	}
}

// authenticate extracts and verifies the admission ticket. Browsers cannot set
// headers on a WebSocket handshake, so ?ticket= is the primary carrier;
// Authorization: Bearer and X-Watch-Party-Ticket are accepted for native clients.
func (h *Hub) authenticate(r *http.Request) (*TicketClaims, error) {
	if len(h.secret) == 0 {
		return nil, ErrTicketsNotConfigured
	}
	raw := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if raw == "" {
		if auth := r.Header.Get("Authorization"); len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
			raw = strings.TrimSpace(auth[7:])
		}
	}
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("X-Watch-Party-Ticket"))
	}
	if raw == "" {
		return nil, ErrMissingTicket
	}
	return parseWatchPartyTicket(h.secret, raw)
}

// reserve accounts for one pending upgrade, so concurrent handshakes cannot all
// observe a free slot and then overshoot the caps together.
func (h *Hub) reserve(roomCode string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-h.stopChan:
		return ErrHubStopped
	default:
	}
	_, roomKnown := h.rooms[roomCode]
	_, pendingKnown := h.reserved[roomCode]
	if !roomKnown && !pendingKnown && len(h.rooms)+len(h.reserved) >= h.opts.MaxRooms {
		return ErrTooManyRooms
	}
	if len(h.rooms[roomCode])+h.reserved[roomCode] >= h.opts.MaxClientsPerRoom {
		return ErrRoomFull
	}
	h.reserved[roomCode]++
	return nil
}

func (h *Hub) release(roomCode string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := h.reserved[roomCode]; n > 1 {
		h.reserved[roomCode] = n - 1
	} else {
		delete(h.reserved, roomCode)
	}
}

// RoomState exposes the room playback state to the rest of the server and, via
// the roomInfo frame, to a joining client. It is the accessor for the
// otherwise-unreachable Redis state helpers.
func (h *Hub) RoomState(ctx context.Context, roomCode string) (*domain.WatchPartyState, error) {
	if h.state == nil {
		return nil, ErrStateUnavailable
	}
	c, cancel := context.WithTimeout(ctx, h.opts.RedisOpTimeout)
	defer cancel()
	return h.state.GetWatchPartyState(c, roomCode)
}

// SaveRoomState lets the HTTP layer seed playback state when a room is created.
func (h *Hub) SaveRoomState(ctx context.Context, roomCode string, state *domain.WatchPartyState) error {
	if h.state == nil {
		return ErrStateUnavailable
	}
	c, cancel := context.WithTimeout(ctx, h.opts.RedisOpTimeout)
	defer cancel()
	return h.state.SetWatchPartyState(c, roomCode, state)
}

// roomInfoEvent builds the frame a joining client needs: who the host is, plus
// the persisted playback position when one exists. It reads h.hosts under a read
// lock and may touch Redis, so it must never be called with the write lock held.
func (h *Hub) roomInfoEvent(roomCode string) *domain.WatchPartyEvent {
	h.mu.RLock()
	hostID := h.hosts[roomCode]
	hostName := ""
	if hostID != "" {
		for c := range h.rooms[roomCode] {
			if c.userID == hostID {
				hostName = c.userName
			}
		}
	}
	h.mu.RUnlock()

	payload := map[string]interface{}{"hostId": hostID, "hostName": hostName}
	if state, err := h.RoomState(context.Background(), roomCode); err == nil && state != nil {
		payload["state"] = state
	}
	return &domain.WatchPartyEvent{
		Action:     ActionRoomInfo,
		RoomCode:   roomCode,
		SenderID:   hostID,
		SenderName: hostName,
		Payload:    payload,
		Timestamp:  time.Now(),
	}
}

// emitRoomInfoAsync publishes roomInfo off the hub loop: the Redis read is a
// network call and must not serialise fan-out for every room.
func (h *Hub) emitRoomInfoAsync(roomCode string) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC RECOVER] roomInfo for %s: %v", roomCode, rec)
			}
		}()
		event := h.roomInfoEvent(roomCode)
		h.publish(event)
		h.enqueue(event)
	}()
}

// persistState records host-authoritative playback so a client that joins later
// is handed a position instead of starting from zero. It is read-modify-write
// with one bounded context and no lock held.
func (h *Hub) persistState(event *domain.WatchPartyEvent) {
	if h.state == nil {
		return
	}
	switch event.Action {
	case ActionPlay, ActionPause, ActionSeek, ActionSpeed, ActionSync:
	default:
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.opts.RedisOpTimeout)
	defer cancel()

	state, err := h.state.GetWatchPartyState(ctx, event.RoomCode)
	if err != nil || state == nil {
		state = &domain.WatchPartyState{}
	}
	state.HostID = event.SenderID
	state.UpdatedAtEpoch = time.Now().UnixMilli()

	switch event.Action {
	case ActionPlay:
		state.IsPlaying = true
	case ActionPause:
		state.IsPlaying = false
	case ActionSeek:
		if ms, ok := event.Payload.(int64); ok {
			state.CurrentPositionMs = ms
		}
	case ActionSpeed:
		if speed, ok := event.Payload.(float64); ok {
			state.PlaybackSpeed = speed
		}
	case ActionSync:
		if m, ok := event.Payload.(map[string]interface{}); ok {
			if ms, ok := m["position"].(int64); ok {
				state.CurrentPositionMs = ms
			}
			if speed, ok := m["speed"].(float64); ok {
				state.PlaybackSpeed = speed
			}
		}
	}

	if err := h.state.SetWatchPartyState(ctx, event.RoomCode, state); err != nil {
		log.Printf("[WS Hub] could not persist state for room %s: %v", event.RoomCode, err)
	}
}

func (c *Client) readPump() {
	defer c.hub.wg.Done()
	defer c.conn.Close()
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] readPump panic for user %s: %v", c.userID, rec)
		}
		c.hub.notifyUnregister(c)
	}()

	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		event, err := validateClientMessage(c, message)
		if err != nil {
			// Malformed, unauthorised or mistyped frames are dropped; the
			// connection survives so a buggy client cannot be turned into a way to
			// evict the rest of the room.
			c.hub.rejectedMsgs.Add(1)
			log.Printf("[WS Hub] dropped frame from %s in %s: %v", c.userID, c.roomCode, err)
			continue
		}
		// Local fan-out first: it is the latency-critical path and must not queue
		// behind a Redis round-trip. The publish that follows is what carries the
		// frame to the other instances.
		c.hub.enqueue(event)
		c.hub.publish(event)
		c.hub.persistState(event)
	}
}

func (c *Client) writePump() {
	defer c.hub.wg.Done()
	ticker := time.NewTicker(20 * time.Second)
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] writePump panic for user %s: %v", c.userID, rec)
		}
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"),
				)
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ensureSubscription opens the Redis subscription for a room if fan-out is
// enabled and no subscription exists yet.
func (h *Hub) ensureSubscription(roomCode string) {
	h.subMu.Lock()
	if !h.subStarted || h.subCtx == nil {
		h.subMu.Unlock()
		return
	}
	if _, exists := h.subs[roomCode]; exists {
		h.subMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(h.subCtx)
	sub := &roomSubscription{id: h.nextSubID(), cancelFn: cancel}
	h.subs[roomCode] = sub
	h.subMu.Unlock()

	go h.pumpSubscription(ctx, roomCode, sub)
}

func (h *Hub) stopSubscription(roomCode string) {
	h.subMu.Lock()
	sub, ok := h.subs[roomCode]
	if ok {
		delete(h.subs, roomCode)
	}
	h.subMu.Unlock()
	if ok {
		sub.cancelFn()
	}
}

func (h *Hub) stopAllSubscriptions() {
	h.subMu.Lock()
	subs := h.subs
	h.subs = make(map[string]*roomSubscription)
	h.subMu.Unlock()
	for _, sub := range subs {
		sub.cancelFn()
	}
}

func (h *Hub) nextSubID() uint64 {
	h.subIDSeq.Add(1)
	return h.subIDSeq.Load()
}

func (h *Hub) pumpSubscription(ctx context.Context, roomCode string, sub *roomSubscription) {
	defer sub.cancelFn()
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] subscription for %s: %v", roomCode, rec)
		}
		h.subMu.Lock()
		if current, ok := h.subs[roomCode]; ok && current.id == sub.id {
			delete(h.subs, roomCode)
		}
		h.subMu.Unlock()
	}()

	pub := h.subscriber.SubscribeWatchPartyEvents(ctx, roomCode)
	if pub == nil {
		return
	}
	defer pub.Close()

	// Wait for the SUBSCRIBE acknowledgement before draining. Without this the
	// window between opening the PubSub and go-redis actually subscribing is a
	// window in which a remote publish is silently lost.
	if _, err := pub.Receive(ctx); err != nil {
		if ctx.Err() == nil {
			log.Printf("[WS Hub] subscribe to room %s failed: %v", roomCode, err)
		}
		return
	}

	messages := pub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopChan:
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			h.handleRemoteEvent(roomCode, []byte(msg.Payload))
		}
	}
}

// handleRemoteEvent admits one event received over the shared bus.
func (h *Hub) handleRemoteEvent(roomCode string, payload []byte) {
	var event domain.WatchPartyEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return
	}
	if event.RoomCode != "" && event.RoomCode != roomCode {
		return
	}
	if !serverActions[event.Action] {
		return
	}
	if h.recent.isLocal(payload) {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	h.enqueue(&event)
}

func writeWSError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q,"message":%q}`+"\n", code, message)
}

// validRoomCode constrains the attacker-chosen key space to what the client
// actually mints (6-character codes) plus a little slack. It keeps Redis channel
// names, log lines and room map keys bounded regardless of what arrives on the
// query string.
func validRoomCode(code string) bool {
	if len(code) < 4 || len(code) > 16 {
		return false
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// recentFingerprints suppresses the echo of events this instance published. A
// shared bus means our own publishes come back to us; without this every local
// client would receive each event twice. Keys are SHA-256 of the exact published
// bytes and expire quickly, so memory stays bounded while a genuinely remote
// duplicate still gets through.
type recentFingerprints struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func newRecentFingerprints(ttl time.Duration) *recentFingerprints {
	return &recentFingerprints{seen: make(map[string]time.Time), ttl: ttl}
}

func fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (r *recentFingerprints) remember(event *domain.WatchPartyEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, at := range r.seen {
		if now.Sub(at) > r.ttl {
			delete(r.seen, key)
		}
	}
	r.seen[fingerprint(data)] = now
}

func (r *recentFingerprints) isLocal(data []byte) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.seen[fingerprint(data)]
	return ok && now.Sub(at) <= r.ttl
}
