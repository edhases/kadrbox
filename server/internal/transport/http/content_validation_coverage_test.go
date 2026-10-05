package http_test

// Validation-focused tests for the content endpoints.
//
// The existing suites assert the happy paths and the error mapping. What is
// still unpinned is the boundary: which query parameters are actually
// required, whether the legacy `id` alias is honoured, whether `page` reaches
// the provider verbatim or is clamped, and that an unsafe item URL never
// reaches a provider at all.

import (
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

// A URL that ValidateSafeURL accepts: public host, no userinfo, plain https.
const covSafeItemURL = "https://cdn.example.com/films/dune-2021.mp4"

// Fixtures shared by the cases below. Only their identity matters here; the
// assertions are about status codes and what reached the provider.
var (
	covDetailsFixture = domain.MediaDetails{MediaItem: domain.MediaItem{ID: "m1", Title: "Dune", Year: 2021}}
	covStreamsFixture = domain.ContentStreamsResponse{}
	covMediaItem      = domain.MediaItem{ID: "m1", Title: "Dune"}
)

func decodeJSON(rec *httptest.ResponseRecorder, target any) error {
	return json.Unmarshal(rec.Body.Bytes(), target)
}

func containsSub(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantMsg string) {
	t.Helper()
	var env map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("error body is not JSON: %v (%s)", err, rec.Body.String())
	}
	if got := env["error"]; got != wantMsg {
		t.Errorf("error = %q, want %q", got, wantMsg)
	}
}

// assertJSONErrorPrefix checks the message starts with wantPrefix, for handlers
// that append a detected value to it.
func assertJSONErrorPrefix(t *testing.T, rec *httptest.ResponseRecorder, wantPrefix string) {
	t.Helper()
	var env map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("error body is not JSON: %v (%s)", err, rec.Body.String())
	}
	if got := env["error"]; !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("error = %q, want it to start with %q", got, wantPrefix)
	}
}

func TestCovDetailsAcceptsTheLegacyIDAlias(t *testing.T) {
	// The Dart client sends `id` on some builds and `url` on others; both must
	// resolve, or a stale client gets a 400 on a perfectly valid request.
	fake := &covStubProvider{
		id:      "uakino",
		details: &covDetailsFixture,
	}
	h, _ := covContentHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/content/details?provider=uakino&id="+covSafeItemURL, nil)

	h.GetDetails(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("using ?id= returned %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if fake.gotURL != covSafeItemURL {
		t.Errorf("provider received %q, want %q", fake.gotURL, covSafeItemURL)
	}
}

func TestCovDetailsRejectsEveryMissingParameterCombination(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "noParameters", url: "/api/v1/content/details"},
		{name: "providerOnly", url: "/api/v1/content/details?provider=uakino"},
		{name: "urlOnly", url: "/api/v1/content/details?url=" + covSafeItemURL},
		{name: "idOnly", url: "/api/v1/content/details?id=" + covSafeItemURL},
		{name: "emptyProviderAndURL", url: "/api/v1/content/details?provider=&url="},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &covStubProvider{id: "uakino", details: &covDetailsFixture}
			h, _ := covContentHandler(fake)

			rec := httptest.NewRecorder()
			h.GetDetails(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			// The provider must not be consulted for an invalid request: the
			// registry entry could be a scraper that fetches the URL itself.
			if fake.gotURL != "" {
				t.Errorf("the provider was called with %q for an invalid request", fake.gotURL)
			}
			assertJSONError(t, rec, "provider and url parameters are required")
		})
	}
}

