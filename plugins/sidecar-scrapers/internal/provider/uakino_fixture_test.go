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
			// Рік на живому сайті лежить у рядку «Рік виходу:» з
			// посиланням /find/year/YYYY/. Старий селектор .movie-date
			// на uakino.biz не трапляється взагалі, тож рік був 0.
			idx: 0, title: "Погані вожаті", year: 2026,
			posterPath: "/uploads/mini/poster/9d/b2eec283.webp", mediaType: "movie",
			id: "https://uakino.biz/filmy/genre_comedy/36141-seredina-90.html",
		},
		{
			// /animeukr/anime-series/ містить і «anime», і «series».
			// Перевірка anime має бути першою, інакше картка
			// прочиталася б як серіал.
			idx: 1, title: "Агенти часу", year: 2025,
			posterPath: "/uploads/mini/poster/7c/animated.webp", mediaType: "anime",
			id: "https://uakino.biz/animeukr/anime-series/35681-seredina-90.html",
		},
		{
			// Poster only in data-src: the lazy-loaded case.
			idx: 2, title: "Снігопад", year: 2019,
			posterPath: "/uploads/mini/poster/aa/snowfall.webp", mediaType: "series",
			id: "https://uakino.biz/seriesss/drama_series/35629-seredina-90.html",
		},
		{
			// /cartoon/cartoonseries/ містить і «cartoon», і «series» —
			// та сама вимога до порядку перевірок.
			idx: 3, title: "Малюк Спіру", year: 2004,
			posterPath: "/uploads/mini/poster/bb/spiroul.webp", mediaType: "cartoon",
			id: "https://uakino.biz/cartoon/cartoonseries/36136-seredina-90.html",
		},
		{
			// Корінь без маркера розділу: тип лишається movie.
			// Такі URL бувають у результатах пошуку.
			idx: 4, title: "Дюна: Частина друга", year: 2024,
			posterPath: "/uploads/mini/poster/dd/dune.webp", mediaType: "movie",
			id: "https://uakino.biz/123-dune-chastyna-druha.html",
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
