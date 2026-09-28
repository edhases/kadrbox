package domain

import "time"

// WatchPartyState описує поточний стан кімнати в пам'яті Redis
type WatchPartyState struct {
	HostID            string `json:"host_id"`
	MediaID           string `json:"media_id"`
	StreamURL         string `json:"stream_url"`
	CurrentPositionMs int64  `json:"current_position_ms"`
	IsPlaying         bool   `json:"is_playing"`
	PlaybackSpeed     float64 `json:"playback_speed"`
	UpdatedAtEpoch    int64  `json:"updated_at_epoch"` // Unix millisecond для точної екстраполяції
}

// WatchPartyEvent описує подію в Redis Pub/Sub та WebSocket
type WatchPartyEvent struct {
	Action     string      `json:"action"` // SYNC, PLAY, PAUSE, SEEK, CHAT, USER_JOINED, USER_LEFT
	RoomCode   string      `json:"room_code"`
	SenderID   string      `json:"sender_id"`
	SenderName string      `json:"sender_name"`
	Payload    interface{} `json:"payload,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}
