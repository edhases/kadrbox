package provider_test

import (
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

// Живої мережевої частини тут більше немає: запит до uakino.best флакає в CI
// (геоблок/зміна домену) і при помилці старий тест робив Logf+return, тобто
// нічого не асертував — цінності нуль. Лише офлайн-перевірка ідентичності.
func TestUakinoProviderBasicAndSearch(t *testing.T) {
	p := provider.NewUakinoProvider(nil)
	if p.ID() != "uakino" {
		t.Errorf("expected ID 'uakino', got '%s'", p.ID())
	}
	if p.Name() != "UAKino" {
		t.Errorf("expected Name 'UAKino', got '%s'", p.Name())
	}
	if p.BaseURL() != "https://uakino.biz" {
		t.Errorf("expected BaseURL 'https://uakino.biz', got '%s'", p.BaseURL())
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"relative with slash", "/uploads/mini/serial/de/poster.jpg", "https://uakino.biz/uploads/mini/serial/de/poster.jpg"},
		{"relative without slash", "uploads/mini/serial/de/poster.jpg", "https://uakino.biz/uploads/mini/serial/de/poster.jpg"},
		{"protocol relative", "//uakino.biz/uploads/poster.jpg", "https://uakino.biz/uploads/poster.jpg"},
		{"absolute https", "https://example.com/poster.jpg", "https://example.com/poster.jpg"},
		{"absolute http", "http://example.com/poster.jpg", "http://example.com/poster.jpg"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.ResolvePosterURL(tc.input)
			if got != tc.expected {
				t.Errorf("ResolvePosterURL(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

