package provider_test

import (
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestLavakinoProviderBasic(t *testing.T) {
	p := provider.NewLavakinoProvider(nil)
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
