package ws

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// ---------------------------------------------------------------------------
// ticket verification
// ---------------------------------------------------------------------------

func TestTicketRoundTrip(t *testing.T) {
	h := startHub(t)
	token := ticket(t, h, "u-1", "Alice", "TICKET1", time.Minute)

	claims, err := parseWatchPartyTicket(h.secret, token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != "u-1" || claims.UserName != "Alice" || claims.RoomCode != "TICKET1" {
		t.Fatalf("unexpected claims %+v", claims)
	}
	if claims.TicketID == "" {
		t.Fatal("ticket has no jti")
	}
	if claims.IsHost {
		t.Fatal("ticket unexpectedly claims host")
	}
}

func TestTicketRejectsForeignSecretAndPurpose(t *testing.T) {
	h := startHub(t)
	issuer := NewTicketIssuer(testSecret)

	// Signed with a different secret.
	other := NewTicketIssuer("another-secret")
	foreign, err := other.IssueWatchPartyTicket("u-1", "Mallory", "TICKET1", time.Minute)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := parseWatchPartyTicket(h.secret, foreign); err == nil {
		t.Fatal("a ticket signed with another secret was accepted")
	}

	// Correctly signed, but for another subsystem: purpose != ws_ticket.
	claims := jwt.MapClaims{
		"purpose": "access_token",
		"room":    "TICKET1",
		"sub":     "u-1",
		"jti":     "abc",
		"exp":     time.Now().Add(time.Minute).Unix(),
	}
	wrongPurpose, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(h.secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := parseWatchPartyTicket(h.secret, wrongPurpose); err == nil {
		t.Fatal("a token minted for another purpose was accepted")
	}

	// "alg: none" must never be honoured.
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"purpose": TicketPurpose, "room": "TICKET1", "sub": "u-1", "jti": "x",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	unsigned, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := parseWatchPartyTicket(h.secret, unsigned); err == nil {
		t.Fatal("an unsigned token was accepted")
	}

	if _, err := issuer.IssueWatchPartyTicket("u-1", "A", "no spaces allowed", time.Minute); err == nil {
		t.Fatal("a ticket was issued for an invalid room code")
	}
}

func TestTicketExpired(t *testing.T) {
	h := startHub(t)

	// Signed with the hub's own secret so only the expiry is wrong.
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"purpose":   TicketPurpose,
		"room":      "TICKET1",
		"sub":       "u-1",
		"user_name": "Alice",
		"jti":       "expired-1",
		"exp":       time.Now().Add(-time.Minute).Unix(),
	}).SignedString(h.secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := parseWatchPartyTicket(h.secret, expired); err == nil {
		t.Fatal("an expired ticket was accepted")
	}

	// A ticket with no expiry at all is refused as well.
	noExp, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"purpose": TicketPurpose, "room": "TICKET1", "sub": "u-1", "jti": "no-exp",
	}).SignedString(h.secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := parseWatchPartyTicket(h.secret, noExp); err == nil {
		t.Fatal("a ticket without an expiry was accepted")
	}
}

func TestValidRoomCode(t *testing.T) {
	valid := []string{"abcd", "ABC123", "room_1", "a-b-c", strings.Repeat("a", 16)}
	invalid := []string{"", "abc", "room 1", "room/1", "room.1", "rooom!", strings.Repeat("a", 17), "кирилиця"}
	for _, code := range valid {
		if !validRoomCode(code) {
			t.Errorf("validRoomCode(%q) = false, want true", code)
		}
	}
	for _, code := range invalid {
		if validRoomCode(code) {
			t.Errorf("validRoomCode(%q) = true, want false", code)
		}
	}
}

// ---------------------------------------------------------------------------
// upgrade admission
// ---------------------------------------------------------------------------

