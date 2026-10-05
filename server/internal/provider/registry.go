package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"golang.org/x/sync/singleflight"
)

// ErrProviderDisabled повертається, коли провайдер вимкнено бекендом (kill-switch).
var ErrProviderDisabled = errors.New("provider disabled")

// ErrProviderNotFound повертається для невідомого ID провайдера.
var ErrProviderNotFound = errors.New("unknown provider")

// SearchFanoutBudget bounds one coalesced multi-provider fan-out. The work runs
// on a context detached from any single caller, so without its own deadline a
// scraper that hangs would keep the singleflight key occupied indefinitely and
// every subsequent identical search would pile up behind it.
//
// It sits below the client's receiveTimeout (30s, lib/core/config/app_config.dart)
// on purpose. When the two were equal, a hung scraper made the client give up at
// the same instant the server did, so the client never saw the 503 + Retry-After
// the handler had produced, and retried a search that was already failing.
const SearchFanoutBudget = 20 * time.Second

// ErrProviderPanic marks an error that came from a recovered provider panic
// rather than from a returned error. It is a distinct sentinel so the HTTP
// layer can keep answering 500 with a JSON body for a panicking scraper
// (the contract clients already see) while still isolating the panic to the
// one provider that caused it.
var ErrProviderPanic = errors.New("provider panicked")

// recoverProvider turns a panicking provider into an error. Every scraper is
// third-party HTML parsing, so a panic there is a real possibility and must
// not be able to take down the request that happened to trigger it — nor, in
// the fan-out case, every concurrent request coalesced behind it.
func recoverProvider(op string, p domain.Provider, err *error) {
	rec := recover()
	if rec == nil {
		return
	}

	// The ID is read defensively: a provider whose ID() itself panics would
	// otherwise re-panic out of this deferred function and take the request with
	// it, which is the exact outcome this exists to prevent.
	id := "<unknown>"
	if p != nil {
		func() {
			defer func() { _ = recover() }()
			id = p.ID()
		}()
	}

	// The stack is what makes a recovered third-party-parser panic diagnosable
	// at all. Without it the log line says which provider crashed and nothing
	// about where.
	stack := make([]byte, 8<<10)
	stack = stack[:runtime.Stack(stack, false)]

	log.Printf("[PANIC RECOVER] Provider %s crashed in %s: %v\n%s", id, op, rec, stack)
	*err = fmt.Errorf("%w: %s during %s: %v", ErrProviderPanic, id, op, rec)
}

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
	// Non-nil so an exhausted search serialises as [] rather than null.
	aggregated := make([]domain.MediaItem, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, p := range providers {
		if !r.IsEnabled(p.ID()) {
			continue
		}
		wg.Add(1)
		go func(prov domain.Provider) {
			defer wg.Done()
			items, err := safeSearch(prov, ctx, query)
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

// safeSearch isolates a panicking scraper. Without it a provider that panics
// takes down every concurrent search with it, since a panic in a goroutine
// cannot be recovered by the request that started it.
func safeSearch(p domain.Provider, ctx context.Context, query string) (items []domain.MediaItem, err error) {
	defer recoverProvider("Search", p, &err)
	return p.Search(ctx, query)
}

// SearchProvider виконує пошук по конкретному провайдеру
func (r *Registry) SearchProvider(ctx context.Context, id, query string) ([]domain.MediaItem, error) {
	p, ok := r.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrProviderNotFound, id)
	}
	if !r.IsEnabled(id) {
		return nil, fmt.Errorf("%w: %s", ErrProviderDisabled, id)
	}
	items, err := safeSearch(p, ctx, query)
	if err != nil {
		r.recordError(id, err)
		return nil, err
	}
	r.recordSuccess(id)
	return nonNilItems(items), nil
}

// SingleFlightSearch запобігає дублюванню однакових одночасних пошукових запитів від багатьох клієнтів
func (r *Registry) SingleFlightSearch(ctx context.Context, query string) ([]domain.MediaItem, error) {
	key := fmt.Sprintf("search:%s", query)

	ch := r.sf.DoChan(key, func() (interface{}, error) {
		// The shared context is created INSIDE the flight, so it is owned by
		// the goroutine that actually runs the work rather than by whichever
		// caller happened to arrive first.
		//
		// Two things were wrong before. Sharing the first caller's context
		// meant one client disconnecting failed every coalesced caller. And
		// creating the detached context outside the flight meant the first
		// caller's `defer cancel()` tore the work down when that caller gave
		// up on its own wait — the same failure by a different route.
		//
		// WithoutCancel drops the first caller's deadline and values are kept;
		// the budget bounds the work so a hanging scraper cannot hold the key
		// forever.
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), SearchFanoutBudget)
		defer cancel()
		return r.SearchAll(shared, query), nil
	})

	// DoChan, not Do: each waiter applies its own cancellation to its own wait
	// and gives up without tearing down the shared work for the others.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		items, ok := res.Val.([]domain.MediaItem)
		if !ok {
			return nil, fmt.Errorf("singleflight search %q: unexpected result type %T", key, res.Val)
		}
		if items == nil {
			items = []domain.MediaItem{}
		}
		return items, nil
	}
}

