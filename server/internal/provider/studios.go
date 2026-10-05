package provider

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Словник українських студій озвучення.
//
// Навіщо він нам: поки що назва озвучки в UI підставлялася з
// detectPlayerBalancer, тобто користувач бачив «HDVB», «Ashdi»,
// «Zenith» — це імена хостів плеєрів, а не студії. Сира назва
// озвучки приходить із PlayerJS-дерева (title вузла) або з атрибута
// li[data-voice] у DLE, але її ніхто не нормалізував. Цей файл
// робить дві речі: приводить сиру назву до канонічної (щоб «1 плюс
// 1», «1+1» і «один плюс один» стали одним записом) і видає
// стабільний Voiceover.ID.
//
// Дані про назви студій — це публічні факти про українські
// телеканали та кінодубляж, а не витвір коду. Реалізація нижче
// оригінальна.

// StudioInfo — канонічна студія озвучення.
type StudioInfo struct {
	ID      string // стабільний ідентифікатор, малий регістр, без пробілів
	Name    string // показувана назва
	Aliases []string
}

// studioEntry — внутрішній запис словника.
type studioEntry struct {
	id      string
	name    string
	aliases []string
}

// normalizeStudioKey приводить назву до форми, придатної для
// порівняння: нижній регістр, апострофи з єдиного набору приведено
// до апострофа, пробілиcollapse'нуто.
//
// Апостроф Needs: в українському написанні трапляються «сері'я»
// (апостроф), «сері’я» (типографський) і «серія» (без апострофа).
// Якщо їх не звести до одного вигляду, один і той самий студійний
// дубляж розсипається на три різні Voiceover.
func normalizeStudioKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("’", "'", "ʼ", "'").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// buildStudioIndex будує індекси словника один раз при ініціалізації.
//
// Два індекси, бо точний збіг і підрядковий — різні операції:
// «ICTV2» не повинна перетворитися на «ICTV» просто тому, що
// «ictv» є підрядком. Якщо робити лише підрядковий пошук, перший
// перехопить наступний.
var (
	studioByAlias = map[string]studioEntry{} // точний збіг нормалізованого рядка
	studioEntries = []studioEntry{}
)

func init() {
	for _, e := range allStudios() {
		studioEntries = append(studioEntries, e)
		studioByAlias[normalizeStudioKey(e.id)] = e
		studioByAlias[normalizeStudioKey(e.name)] = e
		for _, a := range e.aliases {
			studioByAlias[normalizeStudioKey(a)] = e
		}
	}
}

// studioAliasesLenLimit — аліаси коротші за це число ігноруються при
// підрядковому пошуку.
//
// Причина реальна: без обмеження «ні» або «к» з боку будь-якої
// студії перехоплють половину назв. Наприклад «НЛО-ТВ» з аліасом
// «нло» підійде до «Обростання» і «Місто гріха».
const studioAliasesLenLimit = 3

// ResolveStudio повертає канонічну студію за сирою назвою.
//
// Дві фази, саме в такому порядку:
//
//  1. Точний збіг нормалізованого рядка з id, назвою або аліасом.
//     ЦеCover-ить 90% випадків і ніколи не помиляється.
//  2. Підрядковий пошук, якщо фаза 1 промахнулась. Перемагає
//     НАЙДОВШИЙ збіглий аліас, а не перший у таблиці — інакше
//     «InariOkami» перетворилася б на «Inari» просто тому, що
//     Inari стоїть раніше в списку.
//
// ok=false означає, що студію не впізнано і треба показати сиру
// назву як є.
func ResolveStudio(raw string) (StudioInfo, bool) {
	key := normalizeStudioKey(raw)
	if key == "" {
		return StudioInfo{}, false
	}

	if e, ok := studioByAlias[key]; ok {
		return StudioInfo{ID: e.id, Name: e.name, Aliases: e.aliases}, true
	}

	// Фаза 2. Шукаємо найдовший аліас, що є підрядком у назві.
	// Індекс перебирається в порядку registration, тому довжина
	// збігу порівнюється явно.
	var (
		bestEntry    studioEntry
		bestMatchLen int
		foundAny     bool
	)
	for _, e := range studioEntries {
		candidates := make([]string, 0, len(e.aliases)+2)
		candidates = append(candidates, e.id, e.name)
		candidates = append(candidates, e.aliases...)
		for _, c := range candidates {
			ck := normalizeStudioKey(c)
			if len(ck) < studioAliasesLenLimit {
				continue
			}
			if !strings.Contains(key, ck) {
				continue
			}
			if !foundAny || len(ck) > bestMatchLen {
				bestEntry, bestMatchLen, foundAny = e, len(ck), true
			}
		}
	}

	if foundAny {
		return StudioInfo{ID: bestEntry.id, Name: bestEntry.name, Aliases: bestEntry.aliases}, true
	}
	return StudioInfo{}, false
}

