package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
)

const (
	BanderaDefaultBaseURL = "https://bbe.lme.isroot.in/api/v2"
	BanderaDefaultSources = "bambooua,makhno,animeon,franko,starlight"
)

type BanderaProvider struct {
	httpClient *http.Client
	baseURL    string
	sources    string
}

func NewBanderaProvider() *BanderaProvider {
	return NewBanderaProviderWithConfig(BanderaDefaultBaseURL, BanderaDefaultSources, nil)
}

func NewBanderaProviderWithConfig(baseURL, sources string, client *http.Client) *BanderaProvider {
	if baseURL == "" {
		baseURL = BanderaDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if sources == "" {
		sources = BanderaDefaultSources
	}
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}
	return &BanderaProvider{
		httpClient: client,
		baseURL:    baseURL,
		sources:    sources,
	}
}

func (p *BanderaProvider) ID() string {
	return "bandera"
}

func (p *BanderaProvider) Name() string {
	return "Bandera Online"
}

func (p *BanderaProvider) BaseURL() string {
	return p.baseURL
}

func (p *BanderaProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID:                   p.ID(),
		Name:                 p.Name(),
		BaseURL:              p.baseURL,
		ShowOnHome:           true,
		HasFixedStreams:      false,
		ContentTypes:         []string{"movie", "series", "anime"},
		SearchEnabledDefault: true,
	}
}

// BBE Search API Structures
type bbeSearchResponse struct {
	OK    bool            `json:"ok"`
	Items []bbeSearchItem `json:"items"`
}

type bbeSearchItem struct {
	Source    string          `json:"source"`
	Title     string          `json:"title"`
	TitleEn   string          `json:"title_en"`
	Poster    string          `json:"poster"`
	Type      string          `json:"type"`
	Year      json.RawMessage `json:"year"`
	Ref       json.RawMessage `json:"ref"`
	GroupKey  string          `json:"group_key"`
}

// BanderaItemPayload зберігається у MediaItem.URL та серіалізується у json
type BanderaItemPayload struct {
	Source string          `json:"source"`
	Ref    json.RawMessage `json:"ref"`
	Type   string          `json:"type,omitempty"`
	Title  string          `json:"title,omitempty"`
	Poster string          `json:"poster,omitempty"`
	Year   int             `json:"year,omitempty"`
}

