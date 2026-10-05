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
	return NewUakinoProviderWithConfig("https://uakino.biz", client)
}

func NewUakinoProviderWithConfig(baseURL string, client *TLSClient) *UakinoProvider {
	if baseURL == "" {
		baseURL = "https://uakino.biz"
	}
	if client == nil {
		client, _ = NewTLSClient()
	}
	return &UakinoProvider{
		client:  client,
		baseURL: baseURL,
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

// Describe повертає публічні метадані для каталогу провайдерів.
func (p *UakinoProvider) Describe() domain.ProviderInfo {
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

// Search виконує парсинг результатів пошуку UAKino
func (p *UakinoProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	searchURL := fmt.Sprintf("%s/index.php?do=search&subaction=search&story=%s", p.baseURL, url.QueryEscape(query))
	items, err := p.fetchCatalog(ctx, searchURL)
	if err != nil {
		return nil, fmt.Errorf("uakino search request: %w", err)
	}
	return items, nil
}

func (p *UakinoProvider) GetPopular(ctx context.Context, contentType string, page int) ([]domain.MediaItem, error) {
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

func (p *UakinoProvider) GetByCategory(ctx context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	if page < 1 {
		page = 1
	}
	if category == "" {
		return p.GetPopular(ctx, contentType, page)
	}
	encoded := url.PathEscape(category)
	section := p.getSection(contentType)
	var reqURL string
	if section == "" {
		if page == 1 {
			reqURL = fmt.Sprintf("%s/xfsearch/genre/%s/", p.baseURL, encoded)
		} else {
			reqURL = fmt.Sprintf("%s/xfsearch/genre/%s/page/%d/", p.baseURL, encoded, page)
		}
	} else {
		if page == 1 {
			reqURL = fmt.Sprintf("%s/%s/xfsearch/genre/%s/", p.baseURL, section, encoded)
		} else {
			reqURL = fmt.Sprintf("%s/%s/xfsearch/genre/%s/page/%d/", p.baseURL, section, encoded, page)
		}
	}
	return p.fetchCatalog(ctx, reqURL)
}

func (p *UakinoProvider) getSection(contentType string) string {
	switch contentType {
	case "movie":
		return "filmy"
	case "series":
		return "seriesss"
	case "cartoon":
		return "cartoon"
	case "anime":
		return "animeukr"
	default:
		return ""
	}
}

func (p *UakinoProvider) fetchCatalog(ctx context.Context, reqURL string) ([]domain.MediaItem, error) {
	html, err := p.client.Get(ctx, reqURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("uakino fetch catalog (%s): %w", reqURL, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse catalog html: %w", err)
	}

	// Initialised non-nil so an empty catalogue serialises as `[]`, not
	// `null`. json.Marshal turns a nil slice into `null`, which every client
	// then has to special-case.
	items := []domain.MediaItem{}
	doc.Find(".movie-item, .short-story").Each(func(i int, s *goquery.Selection) {
		linkElem := s.Find(".movie-title a, a.movie-title, h2.title a").First()
		title := strings.TrimSpace(linkElem.Text())
		href, exists := linkElem.Attr("href")
		if !exists || title == "" {
			return
		}

		imgElem := s.Find(".movie-img img, .movie-img-1 img, .movie-img-inner img, .movie-poster img, .poster img, img").First()
		poster, _ := imgElem.Attr("src")
		if poster == "" {
			poster, _ = imgElem.Attr("data-src")
		}
		poster = p.ResolvePosterURL(poster)

		yearStr := s.Find(".movie-date, .year").Text()
		year := parseYear(yearStr)

		mediaType := "movie"
		if strings.Contains(strings.ToLower(href), "serial") || strings.Contains(strings.ToLower(title), "серіал") {
			mediaType = "series"
		} else if strings.Contains(strings.ToLower(href), "anime") || strings.Contains(strings.ToLower(title), "аніме") {
			mediaType = "anime"
		} else if strings.Contains(strings.ToLower(href), "cartoon") || strings.Contains(strings.ToLower(title), "мультфільм") {
			mediaType = "cartoon"
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

	title := strings.TrimSpace(doc.Find("#dle-content h1, .alltitle h1, h1.movie-title, .solototle, .solotitle, h1").First().Text())
	origTitle := strings.TrimSpace(doc.Find("#dle-content [itemprop='alternateName'], span.origintitle, [itemprop='alternateName']").First().Text())
	if origTitle == "" {
		origTitle, _ = doc.Find("meta[itemprop='alternateName']").Attr("content")
		origTitle = strings.TrimSpace(origTitle)
	}

	posterElem := doc.Find("#dle-content img[itemprop='image'], img[itemprop='image'], #dle-content .full-poster img, .full-poster img, .fposter img, #dle-content .movie-img img").First()
	if posterElem.Length() == 0 {
		posterElem = doc.Find(".movie-img img, .movie-img-1 img, .movie-poster img").First()
	}
	poster, _ := posterElem.Attr("src")
	if poster == "" {
		poster, _ = posterElem.Attr("data-src")
	}
	poster = p.ResolvePosterURL(poster)
	desc := strings.TrimSpace(doc.Find("#dle-content [itemprop='description'], [itemprop='description'], #dle-content .full-text, .full-text, .fdesc").First().Text())

	var genres []string
	var countries []string
	var actors []string
	var director string
	var duration string
	var year int

	// Release year from explicit year links
	if yrLink := doc.Find("#dle-content a[href*='/find/year/'], a[href*='/find/year/']").First().Text(); yrLink != "" {
		year = parseYear(yrLink)
	}

	// .film-info, .flist, .fi-item rows
	doc.Find(".fi-item-s, .fi-item, .film-info div, .flist li").Each(func(i int, s *goquery.Selection) {
		label := strings.ToLower(strings.TrimSpace(s.Find(".fi-label").Text()))
		if label == "" {
			label = strings.ToLower(s.Text())
		}
		valSel := s.Find(".fi-desc")
		if valSel.Length() == 0 {
			valSel = s
		}

		if strings.Contains(label, "режисер") && director == "" {
			director = strings.TrimSpace(valSel.Find("a").First().Text())
			if director == "" {
				parts := strings.Split(valSel.Text(), ":")
				director = strings.TrimSpace(parts[len(parts)-1])
			}
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

	if len(genres) == 0 {
		doc.Find("a[href*='/genre/']").Each(func(i int, s *goquery.Selection) {
			g := strings.TrimSpace(s.Text())
			if g != "" {
				genres = append(genres, g)
			}
		})
	}

	if year == 0 {
		year = parseYear(doc.Find(".film-info, .flist").Text())
	}

	// Schema.org metas fallback
	if len(genres) == 0 {
		if g, ok := doc.Find("meta[itemprop='genre']").Attr("content"); ok {
			for _, part := range strings.Split(g, ",") {
				if t := strings.TrimSpace(part); t != "" {
					genres = append(genres, t)
				}
			}
		}
	}
	if len(countries) == 0 {
		if c, ok := doc.Find("meta[itemprop='contentLocation'], meta[itemprop='countryOfOrigin']").Attr("content"); ok {
			for _, part := range strings.Split(c, ",") {
				if t := strings.TrimSpace(part); t != "" {
					countries = append(countries, t)
				}
			}
		}
	}
	if len(actors) == 0 {
		if a, ok := doc.Find("meta[itemprop='actors']").Attr("content"); ok {
			for _, part := range strings.Split(a, ",") {
				if t := strings.TrimSpace(part); t != "" {
					actors = append(actors, t)
				}
			}
		}
	}
	if director == "" {
		if d, ok := doc.Find("meta[itemprop='director']").Attr("content"); ok {
			director = strings.TrimSpace(d)
		}
	}
	if year == 0 {
		if yrStr, ok := doc.Find("meta[itemprop='dateCreated']").Attr("content"); ok {
			year = parseYear(yrStr)
		}
	}

	var rating float64
	ratesText := doc.Find(".rate-num, .imdb-rate, .movie-rating, [itemprop='ratingValue']").Text()
	m := reRatingNum.FindString(ratesText)
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

// GetStreams знаходить плеєр у iframe та резолвить його у прямий медіа-потік.
//
// Раніше метод повертав URL самого iframe. Це HTML-сторінка, яку libmpv не може
// демодулювати ("Failed to recognize file format."), тобто 100% відмова відтворення.
// Тепер iframe лише завантажується і розбирається через ResolvePlayerHTML; якщо
// жоден потік не розпізнано — повертається порожній список + ErrUnresolvablePlayer.
// ONE budget covers the item-page fetch AND the iframe fan-out, rather than the
// item page running under the client's bare 15s timeout while resolve got a
// separate 20s (35s worst case). Trade-off: a slow item page now eats into the
// resolve budget, so a provider that needs 12s to answer the item page has 8s
// left instead of a fresh 20s. That is deliberate — the old stacking meant one
// call could occupy a request for 35s.
func (p *UakinoProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, PlayerResolveTimeout)
	defer cancel()

	html, err := p.client.Get(ctx, itemURL, p.baseURL)
	if err != nil {
		return nil, fmt.Errorf("uakino get streams html: %w", err)
	}

	return resolveStreamsFromItemPage(ctx, p.client, p.ID(), itemURL, html, season, episode, voiceID)
}

// reRatingNum extracts the first decimal number out of a rating label.
// Shared by uakino and eneyida GetDetails; hoisted to package level because it
// was compiled per request (regexp.MustCompile costs ~10µs and 5-15KB per call).
var reRatingNum = regexp.MustCompile(`([\d.]+)`)

// parseYear extracts a 1900-2099 year from arbitrary text.
//
// The pattern is the one already compiled once at package level in
// bandera_types.go (reYearPattern) — recompiling it here ran on every card of
// every catalogue page.
func parseYear(text string) int {
	match := reYearPattern.FindString(text)
	if match != "" {
		if yr, err := strconv.Atoi(match); err == nil {
			return yr
		}
	}
	return 0
}

func (p *UakinoProvider) ResolvePosterURL(poster string) string {
	poster = strings.TrimSpace(poster)
	if poster == "" {
		return ""
	}
	// Migrate legacy/blocked uakino.best or uakino.me domains to current baseURL
	poster = strings.ReplaceAll(poster, "uakino.best", "uakino.biz")
	poster = strings.ReplaceAll(poster, "uakino.me", "uakino.biz")
	if strings.HasPrefix(poster, "//") {
		return "https:" + poster
	}
	if strings.HasPrefix(poster, "http://") || strings.HasPrefix(poster, "https://") {
		return poster
	}
	return strings.TrimRight(p.baseURL, "/") + "/" + strings.TrimLeft(poster, "/")
}
