package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
)

// BanderaProvider є агрегатором українського контенту (Lampa/Bandera Online API).
type BanderaProvider struct {
	client  *BanderaClient
	sources string
}

func NewBanderaProvider() *BanderaProvider {
	return NewBanderaProviderWithConfig(BanderaDefaultBaseURL, BanderaDefaultSources, nil)
}

func NewBanderaProviderWithConfig(baseURL, sources string, client *http.Client) *BanderaProvider {
	bClient := NewBanderaClient(baseURL, client)
	if sources == "" {
		sources = BanderaDefaultSources
	}
	return &BanderaProvider{
		client:  bClient,
		sources: sources,
	}
}

func (p *BanderaProvider) ID() string {
	return "bandera"
}

func (p *BanderaProvider) Name() string {
	return "Bandera Online"
}

func (p *BanderaProvider) BaseURL() string {
	return p.client.baseURL
}

func (p *BanderaProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID:                   p.ID(),
		Name:                 p.Name(),
		BaseURL:              p.BaseURL(),
		ShowOnHome:           true,
		SearchEnabledDefault: true,
		HasFixedStreams:      false,
		ContentTypes:         []string{"movie", "series", "anime"},
	}
}

func GenerateStableContentID(source, title string, year int, ref json.RawMessage) string {
	h := sha1.New()
	h.Write([]byte(source))
	h.Write([]byte(":"))
	h.Write([]byte(title))
	h.Write([]byte(":"))
	h.Write([]byte(strconv.Itoa(year)))
	if len(ref) > 0 {
		h.Write([]byte(":"))
		h.Write(ref)
	}
	return "bo_" + source + "_" + hex.EncodeToString(h.Sum(nil))[:10]
}

func generateStableContentID(source, title string, year int, ref json.RawMessage) string {
	return GenerateStableContentID(source, title, year, ref)
}

// convertSearchItems конвертує елементи відповіді Bandera в domain.MediaItem
func (p *BanderaProvider) convertSearchItems(rawItems []BanderaSearchItem) []domain.MediaItem {
	var results []domain.MediaItem
	for _, item := range rawItems {
		year := ParseFlexibleYear(item.Year)
		stableID := generateStableContentID(item.Source, item.Title, year, item.Ref)

		mediaType := item.Type.String()
		if mediaType == "" {
			if item.Serial.Int() == 1 {
				mediaType = "series"
			} else {
				mediaType = "movie"
			}
		}

		payload := BanderaItemPayload{
			ID:        stableID,
			Source:    item.Source,
			Ref:       item.Ref,
			Type:      mediaType,
			Title:     item.Title,
			Poster:    item.Poster.String(),
			Year:      year,
			IsItemRef: true,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			continue
		}

		results = append(results, domain.MediaItem{
			ID:            stableID,
			ProviderID:    p.ID(),
			Title:         item.Title,
			OriginalTitle: item.TitleEn.String(),
			PosterURL:     item.Poster.String(),
			Year:          year,
			Type:          mediaType,
			URL:           string(payloadBytes),
		})
	}
	return results
}

// rankAndSortMediaItems дедуплікує елементи за назвою та роком, та сортує за цільовим типом та роком (новіші спочатку).
func rankAndSortMediaItems(items []domain.MediaItem, targetType string) []domain.MediaItem {
	if len(items) <= 1 {
		return items
	}

	seen := make(map[string]int)
	var unique []domain.MediaItem

	for _, item := range items {
		normTitle := strings.ToLower(strings.TrimSpace(item.Title))
		key := fmt.Sprintf("%s:%d", normTitle, item.Year)
		if idx, exists := seen[key]; exists {
			if unique[idx].PosterURL == "" && item.PosterURL != "" {
				unique[idx].PosterURL = item.PosterURL
			}
			if unique[idx].Rating == 0 && item.Rating > 0 {
				unique[idx].Rating = item.Rating
			}
			continue
		}
		seen[key] = len(unique)
		unique = append(unique, item)
	}

	sort.SliceStable(unique, func(i, j int) bool {
		a := unique[i]
		b := unique[j]

		// 1. Пріоритет за типом контенту (якщо вказано)
		if targetType != "" {
			aMatch := a.Type == targetType
			bMatch := b.Type == targetType
			if aMatch != bMatch {
				return aMatch
			}
		}

		// 2. Пріоритет за роком (найновіші першими)
		if a.Year != b.Year {
			return a.Year > b.Year
		}

		// 3. За наявністю постера
		aHasPoster := a.PosterURL != ""
		bHasPoster := b.PosterURL != ""
		if aHasPoster != bHasPoster {
			return aHasPoster
		}

		return false
	})

	return unique
}

