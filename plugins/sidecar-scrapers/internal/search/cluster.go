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
	Score      float64           `json:"score"`
	MatchedBy  string            `json:"matched_by"`
	ClusterKey string            `json:"cluster"`
	Sources    []SearchSourceRef `json:"sources"`
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

	// Phase 1: group by title + type, ignoring the year.
	byBase := make(map[string][]ScoredSearchItem)
	for _, item := range candidates {
		bk := baseClusterKey(item.Title, item.Type)
		byBase[bk] = append(byBase[bk], item)
	}

	// Phase 2: split each group by year.
	clusters := make(map[string][]ScoredSearchItem)
	for _, group := range byBase {
		for year, items := range splitByYear(group) {
			key := clusterKeyForYear(group[0].Title, group[0].Type, year)
			clusters[key] = append(clusters[key], items...)
		}
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
			// Якщо в самого елемента не було Sources, додаємо базовий реф (SourceKey = ProviderID)
			if len(it.Sources) == 0 && it.ID != "" {
				k := it.ProviderID + ":" + it.ProviderID + ":" + it.ID
				if !seenSources[k] {
					seenSources[k] = true
					combinedSources = append(combinedSources, SearchSourceRef{
						ProviderID: it.ProviderID,
						SourceKey:  it.ProviderID,
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

// normaliseYearType приводить тип медіа до канонічного вигляду.
// Порожній тип вважаємо фільмом — так само, як у GenerateClusterKey.
func normaliseYearType(itemType string) string {
	t := strings.ToLower(itemType)
	if t == "" {
		return "movie"
	}
	return t
}

// validClusterYear повертає рік, якщо він придатний для ключа,
// або 0 якщо рік невідомий/некоректний.
//
// GenerateClusterKey вже робить саме це (y<1900 || y>2100 -> 0),
// але нам потрібно знати ДО ключа, щоб вирішити, чи прилипає
// елемент до відомого року чи ні.
func validClusterYear(year int) int {
	if year < 1900 || year > 2100 {
		return 0
	}
	return year
}

// baseClusterKey — ключ БЕЗ року: foldedTitle|type.
//
// Це основа дворядної кластеризації. Рік свідомо винесено з
// базового ключа, бо саме через нього один і той самий серіал
// розпадався на дублі: lavakino знав рік 2011, uakino — ні.
func baseClusterKey(title, itemType string) string {
	clean := NormalizeTitleForMatch(title)
	folded := FoldConfusables(clean)
	return fmt.Sprintf("%s|%s", folded, normaliseYearType(itemType))
}

// clusterKeyForYear збирає повний ключ кластера для конкретного року.
// Формат той самий, що в GenerateClusterKey, тож зовнішні тести
// й клієнт не ламаються.
func clusterKeyForYear(title, itemType string, year int) string {
	clean := NormalizeTitleForMatch(title)
	folded := FoldConfusables(clean)
	return fmt.Sprintf("%s|%d|%s", folded, validClusterYear(year), normaliseYearType(itemType))
}

// splitByYear розподіляє групу елементів за роком.
//
// Правила — навмисно лагідні:
//
//   - Елементи з однаковим відомим роком лишаються разом.
//   - Елементи з РІЗНИМИ відомими роками ніколи не зливаються:
//     це захист від склейки ремейків («Форрест Гамп» 1994 і 2022).
//   - Елементи без року (0) прилипають до кластера з відомим роком,
//     якщо такий є. Якщо відомих років кілька, вони йдуть до
//     НАЙБІЛЬШОГО за розміром кластера: це компроміс на користь
//     відсутності дублів на головній.
//
// Повертає map[year][]item. Рік 0 у ключі означає «рік невідомий
// і зливатися було з чим».
func splitByYear(group []ScoredSearchItem) map[int][]ScoredSearchItem {
	out := make(map[int][]ScoredSearchItem)

	var yearless []ScoredSearchItem
	knownCount := make(map[int]int)

	for _, item := range group {
		y := validClusterYear(item.Year)
		if y == 0 {
			yearless = append(yearless, item)
			continue
		}
		out[y] = append(out[y], item)
		knownCount[y]++
	}

	if len(yearless) == 0 {
		return out
	}

	if len(knownCount) == 0 {
		// Жодного відомого року — нема з ким зливати, окремий кластер.
		out[0] = yearless
		return out
	}

	// Обираємо найбільший кластер за відомим роком.
	target, best := 0, -1
	for y, c := range knownCount {
		if c > best || (c == best && y < target) {
			target, best = y, c
		}
	}
	out[target] = append(out[target], yearless...)
	return out
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
