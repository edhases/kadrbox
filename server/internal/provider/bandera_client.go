package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// BanderaDefaultBaseURL — офіційний API endpoint для Bandera Online
const BanderaDefaultBaseURL = "https://bbe.lme.isroot.in/api/v2"

// BanderaDefaultSources — stale fallback на випадок, коли /sources недоступний (перевірено 2026-09-30).
// В основному потоці джерела завантажуються динамічно через GET /sources.
const BanderaDefaultSources = "uaflix,makhno,filmix,bambooua,animeon,mikai,starlight,franko"

// BanderaClient інкапсулює мережеву взаємодію з API bbe.lme.isroot.in
type BanderaClient struct {
	baseURL    string
	httpClient *http.Client

	sourcesMu        sync.RWMutex
	sourcesTTL       time.Duration
	lastSourcesFetch time.Time
	sourceMetaMap    map[string]SourceMeta
	enabledSearchStr string
}

func NewBanderaClient(baseURL string, client *http.Client) *BanderaClient {
	if baseURL == "" {
		baseURL = BanderaDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if client == nil {
		client = &http.Client{
			Timeout: 20 * time.Second,
		}
	}
	return &BanderaClient{
		baseURL:       baseURL,
		httpClient:    client,
		sourcesTTL:    10 * time.Minute,
		sourceMetaMap: make(map[string]SourceMeta),
	}
}

// GetSources виконує GET /sources із кешуванням на 10 хвилин
func (c *BanderaClient) GetSources(ctx context.Context) (map[string]SourceMeta, error) {
	c.sourcesMu.RLock()
	if time.Since(c.lastSourcesFetch) < c.sourcesTTL && len(c.sourceMetaMap) > 0 {
		metaCopy := make(map[string]SourceMeta, len(c.sourceMetaMap))
		for k, v := range c.sourceMetaMap {
			metaCopy[k] = v
		}
		c.sourcesMu.RUnlock()
		return metaCopy, nil
	}
	c.sourcesMu.RUnlock()

	c.sourcesMu.Lock()
	defer c.sourcesMu.Unlock()

	// Подвійна перевірка після захоплення write lock
	if time.Since(c.lastSourcesFetch) < c.sourcesTTL && len(c.sourceMetaMap) > 0 {
		metaCopy := make(map[string]SourceMeta, len(c.sourceMetaMap))
		for k, v := range c.sourceMetaMap {
			metaCopy[k] = v
		}
		return metaCopy, nil
	}

	reqURL := fmt.Sprintf("%s/sources", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create sources request: %w", err)
	}
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Якщо мережевий запит впав, але є кеш — повертаємо старий кеш
		if len(c.sourceMetaMap) > 0 {
			metaCopy := make(map[string]SourceMeta, len(c.sourceMetaMap))
			for k, v := range c.sourceMetaMap {
				metaCopy[k] = v
			}
			return metaCopy, nil
		}
		return nil, fmt.Errorf("fetch sources: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(c.sourceMetaMap) > 0 {
			metaCopy := make(map[string]SourceMeta, len(c.sourceMetaMap))
			for k, v := range c.sourceMetaMap {
				metaCopy[k] = v
			}
			return metaCopy, nil
		}
		return nil, fmt.Errorf("sources returned status %d: %s", resp.StatusCode, string(body))
	}

	var sourcesResp BanderaSourcesResponse
	if err := json.NewDecoder(resp.Body).Decode(&sourcesResp); err != nil {
		return nil, fmt.Errorf("decode sources response: %w", err)
	}

	newMetaMap := make(map[string]SourceMeta, len(sourcesResp.Sources))
	var enabledList []string

	for _, s := range sourcesResp.Sources {
		meta := SourceMeta{
			Key:         s.Key,
			Name:        s.Name,
			Enabled:     s.Enabled,
			ContentKeys: s.Inputs.Content,
			StreamKeys:  s.Inputs.Stream,
			CanSearch:   s.Capabilities.Search,
			CanContent:  s.Capabilities.Content,
			CanStream:   s.Capabilities.Stream,
		}
		newMetaMap[s.Key] = meta
		if s.Enabled && s.Capabilities.Search {
			enabledList = append(enabledList, s.Key)
		}
	}

	c.sourceMetaMap = newMetaMap
	if len(enabledList) > 0 {
		c.enabledSearchStr = strings.Join(enabledList, ",")
	} else {
		c.enabledSearchStr = BanderaDefaultSources
	}
	c.lastSourcesFetch = time.Now()

	metaCopy := make(map[string]SourceMeta, len(newMetaMap))
	for k, v := range newMetaMap {
		metaCopy[k] = v
	}
	return metaCopy, nil
}

