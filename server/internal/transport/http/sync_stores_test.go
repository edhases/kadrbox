package http_test

// In-memory favourites and history stores for the sync handler, plus a rig
// that assembles the handler the way cmd/api does.

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/edhases/oxide-server/internal/domain"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

type memFavoritesStore struct {
	mu sync.Mutex

	// keyed by userID|mediaID|providerID, mirroring the unique constraint.
	items map[string]*domain.Favorite

	removed   int
	added     int
	lastUser  uuid.UUID
	lastMedia string
	lastProv  string
	addErr    error
	removeErr error
	listErr   error
	isFavErr  error
}

func newMemFavoritesStore() *memFavoritesStore {
	return &memFavoritesStore{items: map[string]*domain.Favorite{}}
}

func favKey(userID uuid.UUID, mediaID, providerID string) string {
	return userID.String() + "|" + mediaID + "|" + providerID
}

func (s *memFavoritesStore) AddFavorite(ctx context.Context, f *domain.Favorite) error {
	if s.addErr != nil {
		return s.addErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.added++
	stored := *f
	if stored.ID == uuid.Nil {
		stored.ID = uuid.New()
	}
	stored.UserID = f.UserID
	s.items[favKey(f.UserID, f.MediaID, f.ProviderID)] = &stored
	return nil
}

func (s *memFavoritesStore) GetUserFavorites(ctx context.Context, userID uuid.UUID) ([]domain.Favorite, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []domain.Favorite
	for key, f := range s.items {
		if f.UserID == userID {
			_ = key
			out = append(out, *f)
		}
	}
	// Stable order so assertions do not depend on map iteration.
	sort.Slice(out, func(i, j int) bool { return out[i].MediaID < out[j].MediaID })
	return out, nil
}

func (s *memFavoritesStore) IsFavorite(ctx context.Context, userID uuid.UUID, mediaID, providerID string) (bool, error) {
	if s.isFavErr != nil {
		return false, s.isFavErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[favKey(userID, mediaID, providerID)]
	return ok, nil
}

func (s *memFavoritesStore) RemoveFavorite(ctx context.Context, userID uuid.UUID, mediaID, providerID string) error {
	if s.removeErr != nil {
		return s.removeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removed++
	s.lastUser = userID
	s.lastMedia = mediaID
	s.lastProv = providerID
	delete(s.items, favKey(userID, mediaID, providerID))
	return nil
}

type memHistoryStore struct {
	mu sync.Mutex

	items map[string]*domain.WatchHistory

	lastUpsert *domain.WatchHistory
	upsertErr  error
	listErr    error
}

func newMemHistoryStore() *memHistoryStore {
	return &memHistoryStore{items: map[string]*domain.WatchHistory{}}
}

func histKey(userID uuid.UUID, mediaID, providerID string, season, episode *int) string {
	s := "-"
	e := "-"
	if season != nil {
		s = string(rune('0' + *season))
	}
	if episode != nil {
		e = string(rune('0' + *episode))
	}
	return userID.String() + "|" + mediaID + "|" + providerID + "|" + s + "|" + e
}

func (s *memHistoryStore) UpsertWatchHistory(ctx context.Context, h *domain.WatchHistory) error {
	if s.upsertErr != nil {
		return s.upsertErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := *h
	s.lastUpsert = &stored
	if stored.ID == uuid.Nil {
		stored.ID = uuid.New()
	}
	s.items[histKey(h.UserID, h.MediaID, h.ProviderID, h.Season, h.Episode)] = &stored
	return nil
}

func (s *memHistoryStore) GetUserHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.WatchHistory, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var all []domain.WatchHistory
	for _, h := range s.items {
		if h.UserID == userID {
			all = append(all, *h)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].MediaID < all[j].MediaID })
	if offset > 0 {
		if offset >= len(all) {
			return nil, nil
		}
		all = all[offset:]
	}
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func (s *memHistoryStore) GetContinueWatching(ctx context.Context, userID uuid.UUID, limit int) ([]domain.WatchHistory, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []domain.WatchHistory
	for _, h := range s.items {
		// Anything under a minute in is noise, not a resume point.
		if h.UserID == userID && h.DurationMs > 0 {
			watched := float64(h.PositionMs) / float64(h.DurationMs)
			if watched > 0.01 && watched < 0.95 {
				out = append(out, *h)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MediaID < out[j].MediaID })
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

type syncRig struct {
	handler *transporthttp.SyncHandler
	favs    *memFavoritesStore
	hist    *memHistoryStore
}

func newSyncRig() *syncRig {
	favs := newMemFavoritesStore()
	hist := newMemHistoryStore()
	return &syncRig{
		handler: transporthttp.NewSyncHandler(hist, favs),
		favs:    favs,
		hist:    hist,
	}
}

var (
	_ transporthttp.FavoritesStore = (*memFavoritesStore)(nil)
	_ transporthttp.HistoryStore   = (*memHistoryStore)(nil)
)

var _ = errors.New
