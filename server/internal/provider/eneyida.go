package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

type EneyidaProvider struct {
	client  *TLSClient
	baseURL string
}

func NewEneyidaProvider(client *TLSClient) *EneyidaProvider {
	return &EneyidaProvider{
		client:  client,
		baseURL: "https://eneyida.tv",
	}
}

func (p *EneyidaProvider) ID() string {
	return "eneyida"
}

func (p *EneyidaProvider) Name() string {
	return "Eneyida"
}

func (p *EneyidaProvider) BaseURL() string {
	return p.baseURL
}

// Describe повертає публічні метадані для каталогу провайдерів.
func (p *EneyidaProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID:                   p.ID(),
		Name:                 p.Name(),
		BaseURL:              p.baseURL,
		ShowOnHome:           true,
		HasFixedStreams:      false,
		ContentTypes:         []string{"movie", "series", "cartoon", "anime"},
		SearchEnabledDefault: true,
	}
}

func (p *EneyidaProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	searchURL := fmt.Sprintf("%s/index.php?do=search&subaction=search&story=%s", p.baseURL, url.QueryEscape(query))
	html, err := p.client.Get(ctx, searchURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida search error: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse eneyida search: %w", err)
	}

	var items []domain.MediaItem
	doc.Find("article.short, .short-story").Each(func(i int, s *goquery.Selection) {
		linkElem := s.Find("h2.short_title a, .short_title a, a.short_btn")
		title := strings.TrimSpace(linkElem.Text())
		href, exists := linkElem.Attr("href")
		if !exists || title == "" {
			return
		}

		imgElem := s.Find(".short_img img")
		poster, _ := imgElem.Attr("src")
		if poster != "" && !strings.HasPrefix(poster, "http") {
			poster = p.baseURL + poster
		}

		items = append(items, domain.MediaItem{
			ID:         href,
			ProviderID: p.ID(),
			Title:      title,
			PosterURL:  poster,
			URL:        href,
			Type:       "movie",
		})
	})

	return items, nil
}

func (p *EneyidaProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida get details: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse details html: %w", err)
	}

	title := strings.TrimSpace(doc.Find("h1.full-title").First().Text())
	poster, _ := doc.Find(".full-poster img").Attr("src")
	if poster != "" && !strings.HasPrefix(poster, "http") {
		poster = p.baseURL + poster
	}
	desc := strings.TrimSpace(doc.Find(".full-text").First().Text())

	return &domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:         itemURL,
			ProviderID: p.ID(),
			Title:      title,
			PosterURL:  poster,
			URL:        itemURL,
			Type:       "movie",
		},
		Description: desc,
	}, nil
}

func (p *EneyidaProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida get streams: %w", err)
	}

	iframeRe := regexp.MustCompile(`<iframe[^>]+src=["']([^"']+)["']`)
	matches := iframeRe.FindStringSubmatch(html)

	var streams []domain.StreamSource
	if len(matches) > 1 {
		iframeURL := matches[1]
		if strings.HasPrefix(iframeURL, "//") {
			iframeURL = "https:" + iframeURL
		}

		streams = append(streams, domain.StreamSource{
			Quality:       "Auto",
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
