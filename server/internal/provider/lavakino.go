package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

type LavakinoProvider struct {
	client  *TLSClient
	baseURL string
}

func NewLavakinoProvider(client *TLSClient) *LavakinoProvider {
	return &LavakinoProvider{
		client:  client,
		baseURL: "https://lavakino.net",
	}
}

func (p *LavakinoProvider) ID() string {
	return "lavakino"
}

func (p *LavakinoProvider) Name() string {
	return "Lavakino"
}

func (p *LavakinoProvider) BaseURL() string {
	return p.baseURL
}

// Describe повертає публічні метадані для каталогу провайдерів.
func (p *LavakinoProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID:                   p.ID(),
		Name:                 p.Name(),
		BaseURL:              p.baseURL,
		IconURL:              p.baseURL + "/favicon.ico",
		ShowOnHome:           true,
		HasFixedStreams:      false,
		ContentTypes:         []string{"movie", "series", "cartoon", "anime"},
		SearchEnabledDefault: true,
	}
}

// Search виконує пошук через DLE form POST на lavakino
func (p *LavakinoProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	searchURL := fmt.Sprintf("%s/index.php?do=search", p.baseURL)
	formData := fmt.Sprintf("do=search&subaction=search&story=%s", url.QueryEscape(query))

	html, err := p.client.PostForm(ctx, searchURL, formData, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("lavakino search request: %w", err)
	}

	return p.parseCatalogHtml(html)
}

func (p *LavakinoProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}
	section := p.getSection(contentType)
	var reqURL string
	if section == "" {
		if page == 1 {
			reqURL = p.baseURL + "/"
		} else {
			reqURL = fmt.Sprintf("%s/page/%d/", p.baseURL, page)
		}
	} else {
		if page == 1 {
			reqURL = fmt.Sprintf("%s/%s/", p.baseURL, section)
		} else {
			reqURL = fmt.Sprintf("%s/%s/page/%d/", p.baseURL, section, page)
		}
	}
	html, err := p.client.Get(ctx, reqURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("lavakino get popular: %w", err)
	}
	return p.parseCatalogHtml(html)
}

func (p *LavakinoProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}
	if category == "" {
		return p.GetPopular(ctx, contentType, page)
	}
	encoded := url.PathEscape(category)
	var reqURL string
	if page == 1 {
		reqURL = fmt.Sprintf("%s/f/%s/", p.baseURL, encoded)
	} else {
		reqURL = fmt.Sprintf("%s/f/%s/page/%d/", p.baseURL, encoded, page)
	}
	html, err := p.client.Get(ctx, reqURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("lavakino get category: %w", err)
	}
	return p.parseCatalogHtml(html)
}

func (p *LavakinoProvider) getSection(contentType string) string {
	switch contentType {
	case "movie":
		return "film"
	case "series":
		return "serial"
	case "cartoon":
		return "mult"
	case "anime":
		return "anime"
	default:
		return ""
	}
}

func (p *LavakinoProvider) parseCatalogHtml(html string) ([]domain.MediaItem, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse catalog html: %w", err)
	}

	var items []domain.MediaItem
	doc.Find("div.short, .short-story").Each(func(i int, s *goquery.Selection) {
		linkElem := s.Find("a.short-title, h2.title a, .short-text a").First()
		title := strings.TrimSpace(linkElem.Text())
		href, exists := linkElem.Attr("href")
		if !exists || title == "" {
			return
		}

		imgElem := s.Find(".short-img img, img").First()
		poster, _ := imgElem.Attr("src")
		if poster == "" {
			poster, _ = imgElem.Attr("data-src")
		}
		if poster != "" && !strings.HasPrefix(poster, "http") {
			poster = p.baseURL + poster
		}

		// Year & Rating
		yearStr := s.Find(".sd-line").Text()
		year := parseYear(yearStr)
		if year == 0 {
			year = parseYear(s.Text())
		}

		var rating float64
		ratingStr := s.Find(".m-imdb").Text()
		if ratingStr != "" {
			ratingStr = strings.ReplaceAll(strings.TrimSpace(ratingStr), ",", ".")
			if r, err := strconv.ParseFloat(ratingStr, 64); err == nil {
				rating = r
			}
		}

		mediaType := "movie"
		lowerHref := strings.ToLower(href)
		if strings.Contains(lowerHref, "serial") {
			mediaType = "series"
		} else if strings.Contains(lowerHref, "mult") {
			mediaType = "cartoon"
		} else if strings.Contains(lowerHref, "anime") {
			mediaType = "anime"
		}

		items = append(items, domain.MediaItem{
			ID:         href,
			ProviderID: p.ID(),
			Title:      title,
			PosterURL:  poster,
			Year:       year,
			Rating:     rating,
			Type:       mediaType,
			URL:        href,
		})
	})

	return items, nil
}

