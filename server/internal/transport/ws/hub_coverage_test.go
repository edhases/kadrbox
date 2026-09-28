package ws

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
)

// covNewClient створює клієнта вручну без реального з'єднання (conn лишається nil;
// conn чіпається лише у stop-гілці Run та в read/writePump, тож поза ними nil безпечний).
func covNewClient(h *Hub, room, id, name string) *Client {
	return &Client{hub: h, send: make(chan []byte, 256), roomCode: room, userID: id, userName: name}
}

// covWait чекає одне повідомлення з каналу з таймаутом 2с.
func covWait(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("таймаут очікування повідомлення")
		return nil
	}
}

// covParse розбирає JSON події та перевіряє Action і RoomCode.
func covParse(t *testing.T, msg []byte, wantAction, wantRoom string) domain.WatchPartyEvent {
	t.Helper()
	var ev domain.WatchPartyEvent
	if err := json.Unmarshal(msg, &ev); err != nil {
		t.Fatalf("невалідний JSON події: %v", err)
	}
	if ev.Action != wantAction {
		t.Fatalf("Action = %q, want %q", ev.Action, wantAction)
	}
	if ev.RoomCode != wantRoom {
		t.Fatalf("RoomCode = %q, want %q", ev.RoomCode, wantRoom)
	}
	return ev
}

// covWaitRoomsEmpty чекає, поки hub.rooms стане порожнім (синхронізація перед GracefulStop,
// бо stop-гілка чіпає client.conn і безпечна лише з порожнім rooms).
func covWaitRoomsEmpty(t *testing.T, h *Hub) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.RLock()
		n := len(h.rooms)
		h.mu.RUnlock()
		if n == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("rooms не спорожніли вчасно")
}

// covUnregisterAll знімає клієнтів з реєстрації, чекає порожніх rooms і зупиняє хаб.
func covUnregisterAll(t *testing.T, h *Hub, clients ...*Client) {
	t.Helper()
	for _, c := range clients {
		h.unregister <- c
	}
	covWaitRoomsEmpty(t, h)
	h.GracefulStop()
}

// TestCovHubHandleWebSocketMissingParams: без room / без user_id / без обох —
// хендлер повертається до upgrade, тож Run не запускаємо (інакше валідний запит
// заблокувався б на rendezvous-каналі register).
func TestCovHubHandleWebSocketMissingParams(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"без room", "user_id=U&user_name=X"},
		{"без user_id", "room=R&user_name=X"},
		{"без обох", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHub(nil)
			req := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
			rec := httptest.NewRecorder()
			h.HandleWebSocket(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("код = %d, want 400", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "missing room or user_id") {
				t.Fatalf("тіло %q не містить %q", rec.Body.String(), "missing room or user_id")
			}
		})
	}
}

// TestCovHubUpgradeDespiteForeignOrigin — ХАРАКТЕРИЗАЦІЯ: CheckOrigin завжди
// повертає true, тому будь-який Origin (навіть чужий) проходить upgrade.
func TestCovHubUpgradeDespiteForeignOrigin(t *testing.T) {
	h := NewHub(nil)
	go h.Run()
	srv := httptest.NewServer(http.HandlerFunc(h.HandleWebSocket))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	raw := "GET /?room=R&user_id=U HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Origin: https://evil.example\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		t.Fatalf("код = %d, want 101 (чужий Origin не відхилено)", resp.StatusCode)
	}
	// Закриваємо TCP, чекаємо поки readPump сервера впаде і unregister спорожнить
	// rooms, і лише тоді зупиняємось: stop-гілка чіпає client.conn, тож з живим
	// клієнтом вона ганяється з writePump (concurrent write) — див. hub.go:61.
	conn.Close()
	covWaitRoomsEmpty(t, h)
	h.GracefulStop()
}

// TestCovHubRegisterBroadcastsUserJoined: гілка register шле userJoined у broadcast,
// а ехо-фільтр НЕ зачіпає userJoined — тож A бачить свій і чужий join, B бачить свій.
func TestCovHubRegisterBroadcastsUserJoined(t *testing.T) {
	h := NewHub(nil)
	go h.Run()

	a := covNewClient(h, "R1", "u-a", "A")
	b := covNewClient(h, "R1", "u-b", "B")

	h.register <- a
	covParse(t, covWait(t, a.send), "userJoined", "R1")

	h.register <- b
	covParse(t, covWait(t, a.send), "userJoined", "R1")
	covParse(t, covWait(t, b.send), "userJoined", "R1")

	covUnregisterAll(t, h, a, b)
}

