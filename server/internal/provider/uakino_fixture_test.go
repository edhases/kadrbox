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

// The fixture lives under testdata/ rather than in the Dart tree's
// test/fixtures/parsers/ for two reasons. Go's convention puts testdata next to
// the package, and the server CI job only triggers on paths under server/** —
// with the fixture outside the module, a commit that touched only the fixture
// ran zero Go tests, so a parser change and the fixture that should have caught
// it were reviewed in isolation.
//
// t.Fatalf rather than t.Skipf on a missing fixture: a skip makes a deleted or
// renamed fixture a green build, which is exactly the failure this test exists
// to prevent.
func TestUakino_ParsesCatalogFixture(t *testing.T) {
	fixturePath := filepath.Join("testdata", "uakino_catalog.html")
	htmlBytes, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("cannot read fixture %s: %v", fixturePath, err)
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

	// Five cards carry an href; the sixth has none and must be skipped.
	if len(items) != 5 {
		t.Fatalf("expected 5 items parsed from fixture, got %d: %+v", len(items), items)
	}

	tests := []struct {
		idx        int
		title      string
		year       int
		posterPath string
		mediaType  string
		id         string
	}{
		{
			idx: 0, title: "Sample Movie", year: 2021,
			posterPath: "/uploads/posts/sample.jpg", mediaType: "movie",
			id: "https://uakino.biz/1-sample-movie.html",
		},
		{
			// Two anchors in .movie-title. Without .First() the title came out
			// as "Дюна: Частина друга Фантастика" paired with the first href.
			idx: 1, title: "Дюна: Частина друга", year: 2024,
			posterPath: "/uploads/posts/dune-2.jpg", mediaType: "movie",
			id: "https://uakino.biz/123-dune-chastyna-druha.html",
		},
		{
			// Poster only in data-src: the lazy-loaded case.
			idx: 2, title: "Атака титанів", year: 2013,
			posterPath: "/uploads/posts/shingeki.jpg", mediaType: "movie",
			id: "https://uakino.biz/456-ataka-titanov.html",
		},
		{
			idx: 3, title: "Гравіті Фоллз", year: 2012,
			posterPath: "/uploads/posts/gravity-falls.jpg", mediaType: "series",
			id: "https://uakino.biz/serials/789-gravity-falls.html",
		},
		{
			idx: 4, title: "Людина-паук", year: 2018,
			posterPath: "/uploads/posts/spiderman.jpg", mediaType: "cartoon",
			id: "https://uakino.biz/cartoon/890-spiderman.html",
		},
	}

	for _, tc := range tests {
		got := items[tc.idx]
		if got.Title != tc.title {
			t.Errorf("item %d: title = %q, want %q", tc.idx, got.Title, tc.title)
		}
		if got.Year != tc.year {
			t.Errorf("item %d (%s): year = %d, want %d", tc.idx, tc.title, got.Year, tc.year)
		}
		if got.Type != tc.mediaType {
			t.Errorf("item %d (%s): type = %q, want %q", tc.idx, tc.title, got.Type, tc.mediaType)
		}
		// The fixture's poster is root-relative, so this asserts the join.
		if got.PosterURL != server.URL+tc.posterPath {
			t.Errorf("item %d (%s): poster = %q, want %q", tc.idx, tc.title, got.PosterURL, server.URL+tc.posterPath)
		}
		// href is already absolute in the fixture, so the provider must pass it
		// through untouched rather than re-joining it onto the base URL.
		if got.ID != tc.id {
			t.Errorf("item %d (%s): id = %q, want %q", tc.idx, tc.title, got.ID, tc.id)
		}
		if got.URL != got.ID {
			t.Errorf("item %d (%s): url %q should equal id %q", tc.idx, tc.title, got.URL, got.ID)
		}
		if got.ProviderID != "uakino" {
			t.Errorf("item %d (%s): providerID = %q, want uakino", tc.idx, tc.title, got.ProviderID)
		}
	}
}