func TestUpgradeRequiresTicket(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	// Legacy identity in the query string must no longer buy admission.
	_, resp, err := dialWS(srv, "/?room=TICKET1&user_id=u-1&user_name=Alice", nil)
	if err == nil {
		t.Fatal("an unauthenticated upgrade succeeded")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}
	if totalRooms(h) != 0 {
		t.Fatal("a rejected upgrade created a room")
	}
}

func TestUpgradeRejectsBadTickets(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	// An expired ticket: correctly signed, past its expiry.
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"purpose": TicketPurpose, "room": "TICKET1", "sub": "u-1", "jti": "expired-1",
		"exp": time.Now().Add(-time.Minute).Unix(),
	}).SignedString(h.secret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"invalid signature", "/?room=TICKET1&ticket=" + ticket(t, NewHub(nil, Options{JWTSecret: "other"}), "u-1", "A", "TICKET1", time.Minute), http.StatusUnauthorized},
		{"expired", "/?room=TICKET1&ticket=" + expired, http.StatusUnauthorized},
		{"wrong room", "/?room=TICKET1&ticket=" + ticket(t, h, "u-1", "A", "OTHER2", time.Minute), http.StatusForbidden},
		{"room missing", "/?ticket=" + ticket(t, h, "u-1", "A", "TICKET1", time.Minute), http.StatusBadRequest},
		{"room malformed", "/?room=a%20b&ticket=" + ticket(t, h, "u-1", "A", "TICKET1", time.Minute), http.StatusBadRequest},
		{"garbage token", "/?room=TICKET1&ticket=not-a-jwt", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn, resp, err := dialWS(srv, tc.query, nil)
			if err == nil {
				conn.Close()
				t.Fatal("upgrade was admitted")
			}
			if resp == nil || resp.StatusCode != tc.want {
				t.Fatalf("status = %v, want %d", resp, tc.want)
			}
		})
	}
	if totalRooms(h) != 0 {
		t.Fatalf("rejected upgrades left %d rooms", totalRooms(h))
	}
}

func TestUpgradeWithoutConfiguredSecretFailsClosed(t *testing.T) {
	h := NewHub(nil) // no JWTSecret: nobody can verify a ticket
	srv := hubServer(t, h)

	_, resp, err := dialWS(srv, "/?room=TICKET1&ticket=whatever", nil)
	if err == nil {
		t.Fatal("upgrade was admitted without a configured issuer")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %v, want 503", resp)
	}
}

func TestUpgradeAcceptsBearerHeader(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+ticket(t, h, "u-1", "Alice", "HEADR01", time.Minute))
	conn, _, err := dialWS(srv, "/?room=HEADR01", header)
	if err != nil {
		t.Fatalf("bearer-authenticated upgrade failed: %v", err)
	}
	defer conn.Close()
	waitRoomSize(t, h, "HEADR01", 1, time.Second)

	if ev := awaitAction(t, conn, ActionUserJoined, 2*time.Second); ev.SenderName != "Alice" {
		t.Fatalf("senderName = %q, want Alice (identity must come from the ticket)", ev.SenderName)
	}
}

// ---------------------------------------------------------------------------
// origin policy
// ---------------------------------------------------------------------------

func TestCrossOriginUpgradeRejected(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	header := http.Header{}
	header.Set("Origin", "https://evil.example")
	_, resp, err := dialWS(srv, "/?room=ORIGIN1&ticket="+ticket(t, h, "u-1", "A", "ORIGIN1", time.Minute), header)
	if err == nil {
		t.Fatal("a foreign Origin was upgraded")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %v, want 403", resp)
	}
	if h.Stats().RejectedOrigins == 0 {
		t.Fatal("origin rejection was not counted")
	}
}

func TestNoOriginUpgradeAllowed(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	// A native/desktop client sends no Origin header at all.
	conn := mustDial(t, srv, "/?room=ORIGIN2&ticket="+ticket(t, h, "u-1", "A", "ORIGIN2", time.Minute))
	_ = awaitAction(t, conn, ActionUserJoined, 2*time.Second)
	waitRoomSize(t, h, "ORIGIN2", 1, time.Second)
}

