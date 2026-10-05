package provider

import (
	"regexp"
	"sort"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// Ваги озвучки для впорядкування джерел.
//
// Навіщо це потрібно. Без ваг `extractStreamsFromPlaylistTree`
// повертає потоки в тому порядку, в якому вони лежать у JSON
// плеєра. Для користувача це означає, що першим у списку може
// опинитись оригінал без перекладу, а український дубляж — внизу,
// за межами першого екрана. Сортування за вагою робить порядок
// передбачуваним: спершу повноцінний український дубляж, потім
// багатоголос, і лише в кінці оригінал із субтитрами.
//
// Ваги запозичені з uafilms/core (src/utils/sort.ts, getDubWeight) —
// це не наша вигадка, а напрацьована ними евристика.

// DubPriority — вага джерела за типом озвучення. Більше = краще.
type DubPriority int

const (
	DubPrioritySubtitles DubPriority = 10 // лише субтитри, оригінал
	DubPriorityOriginal  DubPriority = 20 // оригінальна доріжка без перекладу
	DubPriorityMono      DubPriority = 40 // одноголосий переклад
	DubPriorityTwoVoice  DubPriority = 60 // двоголосий переклад (старі озвучки)
	DubPriorityMulti     DubPriority = 80 // багатоголосий переклад
	DubPriorityDub       DubPriority = 100 // повноцінний студійний дубляж
)

// dubPriorityWords — таблиця ключових слів. Порядок має значення:
// перший збіг визначає вагу, тому специфічніші формули стоять вище
// за загальні («багатоголос» раніше за «дубляж»).
//
// Збіг виконується на нормалізованому рядку, тому «ДУБЛЯЖ»,
// «Дубляж» і «Дубляж (2 голоси)» дають однакову відповідь.
var dubPriorityWords = []struct {
	word string
	prio DubPriority
}{
	{"субтит", DubPrioritySubtitles},
	{"субтитр", DubPrioritySubtitles},
	{" оригінал", DubPriorityOriginal},
	{"оригінал", DubPriorityOriginal},
	{"оригінальна", DubPriorityOriginal},
	{"багатоголос", DubPriorityMulti},
	{"багато-голос", DubPriorityMulti},
	{"мультивокал", DubPriorityMulti},
	{"многоголос", DubPriorityMulti},
	{"2голос", DubPriorityTwoVoice},
	{"2 голос", DubPriorityTwoVoice},
	{"двоголос", DubPriorityTwoVoice},
	{"одноголос", DubPriorityMono},
	{"1голос", DubPriorityMono},
	{"1 голос", DubPriorityMono},
	{"моноголос", DubPriorityMono},
	{"моно", DubPriorityMono},
	{"дубляж", DubPriorityDub},
	{"дубльован", DubPriorityDub},
	{"озвучка", DubPriorityDub},
	{"озвучено", DubPriorityDub},
	{"переклад", DubPriorityDub},
}

// reNoiseInDubLabel прибирає службові частини назви, щоб «1+1
// (Дубляж)» і «1+1» не відрізнялися за вагою.
var reNoiseInDubLabel = regexp.MustCompile(`(?i)\s*[(\[-]\s*(?:uk|ua|украин|россий|рус)\w*\s*[)\]]`)

// DubWeight визначає вагу озвучки за її назвою.
//
// Назва студії на кшталт «1+1» або «Postmodern» не містить жодного
// ключового слова, тому для неї повертається DubPriorityDub: це
// повноцінна студійна озвучка, а не оригінал. Такий підхід
// відповідає реальності українських кіносайтів — там наявність
// назви студії вже означає дубляж.
func DubWeight(name string) DubPriority {
	normalized := normalizeDubLabel(name)
	if normalized == "" {
		return DubPriorityOriginal
	}

	for _, entry := range dubPriorityWords {
		if strings.Contains(normalized, entry.word) {
			return entry.prio
		}
	}

	// Назва студії без ключових слів: вважаємо повноцінним дубляжем.
	return DubPriorityDub
}

// normalizeDubLabel приводить назву до нижнього регістру, прибирає
// службові дужки та знищує пробіли, щоб «2  голоси» і «2голоси»
// збігалися.
func normalizeDubLabel(name string) string {
	s := strings.ToLower(name)
	s = reNoiseInDubLabel.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

// SortStreamsByDubWeight впорядковує джерела за вагою озвучки.
//
// Сортування стабільне (SortStable), тому джерела з однаковою
// вагою лишаються в тому порядку, в якому їх дав CDN, — тобто
// порядок якостей 1080p/720p/480p усередині однієї озвучки не
// псується.
func SortStreamsByDubWeight(streams []domain.StreamSource) {
	sort.SliceStable(streams, func(i, j int) bool {
		return DubWeight(streams[i].Voiceover) > DubWeight(streams[j].Voiceover)
	})
}

// SortVoiceoversByWeight впорядковує список озвучальних студій так
// само, як SortStreamsByDubWeight впорядковує джерела: найкраща
// озвучка — першою. Клієнт показує цей список без сортування.
func SortVoiceoversByWeight(voiceovers []domain.Voiceover) {
	sort.SliceStable(voiceovers, func(i, j int) bool {
		return DubWeight(voiceovers[i].Name) > DubWeight(voiceovers[j].Name)
	})
}

// ResolveVoiceoverName повертає назву озвучки, яку показати
// користувачеві.
//
// Якщо назва порожня, але відомий плеєр, показуємо назву плеєра —
// це гірше за студію, але краще за порожній рядок. Джерело назви
// позначається в Player, щоб клієнт міг відрізнити справжню
// озвучку від імені хоста.
func ResolveVoiceoverName(dub, playerLabel string) string {
	if dub != "" {
		return dub
	}
	return playerLabel
}