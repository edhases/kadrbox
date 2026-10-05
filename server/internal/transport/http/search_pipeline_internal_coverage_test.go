package http

// Internal coverage for the search pipeline's pure assembly layer.
//
// These are package-private functions with no I/O of their own: the tests
// therefore drive them directly with a fake metaSearcher rather than going
// through a request. That is what makes the serial flag, the drop counter and
// the segment-status fold reachable without a network.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	"github.com/edhases/oxide-server/internal/search"
)

// fakeMetaSearcher records what the pipeline asked for, so the tests can assert
// the arguments and not only the output.
type fakeMetaSearcher struct {
	id     string
	resp   *provider.BanderaSearchResponse
	err    error
	gotIDs []string

	gotQuery  string
	gotYear   int
	gotSerial int
}

func (f *fakeMetaSearcher) ID() string { return f.id }

func (f *fakeMetaSearcher) SearchWithMeta(_ context.Context, query string, year, serial int) (*provider.BanderaSearchResponse, error) {
	f.gotQuery = query
	f.gotYear = year
	f.gotSerial = serial
	return f.resp, f.err
}

func TestCovBuildSearchResponseReportsASeriesQueryAsSerial(t *testing.T) {
	// serial=1 is the only signal the aggregator gets that "the next unseen
	// episode" is wanted; a movie query must not set it, because the aggregator
	// would answer with a series' next episode instead of the film.
	for _, tc := range []struct {
		query      string
		wantSerial int
	}{
		{query: "dune", wantSerial: 0},
		{query: "Dune series", wantSerial: 1},
		{query: "Doctor Who season 3", wantSerial: 1},
	} {
		t.Run(tc.query, func(t *testing.T) {
			fs := &fakeMetaSearcher{id: "bandera", resp: &provider.BanderaSearchResponse{OK: true}}
			plan := search.BuildQueryPlan(tc.query)
			start := time.Now()

			resp, err := buildSearchResponse(t.Context(), fs, plan, tc.query, start)
			if err != nil {
				t.Fatalf("buildSearchResponse: %v", err)
			}
			if fs.gotSerial != tc.wantSerial {
				t.Errorf("serial = %d for %q, want %d", fs.gotSerial, tc.query, tc.wantSerial)
			}
			if resp.Query != tc.query {
				t.Errorf("Query = %q, want the caller's raw query %q", resp.Query, tc.query)
			}
			if resp.Canonical != plan.Canonical {
				t.Errorf("Canonical = %q, want the plan's %q", resp.Canonical, plan.Canonical)
			}
		})
	}
}

func TestCovBuildSearchResponseScoresAndClustersUpstreamItems(t *testing.T) {
	fs := &fakeMetaSearcher{
		id: "bandera",
		resp: &provider.BanderaSearchResponse{
			OK: true,
			Items: []provider.BanderaSearchItem{{
				Source: "filmix", Title: "Dune", Type: "movie",
				Year: json.RawMessage(`2021`),
				Ref:  json.RawMessage(`{"id":"1"}`),
			}},
			Meta: &provider.BanderaSearchMetaResponse{
				Statuses: map[string]provider.BanderaSourceStatus{
					"filmix": {Status: "ok", Count: 1, ElapsedMs: 12},
				},
			},
		},
	}
	plan := search.BuildQueryPlan("Dune")

	resp, err := buildSearchResponse(t.Context(), fs, plan, "Dune", time.Now())
	if err != nil {
		t.Fatalf("buildSearchResponse: %v", err)
	}

	if len(resp.Items) != 1 {
		t.Fatalf("got %d items, want 1: %+v", len(resp.Items), resp.Items)
	}
	item := resp.Items[0]
	if item.ProviderID != "bandera" {
		t.Errorf("ProviderID = %q, want the meta-searcher's id", item.ProviderID)
	}
	if item.Title != "Dune" {
		t.Errorf("Title = %q, want Dune", item.Title)
	}
	if item.Year != 2021 {
		t.Errorf("Year = %d, want 2021 parsed out of the raw JSON", item.Year)
	}
	if item.ClusterKey == "" {
		t.Error("ClusterKey is empty; duplicate titles would never merge")
	}
	if len(item.Sources) != 1 || item.Sources[0].SourceKey != "filmix" {
		t.Errorf("Sources = %+v, want one ref naming filmix", item.Sources)
	}
	if len(resp.Segments) != 1 || resp.Segments[0].ID != "bandera" {
		t.Fatalf("Segments = %+v, want a single bandera segment", resp.Segments)
	}
	if resp.Segments[0].Status != "ok" {
		t.Errorf("segment status = %q, want ok for a successful source", resp.Segments[0].Status)
	}
	if resp.Segments[0].Sources["filmix"].ElapsedMs != 12 {
		t.Errorf("elapsed = %d, want the upstream 12ms carried through", resp.Segments[0].Sources["filmix"].ElapsedMs)
	}
}

