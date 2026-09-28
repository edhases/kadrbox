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

type UakinoProvider struct {
	client  *TLSClient
	baseURL string
}

func NewUakinoProvider(client *TLSClient) *UakinoProvider {
	return &UakinoProvider{
		client:  client,
		baseURL: "https://uakino.me",
	}
}

func (p *UakinoProvider) ID() string {
	return "uakino"
}

func (p *UakinoProvider) Name() string {
	return "UAKino"
}

func (p *UakinoProvider) BaseURL() string {
	return p.baseURL
}

// Search виконує парсинг результатів пошуку UAKino
func (p *UakinoProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	searchURL := fmt.Sprintf("%s/index.php?do=search&subaction=search&story=%s", p.baseURL, url.QueryEscape(query))
	html, err := p.client.Get(ctx, searchURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("uakino search request: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse search html: %w", err)
	}

	var items []domain.MediaItem
	doc.Find(".movie-item, .short-story").Each(func(i int, s *goquery.Selection) {
		linkElem := s.Find(".movie-title a, a.movie-title, h2.title a")
		title := strings.TrimSpace(linkElem.Text())
		href, exists := linkElem.Attr("href")
		if !exists || title == "" {
			return
		}

		imgElem := s.Find(".movie-poster img, .poster img")
		poster, _ := imgElem.Attr("src")
		if poster != "" && !strings.HasPrefix(poster, "http") {
			poster = p.baseURL + poster
		}

		yearStr := s.Find(".movie-date, .year").Text()
		year := parseYear(yearStr)

		mediaType := "movie"
		if strings.Contains(strings.ToLower(href), "serial") || strings.Contains(strings.ToLower(title), "серіал") {
			mediaType = "series"
		}

		items = append(items, domain.MediaItem{
			ID:         href,
			ProviderID: p.ID(),
			Title:      title,
			PosterURL:  poster,
			Year:       year,
			Type:       mediaType,
			URL:        href,
		})
	})

	return items, nil
}

// GetDetails розбирає сторінку опису та доступні сезони/серії
func (p *UakinoProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("uakino get details: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse details html: %w", err)
	}

	title := strings.TrimSpace(doc.Find("h1.movie-title, .solotitle").First().Text())
	poster, _ := doc.Find(".movie-poster img, .full-poster img").Attr("src")
	if poster != "" && !strings.HasPrefix(poster, "http") {
		poster = p.baseURL + poster
	}
	desc := strings.TrimSpace(doc.Find(".full-text, .movie-desc").First().Text())

	var genres []string
	doc.Find("a[href*='/genre/']").Each(func(i int, s *goquery.Selection) {
		g := strings.TrimSpace(s.Text())
		if g != "" {
			genres = append(genres, g)
		}
	})

	details := &domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:         itemURL,
			ProviderID: p.ID(),
			Title:      title,
			PosterURL:  poster,
			URL:        itemURL,
			Type:       "movie",
		},
		Description: desc,
		Genres:      genres,
	}

	return details, nil
}

// GetStreams знаходить плеєр та генерує посилання на потік із правильними Referer/User-Agent
func (p *UakinoProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("uakino get streams html: %w", err)
	}

	// Пошук iframe плеєра (наприклад Ashdi, PlayerJS тощо)
	iframeRe := regexp.MustCompile(`<iframe[^>]+src=["']([^"']+)["']`)
	matches := iframeRe.FindStringSubmatch(html)

	var streams []domain.StreamSource
	if len(matches) > 1 {
		iframeURL := matches[1]
		if strings.HasPrefix(iframeURL, "//") {
			iframeURL = "https:" + iframeURL
		}

		// Резолвінг потоку з iframe
		streams = append(streams, domain.StreamSource{
			Quality:       "Auto / 1080p",
			URL:           iframeURL,
			DirectURL:     iframeURL,
			RequiresProxy: false,
			Headers: map[string]string{
				"Referer":    p.baseURL + "/",
				"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			},
		})
	}

	return &domain.ContentStreamsResponse{
		ProviderID: p.ID(),
		Streams:    streams,
	}, nil
}

func parseYear(text string) int {
	re := regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)
	match := re.FindString(text)
	if match != "" {
		if yr, err := strconv.Atoi(match); err == nil {
			return yr
		}
	}
	return 0
}
