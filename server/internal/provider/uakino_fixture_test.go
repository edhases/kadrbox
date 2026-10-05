package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestUakino_ParsesCatalogFixture(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "test", "fixtures", "parsers", "uakino_sample.html")
	htmlBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("fixture file %s not found: %v", fixturePath, err)
		return
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(htmlBytes)
	}))
	defer server.Close()

	p := provider.NewUakinoProviderWithConfig(server.URL, nil)
	items, err := p.GetPopular(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("GetPopular failed with fixture: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item parsed from fixture, got %d", len(items))
	}
	if items[0].Title != "Sample Movie" {
		t.Errorf("expected title 'Sample Movie', got %q", items[0].Title)
	}
	if items[0].Year != 2021 {
		t.Errorf("expected year 2021, got %d", items[0].Year)
	}
}
