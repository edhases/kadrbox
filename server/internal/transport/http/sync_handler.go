package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req saveProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	mediaID := req.MediaID
	if mediaID == "" {
		mediaID = req.MediaId
	}
	providerID := req.ProviderID
	if providerID == "" {
		providerID = req.ProviderId
	}
	posterURL := req.PosterURL
	if posterURL == "" {
		posterURL = req.PosterUrl
	}
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = req.MediaTypeAlt
	}
	episodeTitle := req.EpisodeTitle
	if episodeTitle == "" {
		episodeTitle = req.EpisodeTitleA
	}
	positionMs := req.PositionMs
	if positionMs == 0 {
		positionMs = req.PositionMsAlt
	}
	durationMs := req.DurationMs
	if durationMs == 0 {
		durationMs = req.DurationMsAlt
	}
	lastStreamURL := req.LastStreamURL
	if lastStreamURL == "" {
		lastStreamURL = req.LastStreamUrl
	}
	ratingSource := req.RatingSource
	if ratingSource == nil {
		ratingSource = req.RatingSourceA
	}
	watchedAt := time.Now()
	if req.WatchedAt != nil {
		watchedAt = *req.WatchedAt
	} else if req.WatchedAtAlt != nil {
		watchedAt = *req.WatchedAtAlt
	}

	item := domain.WatchHistory{
		UserID:        userID,
		MediaID:       mediaID,
		ProviderID:    providerID,
		Title:         req.Title,
		PosterURL:     posterURL,
		Year:          req.Year,
		MediaType:     mediaType,
		Season:        req.Season,
		Episode:       req.Episode,
		EpisodeTitle:  episodeTitle,
		PositionMs:    positionMs,
		DurationMs:    durationMs,
		LastStreamURL: lastStreamURL,
		Voiceover:     req.Voiceover,
		Rating:        req.Rating,
		RatingSource:  ratingSource,
		WatchedAt:     watchedAt,
	}

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

// GetFavorites повертає всі закладки користувача
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

// ToggleFavorite додає або видаляє тайтл з обраного
func (h *SyncHandler) ToggleFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req toggleFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid payload"}`, http.StatusBadRequest)
		return
	}

	mediaID := req.MediaID
	if mediaID == "" {
		mediaID = req.MediaId
	}
	providerID := req.ProviderID
	if providerID == "" {
		providerID = req.ProviderId
	}
	posterURL := req.PosterURL
	if posterURL == "" {
		posterURL = req.PosterUrl
	}
	mediaType := req.MediaType
	if mediaType == "" {
		mediaType = req.MediaTypeAlt
	}
	ratingSource := req.RatingSource
	if ratingSource == nil {
		ratingSource = req.RatingSourceA
	}

	fav := domain.Favorite{
		UserID:       userID,
		MediaID:      mediaID,
		ProviderID:   providerID,
		Title:        req.Title,
		PosterURL:    posterURL,
		Year:         req.Year,
		MediaType:    mediaType,
		Rating:       req.Rating,
		RatingSource: ratingSource,
	}

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

// RemoveFavorite видаляє фільм з обраного
func (h *SyncHandler) RemoveFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
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
			if mediaID == "" {
				mediaID = body.MediaID
				if mediaID == "" {
					mediaID = body.MediaId
				}
			}
			if providerID == "" {
				providerID = body.ProviderID
				if providerID == "" {
					providerID = body.ProviderId
				}
			}
		}
	}

	if mediaID == "" || providerID == "" {
		http.Error(w, `{"error":"media_id and provider_id are required"}`, http.StatusBadRequest)
		return
	}

	if err := h.favoritesRepo.RemoveFavorite(r.Context(), userID, mediaID, providerID); err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"success":true}`))
}
