package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/edhases/oxide-server/internal/domain"
)

// Wire actions. Everything a client may originate is listed in clientActions;
// userJoined/userLeft/roomInfo are server-only and are rejected if a client
// sends them.
const (
	ActionPlay        = "play"
	ActionPause       = "pause"
	ActionSeek        = "seek"
	ActionSpeed       = "speed"
	ActionSync        = "sync"
	ActionChat        = "chat"
	ActionRequestSync = "requestSync"

	ActionUserJoined = "userJoined"
	ActionUserLeft   = "userLeft"
	ActionRoomInfo   = "roomInfo"
)

const (
	// maxChatRunes caps a chat payload. The Dart client renders payload as String
	// without a length check, so an uncapped string is both a memory amplifier and
	// a UI hijack vector.
	maxChatRunes = 500

	// minPlaybackSpeed / maxPlaybackSpeed bound the speed payload. Anything else
	// is dropped rather than forwarded, because the client applies it verbatim to
	// the media pipeline.
	minPlaybackSpeed = 0.25
	maxPlaybackSpeed = 4.0

	// maxSeekMs is a week; a larger value can only be a bug or an attempt to
	// make a client's seek() throw.
	maxSeekMs = 7 * 24 * 60 * 60 * 1000
)

var errNoPayload = errors.New("ws: action requires a payload")

// inboundMessage is the only shape a client frame is decoded into. Note what is
// absent: senderId/senderName/roomCode/timestamp from the client are never read,
// so identity cannot be spoofed by any field combination.
type inboundMessage struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// actionSpec describes one permitted client action.
type actionSpec struct {
	// hostOnly marks an action that changes shared playback. Only the room host
	// may originate it.
	hostOnly bool
	// build validates and normalises the payload. Returning an error drops the
	// frame; the connection stays up.
	build func(raw json.RawMessage) (interface{}, error)
}

// clientActions is the allow-list. A missing entry means "rejected", which is
// why the map is consulted instead of a switch with a default.
var clientActions = map[string]actionSpec{
	ActionPlay:        {hostOnly: true, build: ignorePayload},
	ActionPause:       {hostOnly: true, build: ignorePayload},
	ActionSeek:        {hostOnly: true, build: buildSeek},
	ActionSpeed:       {hostOnly: true, build: buildSpeed},
	ActionSync:        {hostOnly: true, build: buildSync},
	ActionChat:        {hostOnly: false, build: buildChat},
	ActionRequestSync: {hostOnly: false, build: ignorePayload},
}

// serverActions is what StartSubscription accepts from Redis. Redis is a shared
// bus, so its frames are filtered against the same vocabulary.
var serverActions = map[string]bool{
	ActionPlay: true, ActionPause: true, ActionSeek: true, ActionSpeed: true,
	ActionSync: true, ActionChat: true, ActionRequestSync: true,
	ActionUserJoined: true, ActionUserLeft: true, ActionRoomInfo: true,
}

// validateClientMessage turns a raw client frame into a fully stamped event, or
// reports why the frame was dropped.
//
// Host-role rule (implemented here, single source of truth):
//
//	The host of a room is the client whose ticket carried is_host=true; if no
//	ticket does, the first authenticated client to register becomes host. When
//	the host leaves, the longest-present remaining client is promoted.
//	play, pause, seek, speed and sync change shared playback and are accepted
//	only from the host; a guest's attempt is DROPPED (not forwarded, not
//	downgraded), so there is never a second, competing authority for a room.
//	chat and requestSync are guest-safe: chat is everyone's, requestSync asks
//	the host for a sync frame. userJoined, userLeft and roomInfo are
//	server-only and are dropped if a client sends them.
func validateClientMessage(c *Client, raw []byte) (*domain.WatchPartyEvent, error) {
	var in inboundMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("ws: malformed frame: %w", err)
	}
	spec, ok := clientActions[in.Action]
	if !ok {
		return nil, fmt.Errorf("ws: action %q is not accepted from clients", in.Action)
	}
	if spec.hostOnly && !c.hub.isRoomHost(c) {
		return nil, fmt.Errorf("ws: %q is host-only and %q is not the host of %q", in.Action, c.userID, c.roomCode)
	}
	payload, err := spec.build(in.Payload)
	if err != nil {
		return nil, fmt.Errorf("ws: invalid %q payload: %w", in.Action, err)
	}
	return &domain.WatchPartyEvent{
		Action:     in.Action,
		RoomCode:   c.roomCode,
		SenderID:   c.userID,
		SenderName: c.userName,
		Payload:    payload,
		Timestamp:  time.Now(),
	}, nil
}

func isJSONNull(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "null"
}

// ignorePayload accepts any payload shape but drops it: play/pause carry no
// data, and forwarding an attacker-chosen body for them is pointless.
func ignorePayload(raw json.RawMessage) (interface{}, error) {
	return nil, nil
}

func buildSeek(raw json.RawMessage) (interface{}, error) {
	if isJSONNull(raw) {
		return nil, errNoPayload
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return nil, errors.New("expected a JSON number of milliseconds")
	}
	ms, err := strconv.ParseInt(num.String(), 10, 64)
	if err != nil {
		return nil, errors.New("expected an integer number of milliseconds")
	}
	if ms < 0 || ms > maxSeekMs {
		return nil, fmt.Errorf("seek out of range: %d", ms)
	}
	return ms, nil
}

func buildSpeed(raw json.RawMessage) (interface{}, error) {
	if isJSONNull(raw) {
		return nil, errNoPayload
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return nil, errors.New("expected a JSON number")
	}
	speed, err := strconv.ParseFloat(num.String(), 64)
	if err != nil {
		return nil, errors.New("expected a JSON number")
	}
	if speed != speed /* NaN */ || speed < minPlaybackSpeed || speed > maxPlaybackSpeed {
		return nil, fmt.Errorf("speed out of range: %v", speed)
	}
	return speed, nil
}

func buildChat(raw json.RawMessage) (interface{}, error) {
	if isJSONNull(raw) {
		return nil, errNoPayload
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, errors.New("expected a JSON string")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty message")
	}
	if utf8.RuneCountInString(text) > maxChatRunes {
		text = string([]rune(text)[:maxChatRunes])
	}
	return text, nil
}

// buildSync keeps the {position, speed} shape the Dart client already parses, but
// re-emits it from typed values so downstream consumers get an int64 position
// and a float64 speed instead of whatever float the decoder produced.
func buildSync(raw json.RawMessage) (interface{}, error) {
	if isJSONNull(raw) {
		return nil, errNoPayload
	}
	var in struct {
		Position *json.Number `json:"position"`
		Speed    *json.Number `json:"speed"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, errors.New("expected an object with position/speed")
	}
	if in.Position == nil {
		return nil, errors.New("missing position")
	}
	ms, err := strconv.ParseInt(in.Position.String(), 10, 64)
	if err != nil || ms < 0 || ms > maxSeekMs {
		return nil, errors.New("position out of range")
	}
	out := map[string]interface{}{"position": ms}
	if in.Speed != nil {
		speed, err := strconv.ParseFloat(in.Speed.String(), 64)
		if err != nil || speed < minPlaybackSpeed || speed > maxPlaybackSpeed {
			return nil, errors.New("speed out of range")
		}
		out["speed"] = speed
	}
	return out, nil
}