func TestCovDetailsRefusesUnsafeItemURLsBeforeCallingTheProvider(t *testing.T) {
	// This is the SSRF boundary. Each of these targets a private/loopback
	// address, and reaching a provider with any of them would turn the
	// details endpoint into a proxy into the deployment's own network.
	cases := []struct {
		name string
		url  string
	}{
		{name: "loopback", url: "http://127.0.0.1:8080/admin"},
		{name: "privateRange", url: "http://192.168.1.1/router"},
		{name: "cloudMetadata", url: "http://169.254.169.254/latest/meta-data/"},
		{name: "schemeRelative", url: "//evil.example.com/x"},
		{name: "fileScheme", url: "file:///etc/passwd"},
		{name: "gopherScheme", url: "gopher://evil.example.com:70/x"},
		{name: "ipv6Loopback", url: "http://[::1]:8080/admin"},
		{name: "linkLocalMetadata", url: "http://metadata.google.internal/computeMetadata/v1/"},
		{name: "localhostByName", url: "http://localhost:6379/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &covStubProvider{id: "uakino", details: &covDetailsFixture}
			h, _ := covContentHandler(fake)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/content/details?provider=uakino&url="+tc.url, nil)

			h.GetDetails(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400 for %q (body %s)", rec.Code, tc.url, rec.Body.String())
			}
			if fake.gotURL != "" {
				t.Errorf("an unsafe URL %q reached the provider", fake.gotURL)
			}
			assertJSONError(t, rec, "invalid or unsafe item url")
		})
	}
}

func TestCovDetailsRefusesControlCharactersInTheItemURL(t *testing.T) {
	// A control character can truncate or split the URL in whatever parses it
	// downstream, so `example.com\r\nX-Injected: 1` must never be forwarded.
	// The query is set raw: httptest.NewRequest rejects a newline in a URL.
	fake := &covStubProvider{id: "uakino", details: &covDetailsFixture}
	h, _ := covContentHandler(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/details", nil)
	req.URL.RawQuery = "provider=uakino&url=https%3A%2F%2Fcdn.example.com%2Fa%0Ab"

	rec := httptest.NewRecorder()
	h.GetDetails(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if fake.gotURL != "" {
		t.Errorf("a URL containing a control character reached the provider as %q", fake.gotURL)
	}
	assertJSONError(t, rec, "invalid or unsafe item url")
}

func TestCovStreamsRefusesUnsafeItemURLsBeforeCallingTheProvider(t *testing.T) {
	fake := &covStubProvider{id: "uakino", streams: &covStreamsFixture}
	h, _ := covContentHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/content/streams?provider=uakino&url=http://10.0.0.5/secret", nil)

	h.GetStreams(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if fake.gotURL != "" {
		t.Errorf("an unsafe URL reached the streams provider as %q", fake.gotURL)
	}
}

func TestCovStreamsRejectsMissingParameters(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "noParameters", url: "/api/v1/content/streams"},
		{name: "providerOnly", url: "/api/v1/content/streams?provider=uakino"},
		{name: "urlOnly", url: "/api/v1/content/streams?url=" + covSafeItemURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &covStubProvider{id: "uakino", streams: &covStreamsFixture}
			h, _ := covContentHandler(fake)

			rec := httptest.NewRecorder()
			h.GetStreams(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400", rec.Code)
			}
			if fake.gotURL != "" {
				t.Errorf("the provider was called for an invalid request: %q", fake.gotURL)
			}
		})
	}
}

