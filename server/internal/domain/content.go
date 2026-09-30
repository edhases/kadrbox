package domain

// StreamSource описує один потік відео з окремими заголовками для клієнтського плеєра libmpv/media_kit
type StreamSource struct {
	Quality       string            `json:"quality"`
	URL           string            `json:"url"`           // URL для відтворення (може бути проксі маніфесту або direct)
	DirectURL     string            `json:"direct_url"`    // Пряме CDN посилання
	RequiresProxy bool              `json:"requires_proxy"`
	Headers       map[string]string `json:"headers"`       // Важливо: Referer, User-Agent для передачі в media_kit!
}

// SubtitleSource описує субтитри
type SubtitleSource struct {
	Language string `json:"language"`
	Label    string `json:"label"`
	URL      string `json:"url"`
}

// MediaItem описує базовий елемент каталогу/пошуку
type MediaItem struct {
	ID          string   `json:"id"`
	ProviderID  string   `json:"provider_id"`
	Title       string   `json:"title"`
	OriginalTitle string `json:"original_title,omitempty"`
	PosterURL   string   `json:"poster_url,omitempty"`
	Year        int      `json:"year,omitempty"`
	Type        string   `json:"type"` // movie, series, anime, cartoon
	Rating      float64  `json:"rating,omitempty"`
	URL         string   `json:"url"`
}

// MediaDetails описує повні метадані медіа
type MediaDetails struct {
	MediaItem
	Description string      `json:"description,omitempty"`
	Genres      []string    `json:"genres,omitempty"`
	Countries   []string    `json:"countries,omitempty"`
	Director    string      `json:"director,omitempty"`
	Actors      []string    `json:"actors,omitempty"`
	Duration    string      `json:"duration,omitempty"`
	TrailerURL  string      `json:"trailer_url,omitempty"`
	Seasons     []Season    `json:"seasons,omitempty"`
	Voiceovers  []Voiceover `json:"voiceovers,omitempty"`
}

// Season описує сезон серіалу
type Season struct {
	Number   int       `json:"number"`
	Title    string    `json:"title,omitempty"`
	Episodes []Episode `json:"episodes"`
}

// Episode описує серію
type Episode struct {
	Number int    `json:"number"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Voiceover описує варіант студії озвучення
type Voiceover struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ContentStreamsResponse - фінальний контракт відповіді для ендпоінта отримання потоків
type ContentStreamsResponse struct {
	ProviderID string           `json:"provider_id"`
	Streams    []StreamSource   `json:"streams"`
	Subtitles  []SubtitleSource `json:"subtitles"`
}