func (p *BanderaProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	reqURL := fmt.Sprintf("%s/search?sources=%s&title=%s", p.baseURL, url.QueryEscape(p.sources), url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}

	req.Header.Set("User-Agent", "OxideFilm/1.0 (Bandera)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp bbeSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	var items []domain.MediaItem
	for i, item := range searchResp.Items {
		itemType := item.Type
		if itemType == "" {
			itemType = "movie"
		}

		yearInt := parseYearFromRaw(item.Year)

		payload := BanderaItemPayload{
			Source: item.Source,
			Ref:    item.Ref,
			Type:   itemType,
			Title:  item.Title,
			Poster: item.Poster,
			Year:   yearInt,
		}
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		itemURL := string(payloadBytes)

		// Унікальний ID
		itemID := fmt.Sprintf("bo_%s_%d", item.Source, i)
		var refMap map[string]interface{}
		if err := json.Unmarshal(item.Ref, &refMap); err == nil {
			if idVal, ok := refMap["id"]; ok {
				itemID = fmt.Sprintf("bo_%s_%v", item.Source, idVal)
			} else if hrefVal, ok := refMap["href"].(string); ok && hrefVal != "" {
				itemID = fmt.Sprintf("bo_%s_%s", item.Source, hrefVal)
			}
		}

		items = append(items, domain.MediaItem{
			ID:            itemID,
			ProviderID:    p.ID(),
			Title:         item.Title,
			OriginalTitle: item.TitleEn,
			PosterURL:     item.Poster,
			Year:          yearInt,
			Type:          itemType,
			URL:           itemURL,
		})
	}

	return items, nil
}

func (p *BanderaProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	// BBE API v2 не має окремого каталогу популярного.
	// Виконуємо кілька паралельних запитів по популярних термінах і об'єднуємо результати.
	var queries []string
	switch contentType {
	case "series":
		queries = []string{"серіал", "серіали", "сезон"}
	case "anime":
		queries = []string{"аніме", "anime"}
	case "cartoon":
		queries = []string{"мультфільм", "мультсеріал"}
	case "dorama":
		queries = []string{"дорама", "dorama"}
	default:
		// movie або all — беремо загальні популярні
		queries = []string{"фільм", "бойовик", "комедія", "драма"}
	}

	// Ротація за page щоб різні сторінки давали різні результати
	if page < 1 {
		page = 1
	}
	query := queries[(page-1)%len(queries)]

	return p.Search(ctx, query)
}

func (p *BanderaProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	if category == "" {
		return p.GetPopular(ctx, contentType, page)
	}
	// category може бути жанром ("Драма", "drama", "action" тощо)
	return p.Search(ctx, category)
}


// BBE Content API Structures
type bbeContentRequest struct {
	Source string          `json:"source"`
	Ref    json.RawMessage `json:"ref"`
}

type bbeContentResponse struct {
	OK      bool              `json:"ok"`
	Source  string            `json:"source"`
	Type    string            `json:"type"`
	Info    *bbeContentInfo   `json:"info"`
	Streams []bbeMovieStream  `json:"streams"`
	Voices  []bbeVoiceover    `json:"voices"`
	Seasons []bbeSimpleSeason `json:"seasons"`
}

type bbeContentInfo struct {
	ID          interface{} `json:"id"`
	Title       *string     `json:"title"`
	TitleEn     *string     `json:"title_en"`
	Description *string     `json:"description"`
	ReleaseDate *string     `json:"release_date"`
	EpisodeTime *string     `json:"episode_time"`
	Trailer     *string     `json:"trailer"`
	Rating      *string     `json:"rating"`
	Genres      []string    `json:"genres"`
	Image       string      `json:"image"`
}

type bbeMovieStream struct {
	Title string          `json:"title"`
	Ref   json.RawMessage `json:"ref"`
}

type bbeVoiceover struct {
	ID          string            `json:"id"`
	DisplayName string            `json:"display_name"`
	Seasons     []bbeVoiceSeason  `json:"seasons"`
	Episodes    []bbeVoiceEpisode `json:"episodes"`
}

type bbeVoiceSeason struct {
	Title    interface{}       `json:"title"`
	Episodes []bbeVoiceEpisode `json:"episodes"`
}

type bbeVoiceEpisode struct {
	Number int             `json:"number"`
	Title  *string         `json:"title"`
	Ref    json.RawMessage `json:"ref"`
}

type bbeSimpleSeason struct {
	Title      string `json:"title"`
	SeasonSlug string `json:"season_slug"`
}

// Stream Ref Wrapper зберігає ref епізоду або фільму разом із source
type BanderaStreamRef struct {
	Source string          `json:"source"`
	Ref    json.RawMessage `json:"ref"`
}

func (p *BanderaProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	var payload BanderaItemPayload
	if err := json.Unmarshal([]byte(itemURL), &payload); err != nil {
		return nil, fmt.Errorf("invalid item url payload: %w", err)
	}

	reqBody, err := json.Marshal(bbeContentRequest{
		Source: payload.Source,
		Ref:    payload.Ref,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal content request: %w", err)
	}

	contentURL := fmt.Sprintf("%s/content", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create content request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OxideFilm/1.0 (Bandera)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("content request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("content returned status %d: %s", resp.StatusCode, string(body))
	}

	var contentResp bbeContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&contentResp); err != nil {
		return nil, fmt.Errorf("decode content response: %w", err)
	}

	title := payload.Title
	originalTitle := ""
	description := ""
	posterURL := payload.Poster
	year := payload.Year
	duration := ""
	trailerURL := ""
	genres := []string{}
	var rating float64

	if contentResp.Info != nil {
		if contentResp.Info.Title != nil && *contentResp.Info.Title != "" {
			title = *contentResp.Info.Title
		}
		if contentResp.Info.TitleEn != nil && *contentResp.Info.TitleEn != "" {
			originalTitle = *contentResp.Info.TitleEn
		}
		if contentResp.Info.Description != nil {
			description = *contentResp.Info.Description
		}
		if contentResp.Info.Image != "" {
			posterURL = contentResp.Info.Image
		}
		if contentResp.Info.ReleaseDate != nil && *contentResp.Info.ReleaseDate != "" {
			if y, err := strconv.Atoi(strings.TrimSpace(*contentResp.Info.ReleaseDate)); err == nil && y > 1900 {
				year = y
			}
		}
		if contentResp.Info.EpisodeTime != nil {
			duration = *contentResp.Info.EpisodeTime
		}
		if contentResp.Info.Trailer != nil {
			trailerURL = *contentResp.Info.Trailer
		}
		if len(contentResp.Info.Genres) > 0 {
			genres = contentResp.Info.Genres
		}
		if contentResp.Info.Rating != nil && *contentResp.Info.Rating != "" {
			if r, err := strconv.ParseFloat(strings.TrimSpace(*contentResp.Info.Rating), 64); err == nil {
				rating = r
			}
		}
	}

	resolvedType := payload.Type
	if resolvedType == "" {
		resolvedType = contentResp.Type
	}
	if resolvedType == "" {
		resolvedType = "movie"
	}

	var voiceovers []domain.Voiceover
	var seasons []domain.Season

	// Якщо є voices (серіали або мультиваріантні джерела)
	if len(contentResp.Voices) > 0 {
		for _, v := range contentResp.Voices {
			voiceName := v.DisplayName
			if voiceName == "" {
				voiceName = v.ID
			}
			voiceovers = append(voiceovers, domain.Voiceover{
				ID:   v.ID,
				Name: voiceName,
			})
		}

		// Беремо перший voiceover як дефолтний для побудови дерева Seasons / Episodes
		defaultVoice := contentResp.Voices[0]
		if len(defaultVoice.Seasons) > 0 {
			for idx, s := range defaultVoice.Seasons {
				seasonNum := idx + 1
				switch t := s.Title.(type) {
				case float64:
					seasonNum = int(t)
				case string:
					if n, err := strconv.Atoi(t); err == nil {
						seasonNum = n
					}
				}

				var episodes []domain.Episode
				for _, ep := range s.Episodes {
					epTitle := ""
					if ep.Title != nil {
						epTitle = *ep.Title
					}
					if epTitle == "" {
						epTitle = fmt.Sprintf("Серія %d", ep.Number)
					}

					epRefBytes, _ := json.Marshal(BanderaStreamRef{
						Source: payload.Source,
						Ref:    ep.Ref,
					})

					episodes = append(episodes, domain.Episode{
						Number: ep.Number,
						Title:  epTitle,
						URL:    string(epRefBytes),
					})
				}

				seasons = append(seasons, domain.Season{
					Number:   seasonNum,
					Title:    fmt.Sprintf("Сезон %d", seasonNum),
					Episodes: episodes,
				})
			}
		} else if len(defaultVoice.Episodes) > 0 {
			// Якщо епізоди безпосередньо у voiceover
			var episodes []domain.Episode
			for _, ep := range defaultVoice.Episodes {
				epTitle := ""
				if ep.Title != nil {
					epTitle = *ep.Title
				}
				if epTitle == "" {
					epTitle = fmt.Sprintf("Серія %d", ep.Number)
				}
				epRefBytes, _ := json.Marshal(BanderaStreamRef{
					Source: payload.Source,
					Ref:    ep.Ref,
				})
				episodes = append(episodes, domain.Episode{
					Number: ep.Number,
					Title:  epTitle,
					URL:    string(epRefBytes),
				})
			}
			seasons = append(seasons, domain.Season{
				Number:   1,
				Title:    "Сезон 1",
				Episodes: episodes,
			})
		}
	} else if len(contentResp.Streams) > 0 {
		// Для фільмів зі streams
		for i, st := range contentResp.Streams {
			vName := st.Title
			if vName == "" {
				vName = fmt.Sprintf("Джерело %d", i+1)
			}
			voiceovers = append(voiceovers, domain.Voiceover{
				ID:   strconv.Itoa(i),
				Name: vName,
			})
		}
	}

	details := &domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:            payload.Source + "_" + strconv.Itoa(year) + "_" + title,
			ProviderID:    p.ID(),
			Title:         title,
			OriginalTitle: originalTitle,
			PosterURL:     posterURL,
			Year:          year,
			Type:          resolvedType,
			Rating:        rating,
			URL:           itemURL,
		},
		Description: description,
		Genres:      genres,
		Duration:    duration,
		TrailerURL:  trailerURL,
		Seasons:     seasons,
		Voiceovers:  voiceovers,
	}

	return details, nil
}

