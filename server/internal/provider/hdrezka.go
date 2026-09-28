package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/edhases/oxide-server/internal/domain"
)

type HdrezkaProvider struct {
	client  *TLSClient
	baseURL string
}

func NewHdrezkaProvider(client *TLSClient) *HdrezkaProvider {
	return &HdrezkaProvider{
		client:  client,
		baseURL: "https://hdrezka.me",
	}
}

func (p *HdrezkaProvider) ID() string {
	return "hdrezka"
}

func (p *HdrezkaProvider) Name() string {
	return "HDRezka"
}

func (p *HdrezkaProvider) BaseURL() string {
	return p.baseURL
}

func (p *HdrezkaProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	searchURL := fmt.Sprintf("%s/search/?do=search&subaction=search&q=%s", p.baseURL, url.QueryEscape(query))
	html, err := p.client.Get(ctx, searchURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("hdrezka search request: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse search html: %w", err)
	}

	var items []domain.MediaItem
	doc.Find(".b-content__inline_item").Each(func(i int, s *goquery.Selection) {
		linkElem := s.Find(".b-content__inline_item-link a")
		title := strings.TrimSpace(linkElem.Text())
		href, exists := linkElem.Attr("href")
		if !exists || title == "" {
			return
		}

		imgElem := s.Find(".b-content__inline_item-cover img")
		poster, _ := imgElem.Attr("src")

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

func (p *HdrezkaProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("hdrezka get details: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse hdrezka details: %w", err)
	}

	title := strings.TrimSpace(doc.Find(".b-post__title h1").First().Text())
	poster, _ := doc.Find(".b-sidecover img").Attr("src")
	desc := strings.TrimSpace(doc.Find(".b-post__description_text").First().Text())

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

func (p *HdrezkaProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	// HDRezka віддає потоки або через CDN streams або через закодовані PlayerJS рядки
	headers := map[string]string{
		"Referer":    p.baseURL + "/",
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}

	return &domain.ContentStreamsResponse{
		ProviderID: p.ID(),
		Streams: []domain.StreamSource{
			{
				Quality:       "Auto / 1080p",
				URL:           itemURL,
				DirectURL:     itemURL,
				RequiresProxy: false,
				Headers:       headers,
			},
		},
	}, nil
}

// DecodeStreamURL деобфускує закодовані Base64 сміттєві рядки HDRezka CDN
func DecodeStreamURL(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "#h") {
		return encoded, nil
	}

	raw := encoded[2:] // Зрізаємо префікс #h
	trashList := []string{
		"$$#!!@#!@##", "_@#@_#@_###", "@@@@@!#!@#",
	}

	for _, trash := range trashList {
		raw = strings.ReplaceAll(raw, trash, "")
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}

	return string(decodedBytes), nil
}
