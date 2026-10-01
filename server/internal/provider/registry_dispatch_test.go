package provider

// Registry-level coverage for the provider-facing dispatch methods.
//
// These sit between the HTTP handlers and the providers and own the error
// taxonomy the handlers map onto status codes — ErrProviderNotFound becomes
// 404, ErrProviderDisabled becomes 403, anything else becomes 500. They also
// own health recording, which the background cache-purge worker uses to decide
// whether a provider is worth retrying. All of it was previously uncovered,
// which meant a change to the sentinel wrapping would have silently turned
// every 404 into a 500.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
)

// regStub is a provider whose catalogue and search results are configurable,
// and which counts how often it was called so aggregation can be asserted.
type regStub struct {
	id   string
	home bool

	items       []domain.MediaItem
	popularErr  error
	categoryErr error
	searchErr   error

	popularCalls   atomic.Int32
	categoryCalls  atomic.Int32
	searchCalls    atomic.Int32
	gotContentType string
	gotCategory    string
	gotPage        int
	mu             sync.Mutex
}

func newRegStub(id string, home bool, items ...domain.MediaItem) *regStub {
	return &regStub{id: id, home: home, items: items}
}

func (s *regStub) ID() string      { return s.id }
func (s *regStub) Name() string    { return "stub-" + s.id }
func (s *regStub) BaseURL() string { return "https://" + s.id + ".example.com" }
func (s *regStub) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{
		ID: s.id, Name: s.Name(), BaseURL: s.BaseURL(), ShowOnHome: s.home,
		SearchEnabledDefault: true,
	}
}

func (s *regStub) Search(_ context.Context, _ string) ([]domain.MediaItem, error) {
	s.searchCalls.Add(1)
	return s.items, s.searchErr
}

func (s *regStub) GetPopular(_ context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	s.popularCalls.Add(1)
	s.mu.Lock()
	s.gotContentType, s.gotPage = contentType, page
	s.mu.Unlock()
	return s.items, s.popularErr
}

func (s *regStub) GetNew(context.Context, string, int) ([]domain.MediaItem, error) {
	return s.items, nil
}

func (s *regStub) GetByCategory(_ context.Context, category, _ string, page int) ([]domain.MediaItem, error) {
	s.categoryCalls.Add(1)
	s.mu.Lock()
	s.gotCategory, s.gotPage = category, page
	s.mu.Unlock()
	return s.items, s.categoryErr
}

func (s *regStub) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	return nil, nil
}

func (s *regStub) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

func item(id, title string) domain.MediaItem {
	return domain.MediaItem{ID: id, ProviderID: "x", Title: title, Type: "movie", URL: "https://x/" + id}
}

// ---- SearchProvider --------------------------------------------------------

func TestRegistrySearchProvider(t *testing.T) {
	t.Run("routes to the requested provider and passes the query", func(t *testing.T) {
		stub := newRegStub("uakino", true, item("1", "Дюна"))
		reg := NewRegistry()
		reg.Register(stub)

		got, err := reg.SearchProvider(context.Background(), "uakino", "Дюна")
		if err != nil {
			t.Fatalf("SearchProvider: %v", err)
		}
		if len(got) != 1 || got[0].Title != "Дюна" {
			t.Errorf("unexpected items: %+v", got)
		}
		if stub.searchCalls.Load() != 1 {
			t.Errorf("provider called %d times, want 1", stub.searchCalls.Load())
		}
	})

	t.Run("wraps an unknown id so the handler can answer 404", func(t *testing.T) {
		reg := NewRegistry()
		_, err := reg.SearchProvider(context.Background(), "ghost", "q")
		if !errors.Is(err, ErrProviderNotFound) {
			t.Fatalf("error = %v, want it to wrap ErrProviderNotFound", err)
		}
		// The id must survive wrapping, otherwise the log line is useless.
		if got := err.Error(); got == "" || !contains(got, "ghost") {
			t.Errorf("error text %q should name the provider", got)
		}
	})

	t.Run("reports a disabled provider distinctly from a missing one", func(t *testing.T) {
		stub := newRegStub("uakino", true)
		reg := NewRegistry()
		reg.Register(stub)
		reg.SetEnabled("uakino", false)

		_, err := reg.SearchProvider(context.Background(), "uakino", "q")
		if !errors.Is(err, ErrProviderDisabled) {
			t.Fatalf("error = %v, want it to wrap ErrProviderDisabled", err)
		}
		if errors.Is(err, ErrProviderNotFound) {
			t.Error("a disabled provider must not also look like a missing one")
		}
		if stub.searchCalls.Load() != 0 {
			t.Error("a disabled provider must not be called at all")
		}
	})

	t.Run("a provider error is propagated unwrapped", func(t *testing.T) {
		stub := newRegStub("uakino", true)
		stub.searchErr = errors.New("upstream 502")
		reg := NewRegistry()
		reg.Register(stub)

		_, err := reg.SearchProvider(context.Background(), "uakino", "q")
		if err == nil || err.Error() != "upstream 502" {
			t.Fatalf("error = %v, want the provider's own error", err)
		}
		// Wrapping it as not-found or disabled would change the HTTP status.
		if errors.Is(err, ErrProviderNotFound) || errors.Is(err, ErrProviderDisabled) {
			t.Error("an upstream failure must not masquerade as a routing error")
		}
	})
}