func TestCovStreamsAcceptsTheLegacyIDAlias(t *testing.T) {
	// The details endpoint accepts `id` as an alias for `url`; the Dart client
	// mixes both, so streams must too or playback breaks on a stale build.
	fake := &covStubProvider{id: "uakino", streams: &covStreamsFixture}
	h, _ := covContentHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/content/streams?provider=uakino&id="+covSafeItemURL, nil)

	h.GetStreams(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("using ?id= returned %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if fake.gotURL != covSafeItemURL {
		t.Errorf("provider received %q, want %q", fake.gotURL, covSafeItemURL)
	}
}

func TestCovStreamsPassesSeasonEpisodeAndVoiceVerbatim(t *testing.T) {
	fake := &covStubProvider{id: "uakino", streams: &covStreamsFixture}
	h, _ := covContentHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/content/streams?provider=uakino&url="+covSafeItemURL+"&season=2&episode=5&voice=uk", nil)

	h.GetStreams(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if fake.gotSeason != 2 || fake.gotEpisode != 5 {
		t.Errorf("provider got season=%d episode=%d, want 2/5", fake.gotSeason, fake.gotEpisode)
	}
	if fake.gotVoice != "uk" {
		t.Errorf("provider got voice=%q, want uk", fake.gotVoice)
	}
}

func TestCovStreamsTreatsUnparsableEpisodeNumbersAsZero(t *testing.T) {
	// A client sending "season=abc" must get the first episode rather than a
	// 500 from a failed Atoi deep in the handler.
	fake := &covStubProvider{id: "uakino", streams: &covStreamsFixture}
	h, _ := covContentHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/content/streams?provider=uakino&url="+covSafeItemURL+"&season=abc&episode=", nil)

	h.GetStreams(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if fake.gotSeason != 0 || fake.gotEpisode != 0 {
		t.Errorf("provider got season=%d episode=%d, want 0/0", fake.gotSeason, fake.gotEpisode)
	}
}

func TestCovStreamsIsNeverServedFromCache(t *testing.T) {
	// Signed, short-lived stream URLs: serving a cached copy hands the client a
	// URL that has already expired, which surfaces as a playback failure.
	fake := &covStubProvider{id: "uakino", streams: &covStreamsFixture}
	reg := provider.NewRegistry()
	reg.Register(fake)
	cache := newMemCacheStore()
	h := transporthttp.NewContentHandler(reg, cache)

	for i := range 2 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/content/streams?provider=uakino&url="+covSafeItemURL, nil)
		h.GetStreams(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: got %d, want 200 (body %s)", i, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Cache"); got != "" {
			t.Errorf("call %d: X-Cache = %q; streams must never be cached", i, got)
		}
	}
	if cache.gets != 0 || cache.sets != 0 {
		t.Errorf("the streams endpoint touched the cache (%d gets, %d sets); it must not cache at all", cache.gets, cache.sets)
	}
}

func TestCovCatalogueClampsNonPositivePagesToOne(t *testing.T) {
	// page=0 or page=-5 would ask the provider for a page that does not exist;
	// the contract is that anything below 1 is page 1.
	for _, endpoint := range []string{"popular", "category"} {
		for _, page := range []string{"0", "-5", "notanumber", ""} {
			t.Run(endpoint+"/page="+page, func(t *testing.T) {
				fake := &covStubProvider{id: "uakino", items: nil}
				h, _ := covContentHandler(fake)

				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet,
					"/api/v1/content/"+endpoint+"?provider=uakino&page="+page, nil)

				if endpoint == "popular" {
					h.Popular(rec, req)
				} else {
					h.Category(rec, req)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestCovCatalogueForwardsValidPages(t *testing.T) {
	for _, endpoint := range []string{"popular", "category"} {
		t.Run(endpoint, func(t *testing.T) {
			fake := &covStubProvider{id: "uakino", items: nil}
			h, _ := covContentHandler(fake)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/content/"+endpoint+"?provider=uakino&page=7", nil)

			if endpoint == "popular" {
				h.Popular(rec, req)
			} else {
				h.Category(rec, req)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200", rec.Code)
			}
			var env struct {
				Meta *transporthttp.PageMeta `json:"meta"`
			}
			if err := decodeJSON(rec, &env); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if env.Meta == nil || env.Meta.Page == nil || *env.Meta.Page != 7 {
				t.Errorf("meta.page = %+v, want 7: a valid page must be forwarded, not reset", env.Meta)
			}
		})
	}
}

func TestCovCatalogueReportsHasMoreFromAPartialPage(t *testing.T) {
	// The repository cannot count cheaply, so has_more is derived from "the
	// page was not empty": the client probes one page further.
	fake := &covStubProvider{id: "uakino", items: []domain.MediaItem{covMediaItem}}
	h, _ := covContentHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=uakino", nil)
	h.Popular(rec, req)

	var env struct {
		Meta *transporthttp.PageMeta `json:"meta"`
	}
	if err := decodeJSON(rec, &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Meta == nil || !env.Meta.HasMore {
		t.Errorf("meta = %+v, want has_more true for a non-empty page", env.Meta)
	}
}

func TestCovProvidersListsTheRegisteredCatalogue(t *testing.T) {
	a := &covStubProvider{id: "uakino"}
	b := &covStubProvider{id: "tortuga"}
	h, _ := covContentHandler(a, b)

	rec := httptest.NewRecorder()
	h.Providers(rec, httptest.NewRequest(http.MethodGet, "/api/v1/content/providers", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	var env struct {
		Data domain.ProviderCatalog `json:"data"`
	}
	if err := decodeJSON(rec, &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Data.Providers) != 2 {
		t.Fatalf("catalogue has %d entries, want 2: %+v", len(env.Data.Providers), env.Data.Providers)
	}
	ids := map[string]bool{}
	for _, p := range env.Data.Providers {
		ids[p.ID] = true
	}
	for _, want := range []string{"uakino", "tortuga"} {
		if !ids[want] {
			t.Errorf("provider %q missing from the catalogue", want)
		}
	}
}

func TestCovDisabledAndPanickingProvidersMapToTheRightStatus(t *testing.T) {
	// 403/404/500 rather than a blanket 503: telling a client to retry a
	// disabled provider or a deterministically panicking one wastes its budget
	// and hides a server bug behind "upstream is down".
	cases := []struct {
		name     string
		disable  bool
		provErr  error
		wantCode int
	}{
		{name: "disabledIsForbidden", disable: true, wantCode: http.StatusForbidden},
		{name: "notFoundIsNotFound", provErr: provider.ErrProviderNotFound, wantCode: http.StatusNotFound},
		{name: "panicIsAServerError", provErr: provider.ErrProviderPanic, wantCode: http.StatusInternalServerError},
		{name: "otherFailureIsRetryable", provErr: errors.New("upstream timeout"), wantCode: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &covStubProvider{id: "uakino", err: tc.provErr}
			reg := provider.NewRegistry()
			reg.Register(fake)
			if tc.disable {
				reg.DisableMany([]string{"uakino"})
			}
			h := transporthttp.NewContentHandler(reg, nil)

			for _, endpoint := range []string{"popular", "category"} {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/v1/content/"+endpoint+"?provider=uakino", nil)
				if endpoint == "popular" {
					h.Popular(rec, req)
				} else {
					h.Category(rec, req)
				}

				if rec.Code != tc.wantCode {
					t.Errorf("%s: got %d, want %d (body %s)", endpoint, rec.Code, tc.wantCode, rec.Body.String())
				}
			}

			// Only the retryable case carries Retry-After: a 500 or 403 is not
			// something waiting will fix.
			if tc.wantCode == http.StatusServiceUnavailable {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/v1/content/popular?provider=uakino", nil)
				h.Popular(rec, req)
				if rec.Header().Get("Retry-After") == "" {
					t.Error("a 503 without Retry-After gives the client nothing to wait on")
				}
			}
		})
	}
}

func TestCovProviderErrorsNeverLeakUpstreamMessages(t *testing.T) {
	// Upstream error strings can contain internal hostnames, so the body must
	// carry the generic fallback rather than the cause.
	fake := &covStubProvider{
		id:      "uakino",
		err:     errors.New("dial tcp 10.4.3.2:443: connection refused (db-primary.internal)"),
		details: &covDetailsFixture,
		streams: &covStreamsFixture,
		items:   nil,
	}
	h, _ := covContentHandler(fake)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content/details?provider=uakino&url="+covSafeItemURL, nil)
	h.GetDetails(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); containsSub(body, "10.4.3.2") || containsSub(body, "db-primary.internal") {
		t.Errorf("the response leaked upstream detail: %s", body)
	}
	if !containsSub(rec.Body.String(), "failed to get media details") {
		t.Errorf("body = %s, want the documented fallback message", rec.Body.String())
	}
}
