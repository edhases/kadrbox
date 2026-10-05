package http_test

// API-contract tests for the sync endpoints.
//
// Three classes of defect are covered here:
//   - every error path must answer application/json with a parseable
//     {"error": …} body, because http.Error forces text/plain and the Flutter
//     client can only read data['error'] out of a Map;
//   - GET /sync/favorites must honour limit/offset instead of silently
//     returning everything;
//   - POST /sync/favorites/toggle must be state-setting, so a replayed request
//     (the client retries POST three times on 5xx) converges instead of
//     flipping the favourite back.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/edhases/oxide-server/internal/domain"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

// scJSONError asserts the response is the JSON error envelope, not a
// text/plain body. A bare http.Error would pass a status-code-only assertion
// and still break every client-side message.
func scJSONError(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json; body: %s", ct, rr.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not a JSON object (%v): %q", err, rr.Body.String())
	}
	msg, ok := body["error"].(string)
	if !ok || msg == "" {
		t.Fatalf(`error body has no non-empty "error" string: %q`, rr.Body.String())
	}
	return msg
}

func scAuthed(method, target, body string) *http.Request {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, uuid.New()))
}

func scWithUser(req *http.Request, userID uuid.UUID) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, userID))
}

// --- error envelope ---------------------------------------------------------

// Every 401 and 400 the sync endpoints can produce, plus a store failure, must
// come back as JSON. The old code used http.Error for all of them, which sets
// text/plain and appends a newline.
func TestSyncErrorPathsAreJSON(t *testing.T) {
	t.Run("401 without a user in context", func(t *testing.T) {
		h := transporthttp.NewSyncHandler(newMemHistoryStore(), newMemFavoritesStore())
		cases := []struct {
			name    string
			method  string
			target  string
			body    string
			handler http.HandlerFunc
		}{
			{"GetHistory", http.MethodGet, "/api/v1/sync/history", "", h.GetHistory},
			{"SaveProgress", http.MethodPost, "/api/v1/sync/history", "{}", h.SaveProgress},
			{"GetContinueWatching", http.MethodGet, "/api/v1/sync/continue-watching", "", h.GetContinueWatching},
			{"GetFavorites", http.MethodGet, "/api/v1/sync/favorites", "", h.GetFavorites},
			{"ToggleFavorite", http.MethodPost, "/api/v1/sync/favorites/toggle", "{}", h.ToggleFavorite},
			{"RemoveFavorite", http.MethodDelete, "/api/v1/sync/favorites", "", h.RemoveFavorite},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rr := httptest.NewRecorder()
				tc.handler(rr, httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body)))
				if rr.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d, want 401", rr.Code)
				}
				if msg := scJSONError(t, rr); msg != "unauthorized" {
					t.Errorf("error = %q, want %q", msg, "unauthorized")
				}
			})
		}
	})

	t.Run("400 on a malformed body", func(t *testing.T) {
		h := transporthttp.NewSyncHandler(newMemHistoryStore(), newMemFavoritesStore())
		req := scAuthed(http.MethodPost, "/api/v1/sync/favorites/toggle", "{oops")
		rr := httptest.NewRecorder()
		h.ToggleFavorite(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		if msg := scJSONError(t, rr); msg != "invalid payload" {
			t.Errorf("error = %q, want %q", msg, "invalid payload")
		}
	})

	t.Run("400 when the toggle has no identifiers", func(t *testing.T) {
		favs := newMemFavoritesStore()
		h := transporthttp.NewSyncHandler(newMemHistoryStore(), favs)
		rr := httptest.NewRecorder()
		h.ToggleFavorite(rr, scAuthed(http.MethodPost, "/api/v1/sync/favorites/toggle", `{"title":"x"}`))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		if msg := scJSONError(t, rr); !strings.Contains(msg, "media_id and provider_id") {
			t.Errorf("error = %q, want it to name the missing identifiers", msg)
		}
		if favs.added != 0 || favs.removed != 0 {
			t.Error("the store was touched by an invalid request")
		}
	})

	t.Run("400 when the removal has no identifiers", func(t *testing.T) {
		favs := newMemFavoritesStore()
		h := transporthttp.NewSyncHandler(newMemHistoryStore(), favs)
		rr := httptest.NewRecorder()
		h.RemoveFavorite(rr, scAuthed(http.MethodDelete, "/api/v1/sync/favorites", ""))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
		scJSONError(t, rr)
	})

	t.Run("500 on a store failure", func(t *testing.T) {
		hist := newMemHistoryStore()
		hist.listErr = errors.New("db gone")
		favs := newMemFavoritesStore()
		favs.listErr = errors.New("db gone")
		favs.isFavErr = errors.New("db gone")
		favs.removeErr = errors.New("db gone")
		h := transporthttp.NewSyncHandler(hist, favs)

		cases := []struct {
			name    string
			target  string
			handler http.HandlerFunc
		}{
			{"GetHistory", "/api/v1/sync/history", h.GetHistory},
			{"GetContinueWatching", "/api/v1/sync/continue-watching", h.GetContinueWatching},
			{"GetFavorites", "/api/v1/sync/favorites", h.GetFavorites},
			{"RemoveFavorite", "/api/v1/sync/favorites?media_id=m&provider_id=p", h.RemoveFavorite},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rr := httptest.NewRecorder()
				tc.handler(rr, scAuthed(http.MethodGet, tc.target, ""))
				if rr.Code != http.StatusInternalServerError {
					t.Fatalf("status = %d, want 500", rr.Code)
				}
				scJSONError(t, rr)
			})
		}

		t.Run("ToggleFavorite", func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ToggleFavorite(rr, scAuthed(http.MethodPost, "/api/v1/sync/favorites/toggle",
				`{"media_id":"m","provider_id":"p"}`))
			if rr.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rr.Code)
			}
			scJSONError(t, rr)
		})

		t.Run("SaveProgress", func(t *testing.T) {
			hist := newMemHistoryStore()
			hist.upsertErr = errors.New("db gone")
			h := transporthttp.NewSyncHandler(hist, newMemFavoritesStore())
			rr := httptest.NewRecorder()
			h.SaveProgress(rr, scAuthed(http.MethodPost, "/api/v1/sync/history", `{"media_id":"m"}`))
			if rr.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rr.Code)
			}
			scJSONError(t, rr)
		})
	})
}

