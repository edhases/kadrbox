package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

type SyncHandler struct {
	historyRepo   *postgres.HistoryRepository
	favoritesRepo *postgres.FavoritesRepository
}

func NewSyncHandler(historyRepo *postgres.HistoryRepository, favoritesRepo *postgres.FavoritesRepository) *SyncHandler {
	return &SyncHandler{
		historyRepo:   historyRepo,
		favoritesRepo: favoritesRepo,
	}
}

// GetHistory повертає історію перегляду користувача
func (h *SyncHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	list, err := h.historyRepo.GetUserHistory(r.Context(), userID, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"failed to get history"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// SaveProgress зберігає прогрес відтворення (ON CONFLICT DO UPDATE)
func (h *SyncHandler) SaveProgress(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var item domain.WatchHistory
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	item.UserID = userID
	if err := h.historyRepo.UpsertWatchHistory(r.Context(), &item); err != nil {
		http.Error(w, `{"error":"failed to save progress"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

// GetContinueWatching повертає список відео у процесі перегляду (5%..95%)
func (h *SyncHandler) GetContinueWatching(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	list, err := h.historyRepo.GetContinueWatching(r.Context(), userID, 20)
	if err != nil {
		http.Error(w, `{"error":"failed to get continue watching"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// GetFavorites повертає список обраного
func (h *SyncHandler) GetFavorites(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	list, err := h.favoritesRepo.GetUserFavorites(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"failed to get favorites"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

// ToggleFavorite додає або видаляє тайтл з обраного
func (h *SyncHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var fav domain.Favorite
	if err := json.NewDecoder(r.Body).Decode(&fav); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	fav.UserID = userID
	isFav, err := h.favoritesRepo.IsFavorite(r.Context(), userID, fav.MediaID, fav.ProviderID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	if isFav {
		_ = h.favoritesRepo.RemoveFavorite(r.Context(), userID, fav.MediaID, fav.ProviderID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_favorite":false}`))
	} else {
		_ = h.favoritesRepo.AddFavorite(r.Context(), &fav)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_favorite":true}`))
	}
}