// GetSearchSourcesStr повертає кому-розділений список увімкнених джерел для пошуку
func (c *BanderaClient) GetSearchSourcesStr(ctx context.Context) string {
	if _, err := c.GetSources(ctx); err == nil && c.enabledSearchStr != "" {
		return c.enabledSearchStr
	}
	return BanderaDefaultSources
}

// GetSourceMeta повертає метадані для конкретного ключа джерела
func (c *BanderaClient) GetSourceMeta(ctx context.Context, sourceKey string) (SourceMeta, bool) {
	sources, err := c.GetSources(ctx)
	if err != nil {
		return SourceMeta{}, false
	}
	m, ok := sources[sourceKey]
	return m, ok
}

// SearchWithMeta виконує пошук та повертає повну відповідь разом із meta.statuses
func (c *BanderaClient) SearchWithMeta(ctx context.Context, query string, year int, serial int) (*BanderaSearchResponse, error) {
	sourcesStr := c.GetSearchSourcesStr(ctx)
	reqURL := fmt.Sprintf("%s/search?sources=%s&title=%s",
		c.baseURL, url.QueryEscape(sourcesStr), url.QueryEscape(query))
	if year > 0 {
		reqURL += fmt.Sprintf("&year=%d", year)
	}
	if serial > 0 {
		reqURL += fmt.Sprintf("&serial=%d", serial)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp BanderaSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	return &searchResp, nil
}

// Search виконує базовий пошук за запитом query
func (c *BanderaClient) Search(ctx context.Context, query string) ([]BanderaSearchItem, error) {
	resp, err := c.SearchWithMeta(ctx, query, 0, 0)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// GetContent запитує /content з обов'язковим прапорцем full: true
func (c *BanderaClient) GetContent(ctx context.Context, source string, ref json.RawMessage) (*BanderaContentResponse, error) {
	reqBody, err := json.Marshal(BanderaContentRequest{
		Source: source,
		Ref:    ref,
		Full:   true, // bo.js завжди надсилає full: true
	})
	if err != nil {
		return nil, fmt.Errorf("marshal content request: %w", err)
	}

	contentURL := fmt.Sprintf("%s/content", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, contentURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create content request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("content request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("content returned status %d: %s", resp.StatusCode, string(body))
	}

	var contentResp BanderaContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&contentResp); err != nil {
		return nil, fmt.Errorf("decode content response: %w", err)
	}

	return &contentResp, nil
}

// GetStream надсилає запит до /stream. Ця операція НІКОЛИ не мемоізується, оскільки URL джерел (як-от animeon)
// є підписаними і мають короткий час життя (?expires=...&sig=...).
func (c *BanderaClient) GetStream(ctx context.Context, source string, ref json.RawMessage) (*BanderaStreamResponse, error) {
	reqBody, err := json.Marshal(BanderaStreamRequest{
		Source: source,
		Ref:    ref,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	streamURL := fmt.Sprintf("%s/stream", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, streamURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", Chrome120UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("stream returned status %d: %s", resp.StatusCode, string(body))
	}

	var streamResp BanderaStreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&streamResp); err != nil {
		return nil, fmt.Errorf("decode stream response: %w", err)
	}

	return &streamResp, nil
}