// Search виконує пошук у Bandera Online
func (p *BanderaProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	items, err := p.client.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("bandera search (%s): %w", query, err)
	}
	return p.convertSearchItems(items), nil
}

// SearchWithMeta виконує пошук та повертає повну відповідь BanderaSearchResponse (з Items та Meta.Statuses)
func (p *BanderaProvider) SearchWithMeta(ctx context.Context, query string, year int, serial int) (*BanderaSearchResponse, error) {
	return p.client.SearchWithMeta(ctx, query, year, serial)
}

// GetPopular виконує пошук типових назв за типом контенту та правильно сортує результати
func (p *BanderaProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}

	var queries []string
	var serial int
	switch contentType {
	case "series":
		queries = []string{"серіал", "драма", "2024", "детектив", "комедія"}
		serial = 1
	case "anime":
		queries = []string{"аніме", "клинок", "магічна битва", "атака титанів", "соло"}
	case "movie":
		queries = []string{"фільм", "2024", "бойовик", "трилер", "комедія"}
		serial = 0
	default:
		queries = []string{"фільм", "серіал", "мультфільм", "2024", "2023"}
	}

	q := queries[(page-1)%len(queries)]
	var items []domain.MediaItem
	var err error

	if serial > 0 || contentType == "series" || contentType == "movie" {
		resp, sErr := p.SearchWithMeta(ctx, q, 0, serial)
		if sErr == nil && resp != nil && len(resp.Items) > 0 {
			items = p.convertSearchItems(resp.Items)
		} else {
			items, err = p.Search(ctx, q)
		}
	} else {
		items, err = p.Search(ctx, q)
	}

	if err != nil {
		return nil, err
	}

	return rankAndSortMediaItems(items, contentType), nil
}

// GetNew виконує пошук новинок поточного або останніх років із сортуванням
func (p *BanderaProvider) GetNew(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}
	currentYear := time.Now().Year()
	years := []int{currentYear, currentYear - 1, currentYear - 2}
	year := years[(page-1)%len(years)]

	var serial int
	if contentType == "series" {
		serial = 1
	}

	resp, err := p.SearchWithMeta(ctx, strconv.Itoa(year), year, serial)
	if err == nil && resp != nil && len(resp.Items) > 0 {
		items := p.convertSearchItems(resp.Items)
		return rankAndSortMediaItems(items, contentType), nil
	}

	return p.GetPopular(ctx, contentType, page)
}

// GetByCategory виконує пошук за назвою категорії із сортуванням
func (p *BanderaProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	if category == "" {
		return p.GetPopular(ctx, contentType, page)
	}
	var serial int
	if contentType == "series" {
		serial = 1
	}
	resp, err := p.SearchWithMeta(ctx, category, 0, serial)
	var items []domain.MediaItem
	if err == nil && resp != nil && len(resp.Items) > 0 {
		items = p.convertSearchItems(resp.Items)
	} else {
		items, err = p.Search(ctx, category)
		if err != nil {
			return nil, err
		}
	}
	return rankAndSortMediaItems(items, contentType), nil
}

