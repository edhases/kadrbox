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

// TestGenerateStableContentID_GoldenValue pins the exact output. The ID is
// persisted client-side as MediaItem.uniqueId and as the server's
// (media_id, provider_id) key, so a change to the hash input, the separator or
// the truncation length silently re-keys every favourites and history row that
// was written before the change. The invariance test above cannot catch that —
// it only proves the inputs agree with each other, not that the output is
// unchanged. Changing this value is therefore a data migration, not a refactor.
func TestGenerateStableContentID_GoldenValue(t *testing.T) {
	ref := json.RawMessage(`{"id":"123","season":1,"source":"uaserials"}`)

	got := provider.GenerateStableContentID("uaserials", "Дюна", 2021, ref)
	const want = "bo_uaserials_ac862563e6"

	if got != want {
		t.Errorf("stable content ID changed.\n got: %s\nwant: %s\n"+
			"this ID is persisted in MediaItem.uniqueId and in the server's "+
			"(media_id, provider_id) key; changing it orphans existing rows and "+
			"requires a data migration", got, want)
	}
}

// TestGenerateStableContentID_DistinguishesRealInputs guards the opposite
// failure: a golden value alone would also pass if the hash stopped
// discriminating between two genuinely different items.
func TestGenerateStableContentID_DistinguishesRealInputs(t *testing.T) {
	ref := json.RawMessage(`{"id":"123","season":1,"source":"uaserials"}`)
	other := json.RawMessage(`{"id":"124","season":1,"source":"uaserials"}`)

	base := provider.GenerateStableContentID("uaserials", "Дюна", 2021, ref)
	cases := []struct {
		name string
		got  string
	}{
		{"different source", provider.GenerateStableContentID("uakino", "Дюна", 2021, ref)},
		{"different title", provider.GenerateStableContentID("uaserials", "Дюна 2", 2021, ref)},
		{"different year", provider.GenerateStableContentID("uaserials", "Дюна", 2022, ref)},
		{"different ref", provider.GenerateStableContentID("uaserials", "Дюна", 2021, other)},
	}
	for _, tc := range cases {
		if tc.got == base {
			t.Errorf("%s produced the same ID as the base item: %s", tc.name, tc.got)
		}
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