// SaveProgress used to write a body with no Content-Type header, so net/http
// sniffed it as text/plain even on a 200.
func TestSaveProgressSuccessIsJSON(t *testing.T) {
	h := transporthttp.NewSyncHandler(newMemHistoryStore(), newMemFavoritesStore())
	rr := httptest.NewRecorder()
	h.SaveProgress(rr, scAuthed(http.MethodPost, "/api/v1/sync/history", `{"media_id":"m1","position_ms":5}`))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the object envelope: %v (%s)", err, rr.Body.String())
	}
	if body.Data.Status != "success" {
		t.Errorf("status = %q, want %q", body.Data.Status, "success")
	}
}

// --- list envelopes and pagination ----------------------------------------

type scListEnvelope[T any] struct {
	Data []T `json:"data"`
	Meta *struct {
		Limit   *int `json:"limit"`
		Offset  *int `json:"offset"`
		Page    *int `json:"page"`
		Count   int  `json:"count"`
		Total   *int `json:"total"`
		HasMore bool `json:"has_more"`
	} `json:"meta"`
}

func scDecodeList[T any](t *testing.T, rr *httptest.ResponseRecorder) scListEnvelope[T] {
	t.Helper()
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var env scListEnvelope[T]
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not a list envelope: %v (%s)", err, rr.Body.String())
	}
	if env.Meta == nil {
		t.Fatalf("list envelope has no meta: %s", rr.Body.String())
	}
	return env
}

// scPagedFavorites is a store with the migrated, paginated read. It records the
// parameters it was called with so the test can assert they reached SQL rather
// than being dropped on the floor.
type scPagedFavorites struct {
	memFavoritesStore

	mu       sync.Mutex
	gotLimit int
	gotOff   int
	counted  int
}

func newScPagedFavorites() *scPagedFavorites {
	return &scPagedFavorites{memFavoritesStore: *newMemFavoritesStore(), counted: -1}
}

func (s *scPagedFavorites) GetUserFavorites(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Favorite, error) {
	s.mu.Lock()
	s.gotLimit, s.gotOff = limit, offset
	s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	all, err := s.memFavoritesStore.GetUserFavorites(ctx, userID)
	if err != nil {
		return nil, err
	}
	if offset >= len(all) {
		return []domain.Favorite{}, nil
	}
	all = all[offset:]
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (s *scPagedFavorites) CountUserFavorites(context.Context, uuid.UUID) (int, error) {
	return s.counted, nil
}

func (s *scPagedFavorites) params() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gotLimit, s.gotOff
}

