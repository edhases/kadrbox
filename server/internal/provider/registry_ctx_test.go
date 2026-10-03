package provider_test

// Context-propagation and panic-isolation tests for the Registry.
//
// The singleflight group shares one execution across every concurrent caller.
// The old code closed over the FIRST caller's context, so a single client
// disconnecting cancelled the shared fan-out and every other coalesced caller
// failed with a context error that had nothing to do with its own request. It
// also did val.([]domain.MediaItem) with no comma-ok, so a value of any other
// type panicked inside a request.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
)

// rcBlockingProvider blocks in Search until its gate is closed, and counts the
// calls so the test can prove the shared work ran exactly once.
type rcBlockingProvider struct {
	id    string
	gate  chan struct{}
	calls int32
}

func (p *rcBlockingProvider) ID() string   { return p.id }
func (p *rcBlockingProvider) Name() string { return "rc-" + p.id }
func (p *rcBlockingProvider) BaseURL() string {
	return "https://" + p.id + ".example.com"
}
func (p *rcBlockingProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), ShowOnHome: true}
}

func (p *rcBlockingProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	atomic.AddInt32(&p.calls, 1)
	select {
	case <-p.gate:
		return []domain.MediaItem{{ID: "1", ProviderID: p.id, Title: query}}, nil
	case <-ctx.Done():
		// A production scraper aborts when its context dies. This is what made
		// one caller's disconnect fail everybody else's search.
		return nil, ctx.Err()
	}
}

func (p *rcBlockingProvider) GetPopular(context.Context, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}

func (p *rcBlockingProvider) GetByCategory(context.Context, string, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}

func (p *rcBlockingProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	return nil, nil
}

func (p *rcBlockingProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

// Cancelling the first caller's context must not fail a second concurrent
// caller that coalesced onto the same singleflight key.
func TestSingleFlightFirstCallerCancellationDoesNotFailOthers(t *testing.T) {
	reg := provider.NewRegistry()
	gate := make(chan struct{})
	p := &rcBlockingProvider{id: "p1", gate: gate}
	reg.Register(p)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()

	type outcome struct {
		items []domain.MediaItem
		err   error
	}
	firstDone := make(chan outcome, 1)
	go func() {
		items, err := reg.SingleFlightSearch(firstCtx, "shared-query")
		firstDone <- outcome{items, err}
	}()

	// Wait until the first caller is inside the fan-out, then start a second
	// caller so it joins the same in-flight key.
	waitForCalls(t, p, 1)

	secondDone := make(chan outcome, 1)
	go func() {
		items, err := reg.SingleFlightSearch(context.Background(), "shared-query")
		secondDone <- outcome{items, err}
	}()
	// Give the second goroutine a chance to join before releasing the gate.
	time.Sleep(50 * time.Millisecond)

	// The first client disconnects. Its cancellation must apply to its own wait
	// only.
	cancelFirst()

	close(gate)

	first := <-firstDone
	second := <-secondDone

	// The first caller is entitled to its own cancellation.
	if first.err == nil {
		t.Logf("first caller completed before its cancellation was observed (acceptable)")
	} else if !errors.Is(first.err, context.Canceled) {
		t.Errorf("first caller: err = %v, want context.Canceled", first.err)
	}

	// The second caller must be unaffected.
	if second.err != nil {
		t.Fatalf("second caller failed because the FIRST caller's context was cancelled: %v", second.err)
	}
	if len(second.items) != 1 || second.items[0].Title != "shared-query" {
		t.Errorf("second caller got %+v, want the fan-out result", second.items)
	}
	if got := atomic.LoadInt32(&p.calls); got != 1 {
		t.Errorf("provider called %d times, want 1: the fan-out was not coalesced", got)
	}
}

// A caller that gives up must not tear down the shared work for the others,
// and a caller that is already waiting must still get the result.
func TestSingleFlightWaiterCancellationIsIsolated(t *testing.T) {
	reg := provider.NewRegistry()
	gate := make(chan struct{})
	p := &rcBlockingProvider{id: "p1", gate: gate}
	reg.Register(p)

	impatient, cancelImpatient := context.WithCancel(context.Background())
	impatientDone := make(chan error, 1)
	go func() {
		_, err := reg.SingleFlightSearch(impatient, "q")
		impatientDone <- err
	}()
	waitForCalls(t, p, 1)

	patientDone := make(chan []domain.MediaItem, 1)
	go func() {
		items, _ := reg.SingleFlightSearch(context.Background(), "q")
		patientDone <- items
	}()
	time.Sleep(50 * time.Millisecond)

	cancelImpatient()
	if err := <-impatientDone; !errors.Is(err, context.Canceled) {
		t.Errorf("impatient caller: err = %v, want context.Canceled", err)
	}

	close(gate)
	items := <-patientDone
	if len(items) != 1 {
		t.Errorf("patient caller got %+v, want 1 item", items)
	}
	if got := atomic.LoadInt32(&p.calls); got != 1 {
		t.Errorf("provider called %d times, want 1", got)
	}
}

// A panicking provider must not take down the request that triggered it. The
// old SearchAll had a recover, but Popular and Category had none, so the home
// screen 500'd on a scraper panic while search silently swallowed it.
func TestRegistryPopularAndCategorySurviveAPanickingProvider(t *testing.T) {
	for _, op := range []string{"popular", "category"} {
		t.Run(op, func(t *testing.T) {
			reg := provider.NewRegistry()
			reg.Register(&rcPanicProvider{id: "boom", op: op})
			reg.Register(&rcPanicProvider{id: "boom2", op: op})
			healthy := &rcFixedProvider{id: "good", items: []domain.MediaItem{
				{ID: "1", ProviderID: "good", Title: "Дюна", Type: "movie"},
			}}
			reg.Register(healthy)

			var (
				items []domain.MediaItem
				err   error
			)
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Fatalf("a panicking provider escaped the %s fan-out: %v", op, rec)
					}
				}()
				if op == "popular" {
					items, err = reg.Popular(context.Background(), "", "", 1)
				} else {
					items, err = reg.Category(context.Background(), "", "comedy", "", 1)
				}
			}()

			if err != nil {
				t.Fatalf("%s: err = %v, want the healthy providers' results", op, err)
			}
			if len(items) != 1 || items[0].Title != "Дюна" {
				t.Errorf("%s: got %+v, want the surviving provider's item", op, items)
			}
			// The panic must be recorded as unhealthy, not silently forgotten.
			cat := reg.Catalog()
			panicked := 0
			for _, e := range cat.Providers {
				if e.ID == "boom" || e.ID == "boom2" {
					if !e.Healthy {
						panicked++
					}
				}
			}
			if panicked != 2 {
				t.Errorf("panicking providers recorded healthy: %d of 2", panicked)
			}
		})
	}
}

