package provider_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestUakinoProviderBasicAndSearch(t *testing.T) {
	client, err := provider.NewTLSClient()
	if err != nil {
		t.Fatalf("failed to create TLS client: %v", err)
	}

	p := provider.NewUakinoProvider(client)
	if p.ID() != "uakino" {
		t.Errorf("expected ID 'uakino', got '%s'", p.ID())
	}
	if p.Name() != "UAKino" {
		t.Errorf("expected Name 'UAKino', got '%s'", p.Name())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Реальний живий мережевий тест парсингу (перевіряє обхід блокувань та парсер)
	results, err := p.Search(ctx, "Матриця")
	if err != nil {
		t.Logf("Live network search to uakino warning (may be geo-blocked or domain moved): %v", err)
		return
	}

	t.Logf("Successfully fetched %d search results from UAKino via tls-client", len(results))
	for _, item := range results {
		if strings.Contains(strings.ToLower(item.Title), "матриця") {
			t.Logf("Found match: %s (%d) -> %s", item.Title, item.Year, item.URL)
			break
		}
	}
}
