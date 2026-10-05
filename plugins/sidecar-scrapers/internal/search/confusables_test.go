package search_test

// Регресія: ґ і г — різні літери, але один звук.
//
// Сайти пишуть назви по-різному («Супергьорл» на одному, «Суперґьорл» на
// іншому), тому до фолдингу два написання давали неперетинні набори
// результатів: елемент з ґ не злипався з елементом з г, а користувач,
// який напише одну з двох літер, не побачить дублікат.
//
// Тест навмисно б'є по ClusterAndDeduplicate, а не по FoldConfusables:
// чиста функція здатна повернути правильний результат, поки місце
// виклику не використовує її (так уже впали дві мутації).

import (
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/search"
)

func scored(title string, year int, itemType, providerID string) search.ScoredSearchItem {
	return search.ScoredSearchItem{
		MediaItem: domain.MediaItem{
			ID:         providerID + "-1",
			Title:      title,
			Year:       year,
			Type:       itemType,
			ProviderID: providerID,
		},
	}
}

func TestFoldConfusablesFoldsGeToG(t *testing.T) {
	got := search.FoldConfusables("суперґьорл")
	want := search.FoldConfusables("супергьорл")

	if got != want {
		t.Errorf("FoldConfusables(%q) = %q, want %q — г/ґ мають зливатися", "суперґьорл", got, want)
	}
}

func TestClusterMergesGeAndGSpellings(t *testing.T) {
	candidates := []search.ScoredSearchItem{
		scored("Супергьорл", 2010, "movie", "bandera"),
		scored("Суперґьорл", 2010, "movie", "uaflix"),
	}

	got := search.ClusterAndDeduplicate(candidates)

	if len(got) != 1 {
		t.Fatalf("ClusterAndDeduplicate вернув %d кластерів, want 1: %+v", len(got), got)
	}
	if len(got[0].Sources) != 2 {
		t.Errorf("кластер має %d джерел, want 2 (г + ґ)", len(got[0].Sources))
		for _, s := range got[0].Sources {
			t.Logf("  джерело: %+v", s)
		}
	}
}

func TestGenerateClusterKeyStableAcrossGeSpellings(t *testing.T) {
	// Стабільний content ID будується з цього ключа. Якщо два написання
	// дають різні ключі, один фільм отримує два рядки в watch_history.
	withG := search.GenerateClusterKey("Супергьорл", 2010, "movie")
	withGe := search.GenerateClusterKey("Суперґьорл", 2010, "movie")

	if withG != withGe {
		t.Errorf("GenerateClusterKey розходиться: %q vs %q", withG, withGe)
	}
}

// Не повинні зливатися різні літери, схожі за написанням.
func TestFoldConfusablesKeepsDistinctLetters(t *testing.T) {
	cases := []struct{ a, b string }{
		{"гора", "холм"},
		{"джак", "джек"},
	}
	for _, tc := range cases {
		if search.FoldConfusables(tc.a) == search.FoldConfusables(tc.b) {
			t.Errorf("різні слова злилися: %q і %q -> %q", tc.a, tc.b, search.FoldConfusables(tc.a))
		}
	}
}