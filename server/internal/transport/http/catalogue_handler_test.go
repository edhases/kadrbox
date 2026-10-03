package http_test

// Catalogue endpoint coverage: /content/popular and /content/category.
//
// These two handlers were entirely uncovered even though the app polls them on
// every home-page load, and the production logs showed real traffic hitting
// them repeatedly. They take no dependency beyond the provider registry, so
// they can be exercised end to end through httptest with in-process fakes —
// no database, no network.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

// catProvider is a provider that records the catalogue arguments it received,
// so the handler's parameter plumbing can be asserted rather than assumed.
type catProvider struct {
	id string

	popular    []domain.MediaItem
	byCat      []domain.MediaItem
	popularErr error
	catErr     error

	gotPopularContentType string
	gotPopularPage        int
	gotCategoryName       string
	gotCategoryType       string
	gotCategoryPage       int
}

func (p *catProvider) ID() string   { return p.id }
func (p *catProvider) Name() string { return "cat-" + p.id }
func (p *catProvider) BaseURL() string {
	return "https://" + p.id + ".example.com"
}
func (p *catProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), BaseURL: p.BaseURL()}
}

func (p *catProvider) Search(context.Context, string) ([]domain.MediaItem, error) {
	return nil, nil
}

func (p *catProvider) GetPopular(_ context.Context, contentType string, page int) ([]domain.MediaItem, error) {
	p.gotPopularContentType = contentType
	p.gotPopularPage = page
	return p.popular, p.popularErr
}

func (p *catProvider) GetNew(context.Context, string, int) ([]domain.MediaItem, error) {
	return nil, nil
}

func (p *catProvider) GetByCategory(_ context.Context, category, contentType string, page int) ([]domain.MediaItem, error) {
	p.gotCategoryName = category
	p.gotCategoryType = contentType
	p.gotCategoryPage = page
	return p.byCat, p.catErr
}

func (p *catProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	return nil, nil
}

func (p *catProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	return nil, nil
}

// panicProvider panics inside every catalogue call, to prove the handlers'
// recover() behaves.
type panicProvider struct{ id string }

func (p *panicProvider) ID() string      { return p.id }
func (p *panicProvider) Name() string    { return "panic-" + p.id }
func (p *panicProvider) BaseURL() string { return "https://panic.example" }
func (p *panicProvider) Describe() domain.ProviderInfo {
	return domain.ProviderInfo{ID: p.id, Name: p.Name(), BaseURL: p.BaseURL()}
}

func (p *panicProvider) Search(context.Context, string) ([]domain.MediaItem, error) {
	panic("boom")
}
func (p *panicProvider) GetPopular(context.Context, string, int) ([]domain.MediaItem, error) {
	panic("boom")
}
func (p *panicProvider) GetNew(context.Context, string, int) ([]domain.MediaItem, error) {
	panic("boom")
}
func (p *panicProvider) GetByCategory(context.Context, string, string, int) ([]domain.MediaItem, error) {
	panic("boom")
}
func (p *panicProvider) GetDetails(context.Context, string) (*domain.MediaDetails, error) {
	panic("boom")
}
func (p *panicProvider) GetStreams(context.Context, string, int, int, string) (*domain.ContentStreamsResponse, error) {
	panic("boom")
}

// catRegistry builds a content handler over a registry containing the given
// providers. The cache repository is unused by these two handlers, so nil is
// passed rather than standing up a database.
func catRegistry(ps ...domain.Provider) (*transporthttp.ContentHandler, *provider.Registry) {
	reg := provider.NewRegistry()
	for _, p := range ps {
		reg.Register(p)
	}
	return transporthttp.NewContentHandler(reg, nil), reg
}