// GetDetails розбирає сторінку опису та метадані фільму/серіалу
func (p *LavakinoProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("lavakino get details: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse details html: %w", err)
	}

	title := strings.TrimSpace(doc.Find("h1").First().Text())
	poster, _ := doc.Find(".fposter img, .short-img img").First().Attr("src")
	if poster == "" {
		poster, _ = doc.Find(".fposter img, .short-img img").First().Attr("data-src")
	}
	if poster != "" && !strings.HasPrefix(poster, "http") {
		poster = p.baseURL + poster
	}

	desc := strings.TrimSpace(doc.Find(".fdesc, .full-text").First().Text())
	origTitle := strings.TrimSpace(doc.Find("[itemprop='alternateName']").First().Text())

	var genres []string
	genreText := doc.Find("[itemprop='genre']").First().Text()
	if genreText != "" {
		for _, g := range strings.Split(genreText, ",") {
			trimmed := strings.TrimSpace(g)
			if trimmed != "" {
				genres = append(genres, trimmed)
			}
		}
	}

	var countries []string
	countryText := doc.Find("[itemprop='countryOfOrigin']").First().Text()
	if countryText != "" {
		for _, c := range strings.Split(countryText, ",") {
			trimmed := strings.TrimSpace(c)
			if trimmed != "" {
				countries = append(countries, trimmed)
			}
		}
	}

	var actors []string
	actorText := doc.Find("[itemprop='actor']").First().Text()
	if actorText != "" {
		for _, a := range strings.Split(actorText, ",") {
			trimmed := strings.TrimSpace(a)
			if trimmed != "" {
				actors = append(actors, trimmed)
			}
		}
	}

	director := strings.TrimSpace(doc.Find("[itemprop='director']").First().Text())
	var duration string

	// Fallback via .sd-line
	doc.Find(".sd-line").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		lower := strings.ToLower(text)
		if len(genres) == 0 && strings.Contains(lower, "жанр:") {
			raw := strings.TrimSpace(strings.TrimPrefix(text, "Жанр:"))
			for _, g := range strings.Split(raw, ",") {
				if t := strings.TrimSpace(g); t != "" {
					genres = append(genres, t)
				}
			}
		} else if len(countries) == 0 && strings.Contains(lower, "країна:") {
			raw := strings.TrimSpace(strings.TrimPrefix(text, "Країна:"))
			for _, c := range strings.Split(raw, ",") {
				if t := strings.TrimSpace(c); t != "" {
					countries = append(countries, t)
				}
			}
		} else if director == "" && strings.Contains(lower, "режисер:") {
			director = strings.TrimSpace(strings.TrimPrefix(text, "Режисер:"))
		} else if len(actors) == 0 && strings.Contains(lower, "актори:") {
			raw := strings.TrimSpace(strings.TrimPrefix(text, "Актори:"))
			for _, a := range strings.Split(raw, ",") {
				if t := strings.TrimSpace(a); t != "" {
					actors = append(actors, t)
				}
			}
		} else if duration == "" && (strings.Contains(lower, "тривалість:") || strings.Contains(lower, "час:")) {
			duration = strings.TrimSpace(strings.TrimPrefix(text, "Тривалість:"))
		}
	})

	yearStr := doc.Find("[itemprop='copyrightYear']").First().Text()
	year := parseYear(yearStr)
	if year == 0 {
		year = parseYear(doc.Find(".sd-line").Text())
	}

	var rating float64
	ratingStr := doc.Find(".m-imdb").First().Text()
	if ratingStr != "" {
		ratingStr = strings.ReplaceAll(strings.TrimSpace(ratingStr), ",", ".")
		if r, err := strconv.ParseFloat(ratingStr, 64); err == nil {
			rating = r
		}
	}

	mediaType := "movie"
	lowerHref := strings.ToLower(itemURL)
	if strings.Contains(lowerHref, "serial") {
		mediaType = "series"
	} else if strings.Contains(lowerHref, "mult") {
		mediaType = "cartoon"
	} else if strings.Contains(lowerHref, "anime") {
		mediaType = "anime"
	}

	return &domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:            itemURL,
			ProviderID:    p.ID(),
			Title:         title,
			OriginalTitle: origTitle,
			PosterURL:     poster,
			Year:          year,
			Rating:        rating,
			Type:          mediaType,
			URL:           itemURL,
		},
		Description: desc,
		Genres:      genres,
		Countries:   countries,
		Director:    director,
		Actors:      actors,
		Duration:    duration,
	}, nil
}

// GetStreams знаходить плеєри (hdvbua, ashdi, zenith) та розбирає прямі стріми
func (p *LavakinoProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("lavakino get streams html: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse stream html: %w", err)
	}

	var streams []domain.StreamSource
	reFile := regexp.MustCompile(`file\s*:\s*["']([^"']+)["']`)

	doc.Find("iframe").Each(func(i int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists || src == "" || strings.Contains(src, "trailer") || strings.Contains(src, "youtube") {
			return
		}
		if strings.HasPrefix(src, "//") {
			src = "https:" + src
		} else if !strings.HasPrefix(src, "http") {
			src = p.baseURL + src
		}

		// Спробуємо отримати сторінку плеєра для розбору прямих потоків (PlayerJS / m3u8)
		frameHTML, err := p.client.Get(ctx, src, p.baseURL+"/")
		if err == nil {
			fileMatch := reFile.FindStringSubmatch(frameHTML)
			if len(fileMatch) > 1 {
				fileURL := fileMatch[1]
				if strings.Contains(fileURL, ".m3u8") || strings.Contains(fileURL, ".mp4") {
					streams = append(streams, domain.StreamSource{
						Quality:       "Auto / 1080p",
						URL:           fileURL,
						DirectURL:     fileURL,
						RequiresProxy: false,
						Headers: map[string]string{
							"Referer":    src,
							"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
						},
					})
					return
				}
			}
		}

		// Резервний варіант: посилання на сам iframe плеєр
		streams = append(streams, domain.StreamSource{
			Quality:       fmt.Sprintf("Плеєр %d", len(streams)+1),
			URL:           src,
			DirectURL:     src,
			RequiresProxy: false,
			Headers: map[string]string{
				"Referer":    p.baseURL + "/",
				"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			},
		})
	})

	return &domain.ContentStreamsResponse{
		ProviderID: p.ID(),
		Streams:    streams,
	}, nil
}
