package search

import (
	"sort"
	"strings"
	"unicode"
)

const (
	// HardCutoff — поріг релевантності. Результати з нижчим балом відсіюються (drop).
	HardCutoff = 0.25
)

// ScoreResult зберігає оцінку релевантності та причину збігу
type ScoreResult struct {
	Score     float64 `json:"score"`
	MatchedBy string  `json:"matched_by"`
	Dropped   bool    `json:"dropped"`
}

// CalculateRelevance вираховує скор релевантності між планом запиту та елементом контенту
func CalculateRelevance(plan *QueryPlan, rawTitle string, itemYear int, itemType string) ScoreResult {
	if plan == nil || plan.Canonical == "" || rawTitle == "" {
		return ScoreResult{Score: 0, Dropped: true}
	}

	itemCanonical := NormalizeTitleForMatch(rawTitle)
	if itemCanonical == "" {
		return ScoreResult{Score: 0, Dropped: true}
	}

	queryCanonical := plan.Canonical

	// 1. Базовий titleScore
	baseScore := 0.0
	matchedBy := "none"

	foldedItem := FoldConfusables(itemCanonical)
	foldedQuery := FoldConfusables(queryCanonical)

	if foldedItem == foldedQuery {
		baseScore = 1.00
		matchedBy = "title_exact"
	} else if itemTokensSorted(foldedItem) == itemTokensSorted(foldedQuery) {
		baseScore = 0.90
		matchedBy = "tokens_exact"
	} else if strings.Contains(foldedItem, foldedQuery) {
		baseScore = 0.75
		matchedBy = "substring_exact"
	} else {
		// Перевірка входження токенів
		matchRatio := tokenOverlapRatio(plan.Tokens, strings.Fields(foldedItem))
		if matchRatio >= 1.0 {
			baseScore = 0.60
			matchedBy = "all_tokens"
		} else if matchRatio >= 0.66 {
			baseScore = 0.45
			matchedBy = "token_set"
		} else if matchRatio > 0 {
			baseScore = 0.35 * matchRatio
			matchedBy = "partial_tokens"
		}
	}

	if baseScore <= 0 {
		return ScoreResult{Score: 0, Dropped: true}
	}

	score := baseScore

	// 2. Штрафи
	// а) Якщо заголовок містить шумові слова
	if containsNoiseWords(rawTitle) {
		score *= 0.60
	}

	// б) Якщо заголовок занадто довгий порівняно з запитом (> 1.8x)
	if len([]rune(itemCanonical)) > int(1.8*float64(len([]rune(queryCanonical)))) {
		score *= 0.75
	}

	// 3. Бонуси та коригування за типом
	if plan.TypeHint != "" && itemType != "" {
		if strings.EqualFold(plan.TypeHint, itemType) {
			score += 0.05
		} else {
			score -= 0.10
		}
	}

	// 4. Бонуси та коригування за роком
	if plan.Year > 0 && itemYear > 0 {
		diff := plan.Year - itemYear
		if diff < 0 {
			diff = -diff
		}
		if diff == 0 {
			score += 0.05
		} else if diff > 5 {
			score -= 0.10
		}
	}

	// Нормалізація в межах 0..1
	if score > 1.0 {
		score = 1.0
	} else if score < 0 {
		score = 0
	}

	dropped := score < HardCutoff
	return ScoreResult{
		Score:     score,
		MatchedBy: matchedBy,
		Dropped:   dropped,
	}
}

// NormalizeTitleForMatch приводить назву до порівнюваного канонічного вигляду
func NormalizeTitleForMatch(raw string) string {
	var clean strings.Builder
	clean.Grow(len(raw))
	for _, r := range strings.ToLower(raw) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			clean.WriteRune(r)
		} else {
			clean.WriteRune(' ')
		}
	}
	tokens := strings.Fields(clean.String())
	return strings.Join(tokens, " ")
}

func itemTokensSorted(s string) string {
	tokens := strings.Fields(s)
	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}

func tokenOverlapRatio(queryTokens, itemTokens []string) float64 {
	if len(queryTokens) == 0 || len(itemTokens) == 0 {
		return 0
	}
	itemSet := make(map[string]struct{}, len(itemTokens))
	for _, t := range itemTokens {
		itemSet[t] = struct{}{}
	}

	matched := 0
	for _, qt := range queryTokens {
		qtFolded := FoldConfusables(qt)
		found := false
		for it := range itemSet {
			if FoldConfusables(it) == qtFolded || strings.HasPrefix(FoldConfusables(it), qtFolded) {
				found = true
				break
			}
		}
		if found {
			matched++
		}
	}
	return float64(matched) / float64(len(queryTokens))
}

func containsNoiseWords(s string) bool {
	lower := strings.ToLower(s)
	for nw := range noiseWords {
		if strings.Contains(lower, nw) {
			return true
		}
	}
	return false
}