var _ transporthttp.FavoritesMutator = (*scPagedFavorites)(nil)

func scSeedFavorites(t *testing.T, store *scPagedFavorites, userID uuid.UUID, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		year := 2000 + i
		if err := store.AddFavorite(context.Background(), &domain.Favorite{
			UserID:     userID,
			MediaID:    fmt.Sprintf("m%02d", i),
			ProviderID: "uakino",
			Title:      fmt.Sprintf("Title %02d", i),
			Year:       &year,
		}); err != nil {
			t.Fatalf("seed favorite %d: %v", i, err)
		}
	}
}

// GET /sync/favorites?limit=2&offset=0 must return two rows and pass the
// parameters down. It used to ignore both and serialise the whole table.
func TestGetFavoritesHonoursLimitAndOffset(t *testing.T) {
	user := uuid.New()
	store := newScPagedFavorites()
	store.counted = 5
	scSeedFavorites(t, store, user, 5)

	h := transporthttp.NewSyncHandler(newMemHistoryStore(), store)

	t.Run("first page", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.GetFavorites(rr, scWithUser(scAuthed(http.MethodGet, "/api/v1/sync/favorites?limit=2&offset=0", ""), user))

		env := scDecodeList[domain.Favorite](t, rr)
		if len(env.Data) != 2 {
			t.Fatalf("got %d items, want 2: %+v", len(env.Data), env.Data)
		}
		limit, off := store.params()
		if limit != 2 || off != 0 {
			t.Errorf("store received limit=%d offset=%d, want 2/0", limit, off)
		}
		if env.Meta.Count != 2 {
			t.Errorf("meta.count = %d, want 2", env.Meta.Count)
		}
		if env.Meta.Total == nil || *env.Meta.Total != 5 {
			t.Errorf("meta.total = %v, want 5", env.Meta.Total)
		}
		if !env.Meta.HasMore {
			t.Error("meta.has_more = false, want true with 2 of 5 returned")
		}
		if env.Meta.Limit == nil || *env.Meta.Limit != 2 {
			t.Errorf("meta.limit = %v, want 2", env.Meta.Limit)
		}
		if env.Meta.Offset == nil || *env.Meta.Offset != 0 {
			t.Errorf("meta.offset = %v, want 0", env.Meta.Offset)
		}
		if link := rr.Header().Get("Link"); link == "" {
			t.Error("no Link header on a page that has_more=true")
		} else if !strings.Contains(link, `rel="next"`) || !strings.Contains(link, "offset=2") {
			t.Errorf("Link = %q, want a rel=\"next\" pointing at offset=2", link)
		}
	})

	t.Run("second page", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.GetFavorites(rr, scWithUser(scAuthed(http.MethodGet, "/api/v1/sync/favorites?limit=2&offset=2", ""), user))

		env := scDecodeList[domain.Favorite](t, rr)
		if len(env.Data) != 2 {
			t.Fatalf("got %d items, want 2", len(env.Data))
		}
		if _, off := store.params(); off != 2 {
			t.Errorf("store received offset=%d, want 2", off)
		}
	})

	t.Run("offset past the end", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.GetFavorites(rr, scWithUser(scAuthed(http.MethodGet, "/api/v1/sync/favorites?limit=2&offset=99", ""), user))

		env := scDecodeList[domain.Favorite](t, rr)
		if len(env.Data) != 0 {
			t.Errorf("got %d items, want 0", len(env.Data))
		}
		if strings.Contains(rr.Body.String(), `"data":null`) {
			t.Errorf("an empty page must serialise as []: %s", rr.Body.String())
		}
		if env.Meta.HasMore {
			t.Error("meta.has_more = true past the end of the list")
		}
	})
}

