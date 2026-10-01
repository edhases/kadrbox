package provider

// Catalogue endpoint coverage for the DLE providers, and the regression test
// for a bug the production logs exposed.
//
// The logs showed Eneyida answering `GET /content/popular` with 200 and a
// 5-byte body. Five bytes is `null` followed by a newline: the parsers
// declared `var items []domain.MediaItem`, and json.Marshal renders a nil
// slice as `null` rather than `[]`. So a source with nothing to offer was
// indistinguishable on the wire from a source that returned a broken payload,
// and every consumer had to special-case null. These tests pin that an empty
// catalogue serialises as an empty JSON array.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
)

// realClient builds the same TLS client production uses. The parsers dereference
// it unconditionally, so a nil client would panic rather than fail.
func realClient(t *testing.T) *TLSClient {
	t.Helper()
	c, err := NewTLSClient()
	if err != nil {
		t.Skipf("TLS client unavailable: %v", err)
	}
	return c
}

// catalogServer serves a fixed body and records the paths it was asked for.
func catalogServer(t *testing.T, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestEneyidaEmptyCatalogSerialisesAsArray(t *testing.T) {
	// A page whose markup does not match the parser's selectors at all.
	srv, _ := catalogServer(t, `<html><body><p>Каталог порожній</p></body></html>`)

	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items, got %d", len(items))
	}

	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != "[]" {
		t.Errorf("empty catalogue encoded as %q, want %q — a nil slice "+
			"marshals to null and clients cannot read that as a list", string(encoded), "[]")
	}
}

func TestUakinoEmptyCatalogSerialisesAsArray(t *testing.T) {
	srv, _ := catalogServer(t, `<html><body><p>Нічого</p></body></html>`)

	p := NewUakinoProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	encoded, _ := json.Marshal(items)
	if string(encoded) != "[]" {
		t.Errorf("empty catalogue encoded as %q, want %q", string(encoded), "[]")
	}
}

func TestLavakinoEmptyCatalogSerialisesAsArray(t *testing.T) {
	srv, _ := catalogServer(t, `<html><body><p>Нічого</p></body></html>`)

	p := NewLavakinoProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	encoded, _ := json.Marshal(items)
	if string(encoded) != "[]" {
		t.Errorf("empty catalogue encoded as %q, want %q", string(encoded), "[]")
	}
}

// ---- section routing --------------------------------------------------------

func TestEneyidaGetSection(t *testing.T) {
	cases := []struct {
		contentType string
		want        string
	}{
		{"movie", "films"},
		{"series", "serials"},
		{"cartoon", "multfilmy"},
		{"anime", "anime"},
		{"", ""},
		{"unknown-type", ""},
		{"Movie", ""}, // matching is case-sensitive; an odd type must not route
	}
	p := NewEneyidaProvider(realClient(t))
	for _, tc := range cases {
		t.Run("type="+tc.contentType, func(t *testing.T) {
			if got := p.getSection(tc.contentType); got != tc.want {
				t.Errorf("getSection(%q) = %q, want %q", tc.contentType, got, tc.want)
			}
		})
	}
}

// The first page and later pages must hit different paths, otherwise
// pagination silently repeats page one forever.
func TestEneyidaPopularRoutesToDistinctPaths(t *testing.T) {
	srv, seen := catalogServer(t, `<html><body></body></html>`)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	if _, err := p.GetPopular(context.Background(), "movie", 1); err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if _, err := p.GetPopular(context.Background(), "movie", 4); err != nil {
		t.Fatalf("page 4: %v", err)
	}
	if _, err := p.GetPopular(context.Background(), "", 3); err != nil {
		t.Fatalf("untyped page 3: %v", err)
	}
	if _, err := p.GetPopular(context.Background(), "movie", 0); err != nil {
		t.Fatalf("page 0 should clamp: %v", err)
	}

	want := []string{"/films/", "/films/page/4/", "/page/3/", "/films/"}
	if len(*seen) != len(want) {
		t.Fatalf("requested %v, want %v", *seen, want)
	}
	for i := range want {
		if (*seen)[i] != want[i] {
			t.Errorf("request %d: path = %q, want %q", i, (*seen)[i], want[i])
		}
	}
}

func TestEneyidaGetByCategoryFallsBackToPopular(t *testing.T) {
	srv, seen := catalogServer(t, `<html><body></body></html>`)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	if _, err := p.GetByCategory(context.Background(), "", "series", 1); err != nil {
		t.Fatalf("GetByCategory: %v", err)
	}
	if len(*seen) != 1 || !strings.Contains((*seen)[0], "serials") {
		t.Errorf("an empty category should reuse the type section, requested %v", *seen)
	}

	if _, err := p.GetByCategory(context.Background(), "comedy", "movie", 1); err != nil {
		t.Fatalf("GetByCategory comedy: %v", err)
	}
	if (*seen)[1] != "/comedy/" {
		t.Errorf("category path = %q, want /comedy/", (*seen)[1])
	}
}

