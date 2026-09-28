package ws

import (
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/gorilla/websocket"
)

func TestCovHubPumpStopWithClients(t *testing.T) {
	rig := covNewPumpRig(t)
	defer rig.srv.Close()

	c1 := rig.dial(t, "R", "u1", "A")
	defer c1.Close()
	c2 := rig.dial(t, "R", "u2", "B")
	defer c2.Close()
	rig.waitRoomSize(t, "R", 2)
	covDrainJoins(t, c1, 2)
	covDrainJoins(t, c2, 1)

	// Даємо writePump запаркуватись: на момент GracefulStop жоден
	// data-write не має бути в польоті, інакше stop-гілка (пише Close
	// в той самий конекшен) дасть "concurrent write to websocket connection".
	time.Sleep(300 * time.Millisecond)

	// Зупинка з живими клієнтами: stop-гілка шле CloseGoingAway
	// справжнім конекшенам, чистить rooms і завершує Run.
	rig.hub.GracefulStop()

	for i, c := range []*websocket.Conn{c1, c2} {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, _, err := c.ReadMessage()
		closeErr, ok := err.(*websocket.CloseError)
		if !ok {
			t.Fatalf("client %d: expected CloseError, got %v", i, err)
		}
		if closeErr.Code != websocket.CloseGoingAway {
			t.Errorf("client %d: expected code 1001 GoingAway, got %d", i, closeErr.Code)
		}
	}

	rig.hub.mu.RLock()
	rooms := len(rig.hub.rooms)
	rig.hub.mu.RUnlock()
	if rooms != 0 {
		t.Errorf("expected rooms reset after stop, got %d", rooms)
	}
}

func TestCovHubPumpUnknownRoom(t *testing.T) {
	rig := covNewPumpRig(t)
	defer rig.close(t)

	c1 := rig.dial(t, "R", "u1", "A")
	defer c1.Close()
	rig.waitRoomSize(t, "R", 1)
	covDrainJoins(t, c1, 1)

	// Подія в неіснуючу кімнату: rooms[code] == nil, цикл нуль ітерацій.
	rig.hub.broadcast <- &domain.WatchPartyEvent{
		Action: "play", RoomCode: "NOPE", SenderID: "ghost",
	}

	_ = c1.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, _, err := c1.ReadMessage(); err == nil {
		t.Error("client received event for another (nonexistent) room")
	}
}