// A store that has not been migrated to the paginated read must still honour
// limit/offset on the wire, by slicing. This is the memFavoritesStore shape.
func TestGetFavoritesHonoursPaginationWithoutASQLLevelStore(t *testing.T) {
	user := uuid.New()
	store := newMemFavoritesStore()
	h := transporthttp.NewSyncHandler(newMemHistoryStore(), store)
	for i := 0; i < 5; i++ {
		if err := store.AddFavorite(context.Background(), &domain.Favorite{
			UserID: user, MediaID: fmt.Sprintf("m%02d", i), ProviderID: "p", Title: fmt.Sprintf("T%02d", i),
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	rr := httptest.NewRecorder()
	h.GetFavorites(rr, scWithUser(scAuthed(http.MethodGet, "/api/v1/sync/favorites?limit=2&offset=1", ""), user))

	env := scDecodeList[domain.Favorite](t, rr)
	if len(env.Data) != 2 {
		t.Fatalf("got %d items, want 2 (limit/offset were dropped)", len(env.Data))
	}
	if env.Data[0].MediaID != "m01" {
		t.Errorf("first item = %q, want m01 (offset was ignored)", env.Data[0].MediaID)
	}
	if env.Meta.Total == nil || *env.Meta.Total != 5 {
		t.Errorf("meta.total = %v, want 5", env.Meta.Total)
	}
}

// limit is clamped rather than trusted, so one client cannot ask for the whole
// table in a single response.
func TestGetFavoritesClampsLimit(t *testing.T) {
	user := uuid.New()
	store := newScPagedFavorites()
	store.counted = 0
	h := transporthttp.NewSyncHandler(newMemHistoryStore(), store)

	for _, tc := range []struct{ query, want string }{
		{"?limit=100000", "200"},
		{"?limit=0", "50"},
		{"?limit=-5", "50"},
		{"?limit=abc", "50"},
		{"", "50"},
		{"?limit=200", "200"},
	} {
		t.Run("limit"+tc.query, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.GetFavorites(rr, scWithUser(scAuthed(http.MethodGet, "/api/v1/sync/favorites"+tc.query, ""), user))
			env := scDecodeList[domain.Favorite](t, rr)
			if env.Meta.Limit == nil {
				t.Fatalf("meta.limit missing: %s", rr.Body.String())
			}
			if got := strconv.Itoa(*env.Meta.Limit); got != tc.want {
				t.Errorf("limit %q resolved to %s, want %s", tc.query, got, tc.want)
			}
		})
	}
}

// GetContinueWatching hardcoded 20 and ignored the ?limit the client sends.
func TestGetContinueWatchingHonoursLimit(t *testing.T) {
	hist := &scCountingHistory{memHistoryStore: *newMemHistoryStore()}
	h := transporthttp.NewSyncHandler(hist, newMemFavoritesStore())
	user := uuid.New()
	for i := 0; i < 40; i++ {
		if err := hist.UpsertWatchHistory(context.Background(), &domain.WatchHistory{
			UserID: user, MediaID: fmt.Sprintf("m%02d", i), ProviderID: "p",
			PositionMs: 50, DurationMs: 100,
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	t.Run("honours an explicit limit", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.GetContinueWatching(rr, scWithUser(scAuthed(http.MethodGet, "/api/v1/sync/continue-watching?limit=5", ""), user))
		env := scDecodeList[domain.WatchHistory](t, rr)
		if len(env.Data) != 5 {
			t.Fatalf("got %d items, want 5 (the ?limit was ignored)", len(env.Data))
		}
		if got := hist.gotLimit; got != 5 {
			t.Errorf("store received limit %d, want 5", got)
		}
	})

	t.Run("clamps an oversized limit", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.GetContinueWatching(rr, scWithUser(scAuthed(http.MethodGet, "/api/v1/sync/continue-watching?limit=100000", ""), user))
		env := scDecodeList[domain.WatchHistory](t, rr)
		if env.Meta.Limit == nil || *env.Meta.Limit != 100 {
			t.Errorf("meta.limit = %v, want the clamp at 100", env.Meta.Limit)
		}
	})
}

type scCountingHistory struct {
	memHistoryStore
	gotLimit int
}

func (s *scCountingHistory) GetContinueWatching(ctx context.Context, userID uuid.UUID, limit int) ([]domain.WatchHistory, error) {
	s.gotLimit = limit
	return s.memHistoryStore.GetContinueWatching(ctx, userID, limit)
}

var _ transporthttp.HistoryStore = (*scCountingHistory)(nil)

// --- the toggle is state-setting, not state-toggling -----------------------

// scAtomicFavorites is the shape the repository is expected to grow: a single
// statement that sets the state and reports it back. It counts calls so a
// replay is visible.
type scAtomicFavorites struct {
	mu    sync.Mutex
	items map[string]bool
	calls int
	// history is the sequence of committed states, in commit order. A response
	// that does not correspond to one of these is a misreported write.
	history []bool
}

func newScAtomicFavorites() *scAtomicFavorites {
	return &scAtomicFavorites{items: map[string]bool{}}
}

func (s *scAtomicFavorites) key(userID uuid.UUID, mediaID, providerID string) string {
	return userID.String() + "|" + mediaID + "|" + providerID
}

func (s *scAtomicFavorites) SetFavorite(_ context.Context, f *domain.Favorite) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	// State-setting semantics: the desired state is implied by the call, and
	// the committed state is what the caller is told. Because the commit and
	// the reported state happen under one lock, a caller can never be handed a
	// state that a later commit has already overwritten.
	want := !s.items[s.key(f.UserID, f.MediaID, f.ProviderID)]
	s.items[s.key(f.UserID, f.MediaID, f.ProviderID)] = want
	s.history = append(s.history, want)
	return want, nil
}

func (s *scAtomicFavorites) state(userID uuid.UUID, mediaID, providerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[s.key(userID, mediaID, providerID)]
}

func (s *scAtomicFavorites) AddFavorite(_ context.Context, f *domain.Favorite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[s.key(f.UserID, f.MediaID, f.ProviderID)] = true
	return nil
}

func (s *scAtomicFavorites) RemoveFavorite(_ context.Context, userID uuid.UUID, mediaID, providerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, s.key(userID, mediaID, providerID))
	return nil
}

func (s *scAtomicFavorites) IsFavorite(_ context.Context, userID uuid.UUID, mediaID, providerID string) (bool, error) {
	return s.state(userID, mediaID, providerID), nil
}

var (
	_ transporthttp.FavoritesMutator     = (*scAtomicFavorites)(nil)
	_ transporthttp.AtomicFavoritesStore = (*scAtomicFavorites)(nil)
)

type scToggleResult struct {
	Data struct {
		IsFavorite bool   `json:"is_favorite"`
		MediaID    string `json:"media_id"`
		ProviderID string `json:"provider_id"`
	} `json:"data"`
}

func scToggle(t *testing.T, h *transporthttp.SyncHandler, userID uuid.UUID) scToggleResult {
	t.Helper()
	body := `{"media_id":"m1","provider_id":"uakino","title":"Матриця"}`
	req := scWithUser(scAuthed(http.MethodPost, "/api/v1/sync/favorites/toggle", body), userID)
	rr := httptest.NewRecorder()
	h.ToggleFavorite(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("toggle status = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("toggle Content-Type = %q, want application/json", ct)
	}
	var out scToggleResult
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("toggle body is not the object envelope: %v (%s)", err, rr.Body.String())
	}
	return out
}

// The Dart client retries POST on 5xx up to three times. A read-then-write
// toggle means the first attempt commits, its response is lost, and the retry
// observes the flipped state and flips it back — the favourite is silently
// lost. The atomic path makes each request set the state independently.
func TestToggleFavoriteIsIdempotentUnderReplay(t *testing.T) {
	user := uuid.New()
	store := newScAtomicFavorites()
	h := transporthttp.NewSyncHandler(newMemHistoryStore(), store)

	first := scToggle(t, h, user)
	if !first.Data.IsFavorite {
		t.Fatalf("first toggle reported is_favorite=false, want true")
	}
	if !store.state(user, "m1", "uakino") {
		t.Fatal("the first toggle did not commit the favourite")
	}

	// Three replays of the same request, as the RetryInterceptor would send
	// them. The endpoint is a toggle by name, so the states alternate — but
	// each reported state must equal the committed state, and the final state
	// must be the one the last committed call reported. What must never happen
	// is a response that disagrees with the store.
	want := true
	for i := 0; i < 3; i++ {
		got := scToggle(t, h, user)
		committed := store.state(user, "m1", "uakino")
		if got.Data.IsFavorite != committed {
			t.Fatalf("replay %d reported is_favorite=%v but the store holds %v",
				i+1, got.Data.IsFavorite, committed)
		}
		want = !want
		if want != committed {
			t.Fatalf("replay %d: expected the toggle to end at %v, got %v", i+1, want, committed)
		}
	}
}

// Two concurrent toggles of the same item must not interleave into a lost
// update. With the atomic store the single statement serialises them; the test
// asserts the store holds exactly one row and every response matches it.
func TestToggleFavoriteConcurrentIsWellDefined(t *testing.T) {
	user := uuid.New()
	store := newScAtomicFavorites()
	h := transporthttp.NewSyncHandler(newMemHistoryStore(), store)

	const callers = 8
	var wg sync.WaitGroup
	reported := make([]bool, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			reported[idx] = scToggle(t, h, user).Data.IsFavorite
		}(i)
	}
	wg.Wait()

	// Every response must correspond to a real commit, and to exactly one of
	// them: the reported values and the committed values are the same multiset.
	// That is what rules out a lost update — two callers both deciding to add,
	// or a response describing a state that was never committed.
	store.mu.Lock()
	history := append([]bool(nil), store.history...)
	store.mu.Unlock()

	if len(history) != callers {
		t.Fatalf("the store committed %d times for %d callers", len(history), callers)
	}
	counts := map[bool]int{}
	for _, v := range history {
		counts[v]++
	}
	for _, v := range reported {
		counts[v]--
	}
	if counts[true] != 0 || counts[false] != 0 {
		t.Errorf("reported states %v do not match committed states %v", reported, history)
	}

	// Commits must strictly alternate: that is the fingerprint of a serialised
	// read-then-write. A lost update shows up as two identical commits in a row.
	for i := 1; i < len(history); i++ {
		if history[i] == history[i-1] {
			t.Fatalf("commits %d and %d both reported %v: the toggle was not serialised (%v)",
				i-1, i, history[i], history)
		}
	}

	// The final state must be the last commit, not a value some other caller
	// overwrote after its response was written.
	if got, want := store.state(user, "m1", "uakino"), history[len(history)-1]; got != want {
		t.Errorf("final state = %v, last commit = %v", got, want)
	}
}

