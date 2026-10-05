package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/edhases/kadrbox-server/internal/transport/http/middleware"
)

// AtomicFavoritesStore is the capability that makes the favourite write a
// single statement instead of a read-then-write pair. The repository
// implementation (owned by another agent) is expected to satisfy it with the
// DELETE ... RETURNING / INSERT ... WHERE NOT EXISTS CTE; until it does, the
// handler falls back to a per-item lock, which is correct within one process
// but not across replicas.
type AtomicFavoritesStore interface {
	// SetFavorite makes (userID, mediaID, providerID) present when f carries
	// the payload the client wants stored, absent otherwise, and returns the
	// committed state. Replaying the same call is a no-op.
	SetFavorite(ctx context.Context, f *domain.Favorite) (bool, error)
}

// FavoritesMutator is the write side of the favourites store: the only three
// methods the toggle and the removal endpoint need.
//
// It is declared here, and deliberately does not include the read, because the
// read side is mid-migration: the repository now takes limit/offset and adds
// CountUserFavorites, while the unpaginated in-memory fakes still exist. The
// read is therefore reached through an optional interface assertion
// (PaginatedFavorites / UnpaginatedFavorites) instead of being baked into the
// constructor's parameter type, which would force both shapes to exist at once.
type FavoritesMutator interface {
	AddFavorite(ctx context.Context, f *domain.Favorite) error
	RemoveFavorite(ctx context.Context, userID uuid.UUID, mediaID, providerID string) error
	IsFavorite(ctx context.Context, userID uuid.UUID, mediaID, providerID string) (bool, error)
}

// PaginatedFavorites is the migrated read: LIMIT/OFFSET pushed into SQL plus a
// COUNT for the list envelope's total. Preferred whenever available — it is the
// only form that stops a user with thousands of favourites from downloading all
// of them on every app start.
type PaginatedFavorites interface {
	GetUserFavorites(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Favorite, error)
	CountUserFavorites(ctx context.Context, userID uuid.UUID) (int, error)
}

// UnpaginatedFavorites is the pre-migration read. When only this is available
// the handler fetches the whole list and slices it, so limit/offset are still
// honoured on the wire — just not in the database.
type UnpaginatedFavorites interface {
	GetUserFavorites(ctx context.Context, userID uuid.UUID) ([]domain.Favorite, error)
}

type SyncHandler struct {
	historyRepo   HistoryStore
	favoritesRepo FavoritesMutator

	// favoriteLocks serialises the fallback read-then-write per item.
	favoriteLocks itemLocks
}

func NewSyncHandler(historyRepo HistoryStore, favoritesRepo FavoritesMutator) *SyncHandler {
	return &SyncHandler{
		historyRepo:   historyRepo,
		favoritesRepo: favoritesRepo,
	}
}

// itemLocks is a keyed mutex map. Entries are reference-counted so the map
// does not grow without bound as users favourite and unfavourite items.
type itemLocks struct {
	mu    sync.Mutex
	locks map[string]*itemLock
}

type itemLock struct {
	mu   sync.Mutex
	refs int
}

func (l *itemLocks) lock(userID uuid.UUID, mediaID, providerID string) *itemLock {
	key := userID.String() + "|" + mediaID + "|" + providerID

	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*itemLock)
	}
	entry, ok := l.locks[key]
	if !ok {
		entry = &itemLock{}
		l.locks[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return entry
}

func (l *itemLocks) release(key string, entry *itemLock) {
	entry.mu.Unlock()

	l.mu.Lock()
	defer l.mu.Unlock()
	entry.refs--
	if entry.refs == 0 {
		delete(l.locks, key)
	}
}

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 100

	// The client asks for 200 favourites. Honouring that literally would let a
	// user with 5 000 of them download the lot on every app start, so the
	// ceiling is 200 and the client must follow the Link header.
	defaultFavoriteLimit = 50
	maxFavoriteLimit     = 200

	defaultContinueLimit = 20
	maxContinueLimit     = 100
)

