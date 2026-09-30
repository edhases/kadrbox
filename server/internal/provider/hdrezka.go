package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
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

// Describe повертає публічні метадані для каталогу провайдерів.
func (p *HdrezkaProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID:                   p.ID(),
		Name:                 p.Name(),
		BaseURL:              p.baseURL,
		ShowOnHome:           false,
		HasFixedStreams:      true,
		ContentTypes:         []string{"movie", "series", "cartoon", "animation"},
		SearchEnabledDefault: false,
	}
}

func (p *HdrezkaProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	searchURL := fmt.Sprintf("%s/search/?do=search&subaction=search&q=%s", p.baseURL, url.QueryEscape(query))
	items, err := p.fetchCatalog(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("hdrezka search request: %w", err)
	}
	return items, nil
}

func (p *HdrezkaProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
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
	return p.fetchCatalog(ctx, reqURL)
}

func (p *HdrezkaProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}
	if category == "" {
		return p.GetPopular(ctx, contentType, page)
	}
	encoded := url.PathEscape(category)
	var reqURL string
	if page == 1 {
		reqURL = fmt.Sprintf("%s/%s/", p.baseURL, encoded)
	} else {
		reqURL = fmt.Sprintf("%s/%s/page/%d/", p.baseURL, encoded, page)
	}
	return p.fetchCatalog(ctx, reqURL)
}

func (p *HdrezkaProvider) getSection(contentType string) string {
	switch contentType {
	case "movie":
		return "films"
	case "series":
		return "series"
	case "cartoon":
		return "cartoons"
	case "anime":
		return "animation"
	default:
		return ""
	}
}

func (p *HdrezkaProvider) fetchCatalog(ctx context.Context, reqURL string) ([]domain.MediaItem, error) {
	html, err := p.client.Get(ctx, reqURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("hdrezka fetch catalog (%s): %w", reqURL, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse hdrezka catalog: %w", err)
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
		if poster == "" {
			poster, _ = imgElem.Attr("data-src")
		}

		miscText := s.Find(".misc").Text()
		year := parseYear(miscText)
		if year == 0 {
			year = parseYear(s.Text())
		}

		mediaType := "movie"
		lowerHref := strings.ToLower(href)
		if strings.Contains(lowerHref, "series") || strings.Contains(lowerHref, "serial") {
			mediaType = "series"
		} else if strings.Contains(lowerHref, "cartoons") || strings.Contains(lowerHref, "animation") {
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
			URL:        href,
			Type:       mediaType,
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
	origTitle := strings.TrimSpace(doc.Find(".b-post__origtitle").First().Text())
	poster, _ := doc.Find(".b-sidecover img").Attr("src")
	desc := strings.TrimSpace(doc.Find(".b-post__description_text").First().Text())

	var genres []string
	var countries []string
	var actors []string
	var director string
	var duration string
	var year int

	doc.Find(".b-post__info tr").Each(func(i int, s *goquery.Selection) {
		label := strings.ToLower(strings.TrimSpace(s.Find("td.l").Text()))
		valTd := s.Find("td.r")

		if strings.Contains(label, "режисер") || strings.Contains(label, "режисс") {
			director = strings.TrimSpace(valTd.Text())
		} else if strings.Contains(label, "актор") || strings.Contains(label, "актер") || strings.Contains(label, "в ролях") {
			valTd.Find("a").Each(func(j int, a *goquery.Selection) {
				act := strings.TrimSpace(a.Text())
				if act != "" {
					actors = append(actors, act)
				}
			})
		} else if strings.Contains(label, "жанр") {
			valTd.Find("a").Each(func(j int, a *goquery.Selection) {
				g := strings.TrimSpace(a.Text())
				if g != "" {
					genres = append(genres, g)
				}
			})
		} else if strings.Contains(label, "країн") || strings.Contains(label, "стран") {
			valTd.Find("a").Each(func(j int, a *goquery.Selection) {
				c := strings.TrimSpace(a.Text())
				if c != "" {
					countries = append(countries, c)
				}
			})
		} else if strings.Contains(label, "рік") || strings.Contains(label, "год") || strings.Contains(label, "дата") {
			year = parseYear(valTd.Text())
		} else if strings.Contains(label, "час") || strings.Contains(label, "врем") {
			duration = strings.TrimSpace(valTd.Text())
		}
	})

	if year == 0 {
		year = parseYear(doc.Find(".b-post__info").Text())
	}

	var rating float64
	ratesText := doc.Find(".b-post__info-rates, .imdb .bold").Text()
	reRating := regexp.MustCompile(`([\d.]+)`)
	m := reRating.FindString(ratesText)
	if m != "" {
		if r, err := strconv.ParseFloat(m, 64); err == nil {
			rating = r
		}
	}

	mediaType := "movie"
	lowerHref := strings.ToLower(itemURL)
	if strings.Contains(lowerHref, "series") || strings.Contains(lowerHref, "serial") {
		mediaType = "series"
	} else if strings.Contains(lowerHref, "cartoons") || strings.Contains(lowerHref, "animation") {
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