func TestEneyidaDescribe(t *testing.T) {
	desc := NewEneyidaProvider(realClient(t)).Describe()

	if desc.ID != "eneyida" {
		t.Errorf("ID = %q, want eneyida", desc.ID)
	}
	if desc.BaseURL != "https://eneyida.tv" {
		t.Errorf("BaseURL = %q, want https://eneyida.tv", desc.BaseURL)
	}
	if !desc.ShowOnHome {
		t.Error("eneyida should appear on the home screen")
	}
	if desc.HasFixedStreams {
		t.Error("eneyida has no fixed streams")
	}
	if !desc.SearchEnabledDefault {
		t.Error("eneyida search should be enabled by default")
	}
	for _, want := range []string{"movie", "series", "cartoon", "anime"} {
		found := false
		for _, got := range desc.ContentTypes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ContentTypes is missing %q, got %v", want, desc.ContentTypes)
		}
	}
}

// A transport failure must surface as an error, never as an empty catalogue:
// a broken scraper and a source with nothing to show are different states.
func TestEneyidaTransportFailureIsNotAnEmptyCatalog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err == nil {
		t.Fatalf("expected an error, got items %v", items)
	}
}

func TestEneyidaSearchHitsSearchEndpoint(t *testing.T) {
	srv, seen := catalogServer(t, `<html><body></body></html>`)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	if _, err := p.Search(context.Background(), "Матриця & Ренесанс"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("expected one request, got %v", *seen)
	}
	if !strings.Contains((*seen)[0], "/index.php") {
		t.Errorf("search path = %q, want the DLE search endpoint", (*seen)[0])
	}
}

// ---- parsing ---------------------------------------------------------------

func TestEneyidaParsesCatalogMarkup(t *testing.T) {
	body := `<html><body>
      <article class="short">
        <div class="short_img"><img src="/img/dune.jpg"></div>
        <h2 class="short_title"><a href="/films/dune-2021.html">Дюна</a></h2>
        <div class="short_info">2021</div>
      </article>
      <article class="short">
        <div class="short_img"><img data-src="/img/serial.jpg"></div>
        <h2 class="short_title"><a href="/serials/fargo.html">Фарґо</a></h2>
        <div class="short_info">2014</div>
      </article>
    </body></html>`
	srv, _ := catalogServer(t, body)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}

	first := items[0]
	if first.Title != "Дюна" {
		t.Errorf("title = %q, want Дюна", first.Title)
	}
	if first.Year != 2021 {
		t.Errorf("year = %d, want 2021", first.Year)
	}
	if first.PosterURL != srv.URL+"/img/dune.jpg" {
		t.Errorf("poster = %q, want the absolute URL", first.PosterURL)
	}
	if first.ProviderID != "eneyida" {
		t.Errorf("providerID = %q, want eneyida", first.ProviderID)
	}
	if first.ID != "/films/dune-2021.html" || first.URL != "/films/dune-2021.html" {
		t.Errorf("href should become both id and url, got id=%q url=%q", first.ID, first.URL)
	}

	// Type is inferred from the href, and data-src is honoured for lazy images.
	if items[1].Type != "series" {
		t.Errorf("serial href should yield type series, got %q", items[1].Type)
	}
	if items[1].PosterURL != srv.URL+"/img/serial.jpg" {
		t.Errorf("data-src poster not used: %q", items[1].PosterURL)
	}
}

// Entries without a usable link or title must be skipped rather than emitted
// as blank cards.
func TestEneyidaSkipsUnusableEntries(t *testing.T) {
	body := `<html><body>
      <article class="short">
        <h2 class="short_title"><a>Без href</a></h2>
      </article>
      <article class="short">
        <h2 class="short_title"><a href="/films/x.html">   </a></h2>
      </article>
      <article class="short">
        <h2 class="short_title"><a href="/films/ok.html">Нормальний</a></h2>
      </article>
    </body></html>`
	srv, _ := catalogServer(t, body)
	p := NewEneyidaProvider(realClient(t))
	p.baseURL = srv.URL

	items, err := p.GetPopular(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("GetPopular: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Нормальний" {
		t.Errorf("expected only the usable entry, got %+v", items)
	}
}

var _ = domain.MediaItem{}
