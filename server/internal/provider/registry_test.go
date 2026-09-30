package provider_test

import (
	"context"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
)

type dummyProvider struct {
	id   string
	name string
}

func (d *dummyProvider) ID() string   { return d.id }
func (d *dummyProvider) Name() string { return d.name }
func (d *dummyProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: d.id, Name: d.name, ShowOnHome: true, SearchEnabledDefault: true}
}
func (d *dummyProvider) BaseURL() string { return "http://dummy" }
func (d *dummyProvider) Search(ctx context.Context, query string) ([]domain.MediaItem, error) {
	return []domain.MediaItem{
		{ID: "1", ProviderID: d.id, Title: query + " from " + d.name},
	}, nil
}
func (d *dummyProvider) GetDetails(ctx context.Context, itemURL string) (*domain.MediaDetails, error) {
	return nil, nil
}
func (d *dummyProvider) GetStreams(ctx context.Context, itemURL string, season, episode int, voiceID string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

func TestRegistrySearchAndSingleFlight(t *testing.T) {
	reg := provider.NewRegistry()
	reg.Register(&dummyProvider{id: "p1", name: "Provider 1"})
	reg.Register(&dummyProvider{id: "p2", name: "Provider 2"})

	if len(reg.List()) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(reg.List()))
	}

	p1, ok := reg.Get("p1")
	if !ok || p1.Name() != "Provider 1" {
		t.Errorf("failed to get p1 from registry")
	}

	// Тестування паралельного пошуку
	results, err := reg.SingleFlightSearch(context.Background(), "Avatar")
	if err != nil {
		t.Fatalf("singleflight search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 aggregated results, got %d", len(results))
	}
}
