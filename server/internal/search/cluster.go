package search

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// SearchSourceRef описує конкретне джерело, з якого доступний елемент
type SearchSourceRef struct {
	ProviderID string `json:"provider_id"`
	SourceKey  string `json:"source_key"`
	ItemID     string `json:"item_id"`
	URL        string `json:"url,omitempty"`
}

// ScoredSearchItem розширює domain.MediaItem оцінкою релевантності та агрегованими джерелами
type ScoredSearchItem struct {
	domain.MediaItem
	Score       float64           `json:"score"`
	MatchedBy   string            `json:"matched_by"`
	ClusterKey  string            `json:"cluster"`
	Sources     []SearchSourceRef `json:"sources"`
}

// SourceStatusInfo описує статистику по окремому підджерелу (наприклад, uaflix, mikai)
type SourceStatusInfo struct {
	Status    string `json:"status"`
	Count     int    `json:"count"`
	ElapsedMs int64  `json:"elapsed_ms,omitempty"`
}

// SearchSegment описує результати сегменту (наприклад, bandera, tmdb)
type SearchSegment struct {
	ID      string                      `json:"id"`
	Status  string                      `json:"status"`
	Count   int                         `json:"count"`
	Sources map[string]SourceStatusInfo `json:"sources,omitempty"`
}

// SearchResponse — уніфікована серверна відповідь на пошуковий запит
type SearchResponse struct {
	Query       string             `json:"query"`
	Canonical   string             `json:"canonical"`
	TookMs      int64              `json:"took_ms"`
	Segments    []SearchSegment    `json:"segments"`
	Items       []ScoredSearchItem `json:"items"`
	FilteredOut int                `json:"filtered_out"`
	NextPage    int                `json:"next_page,omitempty"`
	HasMore     bool               `json:"has_more"`
}

// ClusterAndDeduplicate кластеризує результати з різних джерел в єдині сутності
func ClusterAndDeduplicate(candidates []ScoredSearchItem) []ScoredSearchItem {
	if len(candidates) == 0 {
		return nil
	}

	clusters := make(map[string][]ScoredSearchItem)

	for _, item := range candidates {
		cKey := GenerateClusterKey(item.Title, item.Year, item.Type)
		item.ClusterKey = cKey
		clusters[cKey] = append(clusters[cKey], item)
	}

	var result []ScoredSearchItem

	for cKey, group := range clusters {
		// Обираємо детермінованого представника групи:
		// 1. Найвищий score
		// 2. Наявність рейтингу (вищий рейтинг)
		// 3. Наявність постера
		// 4. Лексикографічно перший ID
		sort.Slice(group, func(i, j int) bool {
			if math.Abs(group[i].Score-group[j].Score) > 0.001 {
				return group[i].Score > group[j].Score
			}
			if (group[i].Rating > 0) != (group[j].Rating > 0) {
				return group[i].Rating > 0
			}
			if group[i].Rating != group[j].Rating {
				return group[i].Rating > group[j].Rating
			}
			hasPosterI := group[i].PosterURL != ""
			hasPosterJ := group[j].PosterURL != ""
			if hasPosterI != hasPosterJ {
				return hasPosterI
			}
			return group[i].ID < group[j].ID
		})

		rep := group[0]
		rep.ClusterKey = cKey

		// Збираємо всі унікальні джерела з групи
		seenSources := make(map[string]bool)
		var combinedSources []SearchSourceRef

		for _, it := range group {
			for _, src := range it.Sources {
				k := src.ProviderID + ":" + src.SourceKey + ":" + src.ItemID
				if !seenSources[k] {
					seenSources[k] = true
					combinedSources = append(combinedSources, src)
				}
			}
			// Якщо в самого елемента не було Sources, додаємо базовий реф
			if len(it.Sources) == 0 && it.ID != "" {
				k := it.ProviderID + "::" + it.ID
				if !seenSources[k] {
					seenSources[k] = true
					combinedSources = append(combinedSources, SearchSourceRef{
						ProviderID: it.ProviderID,
						ItemID:     it.ID,
						URL:        it.URL,
					})
				}
			}
		}

		rep.Sources = combinedSources
		result = append(result, rep)
	}

	// Сортуємо фінальний список за релевантністю (score спадання)
	sort.Slice(result, func(i, j int) bool {
		if math.Abs(result[i].Score-result[j].Score) > 0.001 {
			return result[i].Score > result[j].Score
		}
		if result[i].Rating != result[j].Rating {
			return result[i].Rating > result[j].Rating
		}
		return result[i].Title < result[j].Title
	})

	return result
}

// GenerateClusterKey формує ключ кластера у форматі canonicalTitle|year|type
func GenerateClusterKey(title string, year int, itemType string) string {
	cleanTitle := NormalizeTitleForMatch(title)
	folded := FoldConfusables(cleanTitle)

	t := strings.ToLower(itemType)
	if t == "" {
		t = "movie"
	}

	y := year
	if y < 1900 || y > 2100 {
		y = 0
	}

	return fmt.Sprintf("%s|%d|%s", folded, y, t)
}