// With an explicit provider the panic must surface as ErrProviderPanic, so the
// HTTP layer can still answer 500 with a JSON body.
func TestRegistryExplicitProviderPanicIsATypedError(t *testing.T) {
	for _, op := range []string{"popular", "category"} {
		t.Run(op, func(t *testing.T) {
			reg := provider.NewRegistry()
			reg.Register(&rcPanicProvider{id: "boom", op: op})

			var err error
			if op == "popular" {
				_, err = reg.Popular(context.Background(), "boom", "", 1)
			} else {
				_, err = reg.Category(context.Background(), "boom", "x", "", 1)
			}
			if !errors.Is(err, provider.ErrProviderPanic) {
				t.Errorf("%s: err = %v, want ErrProviderPanic", op, err)
			}
		})
	}
}

// An empty aggregate must be an empty slice, never nil: nil serialises as JSON
// null, which commit 0bd7d00 fixed everywhere except these two paths.
func TestRegistryEmptyAggregatesAreNotNil(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&rcFixedProvider{id: "p1"})
	reg.Register(&rcFixedProvider{id: "p2"})

	popular, err := reg.Popular(context.Background(), "", "", 1)
	if err != nil {
		t.Fatalf("Popular: %v", err)
	}
	if popular == nil {
		t.Error("Popular returned a nil slice, which serialises as null")
	}
	if len(popular) != 0 {
		t.Errorf("Popular returned %d items, want 0", len(popular))
	}

	category, err := reg.Category(context.Background(), "", "x", "", 1)
	if err != nil {
		t.Fatalf("Category: %v", err)
	}
	if category == nil {
		t.Error("Category returned a nil slice, which serialises as null")
	}

	// The single-provider paths have the same obligation.
	one, err := reg.Popular(context.Background(), "p1", "", 1)
	if err != nil {
		t.Fatalf("Popular(p1): %v", err)
	}
	if one == nil {
		t.Error("Popular with an explicit provider returned nil")
	}

	// And so does the search fan-out.
	items := reg.SearchAll(context.Background(), "anything")
	if items == nil {
		t.Error("SearchAll returned a nil slice, which serialises as null")
	}
}

// --- doubles ---------------------------------------------------------------

type rcPanicProvider struct {
	id  string
	op  string
	rec sync.Mutex
	log []string
}

func (p *rcPanicProvider) ID() string   { return p.id }
func (p *rcPanicProvider) Name() string { return "rc-panic-" + p.id }
func (p *rcPanicProvider) BaseURL() string {
	return "https://" + p.id + ".example.com"
}
func (p *rcPanicProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), ShowOnHome: true}
}

func (p *rcPanicProvider) Search(context.Context, string) ([]domain.MediaItem, error) {
	panic("rc: Search panic")
}

func (p *rcPanicProvider) GetPopular(context.Context, string, int) ([]domain.MediaItem, error) {
	if p.op == "popular" {
		panic("rc: GetPopular panic")
	}
	return nil, nil
}

func (p *rcPanicProvider) GetByCategory(context.Context, string, string, int) ([]domain.MediaItem, error) {
	if p.op == "category" {
		panic("rc: GetByCategory panic")
	}
	return nil, nil
}

func (p *rcPanicProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	panic("rc: GetDetails panic")
}

func (p *rcPanicProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	panic("rc: GetStreams panic")
}

type rcFixedProvider struct {
	id    string
	items []domain.MediaItem
}

func (p *rcFixedProvider) ID() string   { return p.id }
func (p *rcFixedProvider) Name() string { return "rc-" + p.id }
func (p *rcFixedProvider) BaseURL() string {
	return "https://" + p.id + ".example.com"
}
func (p *rcFixedProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), ShowOnHome: true}
}

func (p *rcFixedProvider) Search(context.Context, string) ([]domain.MediaItem, error) {
	return p.items, nil
}
func (p *rcFixedProvider) GetPopular(context.Context, string, int) ([]domain.MediaItem, error) {
	return p.items, nil
}
func (p *rcFixedProvider) GetByCategory(context.Context, string, string, int) ([]domain.MediaItem, error) {
	return p.items, nil
}
func (p *rcFixedProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	return &domain.MediaDetails{MediaItem: domain.MediaItem{ID: "1", ProviderID: p.id}}, nil
}
func (p *rcFixedProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	return &domain.ContentStreamsResponse{ProviderID: p.id}, nil
}

func waitForCalls(t *testing.T, p *rcBlockingProvider, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&p.calls) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("provider was not called %d times within the deadline", want)
}