// BBE Stream API Structures
type bbeStreamRequest struct {
	Source string          `json:"source"`
	Ref    json.RawMessage `json:"ref"`
}

type bbeStreamResponse struct {
	OK      bool              `json:"ok"`
	Source  string            `json:"source"`
	Streams []bbeStreamSource `json:"streams"`
}

type bbeStreamSource struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
}

func (p *BanderaProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	var streamRef BanderaStreamRef

	// Перевіряємо чи itemURL сам по собі є BanderaStreamRef
	if err := json.Unmarshal([]byte(itemURL), &streamRef); err == nil && len(streamRef.Ref) > 0 && streamRef.Source != "" {
		// Якщо передано конкретний stream ref (наприклад, з URL епізоду)
	} else {
		// Якщо передано базовий itemURL медіа — запитуємо /content
		var payload BanderaItemPayload
		if err := json.Unmarshal([]byte(itemURL), &payload); err != nil {
			return nil, fmt.Errorf("invalid item url: %w", err)
		}

		contentReqBody, err := json.Marshal(bbeContentRequest{
			Source: payload.Source,
			Ref:    payload.Ref,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal content request: %w", err)
		}

		contentURL := fmt.Sprintf("%s/content", p.baseURL)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentURL, bytes.NewReader(contentReqBody))
		if err != nil {
			return nil, fmt.Errorf("create content request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "OxideFilm/1.0 (Bandera)")
		req.Header.Set("Accept", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("content request for stream: %w", err)
		}
		defer resp.Body.Close()

		var contentResp bbeContentResponse
		if err := json.NewDecoder(resp.Body).Decode(&contentResp); err != nil {
			return nil, fmt.Errorf("decode content response: %w", err)
		}

		if len(contentResp.Streams) > 0 {
			// Якщо це фільм зі списком streams
			streamIdx := 0
			if voiceID != "" {
				if idx, err := strconv.Atoi(voiceID); err == nil && idx < len(contentResp.Streams) {
					streamIdx = idx
				}
			}
			streamRef = BanderaStreamRef{
				Source: payload.Source,
				Ref:    contentResp.Streams[streamIdx].Ref,
			}
		} else if len(contentResp.Voices) > 0 {
			// Якщо це серіал або мультиваріант
			chosenVoice := contentResp.Voices[0]
			if voiceID != "" {
				for _, v := range contentResp.Voices {
					if v.ID == voiceID {
						chosenVoice = v
						break
					}
				}
			}

			found := false
			if len(chosenVoice.Seasons) > 0 {
				for sIdx, s := range chosenVoice.Seasons {
					sNum := sIdx + 1
					switch t := s.Title.(type) {
					case float64:
						sNum = int(t)
					case string:
						if n, err := strconv.Atoi(t); err == nil {
							sNum = n
						}
					}

					if season > 0 && sNum != season {
						continue
					}

					for _, ep := range s.Episodes {
						if episode > 0 && ep.Number != episode {
							continue
						}
						streamRef = BanderaStreamRef{
							Source: payload.Source,
							Ref:    ep.Ref,
						}
						found = true
						break
					}
					if found {
						break
					}
				}
			} else if len(chosenVoice.Episodes) > 0 {
				for _, ep := range chosenVoice.Episodes {
					if episode > 0 && ep.Number != episode {
						continue
					}
					streamRef = BanderaStreamRef{
						Source: payload.Source,
						Ref:    ep.Ref,
					}
					found = true
					break
				}
			}

			if !found {
				return nil, fmt.Errorf("no matching stream found for season %d episode %d voice %s", season, episode, voiceID)
			}
		} else {
			return nil, fmt.Errorf("no streams or voices found in content response")
		}
	}

	streamReqBody, err := json.Marshal(bbeStreamRequest{
		Source: streamRef.Source,
		Ref:    streamRef.Ref,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	streamURL := fmt.Sprintf("%s/stream", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, streamURL, bytes.NewReader(streamReqBody))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OxideFilm/1.0 (Bandera)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("stream returned status %d: %s", resp.StatusCode, string(body))
	}

	var streamResp bbeStreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&streamResp); err != nil {
		return nil, fmt.Errorf("decode stream response: %w", err)
	}

	var streams []domain.StreamSource
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}

	for _, s := range streamResp.Streams {
		if s.URL == "" {
			continue
		}
		quality := s.Quality
		if quality == "" {
			quality = "auto"
		}
		streams = append(streams, domain.StreamSource{
			Quality:       quality,
			URL:           s.URL,
			DirectURL:     s.URL,
			RequiresProxy: false,
			Headers:       headers,
		})
	}

	return &domain.ContentStreamsResponse{
		ProviderID: p.ID(),
		Streams:    streams,
		Subtitles:  []domain.SubtitleSource{},
	}, nil
}

func parseYearFromRaw(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var num int
	if err := json.Unmarshal(raw, &num); err == nil {
		return num
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		str = strings.TrimSpace(str)
		if n, err := strconv.Atoi(str); err == nil {
			return n
		}
	}
	return 0
}
