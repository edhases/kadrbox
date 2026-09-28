package domain

import "context"

// Provider - універсальний інтерфейс джерела контенту
type Provider interface {
	ID() string
	Name() string
	BaseURL() string
	Search(ctx context.Context, query string) ([]MediaItem, error)
	GetDetails(ctx context.Context, itemURL string) (*MediaDetails, error)
	GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*ContentStreamsResponse, error)
}