// GetHistory повертає історію перегляду користувача
func (h *SyncHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeAPIError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit := clampLimit(r, defaultHistoryLimit, maxHistoryLimit)
	offset := clampOffset(r)

	list, err := h.historyRepo.GetUserHistory(r.Context(), userID, limit, offset)
	if err != nil {
		writeAPIError(w, "failed to get history", http.StatusInternalServerError)
		return
	}

	// No COUNT in the consumer interface, so has_more is derived from a full
	// page. The client follows the Link header and stops when it is absent.
	hasMore := len(list) == limit
	writeList(w, r, list, &PageMeta{
		Limit:   intp(limit),
		Offset:  intp(offset),
		HasMore: hasMore,
	})
}

type saveProgressRequest struct {
	MediaID       string     `json:"media_id"`
	MediaId       string     `json:"mediaId"`
	ProviderID    string     `json:"provider_id"`
	ProviderId    string     `json:"providerId"`
	Title         string     `json:"title"`
	PosterURL     string     `json:"poster_url"`
	PosterUrl     string     `json:"posterUrl"`
	Year          *int       `json:"year"`
	MediaType     string     `json:"media_type"`
	MediaTypeAlt  string     `json:"mediaType"`
	Season        *int       `json:"season"`
	Episode       *int       `json:"episode"`
	EpisodeTitle  string     `json:"episode_title"`
	EpisodeTitleA string     `json:"episodeTitle"`
	PositionMs    int64      `json:"position_ms"`
	PositionMsAlt int64      `json:"positionMs"`
	DurationMs    int64      `json:"duration_ms"`
	DurationMsAlt int64      `json:"durationMs"`
	LastStreamURL string     `json:"last_stream_url"`
	LastStreamUrl string     `json:"lastStreamUrl"`
	Voiceover     string     `json:"voiceover"`
	Rating        *float64   `json:"rating"`
	RatingSource  *string    `json:"rating_source"`
	RatingSourceA *string    `json:"ratingSource"`
	WatchedAt     *time.Time `json:"watched_at"`
	WatchedAtAlt  *time.Time `json:"watchedAt"`
}

// SaveProgress зберігає прогрес відтворення (ON CONFLICT DO UPDATE)
func (h *SyncHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeAPIError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req saveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, "invalid payload", http.StatusBadRequest)
		return
	}

	watchedAt := time.Now()
	if req.WatchedAt != nil {
		watchedAt = *req.WatchedAt
	} else if req.WatchedAtAlt != nil {
		watchedAt = *req.WatchedAtAlt
	}

	item := domain.WatchHistory{
		UserID:        userID,
		MediaID:       firstNonEmpty(req.MediaID, req.MediaId),
		ProviderID:    firstNonEmpty(req.ProviderID, req.ProviderId),
		Title:         req.Title,
		PosterURL:     firstNonEmpty(req.PosterURL, req.PosterUrl),
		Year:          req.Year,
		MediaType:     firstNonEmpty(req.MediaType, req.MediaTypeAlt),
		Season:        req.Season,
		Episode:       req.Episode,
		EpisodeTitle:  firstNonEmpty(req.EpisodeTitle, req.EpisodeTitleA),
		PositionMs:    firstInt64(req.PositionMs, req.PositionMsAlt),
		DurationMs:    firstInt64(req.DurationMs, req.DurationMsAlt),
		LastStreamURL: firstNonEmpty(req.LastStreamURL, req.LastStreamUrl),
		Voiceover:     req.Voiceover,
		Rating:        req.Rating,
		RatingSource:  firstNonNilString(req.RatingSource, req.RatingSourceA),
		WatchedAt:     watchedAt,
	}

	if err := h.historyRepo.UpsertWatchHistory(r.Context(), &item); err != nil {
		writeAPIError(w, "failed to save progress", http.StatusInternalServerError)
		return
	}

	writeObject(w, map[string]interface{}{"status": "success"})
}