// GetDetails розбирає карточку контенту, формує сезони, серії та озвучки
func (p *BanderaProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	var payload BanderaItemPayload
	if err := json.Unmarshal([]byte(itemURL), &payload); err != nil {
		return nil, fmt.Errorf("invalid item url payload: %w", err)
	}

	contentResp, err := p.client.GetContent(ctx, payload.Source, payload.Ref)
	if err != nil {
		return nil, fmt.Errorf("bandera get content: %w", err)
	}

	title := payload.Title
	originalTitle := ""
	description := ""
	posterURL := payload.Poster
	year := payload.Year
	duration := ""
	trailerURL := ""
	var genres []string
	var rating float64

	if contentResp.Info != nil {
		if contentResp.Info.Title.String() != "" {
			title = contentResp.Info.Title.String()
		}
		if contentResp.Info.TitleEn.String() != "" {
			originalTitle = contentResp.Info.TitleEn.String()
		}
		if contentResp.Info.Description.String() != "" {
			description = contentResp.Info.Description.String()
		}
		if contentResp.Info.Image.String() != "" {
			posterURL = contentResp.Info.Image.String()
		} else if contentResp.Info.Poster.String() != "" {
			posterURL = contentResp.Info.Poster.String()
		}
		if y := ParseFlexibleYear(contentResp.Info.Year); y > 0 {
			year = y
		} else if contentResp.Info.ReleaseDate.String() != "" {
			rd := contentResp.Info.ReleaseDate.String()
			if len(rd) >= 4 {
				if parsedY, err := strconv.Atoi(rd[:4]); err == nil && parsedY >= 1900 && parsedY <= 2100 {
					year = parsedY
				}
			}
		}
		duration = contentResp.Info.EpisodeTime.String()
		if duration == "" {
			duration = contentResp.Info.Duration.String()
		}
		trailerURL = contentResp.Info.Trailer.String()
		genres = contentResp.Info.Genres
		rating = contentResp.Info.Rating.Float64()
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

	buildSeasonsForVoice := func(v BanderaVoice) []domain.Season {
		var voiceSeasons []domain.Season
		if len(v.Seasons) > 0 {
			for sIdx, s := range v.Seasons {
				sNum := ParseSeasonNumber(s.Title, sIdx+1)
				var episodes []domain.Episode
				for _, ep := range s.Episodes {
					epTitle := ep.Title.String()
					if epTitle == "" {
						epTitle = fmt.Sprintf("Серія %d", ep.Number.Int())
					}
					streamRefBytes, _ := json.Marshal(BanderaStreamRef{
						Source:      payload.Source,
						Ref:         ep.Ref,
						IsStreamRef: true,
					})
					episodes = append(episodes, domain.Episode{
						Number: ep.Number.Int(),
						Title:  epTitle,
						URL:    string(streamRefBytes),
					})
				}
				voiceSeasons = append(voiceSeasons, domain.Season{
					Number:   sNum,
					Title:    fmt.Sprintf("Сезон %d", sNum),
					Episodes: episodes,
				})
			}
		} else if len(v.Episodes) > 0 {
			var episodes []domain.Episode
			for _, ep := range v.Episodes {
				epTitle := ep.Title.String()
				if epTitle == "" {
					epTitle = fmt.Sprintf("Серія %d", ep.Number.Int())
				}
				streamRefBytes, _ := json.Marshal(BanderaStreamRef{
					Source:      payload.Source,
					Ref:         ep.Ref,
					IsStreamRef: true,
				})
				episodes = append(episodes, domain.Episode{
					Number: ep.Number.Int(),
					Title:  epTitle,
					URL:    string(streamRefBytes),
				})
			}
			voiceSeasons = append(voiceSeasons, domain.Season{
				Number:   1,
				Title:    "Сезон 1",
				Episodes: episodes,
			})
		}
		return voiceSeasons
	}

	// Якщо є голоси/озвучки (серіали або багатоваріантний дубляж)
	if len(contentResp.Voices) > 0 {
		for _, v := range contentResp.Voices {
			vName := v.DisplayName.String()
			if vName == "" {
				vName = v.ID.String()
			}
			vSeasons := buildSeasonsForVoice(v)
			voiceovers = append(voiceovers, domain.Voiceover{
				ID:      v.ID.String(),
				Name:    vName,
				Seasons: vSeasons,
			})
		}

		// За замовчуванням seasons беруться з першої доступної озвучки
		if len(voiceovers) > 0 {
			seasons = voiceovers[0].Seasons
		}
	} else if len(contentResp.Seasons) > 0 || len(contentResp.Episodes) > 0 {
		// Серіал без розбивки по голосах (напряму сезони/епізоди)
		vSeasons := buildSeasonsForVoice(BanderaVoice{
			ID:          "default",
			DisplayName: "Основна",
			Seasons:     contentResp.Seasons,
			Episodes:    contentResp.Episodes,
		})
		voiceovers = append(voiceovers, domain.Voiceover{
			ID:      "default",
			Name:    "Основна",
			Seasons: vSeasons,
		})
		seasons = vSeasons
	} else if len(contentResp.Streams) > 0 {
		// Для фільмів зі списком стрімів
		for i, st := range contentResp.Streams {
			vName := st.Title.String()
			if vName == "" {
				vName = fmt.Sprintf("Джерело %d", i+1)
			}
			voiceovers = append(voiceovers, domain.Voiceover{
				ID:   strconv.Itoa(i),
				Name: vName,
			})
		}
	}

	stableID := payload.ID
	if stableID == "" {
		stableID = generateStableContentID(payload.Source, title, year, payload.Ref)
	}

	return &domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:            stableID,
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
	}, nil
}

