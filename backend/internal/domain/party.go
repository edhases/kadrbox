package domain

import "time"

// WatchPartyState описує поточний стан кімнати в пам'яті Redis
type WatchPartyState struct {
	HostID            string  `json:"hostId"`
	MediaID           string  `json:"mediaId"`
	StreamURL         string  `json:"streamUrl"`
	CurrentPositionMs int64   `json:"currentPositionMs"`
	IsPlaying         bool    `json:"isPlaying"`
	PlaybackSpeed     float64 `json:"playbackSpeed"`
	UpdatedAtEpoch    int64   `json:"updatedAtEpoch"`
}

// WatchPartyEvent описує подію в Redis Pub/Sub та WebSocket (уніфіковано під camelCase для Flutter)
type WatchPartyEvent struct {
	Action     string      `json:"action"` // sync, play, pause, seek, chat, userJoined, userLeft
	RoomCode   string      `json:"roomCode"`
	SenderID   string      `json:"senderId"`
	SenderName string      `json:"senderName"`
	Payload    interface{} `json:"payload,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}