func TestAllowedOriginUpgradeAccepted(t *testing.T) {
	h := startHub(t, func(o *Options) {
		o.AllowedOrigins = []string{"https://watch.oxideteam.pp.ua"}
	})
	srv := hubServer(t, h)

	allowed := http.Header{}
	allowed.Set("Origin", "https://watch.oxideteam.pp.ua")
	conn, _, err := dialWS(srv, "/?room=ORIGIN3&ticket="+ticket(t, h, "u-1", "A", "ORIGIN3", time.Minute), allowed)
	if err != nil {
		t.Fatalf("allow-listed origin was rejected: %v", err)
	}
	defer conn.Close()
	waitRoomSize(t, h, "ORIGIN3", 1, time.Second)

	rejected := http.Header{}
	rejected.Set("Origin", "https://watch.evil.example")
	if _, resp, err := dialWS(srv, "/?room=ORIGIN3&ticket="+ticket(t, h, "u-2", "B", "ORIGIN3", time.Minute), rejected); err == nil {
		t.Fatal("an origin that merely shares a suffix was admitted")
	} else if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %v, want 403", resp)
	}
}

func TestOriginNullIsRejected(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	header := http.Header{}
	header.Set("Origin", "null")
	if _, resp, err := dialWS(srv, "/?room=ORIGIN4&ticket="+ticket(t, h, "u-1", "A", "ORIGIN4", time.Minute), header); err == nil {
		t.Fatal("an opaque null origin was admitted")
	} else if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %v, want 403", resp)
	}
}

// ---------------------------------------------------------------------------
// join rate limiting
// ---------------------------------------------------------------------------

func TestJoinRateLimiter(t *testing.T) {
	h := startHub(t, func(o *Options) {
		o.JoinLimiter = JoinLimiterConfig{Window: time.Minute, MaxPerKey: 2, MaxBuckets: 8}
	})
	srv := hubServer(t, h)

	// The same source address shares one bucket, so the third join is throttled.
	for i := 0; i < 2; i++ {
		conn := mustDial(t, srv, "/?room=RATE001&ticket="+ticket(t, h, "u-"+string(rune('a'+i)), "A", "RATE001", time.Minute))
		_ = conn
	}
	_, resp, err := dialWS(srv, "/?room=RATE001&ticket="+ticket(t, h, "u-c", "C", "RATE001", time.Minute), nil)
	if err == nil {
		t.Fatal("the third join from one source was admitted")
	}
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %v, want 429", resp)
	}
}

func TestJoinLimiterIsBounded(t *testing.T) {
	limiter := NewJoinLimiter(JoinLimiterConfig{Window: time.Minute, MaxPerKey: 100, MaxBuckets: 4})
	for i := 0; i < 1000; i++ {
		limiter.Allow("key-" + strings.Repeat("x", i%17) + string(rune(i)))
	}
	if got := limiter.tracked(); got > 4 {
		t.Fatalf("limiter tracks %d keys, want <= 4", got)
	}
	if limiter.Allow("brand-new-key") {
		t.Fatal("a new key was admitted while the key table was full")
	}
}

func TestJoinLimiterWindowRolls(t *testing.T) {
	limiter := NewJoinLimiter(JoinLimiterConfig{Window: 40 * time.Millisecond, MaxPerKey: 1, MaxBuckets: 16})
	if !limiter.Allow("a") {
		t.Fatal("first join was refused")
	}
	if limiter.Allow("a") {
		t.Fatal("second join inside the window was admitted")
	}
	time.Sleep(60 * time.Millisecond)
	if !limiter.Allow("a") {
		t.Fatal("join after the window was refused")
	}
}

