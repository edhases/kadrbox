package search_test

import (
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/search"
)

// Сценарії нижче зняті з живого сервера 2026-10-05 (запит
// «Величне століття»), тому тести не описують вигадку, а
// відтворюють те, що реально приходить.

// media збирає пошуковий елемент у мінімальному вигляді, потрібному
// для кластеризації.
func media(title string, year int, typ, provider string) domain.MediaItem {
	return domain.MediaItem{
		ID:         provider + ":" + title,
		ProviderID: provider,
		Title:      title,
		Year:       year,
		Type:       typ,
	}
}

// --- Головний випадок: один серіал, два провайдери, різний рік ------

// Регресія з живих даних: lavakino знав рік, uakino — ні, тому
// «Величне століття. Роксолана» показувалося двічі.
func TestClusterAndDeduplicate_MergesYearlessIntoKnownYear(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Величне століття. Роксолана", 2011, "series", "lavakino")},
		{MediaItem: media("Величне століття. Роксолана", 0, "series", "uakino")},
	}

	out := search.ClusterAndDeduplicate(items)
	if len(out) != 1 {
		t.Fatalf("want 1 card, got %d: %+v", len(out), out)
	}
	if out[0].Year != 2011 {
		t.Fatalf("representative must keep the known year, got %d", out[0].Year)
	}
}

// --- Захист від надмірного злиття (побоювання власника) -------------

// Ремейк: два різні відомі роки НІКОЛИ не зливаються, навіть за
// однаковою назвою.
func TestClusterAndDeduplicate_KeepsRemakesApart(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Форрест Гамп", 1994, "movie", "a")},
		{MediaItem: media("Форрест Гамп", 2022, "movie", "b")},
	}

	out := search.ClusterAndDeduplicate(items)
	if len(out) != 2 {
		t.Fatalf("remakes must stay separate, got %d cards: %+v", len(out), out)
	}
}

// «Мухтесем Юзиль» 2011 і «Нова володарка» 2015 — різні назви,
// тому не зливаються навіть без року в одному з них.
func TestClusterAndDeduplicate_KeepsDistinctShowsApart(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Величне століття. Роксолана", 2011, "series", "lavakino")},
		{MediaItem: media("Величне століття. Нова володарка", 2015, "series", "lavakino")},
		{MediaItem: media("Величне століття. Нова володарка", 0, "series", "uakino")},
	}

	out := search.ClusterAndDeduplicate(items)
	if len(out) != 2 {
		t.Fatalf("want 2 distinct shows, got %d: %+v", len(out), out)
	}
}

// Тип лишається частиною ідентичності: серіал і фільм з однією
// назвою — різні сутності.
func TestClusterAndDeduplicate_KeepsTypesApart(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Довга дорога", 2021, "series", "a")},
		{MediaItem: media("Довга дорога", 0, "movie", "b")},
	}

	out := search.ClusterAndDeduplicate(items)
	if len(out) != 2 {
		t.Fatalf("type must separate clusters, got %d: %+v", len(out), out)
	}
}

// --- Чого ми свідомо НЕ робимо --------------------------------------

// Різні назви не зливаються, навіть якщо очевидно йдеться про той
// самий серіал. «…Кьосем» і «…Кьосем / Muhtesem Yüzyil: Kösem» —
// два різні рядки, тож два кластери.
//
// Це відома недообробленість, і краще зафіксувати її тестом, ніж
// мовчки спробувати «домислити» назву. Спроба злити їх за
// спільним префіксом зрізала б реальні різні фільми, у яких
// спільний початок.
func TestClusterAndDeduplicate_DoesNotGuessAcrossDifferentTitles(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Величне століття. Нова володарка", 2015, "series", "lavakino")},
		{MediaItem: media("Величне століття. Нова володарка / Величне століття. Кьосем / Muhtesem Yüzyil: Kösem", 0, "series", "bandera")},
	}

	out := search.ClusterAndDeduplicate(items)
	if len(out) != 2 {
		t.Fatalf("different titles must not be merged on a guess, got %d: %+v", len(out), out)
	}
}

// --- Крайові випадки ------------------------------------------------

func TestClusterAndDeduplicate_YearlessOnly(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Фільм", 0, "movie", "a")},
		{MediaItem: media("Фільм", 0, "movie", "b")},
	}

	out := search.ClusterAndDeduplicate(items)
	if len(out) != 1 {
		t.Fatalf("two yearless items with the same title must merge, got %d", len(out))
	}
}

// Рік поза межами 1900..2100 вважається невідомим, а не
// «майбутнім фільмом». Інакше 0 і 1500 дали б різні кластери.
func TestClusterAndDeduplicate_OutOfRangeYearIsUnknown(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Тест", 2019, "movie", "a")},
		{MediaItem: media("Тест", 1500, "movie", "b")},
	}

	out := search.ClusterAndDeduplicate(items)
	if len(out) != 1 {
		t.Fatalf("out-of-range year must be treated as unknown, got %d: %+v", len(out), out)
	}
}

// Кластер мусить нести ключ у старому форматі, бо на нього
// спираються зовнішні тести й клієнт.
func TestClusterAndDeduplicate_ClusterKeyFormatUnchanged(t *testing.T) {
	items := []search.ScoredSearchItem{
		{MediaItem: media("Довга дорога", 2021, "movie", "a")},
		{MediaItem: media("Довга дорога", 0, "movie", "b")},
	}

	out := search.ClusterAndDeduplicate(items)
	want := search.GenerateClusterKey("Довга дорога", 2021, "movie")
	if out[0].ClusterKey != want {
		t.Fatalf("ClusterKey = %q, want %q", out[0].ClusterKey, want)
	}
}