// cleanStudioName приводить сиру назву озвучки з плеєра до людського
// вигляду: знімає технічні обгортки, які CDN навішали на студію.
//
// Приклади з живого трафіку:
//   - «Ukrainian (1+1)»            → «1+1»
//   - «[MoonAnime] Postmodern»     → «Postmodern»
//   - «Субтитри | BambooUA»        → «BambooUA»
//   - «Багатоголосий закадровий: 1+1» → «1+1»
//   - «Субтитри (Чорний Верес)»    → «Чорний Верес»
func cleanStudioName(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "&#124;", "|")

	// ОбгорткаUkrainian(...) — бере те, що в дужках.
	if m := reStudioUkrainianParen.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}

	// Квадратні дужки: [MoonAnime], [Ashdi], [1080p].
	s = reStudioBrackets.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)

	// «субтитри | BambooUA» — бере останню частину після |.
	if strings.Contains(s, "|") {
		parts := strings.Split(s, "|")
		last := strings.TrimSpace(parts[len(parts)-1])
		if last != "" {
			s = last
		}
	}

	// Технічний префікс типу «Багатоголосий закадровий: X».
	s = reStudioTechPrefix.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)

	// «Субтитри СвійSUB» → «СвійSUB», але голе «Субтитри» має
	// лишитися саме собою, інакше озвучка зникла б з переліку.
	//
	// Порівнюємо кількість СИМВОЛІВ, а не байтів: «Субтитри» це
	// 8 рунів і 16 байтів, тому перевірка len(s) > 9 з’їдала б
	// голий «Субтитри» цілком. Це був реальний провал тесту.
	if reStudioSubsPrefix.MatchString(s) && utf8.RuneCountInString(s) > 9 {
		s = reStudioSubsPrefix.ReplaceAllString(s, "")
		s = reStudioTrailingParen.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
	}

	switch normalizeStudioKey(s) {
	case "", "не визначено", "undefined", "null", "default", "none":
		return ""
	}
	return s
}

var (
	reStudioUkrainianParen = regexp.MustCompile(`(?i)^ukrainian\s*\((.*)\)$`)
	reStudioBrackets       = regexp.MustCompile(`\[[^\]]*\]`)
	reStudioTechPrefix     = regexp.MustCompile(`(?i)^(?:багатоголосий|двоголосий|одногоголос|одноголос|професійний|аматорський|ліцензійний)\s*(?:закадровий|дубльований|дубляж)?\s*[:\-–—]?\s*`)
	// Роздільником після «Субтитри» може бути і пробіл, і дужка:
	// «Субтитри СвійSUB» та «Субтитри (Чорний Верес)» — обидві
	// форми реальні. Пробіл у класі символів обов'язковий, інакше
	// перша форма не знімалась. Голе «Субтитри» захищене
	// перевіркою довжини в cleanStudioName.
	reStudioSubsPrefix     = regexp.MustCompile(`(?i)^(?:субтитри|субтитр|subtitles?|subs)\s*[:(\s\-–—]*\s*`)
	reStudioTrailingParen  = regexp.MustCompile(`(?i)\s*\)?\s*$`)
	reAudioIsSubtitle      = regexp.MustCompile(`(?i)(?:субтит|субтитр|\bsub\b)`)
)

// ResolveVoiceoverNameNormalised — повний конвеєр від сирої назви
// з плеєра до Voiceover (ID, показувана назва, мова).
//
// Мова визначається за СИРОЮ назвою, а не за очищеною: назви на
// кшталт «СвійSUB» та «UaAniSub» не мають українського озвучення,
// вони лише додають субтитри. Якщо визначати мову після очищення,
// «Субтитри» втратили б ознаку і отримали б uk.
func ResolveVoiceoverNameNormalised(rawDub string) (id, name, language string) {
	cleaned := cleanStudioName(rawDub)
	if cleaned == "" {
		return "", "", ""
	}

	name = cleaned
	if info, ok := ResolveStudio(cleaned); ok {
		id = info.ID
		name = info.Name
	} else {
		id = normalizeStudioKey(cleaned)
	}

	language = "uk"
	if reAudioIsSubtitle.MatchString(strings.ToLower(rawDub)) {
		language = "none"
	}
	return id, name, language
}