package ws

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TicketPurpose is the claim that pins a token to the WebSocket upgrade. It is
// what stops an ordinary access token (or a ticket for a different subsystem)
// from being replayed at this endpoint, and what stops a ticket from being
// replayed at any other endpoint that accepts HS256 tokens signed with the same
// secret: internal/auth parses into its own Claims struct, which has no
// purpose/room/jti field, so a ticket decoded there carries no user identity.
const TicketPurpose = "ws_ticket"

var (
	// ErrTicketsNotConfigured means the hub has no JWTSecret. Fails closed.
	ErrTicketsNotConfigured = errors.New("ws: watch party ticket issuer not configured")
	// ErrMissingTicket means no ticket was presented at all.
	ErrMissingTicket = errors.New("ws: missing watch party ticket")
	// ErrInvalidTicket covers bad signatures, wrong purpose, wrong signing
	// method, missing jti and expiry failures.
	ErrInvalidTicket = errors.New("ws: invalid watch party ticket")
	// ErrTicketRoomMismatch means a valid ticket for another room.
	ErrTicketRoomMismatch = errors.New("ws: ticket was issued for a different room")
	// ErrEmptyUserName means the ticket carries no display name.
	ErrEmptyUserName = errors.New("ws: ticket carries no user name")
)

// TicketIssuer mints short-lived, single-room WS admission tokens.
type TicketIssuer interface {
	IssueWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error)
}

// TicketClaims is the verified identity of a joining client. Nothing else may
// populate a Client's userID/userName: the query string is ignored outright.
type TicketClaims struct {
	UserID   string
	UserName string
	RoomCode string
	IsHost   bool
	TicketID string
}

// ticketClaims is the wire form.
//
// It is deliberately NOT internal/auth.Claims: reusing that struct would make a
// watch-party ticket structurally indistinguishable from an access token (both
// HS256, same secret, same Subject), so either could be replayed at the other's
// endpoint. A separate claim set with a mandatory purpose/room/jti is the
// least-invasive way to keep the two token classes apart while still reusing the
// configured JWT secret instead of introducing a second one.
type ticketClaims struct {
	Purpose  string `json:"purpose"`
	Room     string `json:"room"`
	UserName string `json:"user_name"`
	IsHost   bool   `json:"is_host"`

	jwt.RegisteredClaims
}

// hmacTicketIssuer is the default TicketIssuer: HS256 over the hub's secret,
// the same algorithm and secret the access tokens use.
type hmacTicketIssuer struct {
	secret []byte
}

// NewTicketIssuer returns the default implementation of TicketIssuer. It returns
// nil for an empty secret so callers cannot accidentally mint tokens that
// nobody can verify.
func NewTicketIssuer(secret string) TicketIssuer {
	if strings.TrimSpace(secret) == "" {
		return nil
	}
	return &hmacTicketIssuer{secret: []byte(secret)}
}

func (i *hmacTicketIssuer) IssueWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error) {
	return i.issue(userID, userName, roomCode, false, ttl)
}

// IssueHostWatchPartyTicket is not on the TicketIssuer interface because only the
// room-creating endpoint may call it, and that endpoint is a distinct
// authorisation decision from "admit this user to this room". It is exposed on
// Hub next to IssueWatchPartyTicket so the HTTP layer needs no extra wiring.
func (i *hmacTicketIssuer) IssueHostWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error) {
	return i.issue(userID, userName, roomCode, true, ttl)
}

func (i *hmacTicketIssuer) issue(userID, userName, roomCode string, isHost bool, ttl time.Duration) (string, error) {
	if len(i.secret) == 0 {
		return "", ErrTicketsNotConfigured
	}
	if !validRoomCode(roomCode) {
		return "", fmt.Errorf("ws: refusing to issue a ticket for invalid room code %q", roomCode)
	}
	if userID == "" {
		return "", errors.New("ws: cannot issue a ticket without a user id")
	}
	if ttl <= 0 {
		ttl = DefaultTicketTTL
	}
	claims := ticketClaims{
		Purpose:  TicketPurpose,
		Room:     roomCode,
		UserName: userName,
		IsHost:   isHost,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-5 * time.Second)),
			ID:        uuid.NewString(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
}

// IssueWatchPartyTicket makes Hub itself usable wherever a TicketIssuer is
// expected, so the HTTP layer that mints tickets only needs the hub.
func (h *Hub) IssueWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error) {
	if h.issuer == nil {
		return "", ErrTicketsNotConfigured
	}
	return h.issuer.IssueWatchPartyTicket(userID, userName, roomCode, ttl)
}

// IssueHostWatchPartyTicket mints a ticket that asserts the host role. Call it
// only from the endpoint that CREATES a room: the holder becomes the room's host
// on arrival, and the host is the only client whose play/pause/seek/speed/sync
// frames are authoritative.
func (h *Hub) IssueHostWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error) {
	hostIssuer, ok := h.issuer.(interface {
		IssueHostWatchPartyTicket(userID, userName, roomCode string, ttl time.Duration) (string, error)
	})
	if !ok {
		return "", ErrTicketsNotConfigured
	}
	return hostIssuer.IssueHostWatchPartyTicket(userID, userName, roomCode, ttl)
}

// parseWatchPartyTicket verifies signature, signing method, purpose, expiry and
// jti, and returns the verified identity. ttl <= 0 issues an already-expired
// token; that path exists only so tests can exercise the rejection.
func parseWatchPartyTicket(secret []byte, raw string) (*TicketClaims, error) {
	if len(secret) == 0 {
		return nil, ErrTicketsNotConfigured
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrMissingTicket
	}

	claims := &ticketClaims{}
	parser := jwt.NewParser(
		// Pin the algorithm: without this a token could arrive as "none" or as an
		// asymmetric algorithm whose verification the keyfunc never rejects.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if _, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		return secret, nil
	}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTicket, err)
	}

	if claims.Purpose != TicketPurpose {
		return nil, fmt.Errorf("%w: purpose %q", ErrInvalidTicket, claims.Purpose)
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("%w: empty subject", ErrInvalidTicket)
	}
	if claims.ID == "" {
		return nil, fmt.Errorf("%w: missing jti", ErrInvalidTicket)
	}
	if !validRoomCode(claims.Room) {
		return nil, fmt.Errorf("%w: bad room claim", ErrInvalidTicket)
	}
	return &TicketClaims{
		UserID:   claims.Subject,
		UserName: claims.UserName,
		RoomCode: claims.Room,
		IsHost:   claims.IsHost,
		TicketID: claims.ID,
	}, nil
}