// TestJoinLimiterReclaimsExpiredKeys proves the TTL sweep actually runs: with the
// key table saturated, an overflow key is refused until the live buckets age out.
func TestJoinLimiterReclaimsExpiredKeys(t *testing.T) {
	limiter := NewJoinLimiter(JoinLimiterConfig{Window: 40 * time.Millisecond, MaxPerKey: 5, MaxBuckets: 4})
	for i := 0; i < 4; i++ {
		if !limiter.Allow("key-" + string(rune('a'+i))) {
			t.Fatalf("key %d refused while the table had room", i)
		}
	}
	if limiter.Allow("overflow") {
		t.Fatal("a key past MaxBuckets was admitted")
	}
	time.Sleep(80 * time.Millisecond)
	if !limiter.Allow("overflow") {
		t.Fatal("expired buckets were never reclaimed")
	}
}

// ---------------------------------------------------------------------------
// host role
// ---------------------------------------------------------------------------

// TestGuestPlaybackIsNotAuthoritative is the core authorisation test: a guest
// whose frames the Dart client executes unconditionally must be unable to move
// anybody else's playback.
func TestGuestPlaybackIsNotAuthoritative(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	host := mustDial(t, srv, "/?room=HOST001&ticket="+ticket(t, h, "u-host", "Host", "HOST001", time.Minute))
	waitRoomSize(t, h, "HOST001", 1, time.Second)
	awaitAction(t, host, ActionUserJoined, 2*time.Second)
	awaitAction(t, host, ActionRoomInfo, 2*time.Second)

	guest := mustDial(t, srv, "/?room=HOST001&ticket="+ticket(t, h, "u-guest", "Guest", "HOST001", time.Minute))
	waitRoomSize(t, h, "HOST001", 2, 2*time.Second)
	awaitAction(t, host, ActionUserJoined, 2*time.Second)
	awaitAction(t, guest, ActionUserJoined, 2*time.Second)

	hostOnly := []string{
		`{"action":"play"}`,
		`{"action":"pause"}`,
		`{"action":"seek","payload":123456}`,
		`{"action":"speed","payload":2.0}`,
		`{"action":"sync","payload":{"position":10,"speed":1.0}}`,
	}
	for _, frame := range hostOnly {
		if err := guest.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			t.Fatalf("guest write: %v", err)
		}
	}
	waitCounter(t, func() int64 { return h.Stats().RejectedMessages }, int64(len(hostOnly)), "dropped guest frames")

	// Chat and requestSync are guest-safe.
	if err := guest.WriteMessage(websocket.TextMessage, []byte(`{"action":"chat","payload":"hello"}`)); err != nil {
		t.Fatalf("guest chat write: %v", err)
	}
	if ev := awaitAction(t, host, ActionChat, 2*time.Second); ev.Payload != "hello" {
		t.Fatalf("chat payload = %v, want hello", ev.Payload)
	}
	if err := guest.WriteMessage(websocket.TextMessage, []byte(`{"action":"requestSync"}`)); err != nil {
		t.Fatalf("guest requestSync write: %v", err)
	}
	if ev := awaitAction(t, host, ActionRequestSync, 2*time.Second); ev.SenderID != "u-guest" {
		t.Fatalf("requestSync sender = %q, want u-guest", ev.SenderID)
	}

	// The host is authoritative.
	if err := host.WriteMessage(websocket.TextMessage, []byte(`{"action":"seek","payload":42000}`)); err != nil {
		t.Fatalf("host write: %v", err)
	}
	ev := awaitAction(t, guest, ActionSeek, 2*time.Second)
	// The decoded value is float64 because JSON has one number type; the wire form
	// is what matters for the Dart `payload as int` cast, and TestSeekMarshalsAsInteger
	// pins that.
	if ev.Payload != float64(42000) {
		t.Fatalf("seek payload = %#v, want 42000", ev.Payload)
	}
	if ev.SenderID != "u-host" {
		t.Fatalf("seek sender = %q, want u-host", ev.SenderID)
	}

	// Finally: nothing further is pending on the host. This must be the last read
	// on this connection (see expectSilence).
	expectSilence(t, host, 400*time.Millisecond)
}

