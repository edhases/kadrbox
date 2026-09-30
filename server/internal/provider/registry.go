package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"golang.org/x/sync/singleflight"
)

// ErrProviderDisabled повертається, коли провайдер вимкнено бекендом (kill-switch).
var ErrProviderDisabled = errors.New("provider disabled")

// ErrProviderNotFound повертається для невідомого ID провайдера.
var ErrProviderNotFound = errors.New("unknown provider")

type providerHealth struct {
	consecutiveErrors int
	lastError         string
	lastSuccessUnix   int64
	lastErrorUnix     int64
}

type Registry struct {
	providers map[string]domain.Provider
	disabled  map[string]bool
	health    map[string]*providerHealth
	rev       int64
	mu        sync.RWMutex
	sf        singleflight.Group
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]domain.Provider),
		disabled:  make(map[string]bool),
		health:    make(map[string]*providerHealth),
	}
}

func (r *Registry) Register(p domain.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
	if _, ok := r.health[p.ID()]; !ok {
		r.health[p.ID()] = &providerHealth{}
	}
	r.rev++
}

func (r *Registry) Get(id string) (domain.Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

func (r *Registry) List() []domain.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]domain.Provider, 0, len(r.providers))
	for _, p := range r.providers {
		list = append(list, p)
	}
	return list
}

// SetEnabled примусово вмикає/вимикає провайдер (kill-switch бекенда).
func (r *Registry) SetEnabled(id string, enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if enabled {
		delete(r.disabled, id)
	} else {
		r.disabled[id] = true
	}
	r.rev++
}

// IsEnabled перевіряє, чи не вимкнено провайдер бекендом.
func (r *Registry) IsEnabled(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.disabled[id]
}

// DisableMany вимикає список провайдерів (наприклад, з DISABLED_PROVIDERS).
func (r *Registry) DisableMany(ids []string) {
	for _, id := range ids {
		if id != "" {
			r.SetEnabled(id, false)
		}
	}
}

// Catalog будує публічний каталог провайдерів для застосунків.
func (r *Registry) Catalog() domain.ProviderCatalog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]domain.ProviderCatalogEntry, 0, len(r.providers))
	for id, p := range r.providers {
		h := r.health[id]
		if h == nil {
			h = &providerHealth{}
		}
		entries = append(entries, domain.ProviderCatalogEntry{
			ProviderInfo: p.Describe(),
			Enabled:      !r.disabled[id],
			Healthy:      h.consecutiveErrors == 0,
			Health: domain.ProviderHealth{
				ConsecutiveErrors: h.consecutiveErrors,
				LastError:         h.lastError,
				LastSuccessUnix:   h.lastSuccessUnix,
				LastErrorUnix:     h.lastErrorUnix,
			},
		})
	}
	return domain.ProviderCatalog{Version: r.rev, Providers: entries}
}

func (r *Registry) recordSuccess(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.health[id]
	if !ok {
		h = &providerHealth{}
		r.health[id] = h
	}
	h.consecutiveErrors = 0
	h.lastError = ""
	h.lastSuccessUnix = time.Now().Unix()
}

func (r *Registry) recordError(id string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.health[id]
	if !ok {
		h = &providerHealth{}
		r.health[id] = h
	}
	h.consecutiveErrors++
	if err != nil {
		h.lastError = err.Error()
	}
	h.lastErrorUnix = time.Now().Unix()
}

// SearchAll виконує паралельний пошук по всіх УВІМКНЕНИХ провайдерах
func (r *Registry) SearchAll(ctx context.Context, query string) []domain.MediaItem {
	providers := r.List()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var aggregated []domain.MediaItem

	for _, p := range providers {
		if !r.IsEnabled(p.ID()) {
			continue
		}
		wg.Add(1)
		go func(prov domain.Provider) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[PANIC RECOVER] Provider %s crashed on query %q: %v", prov.Name(), query, rec)
				}
			}()
			items, err := prov.Search(ctx, query)
			if err != nil {
				r.recordError(prov.ID(), err)
				return
			}
			r.recordSuccess(prov.ID())
			if len(items) > 0 {
				mu.Lock()
				aggregated = append(aggregated, items...)
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()
	return aggregated
}

// SingleFlightSearch запобігає дублюванню однакових одночасних пошукових запитів від багатьох клієнтів
func (r *Registry) SingleFlightSearch(ctx context.Context, query string) ([]domain.MediaItem, error) {
	key := fmt.Sprintf("search:%s", query)
	val, err, _ := r.sf.Do(key, func() (interface{}, error) {
		return r.SearchAll(ctx, query), nil
	})
	if err != nil {
		return nil, err
	}
	return val.([]domain.MediaItem), nil
}

// Details повертає деталі через провайдер з трекінгом здоров'я та kill-switch.
func (r *Registry) Details(ctx context.Context, id, itemURL string) (*domain.MediaDetails, error) {
	p, ok := r.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrProviderNotFound, id)
	}
	if !r.IsEnabled(id) {
		return nil, fmt.Errorf("%w: %s", ErrProviderDisabled, id)
	}
	details, err := p.GetDetails(ctx, itemURL)
	if err != nil {
		r.recordError(id, err)
		return nil, err
	}
	r.recordSuccess(id)
	return details, nil
}

// Streams повертає стріми через провайдер з трекінгом здоров'я та kill-switch.
func (r *Registry) Streams(ctx context.Context, id, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	p, ok := r.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrProviderNotFound, id)
	}
	if !r.IsEnabled(id) {
		return nil, fmt.Errorf("%w: %s", ErrProviderDisabled, id)
	}
	resp, err := p.GetStreams(ctx, itemURL, season, episode, voiceID)
	if err != nil {
		r.recordError(id, err)
		return nil, err
	}
	r.recordSuccess(id)
	return resp, nil
}
