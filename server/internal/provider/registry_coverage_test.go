package provider_test

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

type covErrProvider struct {
	id string
}

func (p *covErrProvider) ID() string      { return p.id }
func (p *covErrProvider) Name() string    { return p.id }
func (p *covErrProvider) BaseURL() string { return "http://cov-err" }
func (p *covErrProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	return nil, errors.New("cov search boom")
}
func (p *covErrProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	return nil, nil
}
func (p *covErrProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

type covCountingProvider struct {
	id    string
	count atomic.Int64
	delay time.Duration
}

func (p *covCountingProvider) ID() string      { return p.id }
func (p *covCountingProvider) Name() string    { return p.id }
func (p *covCountingProvider) BaseURL() string { return "http://cov-count" }
func (p *covCountingProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	p.count.Add(1)
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	return []domain.MediaItem{{ID: "1", ProviderID: p.id, Title: query}}, nil
}
func (p *covCountingProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	return nil, nil
}
func (p *covCountingProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

func TestCovRegistryGetUnknown(t *testing.T) {
	reg := provider.NewRegistry()
	if _, ok := reg.Get("nope"); ok {
		t.Errorf("expected ok==false for unknown id")
	}
}

func TestCovRegistryRegisterOverwrites(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&dummyProvider{id: "dup", name: "first"})
	reg.Register(&dummyProvider{id: "dup", name: "second"})

	got, ok := reg.Get("dup")
	if !ok {
		t.Fatalf("expected to find id dup")
	}
	if got.Name() != "second" {
		t.Errorf("expected last registration to win (second), got %q", got.Name())
	}
	if len(reg.List()) != 1 {
		t.Errorf("expected 1 provider after overwrite, got %d", len(reg.List()))
	}
}

func TestCovRegistryListCopyIndependent(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&dummyProvider{id: "p1", name: "Provider 1"})

	list := reg.List()
	list = append(list, &dummyProvider{id: "evil", name: "Evil"})

	if len(reg.List()) != 1 {
		t.Errorf("append to returned slice must not affect registry, got %d providers", len(reg.List()))
	}
	if _, ok := reg.Get("evil"); ok {
		t.Errorf("appended provider must not appear in registry")
	}
}

func TestCovRegistrySearchAllSkipsErrors(t *testing.T) {
	// ХАРАКТЕРИЗАЦІЯ: SearchAll ковтає помилки провайдерів і не повертає error —
	// результат містить тільки елементи живого провайдера.
	reg := provider.NewRegistry()
	reg.Register(&covErrProvider{id: "broken"})
	reg.Register(&dummyProvider{id: "alive", name: "Alive"})

	got := reg.SearchAll(context.Background(), "q")
	if len(got) != 1 {
		t.Fatalf("expected 1 result from live provider, got %d", len(got))
	}
	if got[0].ProviderID != "alive" {
		t.Errorf("expected result from alive provider, got %q", got[0].ProviderID)
	}
}

func TestCovRegistrySingleFlightDeduplicates(t *testing.T) {
	reg := provider.NewRegistry()
	cp := &covCountingProvider{id: "c", delay: 150 * time.Millisecond}
	reg.Register(cp)

	const n = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _ = reg.SingleFlightSearch(context.Background(), "same-query")
		}()
	}
	close(start)
	wg.Wait()

	if got := cp.count.Load(); got != 1 {
		t.Errorf("expected Search called once for identical concurrent queries, got %d", got)
	}
}

func TestCovRegistrySingleFlightDifferentQueries(t *testing.T) {
	reg := provider.NewRegistry()
	cp := &covCountingProvider{id: "c"}
	reg.Register(cp)

	if _, err := reg.SingleFlightSearch(context.Background(), "query-one"); err != nil {
		t.Fatalf("SingleFlightSearch failed: %v", err)
	}
	if _, err := reg.SingleFlightSearch(context.Background(), "query-two"); err != nil {
		t.Fatalf("SingleFlightSearch failed: %v", err)
	}

	if got := cp.count.Load(); got != 2 {
		t.Errorf("expected 2 Search calls for different queries, got %d", got)
	}
}

func TestCovRegistryConcurrentSearch(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&covCountingProvider{id: "c"})
	reg.Register(&dummyProvider{id: "d", name: "D"})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = reg.SingleFlightSearch(context.Background(), "q")
		}()
	}
	wg.Wait()
}
