package provider_test

import (
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestCovEneyidaIdentity(t *testing.T) {
	p := provider.NewEneyidaProvider(nil)
	if p.ID() != "eneyida" {
		t.Errorf("expected ID eneyida, got %q", p.ID())
	}
	if p.Name() != "Eneyida" {
		t.Errorf("expected Name Eneyida, got %q", p.Name())
	}
	if p.BaseURL() != "https://eneyida.tv" {
		t.Errorf("expected BaseURL https://eneyida.tv, got %q", p.BaseURL())
	}
}