// decodeItems unwraps the list envelope. The endpoints answer
// {"data":[…],"meta":{…}}; a bare JSON array was the pre-envelope contract.
func decodeItems(t *testing.T, rr *httptest.ResponseRecorder) []domain.MediaItem {
	t.Helper()
	var env struct {
		Data []domain.MediaItem `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("відповідь не є конвертом списку (%v): %s", err, rr.Body.String())
	}
	if env.Data == nil {
		t.Fatalf("data декодувався як nil, має бути []: %s", rr.Body.String())
	}
	return env.Data
}

func TestPopularReturnsProviderItems(t *testing.T) {
	p := &catProvider{id: "uakino", popular: []domain.MediaItem{
		{ID: "1", ProviderID: "uakino", Title: "Дюна", Type: "movie", Year: 2021, URL: "https://uakino/1"},
		{ID: "2", ProviderID: "uakino", Title: "Матриця", Type: "movie", Year: 1999, URL: "https://uakino/2"},
	}}
	h, _ := catRegistry(p)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=uakino&page=1", nil)
	rr := httptest.NewRecorder()
	h.Popular(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d (%s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	items := decodeItems(t, rr)
	if len(items) != 2 {
		t.Fatalf("очікувалися 2 елементи, отримано %d", len(items))
	}
	if items[0].Title != "Дюна" || items[1].Title != "Матриця" {
		t.Errorf("порядок або вміст не збігаються: %+v", items)
	}
	if items[0].Year != 2021 {
		t.Errorf("рік не розпізнано: %+v", items[0])
	}
}

func TestPopularPassesThroughContentTypeAndPage(t *testing.T) {
	p := &catProvider{id: "uakino", popular: []domain.MediaItem{}}
	h, _ := catRegistry(p)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=uakino&page=3&type=series", nil)
	rr := httptest.NewRecorder()
	h.Popular(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d", rr.Code)
	}
	if p.gotPopularContentType != "series" {
		t.Errorf("content type = %q, want series", p.gotPopularContentType)
	}
	if p.gotPopularPage != 3 {
		t.Errorf("page = %d, want 3", p.gotPopularPage)
	}
}

// A missing, zero, negative or unparsable page must all resolve to page 1.
// The logs show the client sending page=1 on every call, so an off-by-one here
// would silently repeat the first page forever.
func TestPopularClampsPageToOne(t *testing.T) {
	for _, page := range []string{"", "0", "-4", "abc"} {
		t.Run("page="+page, func(t *testing.T) {
			p := &catProvider{id: "uakino", popular: []domain.MediaItem{}}
			h, _ := catRegistry(p)

			url := "/api/v1/content/popular?provider=uakino"
			if page != "" {
				url += "&page=" + page
			}
			rr := httptest.NewRecorder()
			h.Popular(rr, httptest.NewRequest(http.MethodGet, url, nil))

			if p.gotPopularPage != 1 {
				t.Errorf("page %q resolved to %d, want 1", page, p.gotPopularPage)
			}
		})
	}
}

func TestPopularUnknownProviderIs404(t *testing.T) {
	h, _ := catRegistry()

	rr := httptest.NewRecorder()
	h.Popular(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=nope", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("очікувався 404, отримано %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "unknown provider") {
		t.Errorf("очікувався 'unknown provider', отримано %s", rr.Body.String())
	}
}

func TestPopularDisabledProviderIs403(t *testing.T) {
	p := &catProvider{id: "uakino", popular: []domain.MediaItem{}}
	h, reg := catRegistry(p)
	reg.SetEnabled("uakino", false)

	rr := httptest.NewRecorder()
	h.Popular(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=uakino", nil))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("очікувався 403, отримано %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "provider disabled") {
		t.Errorf("очікувався 'provider disabled', отримано %s", rr.Body.String())
	}
}

func TestPopularUpstreamFailureIs503(t *testing.T) {
	p := &catProvider{
		id:         "lavakino",
		popularErr: errors.New("lavakino get popular: upstream 502"),
	}
	h, _ := catRegistry(p)

	rr := httptest.NewRecorder()
	h.Popular(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=lavakino", nil))

	// 503 + Retry-After, not 500: a scraper failure is not a malformed request,
	// and a 5xx makes the Flutter client retry three times with no method check.
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("очікувався 503, отримано %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("503 без Retry-After")
	}
	// The body must not leak the upstream error to the client.
	if strings.Contains(rr.Body.String(), "502") {
		t.Errorf("деталі upstream просочилися в відповідь: %s", rr.Body.String())
	}
}

// An upstream that legitimately returns nothing must answer 200 with an empty
// list, not an error. The logs showed Eneyida replying 200 with a 5-byte body,
// which is Go's `null`; the client normalises it, but the handler must not
// convert an empty catalogue into a failure.
func TestPopularEmptyCatalogueIs200NotAnError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []domain.MediaItem
	}{
		{"nil slice", nil},
		{"empty slice", []domain.MediaItem{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &catProvider{id: "eneyida", popular: tc.items}
			h, _ := catRegistry(p)

			rr := httptest.NewRecorder()
			h.Popular(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=eneyida", nil))

			if rr.Code != http.StatusOK {
				t.Fatalf("очікувався 200, отримано %d", rr.Code)
			}
			if items := decodeItems(t, rr); len(items) != 0 {
				t.Errorf("очікувався порожній список, отримано %d елементів", len(items))
			}
		})
	}
}

func TestCategoryReturnsProviderItems(t *testing.T) {
	p := &catProvider{id: "uakino", byCat: []domain.MediaItem{
		{ID: "c1", ProviderID: "uakino", Title: "Комедія", Type: "movie", URL: "https://uakino/c1"},
	}}
	h, _ := catRegistry(p)

	rr := httptest.NewRecorder()
	h.Category(rr, httptest.NewRequest(http.MethodGet,
		"/api/v1/content/category?provider=uakino&category=comedy&page=2&type=movie", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("очікувався 200, отримано %d (%s)", rr.Code, rr.Body.String())
	}
	items := decodeItems(t, rr)
	if len(items) != 1 || items[0].Title != "Комедія" {
		t.Fatalf("очікувалася одна категорія, отримано %+v", items)
	}
	if p.gotCategoryName != "comedy" {
		t.Errorf("category = %q, want comedy", p.gotCategoryName)
	}
	if p.gotCategoryType != "movie" {
		t.Errorf("content type = %q, want movie", p.gotCategoryType)
	}
	if p.gotCategoryPage != 2 {
		t.Errorf("page = %d, want 2", p.gotCategoryPage)
	}
}

func TestCategoryClampsPageToOne(t *testing.T) {
	for _, page := range []string{"", "0", "-9", "zzz"} {
		t.Run("page="+page, func(t *testing.T) {
			p := &catProvider{id: "uakino", byCat: []domain.MediaItem{}}
			h, _ := catRegistry(p)

			url := "/api/v1/content/category?provider=uakino&category=comedy"
			if page != "" {
				url += "&page=" + page
			}
			rr := httptest.NewRecorder()
			h.Category(rr, httptest.NewRequest(http.MethodGet, url, nil))

			if p.gotCategoryPage != 1 {
				t.Errorf("page %q resolved to %d, want 1", page, p.gotCategoryPage)
			}
		})
	}
}

func TestCategoryUnknownProviderIs404(t *testing.T) {
	h, _ := catRegistry()

	rr := httptest.NewRecorder()
	h.Category(rr, httptest.NewRequest(http.MethodGet,
		"/api/v1/content/category?provider=ghost&category=x", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("очікувався 404, отримано %d", rr.Code)
	}
}

func TestCategoryDisabledProviderIs403(t *testing.T) {
	p := &catProvider{id: "uakino", byCat: []domain.MediaItem{}}
	h, reg := catRegistry(p)
	reg.SetEnabled("uakino", false)

	rr := httptest.NewRecorder()
	h.Category(rr, httptest.NewRequest(http.MethodGet,
		"/api/v1/content/category?provider=uakino&category=x", nil))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("очікувався 403, отримано %d", rr.Code)
	}
}

func TestCategoryUpstreamFailureIs503(t *testing.T) {
	p := &catProvider{id: "uakino", catErr: errors.New("category blew up")}
	h, _ := catRegistry(p)

	rr := httptest.NewRecorder()
	h.Category(rr, httptest.NewRequest(http.MethodGet,
		"/api/v1/content/category?provider=uakino&category=x", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for a scraper failure, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("503 without Retry-After")
	}
	if strings.Contains(rr.Body.String(), "blew up") {
		t.Errorf("деталі помилки просочилися: %s", rr.Body.String())
	}
}

// The registry recovers provider panics and reports them as ErrProviderPanic,
// so a panicking scraper still produces a well-formed 500 with a JSON body
// rather than tearing down the connection.
func TestCatalogueHandlersRecoverFromProviderPanic(t *testing.T) {
	t.Run("popular", func(t *testing.T) {
		p := &panicProvider{id: "boom"}
		h, _ := catRegistry(p)

		rr := httptest.NewRecorder()
		h.Popular(rr, httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=boom", nil))

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("очікувався 500 після паніки, отримано %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "internal server error") {
			t.Errorf("очікувався коректний JSON-тіло, отримано %s", rr.Body.String())
		}
	})

	t.Run("category", func(t *testing.T) {
		p := &panicProvider{id: "boom"}
		h, _ := catRegistry(p)

		rr := httptest.NewRecorder()
		h.Category(rr, httptest.NewRequest(http.MethodGet,
			"/api/v1/content/category?provider=boom&category=x", nil))

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("очікувався 500 після паніки, отримано %d", rr.Code)
		}
	})
}