// TestCovHubEchoFiltering: звичайні події (play) не повертаються відправнику,
// але доходять до інших клієнтів тієї ж кімнати.
func TestCovHubEchoFiltering(t *testing.T) {
	h := NewHub(nil)
	go h.Run()

	a := covNewClient(h, "R1", "u-a", "A")
	b := covNewClient(h, "R1", "u-b", "B")
	h.register <- a
	covWait(t, a.send)
	h.register <- b
	covWait(t, a.send)
	covWait(t, b.send)

	h.broadcast <- &domain.WatchPartyEvent{
		Action: "play", RoomCode: "R1",
		SenderID: "u-a", SenderName: "A", Timestamp: time.Now(),
	}
	ev := covParse(t, covWait(t, b.send), "play", "R1")
	if ev.SenderID != "u-a" {
		t.Fatalf("SenderID = %q, want %q", ev.SenderID, "u-a")
	}
	select {
	case msg := <-a.send:
		t.Fatalf("відправник отримав ехо: %s", msg)
	case <-time.After(200 * time.Millisecond):
	}

	covUnregisterAll(t, h, a, b)
}

// TestCovHubBroadcastIsolatedByRoom: подія для R1 не доходить до клієнта з R2.
func TestCovHubBroadcastIsolatedByRoom(t *testing.T) {
	h := NewHub(nil)
	go h.Run()

	a := covNewClient(h, "R1", "u-a", "A")
	c := covNewClient(h, "R2", "u-c", "C")
	h.register <- a
	covWait(t, a.send)
	h.register <- c
	covWait(t, c.send)

	h.broadcast <- &domain.WatchPartyEvent{
		Action: "play", RoomCode: "R1",
		SenderID: "u-x", SenderName: "X", Timestamp: time.Now(),
	}
	// Позитивний контроль: клієнт своєї кімнати подію отримує.
	covParse(t, covWait(t, a.send), "play", "R1")
	select {
	case msg := <-c.send:
		t.Fatalf("клієнт іншої кімнати отримав подію: %s", msg)
	case <-time.After(200 * time.Millisecond):
	}

	covUnregisterAll(t, h, a, c)
}

// TestCovHubUnregisterRemovesAndBroadcastsLeft: unregister видаляє клієнта з rooms
// і шле userLeft решті кімнати.
func TestCovHubUnregisterRemovesAndBroadcastsLeft(t *testing.T) {
	h := NewHub(nil)
	go h.Run()

	a := covNewClient(h, "R1", "u-a", "A")
	b := covNewClient(h, "R1", "u-b", "B")
	h.register <- a
	covWait(t, a.send)
	h.register <- b
	covWait(t, a.send)
	covWait(t, b.send)

	h.unregister <- a
	covParse(t, covWait(t, b.send), "userLeft", "R1")

	h.mu.RLock()
	n := len(h.rooms["R1"])
	h.mu.RUnlock()
	if n != 1 {
		t.Fatalf("після unregister у R1 %d клієнтів, want 1", n)
	}

	covUnregisterAll(t, h, b)
}

// TestCovHubUnregisterUnknownIsNoop: unregister фантомного клієнта не панікує,
// хаб лишається живим (приймає наступну реєстрацію).
func TestCovHubUnregisterUnknownIsNoop(t *testing.T) {
	h := NewHub(nil)
	go h.Run()

	ghost := covNewClient(h, "R-ghost", "u-ghost", "Ghost")
	h.unregister <- ghost

	a := covNewClient(h, "R1", "u-a", "A")
	h.register <- a
	covParse(t, covWait(t, a.send), "userJoined", "R1")

	covUnregisterAll(t, h, a)
}

// TestCovHubGracefulStopEmptyRooms: зупинка з порожнім rooms завершує Run.
func TestCovHubGracefulStopEmptyRooms(t *testing.T) {
	h := NewHub(nil)
	done := make(chan struct{})
	go func() { h.Run(); close(done) }()
	h.GracefulStop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершився після GracefulStop")
	}
}

// TestCovHubGracefulStopDoubleClosePanics — ХАРАКТЕРИЗАЦІЯ: GracefulStop не
// ідемпотентний, див. hub.go:141 — другий close(h.stopChan) панікує.
func TestCovHubGracefulStopDoubleClosePanics(t *testing.T) {
	h := NewHub(nil)
	h.GracefulStop()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("очікувався panic від подвійного GracefulStop")
		}
		if !strings.Contains(fmt.Sprint(r), "close of closed channel") {
			t.Fatalf("panic = %v, want %q", r, "close of closed channel")
		}
	}()
	h.GracefulStop()
}
