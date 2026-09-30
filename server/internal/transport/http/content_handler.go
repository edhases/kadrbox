package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	"github.com/edhases/oxide-server/internal/repository/postgres"
)

type ContentHandler struct {
	registry  *provider.Registry
	cacheRepo *postgres.CacheRepository
}

func NewContentHandler(registry *provider.Registry, cacheRepo *postgres.CacheRepository) *ContentHandler {
	return &ContentHandler{
		registry:  registry,
		cacheRepo: cacheRepo,
	}
}

func (h *ContentHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, `{"error":"search query parameter 'q' is required"}`, http.StatusBadRequest)
		return
	}

	providerID := r.URL.Query().Get("provider")
	var results []domain.MediaItem
	var err error

	if providerID != "" {
		results, err = h.registry.SearchProvider(r.Context(), providerID, query)
		if err != nil {
			switch {
			case errors.Is(err, provider.ErrProviderDisabled):
				http.Error(w, `{"error":"provider disabled"}`, http.StatusForbidden)
			case errors.Is(err, provider.ErrProviderNotFound):
				http.Error(w, `{"error":"unknown provider"}`, http.StatusNotFound)
			default:
				http.Error(w, `{"error":"failed to execute search"}`, http.StatusInternalServerError)
			}
			return
		}
	} else {
		results, err = h.registry.SingleFlightSearch(r.Context(), query)
		if err != nil {
			http.Error(w, `{"error":"failed to execute search"}`, http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *ContentHandler) GetDetails(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	itemURL := r.URL.Query().Get("url")

	if providerID == "" || itemURL == "" {
		http.Error(w, `{"error":"provider and url parameters are required"}`, http.StatusBadRequest)
		return
	}

	details, err := h.registry.Details(r.Context(), providerID, itemURL)
	if err != nil {
		switch {
		case errors.Is(err, provider.ErrProviderDisabled):
			http.Error(w, `{"error":"provider disabled"}`, http.StatusForbidden)
		case errors.Is(err, provider.ErrProviderNotFound):
			http.Error(w, `{"error":"unknown provider"}`, http.StatusNotFound)
		default:
			http.Error(w, `{"error":"failed to get media details"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(details)
}

func (h *ContentHandler) GetStreams(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	itemURL := r.URL.Query().Get("url")
	seasonStr := r.URL.Query().Get("season")
	episodeStr := r.URL.Query().Get("episode")
	voiceID := r.URL.Query().Get("voice")

	if providerID == "" || itemURL == "" {
		http.Error(w, `{"error":"provider and url parameters are required"}`, http.StatusBadRequest)
		return
	}

	season, _ := strconv.Atoi(seasonStr)
	episode, _ := strconv.Atoi(episodeStr)

	resp, err := h.registry.Streams(r.Context(), providerID, itemURL, season, episode, voiceID)
	if err != nil {
		switch {
		case errors.Is(err, provider.ErrProviderDisabled):
			http.Error(w, `{"error":"provider disabled"}`, http.StatusForbidden)
		case errors.Is(err, provider.ErrProviderNotFound):
			http.Error(w, `{"error":"unknown provider"}`, http.StatusNotFound)
		case errors.Is(err, provider.ErrUnresolvablePlayer):
			http.Error(w, `{"error":"player page exposes no playable media"}`, http.StatusUnprocessableEntity)
		default:
			http.Error(w, `{"error":"failed to get streams"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Providers — GET /api/v1/content/providers
// Публічний каталог провайдерів: джерело правди для застосунків.
func (h *ContentHandler) Providers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.registry.Catalog())
}

// Popular — GET /api/v1/content/popular
func (h *ContentHandler) Popular(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	contentType := r.URL.Query().Get("type")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	results, err := h.registry.Popular(r.Context(), providerID, contentType, page)
	if err != nil {
		switch {
		case errors.Is(err, provider.ErrProviderDisabled):
			http.Error(w, `{"error":"provider disabled"}`, http.StatusForbidden)
		case errors.Is(err, provider.ErrProviderNotFound):
			http.Error(w, `{"error":"unknown provider"}`, http.StatusNotFound)
		default:
			http.Error(w, `{"error":"failed to load popular content"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

// Category — GET /api/v1/content/category
func (h *ContentHandler) Category(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	category := r.URL.Query().Get("category")
	contentType := r.URL.Query().Get("type")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	results, err := h.registry.Category(r.Context(), providerID, category, contentType, page)
	if err != nil {
		switch {
		case errors.Is(err, provider.ErrProviderDisabled):
			http.Error(w, `{"error":"provider disabled"}`, http.StatusForbidden)
		case errors.Is(err, provider.ErrProviderNotFound):
			http.Error(w, `{"error":"unknown provider"}`, http.StatusNotFound)
		default:
			http.Error(w, `{"error":"failed to load category content"}`, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
