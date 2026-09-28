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
	if p.BaseURL() != "https://uakino.best" {
		t.Errorf("expected BaseURL 'https://uakino.best', got '%s'", p.BaseURL())
	}
}