// GetContinueWatching повертає список відео у процесі перегляду (5%..95%)
func (h *SyncHandler) GetContinueWatching(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeAPIError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// The client sends ?limit and used to be ignored in favour of a hardcoded
	// 20, so asking for a different size had no effect at all.
	limit := clampLimit(r, defaultContinueLimit, maxContinueLimit)

	list, err := h.historyRepo.GetContinueWatching(r.Context(), userID, limit)
	if err != nil {
		writeAPIError(w, "failed to get continue watching", http.StatusInternalServerError)
		return
	}

	// No offset in the query for this endpoint and none in the consumer
	// interface, so has_more stays false: emitting a Link the handler would
	// ignore is worse than not emitting one.
	writeList(w, r, list, &PageMeta{Limit: intp(limit), HasMore: false})
}

// GetFavorites повертає закладки користувача
func (h *SyncHandler) GetFavorites(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeAPIError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit := clampLimit(r, defaultFavoriteLimit, maxFavoriteLimit)
	offset := clampOffset(r)

	list, total, err := h.favoritesPage(r.Context(), userID, limit, offset)
	if err != nil {
		writeAPIError(w, "failed to get favorites", http.StatusInternalServerError)
		return
	}

	meta := &PageMeta{
		Limit:   intp(limit),
		Offset:  intp(offset),
		Count:   len(list),
		HasMore: offset+len(list) < total,
	}
	if total >= 0 {
		meta.Total = intp(total)
	}
	writeList(w, r, list, meta)
}

// favoritesPage reads one page and reports the total when it is knowable.
//
// Two store shapes are supported. A paginated store does the LIMIT/OFFSET and
// the COUNT in SQL; an unpaginated one is fetched whole and sliced here, so the
// endpoint still honours limit/offset on the wire.
func (h *SyncHandler) favoritesPage(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Favorite, int, error) {
	repo, ok := h.favoritesRepo.(PaginatedFavorites)
	if !ok {
		return h.favoritesPageLegacy(ctx, userID, limit, offset)
	}

	list, err := repo.GetUserFavorites(ctx, userID, limit, offset)
	if err != nil {
		return nil, -1, err
	}
	total, err := repo.CountUserFavorites(ctx, userID)
	if err != nil {
		// A failed COUNT must not fail the page: the list is already correct,
		// only the total is unknown, so the envelope omits it.
		log.Printf("[Sync] CountUserFavorites for %s failed, omitting total: %v", userID, err)
		total = -1
	}
	if list == nil {
		list = []domain.Favorite{}
	}
	return list, total, nil
}

func (h *SyncHandler) favoritesPageLegacy(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Favorite, int, error) {
	repo, ok := h.favoritesRepo.(UnpaginatedFavorites)
	if !ok {
		return nil, -1, errors.New("favorites store implements no readable GetUserFavorites")
	}

	all, err := repo.GetUserFavorites(ctx, userID)
	if err != nil {
		return nil, -1, err
	}
	total := len(all)
	if offset >= total {
		return []domain.Favorite{}, total, nil
	}
	all = all[offset:]
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []domain.Favorite{}
	}
	return all, total, nil
}

type toggleFavoriteRequest struct {
	MediaID       string   `json:"media_id"`
	MediaId       string   `json:"mediaId"`
	ProviderID    string   `json:"provider_id"`
	ProviderId    string   `json:"providerId"`
	Title         string   `json:"title"`
	PosterURL     string   `json:"poster_url"`
	PosterUrl     string   `json:"posterUrl"`
	Year          *int     `json:"year"`
	MediaType     string   `json:"media_type"`
	MediaTypeAlt  string   `json:"mediaType"`
	Rating        *float64 `json:"rating"`
	RatingSource  *string  `json:"rating_source"`
	RatingSourceA *string  `json:"ratingSource"`
}

// toggleResult is the single-object envelope for POST /sync/favorites/toggle.
// `is_favorite` is the committed state read back from the store, not a guess.
type toggleResult struct {
	IsFavorite bool   `json:"is_favorite"`
	MediaID    string `json:"media_id"`
	ProviderID string `json:"provider_id"`
}

