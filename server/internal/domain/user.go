package domain

import (
	"time"

	"github.com/google/uuid"
)

// User описує обліковий запис користувача в системі
type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Username     string    `json:"username"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	Bio          string    `json:"bio,omitempty"`
	Role         string    `json:"role"`
	IsVerified   bool      `json:"is_verified"`
	TelegramID   *int64    `json:"telegram_id,omitempty"`
	DiscordID    *string   `json:"discord_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Favorite описує збережений фільм/серіал у списку бажаного
type Favorite struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	MediaID      string    `json:"media_id"`
	ProviderID   string    `json:"provider_id"`
	Title        string    `json:"title"`
	PosterURL    string    `json:"poster_url,omitempty"`
	Year         *int      `json:"year,omitempty"`
	MediaType    string    `json:"media_type,omitempty"`
	Rating       *float64  `json:"rating,omitempty"`
	RatingSource *string   `json:"rating_source,omitempty"`
	AddedAt      time.Time `json:"added_at"`
}

// WatchHistory описує історію перегляду та прогрес
type WatchHistory struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	MediaID       string    `json:"media_id"`
	ProviderID    string    `json:"provider_id"`
	Title         string    `json:"title"`
	PosterURL     string    `json:"poster_url,omitempty"`
	Year          *int      `json:"year,omitempty"`
	MediaType     string    `json:"media_type,omitempty"`
	Season        *int      `json:"season,omitempty"`  // NULL для повнометражних фільмів
	Episode       *int      `json:"episode,omitempty"` // NULL для повнометражних фільмів
	EpisodeTitle  string    `json:"episode_title,omitempty"`
	PositionMs    int64     `json:"position_ms"`
	DurationMs    int64     `json:"duration_ms"`
	LastStreamURL string    `json:"last_stream_url,omitempty"`
	Voiceover     string    `json:"voiceover,omitempty"`
	Rating        *float64  `json:"rating,omitempty"`
	RatingSource  *string   `json:"rating_source,omitempty"`
	WatchedAt     time.Time `json:"watched_at"`
}

// WatchPartyRoom описує постійну сутність кімнати в Postgres
type WatchPartyRoom struct {
	ID        uuid.UUID `json:"id"`
	RoomCode  string    `json:"room_code"`
	HostID    uuid.UUID `json:"host_id"`
	MediaID   string    `json:"media_id"`
	Title     string    `json:"title"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}