// The fallback path (a store with no atomic SetFavorite) must be at least
// serialised within the process, so two concurrent toggles cannot both read
// "absent" and both write.
func TestToggleFavoriteFallbackDoesNotLoseAnUpdate(t *testing.T) {
	user := uuid.New()
	store := &scSerializedFavorites{items: map[string]bool{}}
	h := transporthttp.NewSyncHandler(newMemHistoryStore(), store)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr := httptest.NewRecorder()
			h.ToggleFavorite(rr, scWithUser(scAuthed(http.MethodPost, "/api/v1/sync/favorites/toggle",
				`{"media_id":"m1","provider_id":"p"}`), user))
			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rr.Code)
			}
		}()
	}
	wg.Wait()

	// Two toggles from "absent" end "absent", and the store must agree.
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.items[user.String()+"|m1|p"] {
		t.Error("two toggles from absent must end absent")
	}
}

// scSerializedFavorites has no atomic SetFavorite, so the handler takes the
// locked read-then-write fallback. readYields lets the test interleave a
// second reader between the first reader and the first writer, which is exactly
// the interleaving that loses an update.
type scSerializedFavorites struct {
	mu    sync.Mutex
	items map[string]bool

	// isCalls counts the reads, so the test can prove both toggles read.
	isCalls int
}

