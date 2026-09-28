package http

import (
	"encoding/json"
	"net/http"
	"strconv"

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

	results, err := h.registry.SingleFlightSearch(r.Context(), query)
	if err != nil {
		http.Error(w, `{"error":"failed to execute search"}`, http.StatusInternalServerError)
		return
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

	p, ok := h.registry.Get(providerID)
	if !ok {
		http.Error(w, `{"error":"unknown provider"}`, http.StatusNotFound)
		return
	}

	details, err := p.GetDetails(r.Context(), itemURL)
	if err != nil {
		http.Error(w, `{"error":"failed to get media details"}`, http.StatusInternalServerError)
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

	p, ok := h.registry.Get(providerID)
	if !ok {
		http.Error(w, `{"error":"unknown provider"}`, http.StatusNotFound)
		return
	}

	resp, err := p.GetStreams(r.Context(), itemURL, season, episode, voiceID)
	if err != nil {
		http.Error(w, `{"error":"failed to get streams"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
