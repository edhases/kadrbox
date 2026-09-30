package search_test

import (
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/search"
)

func TestBuildQueryPlan_NoiseAndYear(t *testing.T) {
	// Перевірка вирізки шуму (1080p, sub, ukr), витягування року та типу
	plan := search.BuildQueryPlan("  Дюна 2021 1080p WEB-DL UKR Sub  ")
	if plan.Year != 2021 {
		t.Errorf("expected year 2021, got %d", plan.Year)
	}
	if plan.Canonical != "дюна" {
		t.Errorf("expected canonical 'дюна', got %q", plan.Canonical)
	}
	if plan.Hash == "" {
		t.Errorf("expected non-empty hash")
	}

	// Перевірка type hint
	seriesPlan := search.BuildQueryPlan("Фарґо 2 сезон серіал")
	if seriesPlan.TypeHint != "series" {
		t.Errorf("expected typeHint 'series', got %q", seriesPlan.TypeHint)
	}
}

func TestCalculateRelevance_ScoresAndCutoff(t *testing.T) {
	plan := search.BuildQueryPlan("Дюна 2021")

	// 1. Точний збіг назви і року -> високий бал (> 0.9)
	resExact := search.CalculateRelevance(plan, "Дюна", 2021, "movie")
	if resExact.Dropped || resExact.Score < 0.90 {
		t.Errorf("expected exact match to not be dropped and score >= 0.90, got %+v", resExact)
	}

	// 2. Частковий збіг -> прийнятний бал
	resPartial := search.CalculateRelevance(plan, "Дюна: Частина друга", 2024, "movie")
	if resPartial.Dropped || resPartial.Score < 0.50 {
		t.Errorf("expected partial match to pass cutoff, got %+v", resPartial)
	}

	// 3. Абсолютно нерелевантний тайтл -> менше ніж HardCutoff (0.25) -> Dropped
	resIrrelevant := search.CalculateRelevance(plan, "Матриця: Воскресіння", 2021, "movie")
	if !resIrrelevant.Dropped {
		t.Errorf("expected completely irrelevant item to be dropped, got %+v", resIrrelevant)
	}
	if resIrrelevant.Score >= search.HardCutoff {
		t.Errorf("expected score < %f, got %f", search.HardCutoff, resIrrelevant.Score)
	}
}

func TestClusterAndDeduplicate_CombinesSourcesAndPicksBest(t *testing.T) {
	candidates := []search.ScoredSearchItem{
		{
			MediaItem: domain.MediaItem{
				ID:        "uaflix_dune",
				Title:     "Дюна",
				Year:      2021,
				Type:      "movie",
				PosterURL: "",
				Rating:    7.5,
			},
			Score: 0.95,
			Sources: []search.SearchSourceRef{
				{ProviderID: "bandera", SourceKey: "uaflix", ItemID: "101"},
			},
		},
		{
			MediaItem: domain.MediaItem{
				ID:        "mikai_dune",
				Title:     "Дюна",
				Year:      2021,
				Type:      "movie",
				PosterURL: "https://example.com/poster.jpg",
				Rating:    8.2, // кращий рейтинг і є постер
			},
			Score: 0.95,
			Sources: []search.SearchSourceRef{
				{ProviderID: "bandera", SourceKey: "mikai", ItemID: "202"},
			},
		},
	}

	clustered := search.ClusterAndDeduplicate(candidates)
	if len(clustered) != 1 {
		t.Fatalf("expected 1 clustered item from 2 duplicates, got %d", len(clustered))
	}

	rep := clustered[0]
	// Перевіряємо вибір детермінованого представника (з рейтингом 8.2 та постером)
	if rep.Rating != 8.2 || rep.PosterURL == "" {
		t.Errorf("expected representative with higher rating and poster, got %+v", rep)
	}

	// Перевіряємо об'єднання джерел
	if len(rep.Sources) != 2 {
		t.Fatalf("expected 2 combined sources in representative, got %d", len(rep.Sources))
	}
}