func TestCovBuildSearchResponseCountsDroppedItems(t *testing.T) {
	// FilteredOut must reflect what the relevance gate removed, otherwise a
	// client cannot tell "the aggregator found nothing" from "the aggregator
	// found nothing we kept".
	fs := &fakeMetaSearcher{
		id: "bandera",
		resp: &provider.BanderaSearchResponse{
			OK: true,
			Items: []provider.BanderaSearchItem{
				{Source: "filmix", Title: "Totally Unrelated Documentary", Type: "movie", Year: json.RawMessage(`2019`)},
			},
		},
	}
	plan := search.BuildQueryPlan("Dune Part Two")

	resp, err := buildSearchResponse(t.Context(), fs, plan, "Dune Part Two", time.Now())
	if err != nil {
		t.Fatalf("buildSearchResponse: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("got %d items, want the unrelated title dropped: %+v", len(resp.Items), resp.Items)
	}
	if resp.FilteredOut != 1 {
		t.Errorf("FilteredOut = %d, want 1", resp.FilteredOut)
	}
}

func TestCovBuildSearchResponsePropagatesUpstreamFailure(t *testing.T) {
	// No half-built response: a failure must come back as an error so the
	// handler writes a 503 rather than an empty 200.
	boom := errors.New("aggregator exploded")
	fs := &fakeMetaSearcher{id: "bandera", err: boom}

	plan := search.BuildQueryPlan("Dune")
	resp, err := buildSearchResponse(t.Context(), fs, plan, "Dune", time.Now())
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the upstream failure %v", err, boom)
	}
	if len(resp.Items) != 0 || len(resp.Segments) != 0 {
		t.Errorf("a failed call returned a populated response: %+v", resp)
	}
}

func TestCovBuildSearchResponseFoldsSourceStatuses(t *testing.T) {
	cases := []struct {
		name       string
		meta       *provider.BanderaSearchMetaResponse
		items      int
		wantStatus string
	}{
		{
			name:       "noMetaIsNotAnError",
			meta:       nil,
			items:      0,
			wantStatus: "empty",
		},
		{
			name: "allSourcesFailed",
			meta: &provider.BanderaSearchMetaResponse{
				Statuses: map[string]provider.BanderaSourceStatus{
					"a": {Status: "ok", Error: "upstream refused"},
					"b": {Status: "failed"},
				},
			},
			items:      0,
			wantStatus: "error",
		},
		{
			name: "someSourcesFailedWithResults",
			meta: &provider.BanderaSearchMetaResponse{
				Statuses: map[string]provider.BanderaSourceStatus{
					"a": {Status: "ok", Count: 3},
					"b": {Status: "timeout"},
				},
			},
			items:      3,
			wantStatus: "partial",
		},
		{
			name: "everythingOk",
			meta: &provider.BanderaSearchMetaResponse{
				Statuses: map[string]provider.BanderaSourceStatus{
					"a": {Status: "ok", Count: 3},
				},
			},
			items:      3,
			wantStatus: "ok",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &provider.BanderaSearchResponse{OK: true, Meta: tc.meta}
			for range tc.items {
				resp.Items = append(resp.Items, provider.BanderaSearchItem{Source: "a", Title: "Dune"})
			}
			fs := &fakeMetaSearcher{id: "bandera", resp: resp}

			got, err := buildSearchResponse(t.Context(), fs, search.BuildQueryPlan("Dune"), "Dune", time.Now())
			if err != nil {
				t.Fatalf("buildSearchResponse: %v", err)
			}
			if len(got.Segments) != 1 {
				t.Fatalf("got %d segments, want 1", len(got.Segments))
			}
			if got.Segments[0].Status != tc.wantStatus {
				t.Errorf("segment status = %q, want %q", got.Segments[0].Status, tc.wantStatus)
			}
		})
	}
}

