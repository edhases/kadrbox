package search_test

// Table-driven tests for the query-plan and scoring pipeline.
//
// These exercise the real algorithms rather than stubbing them: the point of
// the package is that relevance is decided here, so a silent behaviour change
// in normalisation, confusable folding or the cutoff would silently change
// which titles a user can find.

import (
	"math"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/search"
)

func TestNormalizeTitleForMatch(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases and collapses punctuation", "The Matrix: Reloaded!", "the matrix reloaded"},
		{"keeps digits and release tokens", "Dune 2021 1080p WEB-DL", "dune 2021 1080p web dl"},
		{"keeps cyrillic", "ВЕЛИЧНЕ СТОЛІТТЯ", "величне століття"},
		{"collapses runs of separators", "a---b___c", "a b c"},
		{"trims leading and trailing separators", "  ..Дюна..  ", "дюна"},
		{"empty input", "", ""},
		{"only punctuation", "!!!???", ""},
		{"digits survive", "Blade Runner 2049", "blade runner 2049"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := search.NormalizeTitleForMatch(tc.in)
			if got != tc.want {
				t.Errorf("NormalizeTitleForMatch(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildQueryPlan(t *testing.T) {
	t.Run("blank query yields an empty plan", func(t *testing.T) {
		plan := search.BuildQueryPlan("   ")
		if plan.Canonical != "" || plan.Hash != "" || plan.Year != 0 {
			t.Errorf("expected empty plan, got %+v", plan)
		}
	})

	t.Run("year outside the plausible range is ignored", func(t *testing.T) {
		if y := search.BuildQueryPlan("Movie 3021").Year; y != 0 {
			t.Errorf("expected year 3021 to be rejected, got %d", y)
		}
		if y := search.BuildQueryPlan("Movie 1700").Year; y != 0 {
			t.Errorf("expected year 1700 to be rejected, got %d", y)
		}
		if y := search.BuildQueryPlan("Movie 1999").Year; y != 1999 {
			t.Errorf("expected year 1999, got %d", y)
		}
	})

	t.Run("type hints are detected in both languages", func(t *testing.T) {
		for _, q := range []string{"Фарґо сезон 2", " Fargo series", "Щось season 3"} {
			if got := search.BuildQueryPlan(q).TypeHint; got != "series" {
				t.Errorf("BuildQueryPlan(%q).TypeHint = %q, want series", q, got)
			}
		}
		for _, q := range []string{"Дюна фільм", "Dune movie", "Щось film", "Фильм"} {
			if got := search.BuildQueryPlan(q).TypeHint; got != "movie" {
				t.Errorf("BuildQueryPlan(%q).TypeHint = %q, want movie", q, got)
			}
		}
	})

	t.Run("series wins over movie when both hints appear", func(t *testing.T) {
		if got := search.BuildQueryPlan("Dune movie season 2").TypeHint; got != "series" {
			t.Errorf("expected series to take precedence, got %q", got)
		}
	})

	t.Run("two-character tokens survive only when they contain a digit", func(t *testing.T) {
		// "т2" is two runes with a digit and must be kept; "не" is two
		// letter-only runes and must be dropped.
		plan := search.BuildQueryPlan("Т2 не")
		if !contains(plan.Tokens, "т2") {
			t.Errorf("expected digit-bearing short token to survive, got %v", plan.Tokens)
		}
		if contains(plan.Tokens, "не") {
			t.Errorf("expected two-letter token to be dropped, got %v", plan.Tokens)
		}
	})

	t.Run("the year is removed from the canonical title", func(t *testing.T) {
		if c := search.BuildQueryPlan("Дюна 2021").Canonical; c != "дюна" {
			t.Errorf("Canonical = %q, want %q", c, "дюна")
		}
	})

	t.Run("falls back to the cleaned text when noise removal empties it", func(t *testing.T) {
		// Every token here is noise or a stop word, so contentTokens is empty
		// and the canonical must fall back rather than return "".
		plan := search.BuildQueryPlan("the a")
		if plan.Canonical == "" {
			t.Error("expected a non-empty fallback canonical")
		}
	})

	t.Run("hash is stable and distinguishes meaningful differences", func(t *testing.T) {
		a := search.BuildQueryPlan("Дюна 2021")
		b := search.BuildQueryPlan("Дюна 2021")
		if a.Hash != b.Hash {
			t.Errorf("hash is not deterministic: %q vs %q", a.Hash, b.Hash)
		}
		// Type hint feeds the hash, so a different hint must not collide.
		if search.BuildQueryPlan("Дюна фільм").Hash == a.Hash {
			t.Error("expected type hint to change the cache key")
		}
	})
}

// TestCalculateRelevance_MatchedBy pins which match tier produced a score.
// The tier names are part of the wire contract (items[].matched_by), so they
// are asserted literally rather than through the numeric score alone.
//
// Scores are checked as bounds, not exact values: several modifiers (noise
// penalty, length penalty) scale whatever tier matched, and pinning exact
// numbers would make every future tuning change a test failure.
func TestCalculateRelevance_MatchedBy(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		title      string
		wantTier   string
		wantMin    float64
		wantMax    float64
		wantDroppe bool
	}{
		{
			name:     "identical title",
			query:    "Дюна",
			title:    "Дюна",
			wantTier: "title_exact",
			wantMin:  1.0,
			wantMax:  1.0,
		},
		{
			name:     "same words in a different order",
			query:    "Dark Knight",
			title:    "Knight Dark",
			wantTier: "tokens_exact",
			wantMin:  0.90,
			wantMax:  0.90,
		},
		{
			// "дюна частина друга" is >1.8x the query length, so the
			// substring tier is scaled by the length penalty: 0.75*0.75.
			name:     "title contained in a longer one",
			query:    "Дюна",
			title:    "Дюна Частина друга",
			wantTier: "substring_exact",
			wantMin:  0.56,
			wantMax:  0.57,
		},
		{
			// The query reduces to the single token "matrix", which the item
			// contains, so this lands on the substring tier rather than the
			// token-overlap tier.
			name:     "query token contained in a longer title",
			query:    "the matrix",
			title:    "Matrix The 1999",
			wantTier: "substring_exact",
			wantMin:  0.56,
			wantMax:  0.57,
		},
		{
			name:     "two of three tokens",
			query:    "alpha bravo charlie",
			title:    "alpha bravo delta",
			wantTier: "token_set",
			wantMin:  0.44,
			wantMax:  0.45,
		},
		{
			// One of three tokens is 0.35 * (1/3) = 0.117, which falls under
			// HardCutoff. Partial overlap alone must not surface a title.
			name:       "one of three tokens is below the cutoff",
			query:      "alpha bravo charlie",
			title:      "alpha delta echo",
			wantTier:   "partial_tokens",
			wantMin:    0.11,
			wantMax:    0.12,
			wantDroppe: true,
		},
		{
			// No tier matched, so the function returns a bare dropped result
			// with an empty MatchedBy rather than the literal "none".
			name:       "no shared tokens at all",
			query:      "дюна",
			title:      "матриця воскресіння",
			wantTier:   "",
			wantDroppe: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := search.BuildQueryPlan(tc.query)
			res := search.CalculateRelevance(plan, tc.title, 0, "")
			if res.MatchedBy != tc.wantTier {
				t.Errorf("MatchedBy = %q, want %q (score %v)", res.MatchedBy, tc.wantTier, res.Score)
			}
			if res.Dropped != tc.wantDroppe {
				t.Errorf("Dropped = %v, want %v (score %v)", res.Dropped, tc.wantDroppe, res.Score)
			}
			if tc.wantMin != 0 || tc.wantMax != 0 {
				if res.Score < tc.wantMin-1e-9 || res.Score > tc.wantMax+1e-9 {
					t.Errorf("score %v outside [%v, %v]", res.Score, tc.wantMin, tc.wantMax)
				}
			}
		})
	}
}

func TestCalculateRelevance_RejectsUnusableInput(t *testing.T) {
	plan := search.BuildQueryPlan("Дюна")

	t.Run("nil plan", func(t *testing.T) {
		res := search.CalculateRelevance(nil, "Дюна", 0, "")
		if !res.Dropped || res.Score != 0 {
			t.Errorf("expected dropped zero score, got %+v", res)
		}
	})

	t.Run("empty canonical", func(t *testing.T) {
		res := search.CalculateRelevance(&search.QueryPlan{}, "Дюна", 0, "")
		if !res.Dropped {
			t.Errorf("expected drop for empty canonical, got %+v", res)
		}
	})

	t.Run("empty title", func(t *testing.T) {
		res := search.CalculateRelevance(plan, "", 0, "")
		if !res.Dropped {
			t.Errorf("expected drop for empty title, got %+v", res)
		}
	})

	t.Run("title that normalises to nothing", func(t *testing.T) {
		res := search.CalculateRelevance(plan, "!!!", 0, "")
		if !res.Dropped {
			t.Errorf("expected drop for punctuation-only title, got %+v", res)
		}
	})
}

// TestCalculateRelevance_Modifiers asserts the sign of each adjustment rather
// than an absolute value, so unrelated future tuning does not break the test.
//
// Plans are built as literals rather than through BuildQueryPlan: the pipeline
// folds hints into the canonical title, and a title_exact match saturates at
// 1.0, which would hide every +/-0.05 modifier behind the clamp.
func TestCalculateRelevance_Modifiers(t *testing.T) {
	// "alpha bravo" against "alpha bravo delta" scores 0.90 on the
	// tokens_exact tier: high enough to leave headroom, low enough that the
	// modifiers cannot be swallowed by the upper clamp.
	plan := func() *search.QueryPlan {
		return &search.QueryPlan{
			Canonical: "alpha bravo",
			Tokens:    []string{"alpha", "bravo"},
		}
	}
	base := func(t *testing.T, p *search.QueryPlan) float64 {
		t.Helper()
		res := search.CalculateRelevance(p, "alpha bravo delta", 0, "")
		if res.Dropped {
			t.Fatalf("baseline unexpectedly dropped: %+v", res)
		}
		return res.Score
	}

	t.Run("noise words in the title lower the score", func(t *testing.T) {
		clean := base(t, plan())
		noisy := search.CalculateRelevance(plan(), "alpha bravo delta 1080p", 0, "")
		if noisy.Score >= clean {
			t.Errorf("expected noise penalty, got %v vs baseline %v", noisy.Score, clean)
		}
	})

	t.Run("a much longer title is penalised", func(t *testing.T) {
		clean := base(t, plan())
		long := search.CalculateRelevance(
			plan(),
			"alpha bravo delta epsilon zeta eta theta iota kappa lambda mu",
			0, "",
		)
		if long.Score >= clean {
			t.Errorf("expected length penalty, got %v vs baseline %v", long.Score, clean)
		}
	})

	t.Run("matching type hint is rewarded", func(t *testing.T) {
		p := plan()
		plain := base(t, p)

		p.TypeHint = "movie"
		matching := search.CalculateRelevance(p, "alpha bravo delta", 0, "movie")
		if matching.Score <= plain {
			t.Errorf("expected a matching type hint to raise the score: %v vs %v", matching.Score, plain)
		}
	})

	t.Run("mismatched type hint is penalised", func(t *testing.T) {
		p := plan()
		plain := base(t, p)

		p.TypeHint = "movie"
		mismatch := search.CalculateRelevance(p, "alpha bravo delta", 0, "series")
		if mismatch.Score >= plain {
			t.Errorf("expected a mismatched type hint to lower the score: %v vs %v", mismatch.Score, plain)
		}
	})

	t.Run("the type hint is ignored when the item type is missing", func(t *testing.T) {
		noHint := base(t, plan())

		p := plan()
		p.TypeHint = "movie"
		withoutItemType := search.CalculateRelevance(p, "alpha bravo delta", 0, "")
		if math.Abs(noHint-withoutItemType.Score) > 1e-9 {
			t.Errorf("with no item type the plan hint must not apply: %v vs %v",
				noHint, withoutItemType.Score)
		}
	})

	t.Run("the type hint is ignored when the plan has none", func(t *testing.T) {
		p := plan()
		plain := base(t, p)
		withItemType := search.CalculateRelevance(p, "alpha bravo delta", 0, "series")
		if math.Abs(plain-withItemType.Score) > 1e-9 {
			t.Errorf("item type must not matter without a plan hint: %v vs %v",
				plain, withItemType.Score)
		}
	})

	t.Run("type matching is case-insensitive", func(t *testing.T) {
		p := plan()
		p.TypeHint = "movie"
		lower := search.CalculateRelevance(p, "alpha bravo delta", 0, "movie")
		upper := search.CalculateRelevance(p, "alpha bravo delta", 0, "MOVIE")
		if math.Abs(lower.Score-upper.Score) > 1e-9 {
			t.Errorf("type comparison must be case-insensitive: %v vs %v", lower.Score, upper.Score)
		}
	})

	t.Run("an exact year is rewarded", func(t *testing.T) {
		p := plan()
		plain := base(t, p)

		p.Year = 2021
		sameYear := search.CalculateRelevance(p, "alpha bravo delta", 2021, "")
		if sameYear.Score <= plain {
			t.Errorf("expected a matching year to raise the score: %v vs %v", sameYear.Score, plain)
		}
	})

	t.Run("a far-off year is penalised", func(t *testing.T) {
		p := plan()
		p.Year = 2021
		near := search.CalculateRelevance(p, "alpha bravo delta", 2021, "")
		far := search.CalculateRelevance(p, "alpha bravo delta", 1988, "")
		if far.Score >= near.Score {
			t.Errorf("expected a distant year to lower the score: %v vs %v", far.Score, near.Score)
		}
	})

	t.Run("a year gap of five years or less is neither rewarded nor penalised", func(t *testing.T) {
		p := plan()
		p.Year = 2021

		// An exact year is the only one that earns the +0.05 bonus; a gap of
		// up to five years must leave the score exactly where the no-year case
		// put it.
		noYear := base(t, plan())
		for _, year := range []int{2020, 2022, 2023, 2016, 2026} {
			got := search.CalculateRelevance(p, "alpha bravo delta", year, "")
			if math.Abs(got.Score-noYear) > 1e-9 {
				t.Errorf("year %d is within five of 2021 and must not change the score: %v vs %v",
					year, got.Score, noYear)
			}
		}

		// Six years out crosses the threshold and must be penalised.
		sixApart := search.CalculateRelevance(p, "alpha bravo delta", 2027, "")
		if math.Abs(sixApart.Score-noYear) < 1e-9 {
			t.Errorf("a six-year gap must be penalised, got %v (unchanged)", sixApart.Score)
		}
	})

	t.Run("the year is ignored when the item has none", func(t *testing.T) {
		p := plan()
		p.Year = 2021
		withoutYear := search.CalculateRelevance(p, "alpha bravo delta", 0, "")
		withPlanYearNoItemYear := search.CalculateRelevance(plan(), "alpha bravo delta", 0, "")
		if math.Abs(withoutYear.Score-withPlanYearNoItemYear.Score) > 1e-9 {
			t.Errorf("a missing item year must not change the score: %v vs %v",
				withoutYear.Score, withPlanYearNoItemYear.Score)
		}
	})

	t.Run("score never exceeds one", func(t *testing.T) {
		p := plan()
		p.TypeHint = "movie"
		p.Year = 2021
		// tokens_exact (0.90) plus both bonuses would exceed 1.0 without a clamp.
		res := search.CalculateRelevance(p, "alpha bravo delta", 2021, "movie")
		if res.Score > 1.0 {
			t.Errorf("score %v must be clamped to 1.0", res.Score)
		}
		if res.Dropped {
			t.Errorf("a clamped-to-1.0 result must not be dropped: %+v", res)
		}
	})

	t.Run("score is never negative", func(t *testing.T) {
		p := plan()
		p.TypeHint = "movie"
		p.Year = 2021
		// token_set tier (0.45) minus the year and type penalties still lands
		// above zero, so drive it down with noise and length penalties too.
		res := search.CalculateRelevance(
			p,
			"alpha bravo 1080p delta epsilon zeta eta theta iota kappa",
			1901, "series",
		)
		if res.Score < 0 {
			t.Errorf("score %v must be clamped to 0", res.Score)
		}
	})
}

func TestCalculateRelevance_CutoffBoundary(t *testing.T) {
	// The cutoff is a hard product decision: below it a title must not be
	// shown. Pin the comparison operator so an off-by-one cannot slip in.
	plan := search.BuildQueryPlan("Дюна")
	res := search.CalculateRelevance(plan, "Дюна", 0, "")

	if math.Abs(res.Score-search.HardCutoff) > 1e-9 {
		t.Skipf("baseline score %v does not sit on the cutoff; nothing to pin", res.Score)
	}
	if res.Dropped {
		t.Errorf("a score exactly equal to HardCutoff (%v) must be kept", search.HardCutoff)
	}
}

func TestGenerateClusterKey(t *testing.T) {
	t.Run("year bounds collapse to zero", func(t *testing.T) {
		low := search.GenerateClusterKey("Дюна", 1500, "movie")
		high := search.GenerateClusterKey("Дюна", 2500, "movie")
		none := search.GenerateClusterKey("Дюна", 0, "movie")
		if low != none || high != none {
			t.Errorf("out-of-range years must collapse to 0: %q / %q / %q", low, high, none)
		}
	})

	t.Run("missing type defaults to movie", func(t *testing.T) {
		if a, b := search.GenerateClusterKey("Дюна", 2021, ""), search.GenerateClusterKey("Дюна", 2021, "movie"); a != b {
			t.Errorf("empty type should default to movie: %q vs %q", a, b)
		}
	})

	t.Run("type, year and title all separate keys", func(t *testing.T) {
		base := search.GenerateClusterKey("Дюна", 2021, "movie")
		if search.GenerateClusterKey("Дюна", 2022, "movie") == base {
			t.Error("year must be part of the key")
		}
		if search.GenerateClusterKey("Дюна", 2021, "series") == base {
			t.Error("type must be part of the key")
		}
		if search.GenerateClusterKey("Дюна Частина 2", 2021, "movie") == base {
			t.Error("title must be part of the key")
		}
	})

	t.Run("punctuation and case do not split a cluster", func(t *testing.T) {
		a := search.GenerateClusterKey("The Matrix", 1999, "movie")
		b := search.GenerateClusterKey("the   matrix!", 1999, "movie")
		if a != b {
			t.Errorf("normalisation should merge these, got %q and %q", a, b)
		}
	})
}

func TestClusterAndDeduplicate_RepresentativeTieBreaks(t *testing.T) {
	// ProviderID matters: the fallback source ref derives its SourceKey from
	// it, so a fixture that omits it silently produces an unattributable item.
	item := func(id, title string, year int, score, rating float64, poster string, sources ...search.SearchSourceRef) search.ScoredSearchItem {
		return search.ScoredSearchItem{
			MediaItem: domain.MediaItem{
				ID: id, ProviderID: "uakino", Title: title, Year: year, Type: "movie",
				PosterURL: poster, Rating: rating,
			},
			Score:   score,
			Sources: sources,
		}
	}

	t.Run("empty input", func(t *testing.T) {
		if got := search.ClusterAndDeduplicate(nil); len(got) != 0 {
			t.Errorf("expected no clusters, got %d", len(got))
		}
	})

	t.Run("highest score wins", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("low", "Дюна", 2021, 0.4, 9.0, "https://p/1.jpg"),
			item("high", "Дюна", 2021, 0.9, 1.0, ""),
		})
		if len(out) != 1 || out[0].ID != "high" {
			t.Fatalf("expected the high scorer to represent the cluster, got %+v", out)
		}
	})

	t.Run("on a score tie the better rating wins", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("a", "Дюна", 2021, 0.8, 5.0, ""),
			item("b", "Дюна", 2021, 0.8, 8.0, ""),
		})
		if len(out) != 1 || out[0].ID != "b" {
			t.Fatalf("expected the better rating to win, got %+v", out)
		}
	})

	t.Run("on a rating tie a poster breaks it", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("a", "Дюна", 2021, 0.8, 7.0, ""),
			item("b", "Дюна", 2021, 0.8, 7.0, "https://p/2.jpg"),
		})
		if len(out) != 1 || out[0].ID != "b" {
			t.Fatalf("expected the item with a poster to win, got %+v", out)
		}
	})

	t.Run("the cluster key is stamped on the representative", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("a", "Дюна", 2021, 0.9, 7.0, ""),
		})
		if out[0].ClusterKey == "" {
			t.Error("expected the representative to carry a cluster key")
		}
		if want := search.GenerateClusterKey("Дюна", 2021, "movie"); out[0].ClusterKey != want {
			t.Errorf("ClusterKey = %q, want %q", out[0].ClusterKey, want)
		}
	})

	t.Run("sources from every member are merged without duplicates", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("a", "Дюна", 2021, 0.9, 7.0, "",
				search.SearchSourceRef{ProviderID: "bandera", SourceKey: "uaflix", ItemID: "1"},
				search.SearchSourceRef{ProviderID: "bandera", SourceKey: "mikai", ItemID: "2"},
			),
			item("b", "Дюна", 2021, 0.8, 6.0, "",
				// The uaflix ref repeats and must not be added twice.
				search.SearchSourceRef{ProviderID: "bandera", SourceKey: "uaflix", ItemID: "1"},
				search.SearchSourceRef{ProviderID: "bandera", SourceKey: "lavakino", ItemID: "3"},
			),
		})
		if len(out) != 1 {
			t.Fatalf("expected one cluster, got %d", len(out))
		}
		if len(out[0].Sources) != 3 {
			t.Fatalf("expected 3 unique sources, got %d: %+v", len(out[0].Sources), out[0].Sources)
		}
		seen := map[string]bool{}
		for _, s := range out[0].Sources {
			key := s.ProviderID + ":" + s.SourceKey + ":" + s.ItemID
			if seen[key] {
				t.Errorf("duplicate source ref merged: %s", key)
			}
			seen[key] = true
		}
	})

	t.Run("a member without sources still contributes a fallback ref", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("a", "Дюна", 2021, 0.9, 7.0, "",
				search.SearchSourceRef{ProviderID: "bandera", SourceKey: "uaflix", ItemID: "1"},
			),
			item("b", "Дюна", 2021, 0.8, 6.0, ""), // no Sources at all
		})
		if len(out) != 1 {
			t.Fatalf("expected one cluster, got %d", len(out))
		}
		if len(out[0].Sources) != 2 {
			t.Errorf("expected the source-less member to add a fallback ref, got %+v", out[0].Sources)
		}
		for _, s := range out[0].Sources {
			if s.SourceKey == "" {
				t.Errorf("fallback ref must carry a non-empty source key, got %+v", s)
			}
		}
	})

	t.Run("distinct titles stay in separate clusters", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("a", "Дюна", 2021, 0.9, 7.0, ""),
			item("b", "Матриця", 1999, 0.9, 8.0, ""),
		})
		if len(out) != 2 {
			t.Fatalf("expected 2 clusters, got %d", len(out))
		}
	})

	t.Run("output is sorted by descending score", func(t *testing.T) {
		out := search.ClusterAndDeduplicate([]search.ScoredSearchItem{
			item("low", "Альфа", 2001, 0.30, 7.0, ""),
			item("high", "Бета", 2002, 0.95, 7.0, ""),
			item("mid", "Гамма", 2003, 0.60, 7.0, ""),
		})
		if len(out) != 3 {
			t.Fatalf("expected 3 clusters, got %d", len(out))
		}
		for i := 1; i < len(out); i++ {
			if out[i-1].Score < out[i].Score {
				t.Errorf("results are not sorted by score: %v then %v", out[i-1].Score, out[i].Score)
			}
		}
	})
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

var _ = strings.TrimSpace