func TestHostIsFirstJoinerAndIsReElected(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	first := mustDial(t, srv, "/?room=HOST002&ticket="+ticket(t, h, "u-1", "First", "HOST002", time.Minute))
	waitRoomSize(t, h, "HOST002", 1, time.Second)
	info := awaitAction(t, first, ActionRoomInfo, 2*time.Second)
	if payload, ok := info.Payload.(map[string]interface{}); !ok || payload["hostId"] != "u-1" {
		t.Fatalf("roomInfo payload = %#v, want hostId u-1", info.Payload)
	}

	second := mustDial(t, srv, "/?room=HOST002&ticket="+ticket(t, h, "u-2", "Second", "HOST002", time.Minute))
	waitRoomSize(t, h, "HOST002", 2, 2*time.Second)
	awaitAction(t, first, ActionUserJoined, 2*time.Second)
	awaitAction(t, second, ActionUserJoined, 2*time.Second)

	// While u-1 is host, u-2 cannot command playback.
	if err := second.WriteMessage(websocket.TextMessage, []byte(`{"action":"play"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitCounter(t, func() int64 { return h.Stats().RejectedMessages }, 1, "dropped guest frames")

	// Host leaves: u-2 is promoted and becomes authoritative.
	first.Close()
	waitRoomSize(t, h, "HOST002", 1, 2*time.Second)
	info = awaitAction(t, second, ActionRoomInfo, 2*time.Second)
	if payload, ok := info.Payload.(map[string]interface{}); !ok || payload["hostId"] != "u-2" {
		t.Fatalf("after re-election roomInfo payload = %#v, want hostId u-2", info.Payload)
	}

	// A fresh observer proves the promotion took effect: the frame now reaches the
	// room instead of being dropped as a guest's.
	observer := mustDial(t, srv, "/?room=HOST002&ticket="+ticket(t, h, "u-3", "Third", "HOST002", time.Minute))
	waitRoomSize(t, h, "HOST002", 2, 2*time.Second)
	awaitAction(t, observer, ActionUserJoined, 2*time.Second)
	awaitAction(t, second, ActionUserJoined, 2*time.Second)

	if err := second.WriteMessage(websocket.TextMessage, []byte(`{"action":"play"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if ev := awaitAction(t, observer, ActionPlay, 2*time.Second); ev.SenderID != "u-2" {
		t.Fatalf("promoted host's frame sender = %q, want u-2", ev.SenderID)
	}
	expectSilence(t, observer, 300*time.Millisecond)
}

func TestExplicitHostClaimWins(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	// The room-creating endpoint mints the host ticket with is_host set.
	hostToken, err := h.IssueHostWatchPartyTicket("u-host", "Host", "CLAIM01", time.Minute)
	if err != nil {
		t.Fatalf("issue host ticket: %v", err)
	}
	claims, err := parseWatchPartyTicket(h.secret, hostToken)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !claims.IsHost {
		t.Fatal("host ticket does not assert is_host")
	}

	host := mustDial(t, srv, "/?room=CLAIM01&ticket="+hostToken)
	waitRoomSize(t, h, "CLAIM01", 1, time.Second)
	awaitAction(t, host, ActionRoomInfo, 2*time.Second)

	// A guest joins later; the claimed host keeps the role.
	guest := mustDial(t, srv, "/?room=CLAIM01&ticket="+ticket(t, h, "u-guest", "Guest", "CLAIM01", time.Minute))
	waitRoomSize(t, h, "CLAIM01", 2, 2*time.Second)
	awaitAction(t, host, ActionUserJoined, 2*time.Second)
	awaitAction(t, guest, ActionUserJoined, 2*time.Second)

	if err := guest.WriteMessage(websocket.TextMessage, []byte(`{"action":"play"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitCounter(t, func() int64 { return h.Stats().RejectedMessages }, 1, "dropped guest frame")
	expectSilence(t, host, 300*time.Millisecond)

	h.mu.RLock()
	hostID := h.hosts["CLAIM01"]
	h.mu.RUnlock()
	if hostID != "u-host" {
		t.Fatalf("host = %q, want u-host", hostID)
	}
}

// ---------------------------------------------------------------------------
// frame validation
// ---------------------------------------------------------------------------

func TestHostFramesAreValidated(t *testing.T) {
	h := startHub(t)
	srv := hubServer(t, h)

	host := mustDial(t, srv, "/?room=VALID01&ticket="+ticket(t, h, "u-host", "Host", "VALID01", time.Minute))
	waitRoomSize(t, h, "VALID01", 1, time.Second)
	awaitAction(t, host, ActionRoomInfo, 2*time.Second)

	// Every frame below must be dropped silently: the connection must survive and
	// nothing may be fanned out. A malformed payload used to reach the Dart client
	// and blow up on `message.payload as int`.
	rejected := []string{
		`{"action":"seek"}`,
		`{"action":"seek","payload":"abc"}`,
		`{"action":"seek","payload":1.5}`,
		`{"action":"seek","payload":-1}`,
		`{"action":"speed","payload":"fast"}`,
		`{"action":"speed","payload":99}`,
		`{"action":"chat","payload":42}`,
		`{"action":"chat","payload":"   "}`,
		`{"action":"userJoined","senderId":"u-victim","senderName":"Admin"}`,
		`{"action":"userLeft"}`,
		`{"action":"roomInfo","payload":{"hostId":"u-attacker"}}`,
		`{"action":"totallyUnknown"}`,
		`{"action":`,
	}
	for _, frame := range rejected {
		if err := host.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			t.Fatalf("write %s: %v", frame, err)
		}
	}
	expectSilence(t, host, 400*time.Millisecond)

	// The connection survived every rejection, and the socket is still writable.
	if err := host.WriteMessage(websocket.TextMessage, []byte(`{"action":"chat","payload":"still alive"}`)); err != nil {
		t.Fatalf("write after rejections: %v", err)
	}
	if got := h.Stats().RejectedMessages; got != int64(len(rejected)) {
		t.Fatalf("rejected %d frames, want exactly %d", got, len(rejected))
	}
}

func TestChatIsCappedAt500Runes(t *testing.T) {
	spec := clientActions[ActionChat]
	long := strings.Repeat("ы", 600) // two-byte runes
	payload, err := spec.build([]byte(`"` + long + `"`))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	text := payload.(string)
	if got := len([]rune(text)); got != maxChatRunes {
		t.Fatalf("chat length = %d runes, want %d", got, maxChatRunes)
	}
}

func TestValidateClientMessageRejectsHostOnlyForGuests(t *testing.T) {
	h := startHub(t)
	guest := &Client{hub: h, roomCode: "UNIT001", userID: "u-guest", userName: "Guest"}

	if _, err := validateClientMessage(guest, []byte(`{"action":"play"}`)); err == nil {
		t.Fatal("a guest's play frame was accepted")
	}
	if _, err := validateClientMessage(guest, []byte(`{"action":"chat","payload":"hi"}`)); err != nil {
		t.Fatalf("a guest's chat frame was rejected: %v", err)
	}

	// Identity fields in the payload body are never honoured.
	host := &Client{hub: h, roomCode: "UNIT001", userID: "u-host", userName: "Host"}
	h.mu.Lock()
	h.hosts["UNIT001"] = "u-host"
	h.mu.Unlock()
	event, err := validateClientMessage(host, []byte(`{"action":"seek","payload":5,"roomCode":"OTHER","senderId":"u-victim","senderName":"Admin","timestamp":"2999-01-01T00:00:00Z"}`))
	if err != nil {
		t.Fatalf("host frame rejected: %v", err)
	}
	if event.RoomCode != "UNIT001" || event.SenderID != "u-host" || event.SenderName != "Host" {
		t.Fatalf("client-supplied identity leaked into the event: %+v", event)
	}
	if event.Timestamp.Year() == 2999 {
		t.Fatal("client-supplied timestamp leaked into the event")
	}
	if _, ok := event.Payload.(int64); !ok {
		t.Fatalf("seek payload type = %T, want int64", event.Payload)
	}
}

// TestSeekMarshalsAsInteger pins the wire form the Dart client depends on:
// `message.payload as int` throws on 42000.0, so the server must emit an integer
// literal for seek and a plain number for speed.
func TestSeekMarshalsAsInteger(t *testing.T) {
	h := startHub(t)
	host := &Client{hub: h, roomCode: "MARSH01", userID: "u-host", userName: "Host"}
	h.mu.Lock()
	h.hosts["MARSH01"] = "u-host"
	h.mu.Unlock()

	seek, err := validateClientMessage(host, []byte(`{"action":"seek","payload":42000}`))
	if err != nil {
		t.Fatalf("seek rejected: %v", err)
	}
	encoded, err := json.Marshal(seek)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"payload":42000`) {
		t.Fatalf("seek wire form = %s, want an integer payload", encoded)
	}

	speed, err := validateClientMessage(host, []byte(`{"action":"speed","payload":1.5}`))
	if err != nil {
		t.Fatalf("speed rejected: %v", err)
	}
	encoded, err = json.Marshal(speed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"payload":1.5`) {
		t.Fatalf("speed wire form = %s, want payload 1.5", encoded)
	}

	sync, err := validateClientMessage(host, []byte(`{"action":"sync","payload":{"position":7,"speed":2}}`))
	if err != nil {
		t.Fatalf("sync rejected: %v", err)
	}
	payload, ok := sync.Payload.(map[string]interface{})
	if !ok || payload["position"] != int64(7) || payload["speed"] != float64(2) {
		t.Fatalf("sync payload = %#v, want position int64(7) and speed float64(2)", sync.Payload)
	}
}

func TestServerActionsAcceptedFromBusOnly(t *testing.T) {
	if serverActions[ActionUserJoined] != true || serverActions[ActionRoomInfo] != true {
		t.Fatal("server actions must be accepted from the shared bus")
	}
	if _, ok := clientActions[ActionUserJoined]; ok {
		t.Fatal("userJoined must not be client-originated")
	}
	if _, ok := clientActions[ActionRoomInfo]; ok {
		t.Fatal("roomInfo must not be client-originated")
	}
}

func TestHandleRemoteEventDropsForeignFrames(t *testing.T) {
	// Deliberately not started: nothing drains the fan-out queue, so the test can
	// assert on exactly what was admitted.
	h := NewHub(nil, Options{JWTSecret: testSecret})

	// Wrong room in the body.
	h.handleRemoteEvent("ROOMX", []byte(`{"action":"play","roomCode":"OTHER"}`))
	// Unknown action.
	h.handleRemoteEvent("ROOMX", []byte(`{"action":"exec","roomCode":"ROOMX"}`))
	// Not JSON.
	h.handleRemoteEvent("ROOMX", []byte(`not json`))
	if len(h.broadcast) != 0 {
		t.Fatalf("queue holds %d frames, want 0", len(h.broadcast))
	}

	var accepted domain.WatchPartyEvent
	for _, action := range []string{ActionPlay, ActionPause, ActionSeek, ActionSpeed, ActionSync, ActionChat, ActionRequestSync, ActionUserJoined, ActionUserLeft, ActionRoomInfo} {
		accepted = domain.WatchPartyEvent{Action: action, RoomCode: "ROOMX"}
		data, err := json.Marshal(&accepted)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		h.handleRemoteEvent("ROOMX", data)
	}
	if got := len(h.broadcast); got != 10 {
		t.Fatalf("queue holds %d frames, want 10", got)
	}
}
