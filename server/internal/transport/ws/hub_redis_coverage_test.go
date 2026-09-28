package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edhases/oxide-server/internal/domain"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/gorilla/websocket"
)

// Хаб із живим Redis-клієнтом (miniredis): гілки Publish у register /
// unregister / readPump виконуються по-справжньому, локальний broadcast
// при цьому не ламається.

func covRedisPumpDial(t *testing.T, srv *httptest.Server, room, userID, userName string) *websocket.Conn {
	t.Helper()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) +
		"?room=" + room + "&user_id=" + userID + "&user_name=" + userName
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	return conn
}

func covRedisPumpRead(t *testing.T, conn *websocket.Conn) domain.WatchPartyEvent {
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

func covRedisPumpDrain(t *testing.T, conn *websocket.Conn, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if ev := covRedisPumpRead(t, conn); ev.Action != "userJoined" {
			t.Fatalf("expected userJoined, got %q", ev.Action)
		}
	}
}

func covRedisPumpWaitRoom(t *testing.T, h *Hub, room string, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.RLock()
		got := len(h.rooms[room])
		h.mu.RUnlock()
		if got == n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("room %s: expected %d clients, timed out", room, n)
}

func TestCovHubPumpWithRedisPublish(t *testing.T) {
	m, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis failed: %v", err)
	}
	defer m.Close()
	rc, err := redisRepo.NewRedisClient(m.Addr(), "")
	if err != nil {
		t.Fatalf("redis connect failed: %v", err)
	}
	defer func() { _ = rc.Close() }()

	h := NewHub(rc)
	go h.Run()
	srv := httptest.NewServer(http.HandlerFunc(h.HandleWebSocket))
	var c1, c2 *websocket.Conn
	defer func() {
		// Той самий безпечний порядок: спочатку рвемо клієнтські TCP
		// і чекаємо порожніх rooms, і лише тоді GracefulStop.
		if c1 != nil {
			_ = c1.Close()
		}
		if c2 != nil {
			_ = c2.Close()
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			h.mu.RLock()
			n := len(h.rooms)
			h.mu.RUnlock()
			if n == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("rooms not drained before stop")
			}
			time.Sleep(20 * time.Millisecond)
		}
		h.GracefulStop()
		srv.Close()
	}()

	c1 = covRedisPumpDial(t, srv, "R", "u1", "A")
	covRedisPumpWaitRoom(t, h, "R", 1)
	if ev := covRedisPumpRead(t, c1); ev.Action != "userJoined" {
		t.Fatalf("expected userJoined via redis-backed hub, got %q", ev.Action)
	}

	// Вхідна подія від клієнта: readPump публікує її в Redis
	// (miniredis приймає PUBLISH) і розсилає локально.
	c2 = covRedisPumpDial(t, srv, "R", "u2", "B")
	covRedisPumpWaitRoom(t, h, "R", 2)
	covRedisPumpDrain(t, c1, 1)
	covRedisPumpDrain(t, c2, 1)

	if err := c1.WriteMessage(websocket.TextMessage, []byte(`{"action":"seek"}`)); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	got := covRedisPumpRead(t, c2)
	if got.Action != "seek" || got.SenderID != "u1" {
		t.Errorf("unexpected event: %+v", got)
	}
}