func TestCovNormalizeSourceStatus(t *testing.T) {
	cases := []struct {
		raw   string
		count int
		err   string
		want  string
	}{
		{raw: "OK", count: 2, want: "ok"},
		{raw: " success ", count: 2, want: "ok"},
		{raw: "anything-else", count: 2, want: "ok"},
		{raw: "TIMEOUT", count: 1, want: "timeout"},
		{raw: "empty", count: 0, want: "empty"},
		{raw: "ok", count: 0, want: "empty"},
		{raw: "failed", count: 3, want: "error"},
		{raw: "whatever", count: 3, err: "boom", want: "error"},
		{raw: "", count: 0, want: "empty"},
	}
	for _, tc := range cases {
		got := normalizeSourceStatus(tc.raw, tc.count, tc.err)
		if got != tc.want {
			t.Errorf("normalizeSourceStatus(%q, %d, %q) = %q, want %q", tc.raw, tc.count, tc.err, got, tc.want)
		}
	}
}

func TestCovScoreCandidatesDropsPostersThatAreNotURLs(t *testing.T) {
	// A relative-looking poster string is useless to the client and used to be
	// carried through as if it were a URL.
	plan := search.BuildQueryPlan("Dune")
	items := []provider.BanderaSearchItem{
		{Source: "filmix", Title: "Dune", Type: "movie", Year: json.RawMessage(`2021`),
			Poster: "https://cdn/p.jpg", Ref: json.RawMessage(`1`)},
		{Source: "filmix", Title: "Dune", Type: "movie", Year: json.RawMessage(`2021`),
			Poster: "not a url", Ref: json.RawMessage(`1`)},
	}

	candidates, filteredOut := scoreCandidates("bandera", items, plan)
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates, want 2 (filtering happens earlier)", len(candidates))
	}
	if candidates[0].PosterURL != "https://cdn/p.jpg" {
		t.Errorf("absolute poster was mangled into %q", candidates[0].PosterURL)
	}
	if candidates[1].PosterURL != "" {
		t.Errorf("non-URL poster survived as %q", candidates[1].PosterURL)
	}
	if filteredOut != 0 {
		t.Errorf("filteredOut = %d, want 0", filteredOut)
	}
}

func TestCovScoreCandidatesDefaultsAnAbsentTypeToMovie(t *testing.T) {
	plan := search.BuildQueryPlan("Dune")
	candidates, _ := scoreCandidates("bandera", []provider.BanderaSearchItem{
		{Source: "filmix", Title: "Dune", Year: json.RawMessage(`2021`), Ref: json.RawMessage(`1`)},
	}, plan)
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	if candidates[0].Type != "movie" {
		t.Errorf("Type = %q for an item with no type, want %q", candidates[0].Type, "movie")
	}
}

func TestCovScoreCandidatesBuildsARoundTrippableItemURL(t *testing.T) {
	// URL is a JSON payload the details endpoint later re-reads; if the
	// pipeline stopped writing a decodable object, playback would break with
	// an opaque error far from here.
	plan := search.BuildQueryPlan("Dune")
	candidates, _ := scoreCandidates("bandera", []provider.BanderaSearchItem{
		{Source: "filmix", Title: "Dune", Type: "movie", Year: json.RawMessage(`2021`), Ref: json.RawMessage(`{"id":"42"}`)},
	}, plan)
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}

	var payload provider.BanderaItemPayload
	if err := json.Unmarshal([]byte(candidates[0].URL), &payload); err != nil {
		t.Fatalf("item URL is not decodable: %v (%q)", err, candidates[0].URL)
	}
	if payload.Source != "filmix" || payload.Title != "Dune" || payload.ID == "" {
		t.Errorf("payload = %+v, want the source/title/stable-id carried through", payload)
	}
	if payload.Year != 2021 {
		t.Errorf("payload.Year = %d, want 2021", payload.Year)
	}
	if !payload.IsItemRef {
		t.Error("payload.IsItemRef = false, so the details endpoint would not treat this as a resolved item")
	}
}