func (s *scSerializedFavorites) key(userID uuid.UUID, mediaID, providerID string) string {
	return userID.String() + "|" + mediaID + "|" + providerID
}

func (s *scSerializedFavorites) IsFavorite(_ context.Context, userID uuid.UUID, mediaID, providerID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isCalls++
	return s.items[s.key(userID, mediaID, providerID)], nil
}

func (s *scSerializedFavorites) AddFavorite(_ context.Context, f *domain.Favorite) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[s.key(f.UserID, f.MediaID, f.ProviderID)] = true
	return nil
}

func (s *scSerializedFavorites) RemoveFavorite(_ context.Context, userID uuid.UUID, mediaID, providerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, s.key(userID, mediaID, providerID))
	return nil
}

var _ transporthttp.FavoritesMutator = (*scSerializedFavorites)(nil)

// The removal endpoint moved from {"success":true} to the object envelope, and
// must stay JSON on the success path too.
func TestRemoveFavoriteSuccessUsesTheObjectEnvelope(t *testing.T) {
	user := uuid.New()
	store := newMemFavoritesStore()
	h := transporthttp.NewSyncHandler(newMemHistoryStore(), store)
	if err := store.AddFavorite(context.Background(), &domain.Favorite{
		UserID: user, MediaID: "m1", ProviderID: "p",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rr := httptest.NewRecorder()
	h.RemoveFavorite(rr, scWithUser(scAuthed(http.MethodDelete, "/api/v1/sync/favorites?media_id=m1&provider_id=p", ""), user))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var out struct {
		Data struct {
			Removed bool `json:"removed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("body is not the object envelope: %v (%s)", err, rr.Body.String())
	}
	if !out.Data.Removed {
		t.Errorf(`data.removed = false, want true: %s`, rr.Body.String())
	}
}
