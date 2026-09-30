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
	items, err := p.fetchCatalog(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida search error: %w", err)
	}
	return items, nil
}

func (p *EneyidaProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
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

func (p *EneyidaProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
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

func (p *EneyidaProvider) getSection(contentType string) string {
	switch contentType {
	case "movie":
		return "films"
	case "series":
		return "serials"
	case "cartoon":
		return "multfilmy"
	case "anime":
		return "anime"
	default:
		return ""
	}
}

func (p *EneyidaProvider) fetchCatalog(ctx context.Context, reqURL string) ([]domain.MediaItem, error) {
	html, err := p.client.Get(ctx, reqURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("eneyida fetch catalog (%s): %w", reqURL, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse eneyida catalog: %w", err)
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
		if poster == "" {
			poster, _ = imgElem.Attr("data-src")
		}
		if poster != "" && !strings.HasPrefix(poster, "http") {
			poster = p.baseURL + poster
		}

		year := parseYear(s.Find(".short_info, .short-info").Text())
		if year == 0 {
			year = parseYear(s.Text())
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
			URL:        href,
			Type:       mediaType,
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

	title := strings.TrimSpace(doc.Find("h1.full-title, h1").First().Text())
	origTitle := strings.TrimSpace(doc.Find(".full_orig-title, [itemprop='alternateName']").First().Text())
	if origTitle == "" {
		origTitle, _ = doc.Find("meta[itemprop='alternateName']").Attr("content")
		origTitle = strings.TrimSpace(origTitle)
	}

	poster, _ := doc.Find(".full-poster img, .full_poster img").Attr("src")
	if poster == "" {
		poster, _ = doc.Find(".full-poster img, .full_poster img").Attr("data-src")
	}
	if poster == "" {
		poster, _ = doc.Find("img[src*='/uploads/posts/']").First().Attr("src")
	}
	if poster != "" && !strings.HasPrefix(poster, "http") {
		poster = p.baseURL + poster
	}
	desc := strings.TrimSpace(doc.Find(".full-text").First().Text())

	var genres []string
	var countries []string
	var actors []string
	var director string
	var duration string
	var year int

	// Schema.org metas
	if g, ok := doc.Find("meta[itemprop='genre']").Attr("content"); ok {
		for _, part := range strings.Split(g, ",") {
			if t := strings.TrimSpace(part); t != "" {
				genres = append(genres, t)
			}
		}
	}
	if c, ok := doc.Find("meta[itemprop='contentLocation'], meta[itemprop='countryOfOrigin']").Attr("content"); ok {
		for _, part := range strings.Split(c, ",") {
			if t := strings.TrimSpace(part); t != "" {
				countries = append(countries, t)
			}
		}
	}
	if a, ok := doc.Find("meta[itemprop='actors']").Attr("content"); ok {
		for _, part := range strings.Split(a, ",") {
			if t := strings.TrimSpace(part); t != "" {
				actors = append(actors, t)
			}
		}
	}
	if d, ok := doc.Find("meta[itemprop='director']").Attr("content"); ok {
		director = strings.TrimSpace(d)
	}
	if yrStr, ok := doc.Find("meta[itemprop='dateCreated']").Attr("content"); ok {
		year = parseYear(yrStr)
	}

	// .full_info-item blocks
	doc.Find(".full_info-item, .fi-row").Each(func(i int, s *goquery.Selection) {
		label := strings.ToLower(strings.TrimSpace(s.Find(".fi-label, span:first-child").Text()))
		valSel := s.Find(".fi-value, span:last-child")

		if strings.Contains(label, "режис") && director == "" {
			director = strings.TrimSpace(valSel.Text())
		} else if strings.Contains(label, "актор") && len(actors) == 0 {
			valSel.Find("a").Each(func(j int, a *goquery.Selection) {
				act := strings.TrimSpace(a.Text())
				if act != "" {
					actors = append(actors, act)
				}
			})
		} else if strings.Contains(label, "жанр") && len(genres) == 0 {
			valSel.Find("a").Each(func(j int, a *goquery.Selection) {
				g := strings.TrimSpace(a.Text())
				if g != "" {
					genres = append(genres, g)
				}
			})
		} else if strings.Contains(label, "країн") && len(countries) == 0 {
			valSel.Find("a").Each(func(j int, a *goquery.Selection) {
				c := strings.TrimSpace(a.Text())
				if c != "" {
					countries = append(countries, c)
				}
			})
		} else if strings.Contains(label, "рік") && year == 0 {
			year = parseYear(valSel.Text())
		} else if strings.Contains(label, "трива") && duration == "" {
			duration = strings.TrimSpace(valSel.Text())
		}
	})

	if year == 0 {
		year = parseYear(doc.Find(".full_info, .full-info").Text())
	}

	var rating float64
	ratingText := doc.Find(".rating, .imdb-rate, .full-rates").Text()
	reRating := regexp.MustCompile(`([\d.]+)`)
	m := reRating.FindString(ratingText)
	if m != "" {
		if r, err := strconv.ParseFloat(m, 64); err == nil {
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