// GetStreams повертає стріми для відтворення.
// Динамічно визначає, чи передано готовий StreamRef чи карточку контенту.
func (p *BanderaProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	var targetSource string
	var targetStreamRef json.RawMessage
	var itemSubtitles []BanderaSubtitleItem

	// 1. Отримуємо метадані джерел (inputs.stream) з кешу /sources
	sourcesMeta, _ := p.client.GetSources(ctx)

	// 2. Спробуємо розпарсити як прямий BanderaStreamRef
	var directRef BanderaStreamRef
	var itemPayload BanderaItemPayload

	isDirectStream := false
	if err := json.Unmarshal([]byte(itemURL), &directRef); err == nil && directRef.IsStreamRef {
		isDirectStream = true
		targetSource = directRef.Source
		targetStreamRef = directRef.Ref
	} else if err := json.Unmarshal([]byte(itemURL), &directRef); err == nil && len(directRef.Ref) > 0 && directRef.Source != "" {
		// Перевіримо через inputs.stream джерела, чи це посилання на стрім!
		meta := sourcesMeta[directRef.Source]
		if IsStreamRef(meta, directRef.Ref) {
			isDirectStream = true
			targetSource = directRef.Source
			targetStreamRef = directRef.Ref
		}
	}

	// 3. Якщо це не прямий stream ref — значить це карточка контенту (фільм/серіал), робимо /content
	if !isDirectStream {
		if err := json.Unmarshal([]byte(itemURL), &itemPayload); err != nil {
			return nil, fmt.Errorf("invalid item url: %w", err)
		}
		targetSource = itemPayload.Source

		contentResp, err := p.client.GetContent(ctx, itemPayload.Source, itemPayload.Ref)
		if err != nil {
			return nil, fmt.Errorf("content request for stream: %w", err)
		}

		if len(contentResp.Streams) > 0 {
			// Якщо це фільм
			streamIdx := 0
			if voiceID != "" {
				if idx, err := strconv.Atoi(voiceID); err == nil && idx >= 0 && idx < len(contentResp.Streams) {
					streamIdx = idx
				}
			}
			st := contentResp.Streams[streamIdx]

			// Якщо потік уже містить прямий URL (без потреби виклику /stream)
			if len(st.Ref) == 0 && st.URL.String() != "" {
				var directStreams []domain.StreamSource
				var allSubs []BanderaSubtitleItem
				for i, sItem := range contentResp.Streams {
					if voiceID != "" {
						if idx, err := strconv.Atoi(voiceID); err == nil && idx != i {
							continue
						}
					}
					if sItem.URL.String() == "" {
						continue
					}
					cleanURL, parsedQuality := ParsePackedStreamURL(sItem.URL.String())
					quality := sItem.Quality.String()
					if quality == "" || quality == "auto" {
						quality = parsedQuality
					}
					vName := sItem.Title.String()
					if vName == "" {
						vName = fmt.Sprintf("Джерело %d", i+1)
					}
					playableURL, directURL, requiresProxy := WrapStreamURL(targetSource, "inner", cleanURL)
					headers := BuildStreamHeaders(playableURL, requiresProxy)
					directStreams = append(directStreams, domain.StreamSource{
						Quality:       quality,
						URL:           playableURL,
						DirectURL:     directURL,
						RequiresProxy: requiresProxy,
						Headers:       headers,
						Player:        targetSource,
						Voiceover:     vName,
					})
					allSubs = append(allSubs, sItem.Subtitles...)
				}
				if len(directStreams) > 0 {
					return &domain.ContentStreamsResponse{
						ProviderID: p.ID(),
						Streams:    directStreams,
						Subtitles:  MergeSubtitles(allSubs, nil),
					}, nil
				}
			}

			targetStreamRef = st.Ref
			itemSubtitles = st.Subtitles
		} else if len(contentResp.Voices) > 0 || len(contentResp.Seasons) > 0 || len(contentResp.Episodes) > 0 {
			// Якщо це серіал
			voices := contentResp.Voices
			if len(voices) == 0 {
				voices = []BanderaVoice{{
					ID:          "default",
					DisplayName: "Основна",
					Seasons:     contentResp.Seasons,
					Episodes:    contentResp.Episodes,
				}}
			}
			ref, subs, found := SelectEpisodeRef(voices, season, episode, voiceID)
			if !found {
				return nil, fmt.Errorf("no matching stream found for season %d episode %d voice %s", season, episode, voiceID)
			}
			targetStreamRef = ref
			itemSubtitles = subs
		} else {
			return nil, fmt.Errorf("no streams or voices found in content response")
		}
	}

	// 4. Валідація: чи містить targetStreamRef хоч один ключ із streamKeys відповідного джерела.
	// Це запобігає надсиланню невалідного запиту в мережу та поверненню 400 MISSING_URL.
	meta := sourcesMeta[targetSource]
	if err := ValidateStreamRef(meta, targetStreamRef); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnresolvablePlayer, err)
	}

	// 5. Запитуємо /stream (ніколи не мемоізується, бо URL можуть бути підписаними)
	streamResp, err := p.client.GetStream(ctx, targetSource, targetStreamRef)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	if !streamResp.OK && streamResp.Error.String() != "" {
		return nil, fmt.Errorf("stream api error (%s): %s", streamResp.ErrorCode, streamResp.Error)
	}

	// 6. Формуємо стріми з урахуванням правил proxy та заголовків
	var streams []domain.StreamSource
	streamVoiceover := targetSource
	if voiceID != "" {
		streamVoiceover = voiceID
	}
	for _, s := range streamResp.Streams {
		rawURL := s.URL.String()
		if rawURL == "" {
			continue
		}

		cleanURL, parsedQuality := ParsePackedStreamURL(rawURL)
		quality := s.Quality.String()
		if quality == "" || quality == "auto" {
			quality = parsedQuality
		}

		playableURL, directURL, requiresProxy := WrapStreamURL(targetSource, "inner", cleanURL)
		headers := BuildStreamHeaders(playableURL, requiresProxy)

		streams = append(streams, domain.StreamSource{
			Quality:       quality,
			URL:           playableURL,
			DirectURL:     directURL,
			RequiresProxy: requiresProxy,
			Headers:       headers,
			Player:        targetSource,
			Voiceover:     streamVoiceover,
		})
	}

	// 7. Об'єднуємо субтитри
	subtitles := MergeSubtitles(streamResp.Subtitles, itemSubtitles)

	if len(streams) == 0 {
		return nil, errors.New("no playable streams returned by provider")
	}

	return &domain.ContentStreamsResponse{
		ProviderID: p.ID(),
		Streams:    streams,
		Subtitles:  subtitles,
	}, nil
}
