package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLavakinoProviderBasic(t *testing.T) {
	p := NewLavakinoProvider(nil)
	if p.ID() != "lavakino" {
		t.Errorf("expected ID 'lavakino', got '%s'", p.ID())
	}
	if p.Name() != "Lavakino" {
		t.Errorf("expected Name 'Lavakino', got '%s'", p.Name())
	}
	if p.BaseURL() != "https://lavakino.net" {
		t.Errorf("expected BaseURL 'https://lavakino.net', got '%s'", p.BaseURL())
	}
	desc := p.Describe()
	if !desc.ShowOnHome {
		t.Errorf("expected ShowOnHome true")
	}
	if !desc.SearchEnabledDefault {
		t.Errorf("expected SearchEnabledDefault true")
	}
}

func TestLavakinoGetSection(t *testing.T) {
	p := NewLavakinoProvider(nil)
	tests := []struct {
		contentType string
		wantSection string
	}{
		{"movie", "filmys"},
		{"series", "serialy"},
		{"cartoon", "cartoonss"},
		{"anime", "anime"},
		{"other", ""},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := p.getSection(tt.contentType)
			if got != tt.wantSection {
				t.Errorf("getSection(%q) = %q, want %q", tt.contentType, got, tt.wantSection)
			}
		})
	}
}

func TestLavakino404ReturnsEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	tls := covTLS(t)
	p := &LavakinoProvider{
		client:  tls,
		baseURL: srv.URL,
	}

	items, err := p.GetPopular(context.Background(), "movie", 1)
	if err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items on 404, got %d", len(items))
	}

	catItems, err := p.GetByCategory(context.Background(), "drama", "movie", 1)
	if err != nil {
		t.Fatalf("expected nil error on 404, got %v", err)
	}
	if len(catItems) != 0 {
		t.Fatalf("expected 0 items on 404, got %d", len(catItems))
	}
}

