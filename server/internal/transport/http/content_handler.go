package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	"github.com/edhases/oxide-server/internal/search"
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
	if providerID != "" {
		results, err := h.registry.SearchProvider(r.Context(), providerID, query)
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
		return
	}

	// 1. Інтелектуальний уніфікований пошук через Bandera Online (Хвиля 3B)
	if p, exists := h.registry.Get("bandera"); exists {
		if banderaProv, ok := p.(*provider.BanderaProvider); ok {
			startTime := time.Now()
		plan := search.BuildQueryPlan(query)

		// Перевірка кешу в PostgreSQL
		cacheKey := "search:" + plan.Hash
		if h.cacheRepo != nil {
			var cached search.SearchResponse
			if hit, _ := h.cacheRepo.Get(r.Context(), cacheKey, &cached); hit {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "HIT")
				_ = json.NewEncoder(w).Encode(cached)
				return
			}
		}

		serial := 0
		if plan.TypeHint == "series" {
			serial = 1
		}

		rawResp, err := banderaProv.SearchWithMeta(r.Context(), plan.Canonical, plan.Year, serial)
		if err != nil {
			http.Error(w, `{"error":"failed to execute search"}`, http.StatusInternalServerError)
			return
		}

		var candidates []search.ScoredSearchItem
		filteredOut := 0

		for _, item := range rawResp.Items {
			year := provider.ParseFlexibleYear(item.Year)
			mediaType := item.Type.String()
			if mediaType == "" {
				mediaType = "movie"
			}

			scoreRes := search.CalculateRelevance(plan, item.Title, year, mediaType)
			if scoreRes.Dropped {
				filteredOut++
				continue
			}

			stableID := provider.GenerateStableContentID(item.Source, item.Title, year, item.Ref)
			payload := provider.BanderaItemPayload{
				ID:        stableID,
				Source:    item.Source,
				Ref:       item.Ref,
				Type:      mediaType,
				Title:     item.Title,
				Poster:    item.Poster.String(),
				Year:      year,
				IsItemRef: true,
			}
			payloadBytes, _ := json.Marshal(payload)

			candidates = append(candidates, search.ScoredSearchItem{
				MediaItem: domain.MediaItem{
					ID:            stableID,
					ProviderID:    banderaProv.ID(),
					Title:         item.Title,
					OriginalTitle: item.TitleEn.String(),
					PosterURL:     item.Poster.String(),
					Year:          year,
					Type:          mediaType,
					URL:           string(payloadBytes),
				},
				Score:      scoreRes.Score,
				MatchedBy:  scoreRes.MatchedBy,
				ClusterKey: search.GenerateClusterKey(item.Title, year, mediaType),
				Sources: []search.SearchSourceRef{
					{
						ProviderID: banderaProv.ID(),
						SourceKey:  item.Source,
						ItemID:     stableID,
					},
				},
			})
		}

		clustered := search.ClusterAndDeduplicate(candidates)

		// Збираємо статистику підджерел
		sourceStatuses := make(map[string]search.SourceStatusInfo)
		if rawResp.Meta != nil {
			for srcKey, st := range rawResp.Meta.Statuses {
				sourceStatuses[srcKey] = search.SourceStatusInfo{
					Status:    st.Status,
					Count:     st.Count,
					ElapsedMs: st.ElapsedMs,
				}
			}
		}

		segments := []search.SearchSegment{
			{
				ID:      "bandera",
				Status:  "ok",
				Count:   len(rawResp.Items),
				Sources: sourceStatuses,
			},
		}

		searchResp := search.SearchResponse{
			Query:       query,
			Canonical:   plan.Canonical,
			TookMs:      time.Since(startTime).Milliseconds(),
			Segments:    segments,
			Items:       clustered,
			FilteredOut: filteredOut,
			HasMore:     false,
		}

		// Зберігаємо результат у кеш
		if h.cacheRepo != nil {
			ttl := 15 * time.Minute
			if len(clustered) == 0 {
				ttl = 60 * time.Second
			}
			_ = h.cacheRepo.Set(r.Context(), cacheKey, "bandera", "search", searchResp, ttl)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResp)
		return
		}
	}

	// 2. Фолбек для оточень без BanderaProvider (наприклад, окремі тестові мок-хендлери)
	results, err := h.registry.SingleFlightSearch(r.Context(), query)
	if err != nil {
		http.Error(w, `{"error":"failed to execute search"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *ContentHandler) GetDetails(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] in GetDetails: %v", rec)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		}
	}()

	providerID := r.URL.Query().Get("provider")
	itemURL := r.URL.Query().Get("url")

	if providerID == "" || itemURL == "" {
		http.Error(w, `{"error":"provider and url parameters are required"}`, http.StatusBadRequest)
		return
	}

	if err := ValidateSafeURL(itemURL); err != nil {
		http.Error(w, `{"error":"invalid or unsafe item url"}`, http.StatusBadRequest)
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
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] in GetStreams: %v", rec)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		}
	}()

	providerID := r.URL.Query().Get("provider")
	itemURL := r.URL.Query().Get("url")
	seasonStr := r.URL.Query().Get("season")
	episodeStr := r.URL.Query().Get("episode")
	voiceID := r.URL.Query().Get("voice")

	if providerID == "" || itemURL == "" {
		http.Error(w, `{"error":"provider and url parameters are required"}`, http.StatusBadRequest)
		return
	}

	if err := ValidateSafeURL(itemURL); err != nil {
		http.Error(w, `{"error":"invalid or unsafe item url"}`, http.StatusBadRequest)
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
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] in Popular: %v", rec)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		}
	}()

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
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[PANIC RECOVER] in Category: %v", rec)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		}
	}()

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
