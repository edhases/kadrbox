package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/gorilla/websocket"
)

// Тести readPump/writePump через справжні websocket-з'єднання
// (httptest-сервер + gorilla Dialer, усе на localhost).

type covPumpRig struct {
	hub   *Hub
	srv   *httptest.Server
	conns []*websocket.Conn
}

func covNewPumpRig(t *testing.T) *covPumpRig {
	t.Helper()
	h := NewHub(nil)
	go h.Run()
	srv := httptest.NewServer(http.HandlerFunc(h.HandleWebSocket))
	return &covPumpRig{hub: h, srv: srv}
}

func (r *covPumpRig) close(t *testing.T) {
	t.Helper()
	// Порядок важливий для відсутності гонки "concurrent write":
	// 1) закриваємо клієнтські TCP — серверні readPump падають,
	//    Run розреєстровує всіх (їхні writePump виходять самі);
	// 2) чекаємо порожніх rooms;
	// 3) лише тоді GracefulStop — stop-гілка нікому не пише;
	// 4) закриваємо слухача.
	for _, c := range r.conns {
		_ = c.Close()
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		r.hub.mu.RLock()
		n := len(r.hub.rooms)
		r.hub.mu.RUnlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rooms not drained before stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.hub.GracefulStop()
	r.srv.Close()
}

func (r *covPumpRig) dial(t *testing.T, room, userID, userName string) *websocket.Conn {
	t.Helper()
	wsURL := strings.Replace(r.srv.URL, "http", "ws", 1) +
		"?room=" + room + "&user_id=" + userID + "&user_name=" + userName
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	r.conns = append(r.conns, conn)
	return conn
}

// covWaitRoomSize чекає поки в кімнаті стане n клієнтів (реєстрація асинхронна).
func (r *covPumpRig) waitRoomSize(t *testing.T, room string, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		r.hub.mu.RLock()
		got := len(r.hub.rooms[room])
		r.hub.mu.RUnlock()
		if got == n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("room %s: expected %d clients, timed out", room, n)
}

func covReadEvent(t *testing.T, conn *websocket.Conn) domain.WatchPartyEvent {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var ev domain.WatchPartyEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatalf("not a WatchPartyEvent: %v", err)
	}
	return ev
}

// covDrainJoins зчитує очікувані userJoined-події (реєстрація шле їх усім).
func covDrainJoins(t *testing.T, conn *websocket.Conn, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		ev := covReadEvent(t, conn)
		if ev.Action != "userJoined" {
			t.Fatalf("expected userJoined, got %q", ev.Action)
		}
	}
}

func TestCovHubPumpUpgradeFailure(t *testing.T) {
	rig := covNewPumpRig(t)
	defer rig.close(t)

	// Звичайний HTTP GET без websocket-handshake: upgrade падає,
	// HandleWebSocket повертається без реєстрації клієнта.
	resp, err := http.Get(rig.srv.URL + "?room=R&user_id=u1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Error("expected failed upgrade, got 101")
	}
	rig.hub.mu.RLock()
	defer rig.hub.mu.RUnlock()
	if len(rig.hub.rooms) != 0 {
		t.Errorf("expected no rooms after failed upgrade, got %v", rig.hub.rooms)
	}
}

func TestCovHubPumpBroadcastDelivery(t *testing.T) {
	rig := covNewPumpRig(t)
	defer rig.close(t)

	c1 := rig.dial(t, "R", "u1", "A")
	rig.waitRoomSize(t, "R", 1)
	covDrainJoins(t, c1, 1) // власний userJoined

	c2 := rig.dial(t, "R", "u2", "B")
	rig.waitRoomSize(t, "R", 2)
	covDrainJoins(t, c1, 1) // userJoined B
	covDrainJoins(t, c2, 1) // власний userJoined

	// Подія від u1: writePump доставляє її c2, ехо-фільтр ріже c1.
	rig.hub.broadcast <- &domain.WatchPartyEvent{
		Action: "play", RoomCode: "R", SenderID: "u1", SenderName: "A",
	}

	ev := covReadEvent(t, c2)
	if ev.Action != "play" || ev.SenderID != "u1" || ev.RoomCode != "R" {
		t.Errorf("unexpected event: %+v", ev)
	}

	_ = c1.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := c1.ReadMessage(); err == nil {
		t.Error("sender received its own message back (echo filter broken)")
	}
}

func TestCovHubPumpReadInbound(t *testing.T) {
	rig := covNewPumpRig(t)
	defer rig.close(t)

	c1 := rig.dial(t, "R", "u1", "A")
	rig.waitRoomSize(t, "R", 1)
	covDrainJoins(t, c1, 1)

	c2 := rig.dial(t, "R", "u2", "B")
	rig.waitRoomSize(t, "R", 2)
	covDrainJoins(t, c1, 1)
	covDrainJoins(t, c2, 1)

	// Валідна подія від клієнта: readPump розбирає, підписує
	// кімнатою/відправником і кладе в broadcast.
	if err := c1.WriteMessage(websocket.TextMessage, []byte(`{"action":"pause"}`)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	ev := covReadEvent(t, c2)
	if ev.Action != "pause" {
		t.Errorf("expected pause, got %q", ev.Action)
	}
	if ev.SenderID != "u1" || ev.SenderName != "A" || ev.RoomCode != "R" {
		t.Errorf("server did not stamp sender/room: %+v", ev)
	}
	if ev.Timestamp.IsZero() {
		t.Error("expected server timestamp to be set")
	}

	// Битий JSON мовчки ігнорується — ніхто нічого не отримує.
	if err := c1.WriteMessage(websocket.TextMessage, []byte(`{"action":`)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_ = c2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := c2.ReadMessage(); err == nil {
		t.Error("broken JSON produced a broadcast")
	}

	// Pong від клієнта проходить PongHandler (продовження read deadline).
	if err := c1.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second)); err != nil {
		t.Errorf("pong write failed: %v", err)
	}
}

func TestCovHubPumpDisconnectCleanup(t *testing.T) {
	rig := covNewPumpRig(t)
	defer rig.close(t)

	c1 := rig.dial(t, "R", "u1", "A")
	rig.waitRoomSize(t, "R", 1)
	covDrainJoins(t, c1, 1)

	// Розрив з боку клієнта: readPump виходить (defer unregister +
	// Close), writePump бачить закритий send і теж виходить.
	if err := c1.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		rig.hub.mu.RLock()
		n := len(rig.hub.rooms)
		rig.hub.mu.RUnlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("room was not cleaned up after disconnect")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