// ---- Popular with an explicit provider --------------------------------------

func TestRegistryPopularWithExplicitProvider(t *testing.T) {
	t.Run("forwards type and page and records success", func(t *testing.T) {
		stub := newRegStub("uakino", true, item("1", "Дюна"))
		reg := NewRegistry()
		reg.Register(stub)

		got, err := reg.Popular(context.Background(), "uakino", "series", 3)
		if err != nil {
			t.Fatalf("Popular: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("items = %+v, want 1", got)
		}
		stub.mu.Lock()
		defer stub.mu.Unlock()
		if stub.gotContentType != "series" || stub.gotPage != 3 {
			t.Errorf("provider received type=%q page=%d, want series/3", stub.gotContentType, stub.gotPage)
		}
	})

	t.Run("unknown provider is ErrProviderNotFound", func(t *testing.T) {
		reg := NewRegistry()
		if _, err := reg.Popular(context.Background(), "ghost", "", 1); !errors.Is(err, ErrProviderNotFound) {
			t.Fatalf("error = %v, want ErrProviderNotFound", err)
		}
	})

	t.Run("disabled provider is ErrProviderDisabled", func(t *testing.T) {
		stub := newRegStub("uakino", true)
		reg := NewRegistry()
		reg.Register(stub)
		reg.SetEnabled("uakino", false)

		if _, err := reg.Popular(context.Background(), "uakino", "", 1); !errors.Is(err, ErrProviderDisabled) {
			t.Fatalf("error = %v, want ErrProviderDisabled", err)
		}
		if stub.popularCalls.Load() != 0 {
			t.Error("a disabled provider must not be called")
		}
	})
}

// ---- Popular without a provider: aggregation -------------------------------

func TestRegistryPopularAggregatesHomeProviders(t *testing.T) {
	t.Run("combines every enabled home provider", func(t *testing.T) {
		a := newRegStub("a", true, item("1", "Alpha"))
		b := newRegStub("b", true, item("2", "Beta"))
		reg := NewRegistry()
		reg.Register(a)
		reg.Register(b)

		got, err := reg.Popular(context.Background(), "", "", 1)
		if err != nil {
			t.Fatalf("Popular: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("aggregated %d items, want 2: %+v", len(got), got)
		}
		// Order is not guaranteed: aggregation runs each provider in its own
		// goroutine. Collect titles instead of comparing slices.
		titles := map[string]bool{}
		for _, it := range got {
			titles[it.Title] = true
		}
		if !titles["Alpha"] || !titles["Beta"] {
			t.Errorf("missing an aggregated title, got %v", titles)
		}
	})

	t.Run("skips providers that are not shown on home", func(t *testing.T) {
		home := newRegStub("home", true, item("1", "Shown"))
		hidden := newRegStub("hidden", false, item("2", "Hidden"))
		reg := NewRegistry()
		reg.Register(home)
		reg.Register(hidden)

		got, err := reg.Popular(context.Background(), "", "", 1)
		if err != nil {
			t.Fatalf("Popular: %v", err)
		}
		if len(got) != 1 || got[0].Title != "Shown" {
			t.Errorf("only the home provider should contribute, got %+v", got)
		}
		if hidden.popularCalls.Load() != 0 {
			t.Error("a non-home provider must not be queried during aggregation")
		}
	})

	t.Run("skips disabled providers", func(t *testing.T) {
		on := newRegStub("on", true, item("1", "On"))
		off := newRegStub("off", true, item("2", "Off"))
		reg := NewRegistry()
		reg.Register(on)
		reg.Register(off)
		reg.SetEnabled("off", false)

		got, _ := reg.Popular(context.Background(), "", "", 1)
		if len(got) != 1 || got[0].Title != "On" {
			t.Errorf("got %+v, want only the enabled provider", got)
		}
	})

	// One broken scraper must not blank the whole home screen.
	t.Run("a failing provider does not sink the others", func(t *testing.T) {
		good := newRegStub("good", true, item("1", "Good"))
		bad := newRegStub("bad", true)
		bad.popularErr = errors.New("upstream 502")
		reg := NewRegistry()
		reg.Register(good)
		reg.Register(bad)

		got, err := reg.Popular(context.Background(), "", "", 1)
		if err != nil {
			t.Fatalf("aggregation should not surface a provider error: %v", err)
		}
		if len(got) != 1 || got[0].Title != "Good" {
			t.Errorf("got %+v, want the healthy provider's item", got)
		}
	})

	t.Run("an empty registry aggregates to no items without erroring", func(t *testing.T) {
		reg := NewRegistry()
		got, err := reg.Popular(context.Background(), "", "", 1)
		if err != nil {
			t.Fatalf("Popular: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("items = %+v, want none", got)
		}
	})

	// Providers are queried concurrently, so this exercises the shared mutexes
	// under -race.
	t.Run("many providers aggregate safely", func(t *testing.T) {
		reg := NewRegistry()
		for i := 0; i < 12; i++ {
			reg.Register(newRegStub(fmt.Sprintf("p%d", i), true, item(fmt.Sprint(i), fmt.Sprintf("T%d", i))))
		}
		got, err := reg.Popular(context.Background(), "", "", 1)
		if err != nil {
			t.Fatalf("Popular: %v", err)
		}
		if len(got) != 12 {
			t.Errorf("aggregated %d items, want 12", len(got))
		}
	})
}

// ---- Category --------------------------------------------------------------

func TestRegistryCategory(t *testing.T) {
	t.Run("forwards the category with an explicit provider", func(t *testing.T) {
		stub := newRegStub("uakino", true, item("1", "Комедія"))
		reg := NewRegistry()
		reg.Register(stub)

		got, err := reg.Category(context.Background(), "uakino", "comedy", "", 2)
		if err != nil {
			t.Fatalf("Category: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("items = %+v, want 1", got)
		}
		stub.mu.Lock()
		defer stub.mu.Unlock()
		if stub.gotCategory != "comedy" || stub.gotPage != 2 {
			t.Errorf("provider received category=%q page=%d, want comedy/2", stub.gotCategory, stub.gotPage)
		}
	})

	t.Run("unknown provider", func(t *testing.T) {
		reg := NewRegistry()
		if _, err := reg.Category(context.Background(), "ghost", "c", "", 1); !errors.Is(err, ErrProviderNotFound) {
			t.Fatalf("error = %v, want ErrProviderNotFound", err)
		}
	})

	t.Run("disabled provider", func(t *testing.T) {
		stub := newRegStub("uakino", true)
		reg := NewRegistry()
		reg.Register(stub)
		reg.SetEnabled("uakino", false)

		if _, err := reg.Category(context.Background(), "uakino", "c", "", 1); !errors.Is(err, ErrProviderDisabled) {
			t.Fatalf("error = %v, want ErrProviderDisabled", err)
		}
		if stub.categoryCalls.Load() != 0 {
			t.Error("a disabled provider must not be called")
		}
	})

	t.Run("aggregates across home providers and isolates failures", func(t *testing.T) {
		good := newRegStub("good", true, item("1", "Good"))
		bad := newRegStub("bad", true)
		bad.categoryErr = errors.New("boom")
		hidden := newRegStub("hidden", false, item("3", "Hidden"))
		reg := NewRegistry()
		reg.Register(good)
		reg.Register(bad)
		reg.Register(hidden)

		got, err := reg.Category(context.Background(), "", "comedy", "", 1)
		if err != nil {
			t.Fatalf("Category: %v", err)
		}
		if len(got) != 1 || got[0].Title != "Good" {
			t.Errorf("got %+v, want only the healthy home provider's item", got)
		}
		if hidden.categoryCalls.Load() != 0 {
			t.Error("a non-home provider must not be queried")
		}
	})

	t.Run("aggregates every home provider when one fails", func(t *testing.T) {
		ok := newRegStub("ok", true, item("1", "Alpha"))
		reg := NewRegistry()
		reg.Register(ok)

		got, err := reg.Category(context.Background(), "", "c", "", 1)
		if err != nil || len(got) != 1 {
			t.Fatalf("Category returned %+v, %v", got, err)
		}
	})
}

// ---- health recording ------------------------------------------------------

func TestRegistryRecordsHealth(t *testing.T) {
	t.Run("a success clears the error streak", func(t *testing.T) {
		stub := newRegStub("p", true, item("1", "A"))
		reg := NewRegistry()
		reg.Register(stub)

		stub.popularErr = errors.New("first failure")
		if _, err := reg.Popular(context.Background(), "p", "", 1); err == nil {
			t.Fatal("expected the first call to fail")
		}
		stub.popularErr = nil
		if _, err := reg.Popular(context.Background(), "p", "", 1); err != nil {
			t.Fatalf("second call: %v", err)
		}

		reg.mu.RLock()
		h := reg.health["p"]
		reg.mu.RUnlock()
		if h == nil {
			t.Fatal("no health recorded for the provider")
		}
		if h.consecutiveErrors != 0 {
			t.Errorf("consecutiveErrors = %d, want 0 after a success", h.consecutiveErrors)
		}
		if h.lastError != "" {
			t.Errorf("lastError = %q, want it cleared", h.lastError)
		}
		if h.lastSuccessUnix == 0 {
			t.Error("lastSuccessUnix was not recorded")
		}
	})

	t.Run("consecutive failures accumulate", func(t *testing.T) {
		stub := newRegStub("p", true)
		stub.popularErr = errors.New("boom")
		reg := NewRegistry()
		reg.Register(stub)

		for i := 1; i <= 3; i++ {
			if _, err := reg.Popular(context.Background(), "p", "", 1); err == nil {
				t.Fatalf("call %d should have failed", i)
			}
			reg.mu.RLock()
			got := reg.health["p"].consecutiveErrors
			reg.mu.RUnlock()
			if got != i {
				t.Fatalf("after %d failures consecutiveErrors = %d", i, got)
			}
		}

		reg.mu.RLock()
		msg := reg.health["p"].lastError
		reg.mu.RUnlock()
		if msg != "boom" {
			t.Errorf("lastError = %q, want boom", msg)
		}
	})

	t.Run("aggregated runs record health per provider", func(t *testing.T) {
		good := newRegStub("good", true, item("1", "A"))
		bad := newRegStub("bad", true)
		bad.popularErr = errors.New("bad news")
		reg := NewRegistry()
		reg.Register(good)
		reg.Register(bad)

		if _, err := reg.Popular(context.Background(), "", "", 1); err != nil {
			t.Fatalf("Popular: %v", err)
		}

		reg.mu.RLock()
		defer reg.mu.RUnlock()
		if reg.health["good"] == nil || reg.health["good"].consecutiveErrors != 0 {
			t.Error("the healthy provider should have a clean streak")
		}
		if reg.health["bad"] == nil || reg.health["bad"].consecutiveErrors == 0 {
			t.Error("the failing provider should have a recorded error")
		}
	})
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 ||
		indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

var _ = http.StatusOK
