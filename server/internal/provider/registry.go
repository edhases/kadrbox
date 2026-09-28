package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/edhases/oxide-server/internal/domain"
	"golang.org/x/sync/singleflight"
)

type Registry struct {
	providers map[string]domain.Provider
	mu        sync.RWMutex
	sf        singleflight.Group
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]domain.Provider),
	}
}

func (r *Registry) Register(p domain.Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
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

// SearchAll виконує паралельний пошук по всіх зареєстрованих провайдерах
func (r *Registry) SearchAll(ctx context.Context, query string) []domain.MediaItem {
	providers := r.List()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var aggregated []domain.MediaItem

	for _, p := range providers {
		wg.Add(1)
		go func(prov domain.Provider) {
			defer wg.Done()
			items, err := prov.Search(ctx, query)
			if err == nil && len(items) > 0 {
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
