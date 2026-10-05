package domain

import "context"

// Provider - універсальний інтерфейс джерела контенту
type Provider interface {
	ID() string
	Name() string
	BaseURL() string
	Describe() ProviderInfo
	Search(ctx context.Context, query string) ([]MediaItem, error)
	GetPopular(ctx context.Context, contentType string, page int) ([]MediaItem, error)
	GetByCategory(ctx context.Context, category, contentType string, page int) ([]MediaItem, error)
	GetDetails(ctx context.Context, itemURL string) (*MediaDetails, error)
	GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*ContentStreamsResponse, error)
}

// ProviderInfo — публічні метадані провайдера для каталогу.
// Бекенд є джерелом правди: застосунок будує списки провайдерів з каталогу.
type ProviderInfo struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	BaseURL              string   `json:"baseUrl"`
	IconURL              string   `json:"iconUrl,omitempty"`
	ShowOnHome           bool     `json:"showOnHome"`
	HasFixedStreams      bool     `json:"hasFixedStreams"`
	ContentTypes         []string `json:"contentTypes"`
	SearchEnabledDefault bool     `json:"searchEnabledDefault"`
}

// ProviderCatalog — відповідь GET /api/v1/content/providers.
type ProviderCatalog struct {
	Version   int64                  `json:"version"`
	Providers []ProviderCatalogEntry `json:"providers"`
}

// ProviderCatalogEntry — метадані + керований бекендом стан.
type ProviderCatalogEntry struct {
	ProviderInfo
	Enabled bool           `json:"enabled"`
	Healthy bool           `json:"healthy"`
	Health  ProviderHealth `json:"health"`
}

// ProviderHealth — зведене здоров'я провайдера.
type ProviderHealth struct {
	ConsecutiveErrors int    `json:"consecutiveErrors"`
	LastError         string `json:"lastError,omitempty"`
	LastSuccessUnix   int64  `json:"lastSuccessUnix,omitempty"`
	LastErrorUnix     int64  `json:"lastErrorUnix,omitempty"`
}