func TestCovScoreCandidatesKeepsEveryItemOfADuplicateTitle(t *testing.T) {
	// Two entries that share a title must survive scoring: clustering happens
	// afterwards and is what decides whether they are one film or two.
	plan := search.BuildQueryPlan("Dune")
	items := []provider.BanderaSearchItem{
		{Source: "filmix", Title: "Dune", Type: "movie", Year: json.RawMessage(`2021`), Ref: json.RawMessage(`1`)},
		{Source: "uaflix", Title: "Dune", Type: "movie", Year: json.RawMessage(`1984`), Ref: json.RawMessage(`2`)},
	}
	candidates, _ := scoreCandidates("bandera", items, plan)
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates, want both", len(candidates))
	}
	if candidates[0].ClusterKey == candidates[1].ClusterKey {
		t.Error("two different years collapsed into one cluster key before clustering ran")
	}
}

// ---- writeList / setNextLink ------------------------------------------------

func TestCovSetNextLinkPaginatesBothShapes(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		meta       *PageMeta
		wantLink   string
		wantNoLink bool
	}{
		{
			name:     "pageBasedAdvancesThePage",
			url:      "/api/v1/content/popular?page=2",
			meta:     &PageMeta{Page: intp(2), HasMore: true, Count: 5},
			wantLink: `</api/v1/content/popular?page=3>; rel="next"`,
		},
		{
			name:     "offsetBasedAdvancesTheOffset",
			url:      "/api/v1/sync/favorites?limit=2",
			meta:     &PageMeta{Limit: intp(2), Offset: intp(4), HasMore: true, Count: 2},
			wantLink: `</api/v1/sync/favorites?limit=2&offset=6>; rel="next"`,
		},
		{
			name:     "offsetOnlyCountsFromZero",
			url:      "/api/v1/sync/favorites",
			meta:     &PageMeta{HasMore: true, Count: 3},
			wantLink: `</api/v1/sync/favorites?offset=3>; rel="next"`,
		},
		{
			name:       "noMorePagesMeansNoLink",
			url:        "/api/v1/content/popular",
			meta:       &PageMeta{Page: intp(1), HasMore: false, Count: 5},
			wantNoLink: true,
		},
		{
			name:       "emptyPageMeansNoLink",
			url:        "/api/v1/content/popular",
			meta:       &PageMeta{Page: intp(1), HasMore: true, Count: 0},
			wantNoLink: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)

			setNextLink(rec, req, tc.meta)

			got := rec.Header().Get("Link")
			if tc.wantNoLink {
				if got != "" {
					t.Errorf("Link = %q, want none", got)
				}
				return
			}
			if got != tc.wantLink {
				t.Errorf("Link = %q, want %q", got, tc.wantLink)
			}
		})
	}
}

func TestCovWriteListCountsAndEmitsTheEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/favorites?limit=2&offset=4", nil)

	items := []domain.MediaItem{{ID: "m1"}, {ID: "m2"}}
	writeList(rec, req, items, &PageMeta{Limit: intp(2), Offset: intp(4), HasMore: true})

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var env struct {
		Data []domain.MediaItem `json:"data"`
		Meta *PageMeta          `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not the list envelope: %v (%s)", err, rec.Body.String())
	}
	if len(env.Data) != 2 {
		t.Errorf("data has %d items, want 2", len(env.Data))
	}
	if env.Meta == nil || env.Meta.Count != 2 {
		t.Fatalf("meta = %+v, want Count 2", env.Meta)
	}
	if link := rec.Header().Get("Link"); link == "" {
		t.Error("no Link header on a page that reports has_more")
	}
}

func TestCovWriteListEmitsAnEmptyArrayForNoRows(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync/favorites", nil)

	writeList(rec, req, []domain.MediaItem(nil), &PageMeta{Limit: intp(20)})

	// The client must never have to distinguish "no results" from null.
	if body := rec.Body.String(); !contains(body, `"data":[]`) {
		t.Errorf("body = %s, want an empty data array", body)
	}
}

func TestCovWriteObjectWrapsInData(t *testing.T) {
	rec := httptest.NewRecorder()
	writeObject(rec, domain.MediaItem{ID: "m1"})

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var env struct {
		Data domain.MediaItem `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not the object envelope: %v", err)
	}
	if env.Data.ID != "m1" {
		t.Errorf("data.id = %q, want m1", env.Data.ID)
	}
}

func TestCovIntpReturnsASettablePointer(t *testing.T) {
	p := intp(7)
	if p == nil || *p != 7 {
		t.Fatalf("intp(7) = %v", p)
	}
}

// contains is a substring check for body assertions; strings.Contains would do,
// but the other assertions in this package compare against a formatted body and
// reading them as literals is clearer.
func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		strings.Contains(haystack, needle)
}
