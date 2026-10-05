package provider_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

func TestGenerateStableContentID_CaseAndJSONInvariance(t *testing.T) {
	ref1 := json.RawMessage(`{"source":"uaserials","id":"123","season":1}`)
	ref2 := json.RawMessage(`{ "season": 1, "source": "uaserials",  "id": "123" }`)

	id1 := provider.GenerateStableContentID("uaserials", "Дюна", 2021, ref1)
	id2 := provider.GenerateStableContentID("uaserials", "дюна", 2021, ref2)
	id3 := provider.GenerateStableContentID("uaserials", "  ДЮНА  ", 2021, ref1)

	if id1 == "" {
		t.Fatal("GenerateStableContentID produced empty id")
	}
	if id1 != id2 {
		t.Errorf("IDs do not match across casing/json order: id1=%q, id2=%q", id1, id2)
	}
	if id1 != id3 {
		t.Errorf("IDs do not match across whitespace/casing: id1=%q, id3=%q", id1, id3)
	}

	// Verify golden stability
	expectedPrefix := "bo_uaserials_"
	if len(id1) < len(expectedPrefix) || id1[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("expected ID prefix %q, got %q", expectedPrefix, id1)
	}
}

func TestBanderaPagination_Bounded(t *testing.T) {
	p := provider.NewBanderaProvider()
	ctx := context.Background()

	// For popular movies, queries length is 5. Page 6 must return empty, not wrap around to page 1.
	items, err := p.GetPopular(ctx, "movie", 6)
	if err != nil {
		t.Fatalf("GetPopular returned unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items for out-of-range page 6, got %d", len(items))
	}

	// For category, page 2 must return empty.
	catItems, err := p.GetByCategory(ctx, "action", "movie", 2)
	if err != nil {
		t.Fatalf("GetByCategory returned unexpected error: %v", err)
	}
	if len(catItems) != 0 {
		t.Errorf("expected 0 items for category page 2, got %d", len(catItems))
	}
}
