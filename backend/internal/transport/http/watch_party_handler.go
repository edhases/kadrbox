package http

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
	"github.com/edhases/kadrbox-server/internal/transport/ws"
)

// roomCodePattern дзеркалить перевірку всередині ws-хаба. Тут вона потрібна,
// щоб не випускати тікет на кімнату, яка все одно буде відхилена на handshake.
var roomCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,16}$`)

// maxDisplayNameRunes bounds the peer-supplied display name. The name is
// broadcast to everyone in the room and rendered in a list, so an unbounded
// string from another user would be both a layout problem and a cheap way to
// push megabytes through every guest's socket.
const maxDisplayNameRunes = 32

// sanitiseDisplayName trims, drops control characters and clamps the length.
//
// It never returns an error: a name that sanitises away to nothing falls back to
// the caller's id rather than failing the handshake, because the name is a label
// and not an authorisation input. What matters is that it cannot carry control
// characters into the UI or an unbounded payload onto the wire.
func sanitiseDisplayName(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7f:
			// Drop other C0 controls and DEL outright.
		case unicode.IsControl(r):
			// Cc: C1 controls.
		case unicode.In(r, unicode.Cf):
			// Cf: format characters. unicode.IsControl does NOT cover these,
			// which is exactly where the dangerous ones live: RLO/LRO can make
			// a name render as something else entirely, and ZWJ or ZWNJ can
			// make two different byte strings render identically. Dropping the
			// whole category is blunt but a display name has no use for it.
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.Join(strings.Fields(b.String()), " ")
	if count := len([]rune(cleaned)); count > maxDisplayNameRunes {
		cleaned = string([]rune(cleaned)[:maxDisplayNameRunes])
	}
	return cleaned
}

type WatchPartyHandler struct {
	// Typed as the interface rather than *ws.Hub so a test can pass a recorder
	// and observe exactly what the handler decided to put in the ticket --
	// including the sanitised display name, which never appears in the response.
	hub TicketIssuer
}

// NewWatchPartyHandler створює хендлер видачі одноразових квиток до кімнати.
// Квитка — єдиний спосіб автентифікувати Watch Party: раніше identity брався
// з query-параметрів, що дозволяло під'єднатися під чужим user_id.
func NewWatchPartyHandler(hub TicketIssuer) *WatchPartyHandler {
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
		UserName string `json:"userName"`
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

	// The display name is the caller's own label for itself and is broadcast to
	// the room. Without it every guest on a server-hosted room would be shown a
	// raw UUID, because the hub takes the name from the ticket claims alone.
	userName := sanitiseDisplayName(body.UserName)
	if userName == "" {
		userName = userID.String()
	}

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