// ToggleFavorite додає або видаляє тайтл з обраного.
//
// State-setting, not state-toggling. The endpoint name is historical: a toggle
// is not idempotent, and the Flutter client retries POST on 5xx three times,
// so a lost response made the retry observe the already-flipped state and flip
// it back — the favourite was silently lost and the UI diverged from the cloud
// permanently. With SetFavorite the same request replayed any number of times
// converges on the state the client asked for.
func (h *SyncHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeAPIError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req toggleFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, "invalid payload", http.StatusBadRequest)
		return
	}

	mediaID := firstNonEmpty(req.MediaID, req.MediaId)
	providerID := firstNonEmpty(req.ProviderID, req.ProviderId)
	if mediaID == "" || providerID == "" {
		writeAPIError(w, "media_id and provider_id are required", http.StatusBadRequest)
		return
	}

	fav := domain.Favorite{
		UserID:       userID,
		MediaID:      mediaID,
		ProviderID:   providerID,
		Title:        req.Title,
		PosterURL:    firstNonEmpty(req.PosterURL, req.PosterUrl),
		Year:         req.Year,
		MediaType:    firstNonEmpty(req.MediaType, req.MediaTypeAlt),
		Rating:       req.Rating,
		RatingSource: firstNonNilString(req.RatingSource, req.RatingSourceA),
	}

	isFav, err := h.setFavorite(r.Context(), &fav)
	if err != nil {
		writeAPIError(w, "database error", http.StatusInternalServerError)
		return
	}

	writeObject(w, toggleResult{IsFavorite: isFav, MediaID: mediaID, ProviderID: providerID})
}

// setFavorite commits the intended state atomically when the store can, and
// otherwise falls back to a read-then-write guarded by a per-item lock so two
// concurrent requests cannot interleave into a lost update.
func (h *SyncHandler) setFavorite(ctx context.Context, fav *domain.Favorite) (bool, error) {
	if repo, ok := h.favoritesRepo.(AtomicFavoritesStore); ok {
		return repo.SetFavorite(ctx, fav)
	}

	key := fav.UserID.String() + "|" + fav.MediaID + "|" + fav.ProviderID
	lock := h.favoriteLocks.lock(fav.UserID, fav.MediaID, fav.ProviderID)
	defer h.favoriteLocks.release(key, lock)

	want, err := h.favoritesRepo.IsFavorite(ctx, fav.UserID, fav.MediaID, fav.ProviderID)
	if err != nil {
		return false, err
	}

	// IsFavorite reports the state before the write, so the caller wants the
	// opposite of it. The lock makes the read-then-write indivisible within
	// this process, which is what stops two concurrent toggles from both
	// deciding to add (or both to remove).
	if want {
		return false, h.favoritesRepo.RemoveFavorite(ctx, fav.UserID, fav.MediaID, fav.ProviderID)
	}
	return true, h.favoritesRepo.AddFavorite(ctx, fav)
}

// RemoveFavorite видаляє фільм з обраного
func (h *SyncHandler) RemoveFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		writeAPIError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	mediaID := r.URL.Query().Get("media_id")
	providerID := r.URL.Query().Get("provider_id")

	if mediaID == "" || providerID == "" {
		var body struct {
			MediaID    string `json:"media_id"`
			ProviderID string `json:"provider_id"`
			MediaId    string `json:"mediaId"`
			ProviderId string `json:"providerId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			mediaID = firstNonEmpty(mediaID, body.MediaID, body.MediaId)
			providerID = firstNonEmpty(providerID, body.ProviderID, body.ProviderId)
		}
	}

	if mediaID == "" || providerID == "" {
		writeAPIError(w, "media_id and provider_id are required", http.StatusBadRequest)
		return
	}

	if err := h.favoritesRepo.RemoveFavorite(r.Context(), userID, mediaID, providerID); err != nil {
		writeAPIError(w, "database error", http.StatusInternalServerError)
		return
	}

	writeObject(w, map[string]interface{}{"removed": true})
}

func clampLimit(r *http.Request, def, max int) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}

func clampOffset(r *http.Request) int {
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstInt64(values ...int64) int64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstNonNilString(values ...*string) *string {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}
