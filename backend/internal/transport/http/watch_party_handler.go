package http

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/edhases/oxide-server/internal/transport/http/middleware"
	"github.com/edhases/oxide-server/internal/transport/ws"
)

// roomCodePattern дзеркалить перевірку всередині ws-хаба. Тут вона потрібна,
// щоб не випускати тікет на кімнату, яка все одно буде відхилена на handshake.
var roomCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,16}$`)

type WatchPartyHandler struct {
	hub *ws.Hub
}

// NewWatchPartyHandler створює хендлер видачі одноразових квиток до кімнати.
// Квитка — єдиний спосіб автентифікувати Watch Party: раніше identity брався
// з query-параметрів, що дозволяло під'єднатися під чужим user_id.
func NewWatchPartyHandler(hub *ws.Hub) *WatchPartyHandler {
	return &WatchPartyHandler{hub: hub}
}

// TicketIssuer — вузький інтерфейс, щоб хендлер не залежав від конкретного Hub.
type TicketIssuer interface {
	IssueWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error)
	IssueHostWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error)
}

var _ TicketIssuer = (*ws.Hub)(nil)

// Issue обслуговує POST /api/v1/watch-party/tickets.
//
// Маршрут живе всередині групи AuthMiddleware, тому userID уже перевірений
// middleware'ом, а не приходить із тіла запиту.
func (h *WatchPartyHandler) Issue(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var body struct {
		RoomCode string `json:"roomCode"`
		IsHost   bool   `json:"isHost"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Хост коду кімнати генерує клієнт, але сервер має перевірити форму
	// до того, як видасть тікет із правом хоста.
	room := strings.ToUpper(strings.TrimSpace(body.RoomCode))
	if !roomCodePattern.MatchString(room) {
		jsonError(w, "invalid room code", http.StatusBadRequest)
		return
	}

	userName := userID.String()
	var ticket string
	var err error
	if body.IsHost {
		ticket, err = h.hub.IssueHostWatchPartyTicket(userID.String(), userName, room, ws.DefaultTicketTTL)
	} else {
		ticket, err = h.hub.IssueWatchPartyTicket(userID.String(), userName, room, ws.DefaultTicketTTL)
	}
	if err != nil {
		jsonError(w, "failed to issue watch party ticket", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ticket":    ticket,
		"roomCode":  room,
		"expiresAt": time.Now().Add(ws.DefaultTicketTTL).UTC(),
	})
}