func safeGetDetails(p domain.Provider, ctx context.Context, itemURL string) (details *domain.MediaDetails, err error) {
	defer recoverProvider("GetDetails", p, &err)
	return p.GetDetails(ctx, itemURL)
}

func safeGetStreams(p domain.Provider, ctx context.Context, itemURL string, season, episode int, voiceID string) (resp *domain.ContentStreamsResponse, err error) {
	defer recoverProvider("GetStreams", p, &err)
	return p.GetStreams(ctx, itemURL, season, episode, voiceID)
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
	details, err := safeGetDetails(p, ctx, itemURL)
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
	resp, err := safeGetStreams(p, ctx, itemURL, season, episode, voiceID)
	if err != nil {
		r.recordError(id, err)
		return nil, err
	}
	r.recordSuccess(id)
	return resp, nil
}

// Popular повертає список популярного контенту для вказаного провайдера або першого доступного
func (r *Registry) Popular(ctx context.Context, id, contentType string, page int) ([]domain.MediaItem, error) {
	if id != "" {
		p, ok := r.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w %q", ErrProviderNotFound, id)
		}
		if !r.IsEnabled(id) {
			return nil, fmt.Errorf("%w: %s", ErrProviderDisabled, id)
		}
		items, err := safeGetPopular(p, ctx, contentType, page)
		if err != nil {
			r.recordError(id, err)
			return nil, err
		}
		r.recordSuccess(id)
		return nonNilItems(items), nil
	}

	// Якщо провайдер не вказано — об'єднуємо з домашніх увімкнених провайдерів
	// A nil slice serialises as `null`, which the client has to special-case;
	// an empty result must be `[]`.
	aggregated := make([]domain.MediaItem, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range r.List() {
		if !r.IsEnabled(p.ID()) || !p.Describe().ShowOnHome {
			continue
		}
		wg.Add(1)
		go func(prov domain.Provider) {
			defer wg.Done()
			// A panicking scraper must not kill the home screen: the other
			// providers' results are still worth returning.
			items, err := safeGetPopular(prov, ctx, contentType, page)
			if err != nil {
				r.recordError(prov.ID(), err)
				return
			}
			r.recordSuccess(prov.ID())
			if len(items) == 0 {
				return
			}
			mu.Lock()
			aggregated = append(aggregated, items...)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return aggregated, nil
}

func safeGetPopular(p domain.Provider, ctx context.Context, contentType string, page int) (items []domain.MediaItem, err error) {
	defer recoverProvider("GetPopular", p, &err)
	return p.GetPopular(ctx, contentType, page)
}

// Category повертає список контенту за категорією/жанром
func (r *Registry) Category(ctx context.Context, id, category, contentType string, page int) ([]domain.MediaItem, error) {
	if id != "" {
		p, ok := r.Get(id)
		if !ok {
			return nil, fmt.Errorf("%w %q", ErrProviderNotFound, id)
		}
		if !r.IsEnabled(id) {
			return nil, fmt.Errorf("%w: %s", ErrProviderDisabled, id)
		}
		items, err := safeGetByCategory(p, ctx, category, contentType, page)
		if err != nil {
			r.recordError(id, err)
			return nil, err
		}
		r.recordSuccess(id)
		return nonNilItems(items), nil
	}

	aggregated := make([]domain.MediaItem, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range r.List() {
		if !r.IsEnabled(p.ID()) || !p.Describe().ShowOnHome {
			continue
		}
		wg.Add(1)
		go func(prov domain.Provider) {
			defer wg.Done()
			items, err := safeGetByCategory(prov, ctx, category, contentType, page)
			if err != nil {
				r.recordError(prov.ID(), err)
				return
			}
			r.recordSuccess(prov.ID())
			if len(items) == 0 {
				return
			}
			mu.Lock()
			aggregated = append(aggregated, items...)
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return aggregated, nil
}

func safeGetByCategory(p domain.Provider, ctx context.Context, category, contentType string, page int) (items []domain.MediaItem, err error) {
	defer recoverProvider("GetByCategory", p, &err)
	return p.GetByCategory(ctx, category, contentType, page)
}

func nonNilItems(items []domain.MediaItem) []domain.MediaItem {
	if items == nil {
		return []domain.MediaItem{}
	}
	return items
}
