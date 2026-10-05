package provider

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/edhases/oxide-server/internal/domain"
)

// Узагальнений обхід дерева PlayerJS-плейлистів.
//
// Навіщо окремий файл, а не ще один case у resolve.go: різні CDN
// вкладають сезони, озвучки та епізоди на різній глибині й у різному
// порядку. Реальні випадки, які ми зустріли на живому трафіку:
//
//	HDVB     folder:[ "1 сезон"  -> folder:[ "1+1"  -> folder:[ 155 епізодів ] ] ]
//	Ashdi    folder:[ "1+1"       -> folder:[ " Серіал 1" -> folder:[ ... ] ] ]
//	Bamboo   folder:[ "Сезон 1"   -> folder:[ "Postmodern" -> folder:[ ... ] ] ]
//	Tortuga  folder:[ "Сезон 1"   -> folder:[ 24 епізоди ] ]
//
// extractStreamsFromPlaylistTree (resolve.go) розбирає рівно три
// відомі вкладеності напрумку і ламається на всьому іншому: коли
// корінь це сезон, а озвучка лежить у його дітей, назва озвучки
// «1+1» читається як номер серії, і запит episode=3 зрізає ціле
// піддерево. Тут роль вузла визначається з його Title, а не з
// позиції у дереві, тому Ashdi (озвучка зверху) і HDVB (озвучка в
// середині) обробляються одним кодом.

// playlistNodeRole — що вузол означає у контексті дерева.
type playlistNodeRole int

const (
	roleUnknown playlistNodeRole = iota // нічого не визначено — контекст успадковується
	roleSeason                          // номер сезону
	roleEpisode                         // номер серії
	roleQuality                         // роздільна здатність
)

// PlaylistContext — контекст, накопичений під час обходу: що вже
// відомо про поточний лист. Нащадки успадковують батьківський
// контекст і перезаписують лише те, що вміють визначити самі.
type PlaylistContext struct {
	Dub     string // студія озвучення ("1+1", "Postmodern")
	Season  int    // 0 — невідомо
	Episode int    // 0 — невідомо
	Quality string // порожній рядок — невідомо
}

// PlaylistLeaf — вузол із прямим файлом, тобто те, що можна
// віддати клієнту.
type PlaylistLeaf struct {
	Ctx      PlaylistContext
	File     string // сирий рядок із PlayerJS
	Subtitle string // сирий рядок із PlayerJS
}

var (
	// reSeasonTitle — «Сезон 1», «1 сезон», «Season 2», «Серіал 3».
	// Число допускається як до, так і після ключового слова.
	//
	// Українські закінчення в дусі «сезон[а-я’їєґ]*» потрібні для
	// форм «сезону» тощо, які трапляються в заголовках розділів
	// DLE-сайтів. Порядкове число з «-й/-ий/-ій» теж реальне:
	// «1-й сезон».
	//
	// УВАЖА: тут НЕ можна писати s[ei]rie?s? з флагом (?i).
	// Такий підхап зробив би «Серія 5» сезоном, бо (?i) робить
	// латинську s такою ж, як кириличну с.
	//
	// Другий важливий момент: в слові «Серіал» кирилична і (U+0456),
	// а НЕ и (U+0438). Написання «сериал» ніколи не збігається з
	// «Серіал» — це вже коштувало одного невдалого тесту.
	reSeasonTitle = regexp.MustCompile("(?i)(?:сезон[а-я'’їєґ]*|серіал\\w*|season|serie)\\s*(\\d{1,3})|(\\d{1,3})\\s*(?:-?(?:й|ий|ій)\\s*)?(?:сезон|season|серіал)")

	// reEpisodeTitle — «Серія 5», «5 серія», «Епізод 12», «Episode 3».
	// В апострофах є українські варіанти написання з апострофом
	// («сері'я») — на живому трафіку вони реальні.
	reEpisodeTitle = regexp.MustCompile("(?i)(?:сері[я'’іїєґ]+|епізод\\w*|episode|ep)\\s*(\\d{1,4})|(\\d{1,4})\\s*(?:сері[я'’їєґ]+|епізод)")

	// reBareEpisode — гола «e5» / «e 5». Умови навмисно асиметричні:
	// літера має бути окремим словом (щоб не з'їсти хвости на кшталт
	// «s01e02»), а перед нею не має бути цифри чи літери.
	reBareEpisode = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])e\.?\s*(\d{1,4})(?:$|[^0-9a-z])`)

	// reQualityTitle — «1080p», «FullHD», «4K».
	reQualityTitle = regexp.MustCompile(`(?i)\b(4k|2160p?|fullhd|fhd|1080p?|720p?|480p?|360p?)\b`)

	// Нумерація з URL для вузлів з порожнім Title.
	reURLsXe   = regexp.MustCompile(`(?i)/s(\d{1,3})e(\d{1,4})(?:[./_?\-]|$)`)
	reURLPath = regexp.MustCompile(`(?i)/(?:s|season[-_]?)(\d{1,3})(?:[/_-](?:e?ep?)?(\d{1,4}))?[./_?\-]`)
)

// dubTitleNoise — суфікси, які CDN дописують до назви студії. Це
// не частина імені, і в переліку озвучки вони виглядають як сміття.
var dubTitleNoise = regexp.MustCompile(`(?i)\s*\(\s*(?:для\s+)?(?:підписників|спонсорів|subscribers?|sponsors?)\s*\)\s*`)

// trailerGuard — URL трейлерів.
//
// Часто містить слово «trailer» лише у внутрішньому шляху m3u8
// (…/hls/trailers/muhtesem_yuzyil_2011/hls/index.m3u8), тоді як у
// самому iframe його немає, а натомість стоїть ?tr=1. Без цього
// фільтра трейлер потрапляє в список стримів як «грабельний»
// варіант, і користувач отримує не ту серію.
var trailerGuard = regexp.MustCompile(`(?i)(?:\btrailers?\b|/trailers?/|[?&]tr=1\b)`)

// IsTrailerURL повідомляє, чи URL веде на трейлер, а не на серію.
func IsTrailerURL(raw string) bool {
	return trailerGuard.MatchString(raw)
}

// classifyPlaylistNode визначає роль вузла за його Title.
//
// ok=false означає «назва нічого не повідомляє»: тоді контекст
// просто успадковується, а вузол із дітьми трактується як
// контейнер (студія озвучення або анонімне групування).
func classifyPlaylistNode(title string) (role playlistNodeRole, num int, quality string, ok bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		return roleUnknown, 0, "", false
	}

	// Якість перевіряється першою: «1080p» не повинно прочитатися
	// як серія «1» з цифри 1080.
	if m := reQualityTitle.FindStringSubmatch(title); m != nil {
		return roleQuality, 0, normalizeQualityLabel(m[1]), true
	}

	if m := reSeasonTitle.FindStringSubmatch(title); m != nil {
		if n, ok2 := firstPositiveInt(m[1:]); ok2 {
			return roleSeason, n, "", true
		}
	}

	if m := reEpisodeTitle.FindStringSubmatch(title); m != nil {
		if n, ok2 := firstPositiveInt(m[1:]); ok2 {
			return roleEpisode, n, "", true
		}
	}

	if m := reBareEpisode.FindStringSubmatch(title); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return roleEpisode, n, "", true
		}
	}

	return roleUnknown, 0, "", false
}

// firstPositiveInt бере перше ненульове ціле з груп регулярки.
// Регулярки двосторонні («1 сезон» і «сезон 1»), тому одна з груп
// завжди порожня.
func firstPositiveInt(groups []string) (int, bool) {
	for _, g := range groups {
		if g == "" {
			continue
		}
		if n, err := strconv.Atoi(g); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// cleanDubTitle прибирає службові суфікси та зовнішні пробіли.
// Провідний пробіл у title (« 1+1») — це маркер рівня вкладеності,
// а не частина імені.
func cleanDubTitle(raw string) string {
	return strings.TrimSpace(dubTitleNoise.ReplaceAllString(raw, ""))
}

// numberFromPlaylistURL намагається дістати (сезон, серія) з URL
// файлу. Потрібно для вузлів з порожнім Title, але осмисленим
// шляхом на кшталт /serials/foo_s01e05/.
func numberFromPlaylistURL(rawURL string) (season, episode int) {
	if m := reURLsXe.FindStringSubmatch(rawURL); m != nil {
		season, _ = strconv.Atoi(m[1])
		episode, _ = strconv.Atoi(m[2])
		return season, episode
	}
	if m := reURLPath.FindStringSubmatch(rawURL); m != nil {
		if m[1] != "" {
			season, _ = strconv.Atoi(m[1])
		}
		if m[2] != "" {
			episode, _ = strconv.Atoi(m[2])
		}
	}
	return season, episode
}

// nodeContext обчислює контекст вузла від батьківського.
//
// isContainer — це вузол із дітьми, незалежно від його ролі.
// Раніше ми повертали false для season/episode/quality, через що
// обхід зупинявся на кореневому «1 сезон» і дерево
// season->dub->episode не давало жодного листка.
//
// Винесено окремо, бо цим же кодом користується filterByDub: якщо
// нумерація рахувалася б в обході, то відкидання «чужих» дубів
// з'їхало б і серія 3 стала б серією 1.
func nodeContext(inherit PlaylistContext, node playerJSPlaylistItem) (ctx PlaylistContext, isContainer bool) {
	ctx = inherit
	title := cleanDubTitle(node.Title)
	isContainer = len(node.Folder) > 0

	role, num, quality, ok := classifyPlaylistNode(title)
	switch {
	case ok && role == roleQuality:
		ctx.Quality = quality
	case ok && role == roleSeason:
		ctx.Season = num
	case ok && role == roleEpisode:
		ctx.Episode = num
	case !ok && isContainer && title != "":
		if ctx.Dub == "" {
			ctx.Dub = title
		} else {
			// Вкладене групування: не затираємо вже відому студію,
			// але й не втрачаємо нову мітку — склеюємо.
			ctx.Dub = ctx.Dub + " / " + title
		}
	}

	// Tortuga ховає озвучку не в title, а у самому значенні file:
	// «{1+1}https://…/index.m3u8(subtitle:)». Дістаємо її тут, а не
	// у WalkPlaylist, бо filterByDub викликає саме nodeContext — і
	// якщо діставати лише в обході, фільтрація за озвучкою втрачала
	// б усі листки й BuildVoiceovers повертав порожні сезони.
	if ctx.Dub == "" {
		ApplyTortugaDub(&ctx, rawPlaylistFile(node.File))
	}

	return ctx, isContainer
}

// WalkPlaylist обходить дерево плейлиста і викликає onLeaf для
// кожного вузла з файлом.
//
// fallbackIndex — 1-based позиція серед siblings. Якщо нумерації
// немає ні в Title, ні в URL, лист отримує саме цю позицію, щоб
// епізоди не скинулися на серію 1.
//
// Обхід детермінований (завжди перша дитина першою), тому результат
// не залежить від порядку ключів у JSON.
func WalkPlaylist(items []playerJSPlaylistItem, inherit PlaylistContext, fallbackIndex int, onLeaf func(PlaylistLeaf)) {
	if len(items) == 0 {
		return
	}

	for idx, node := range items {
		pos := fallbackIndex
		if pos == 0 {
			pos = idx + 1
		}

		ctx, isContainer := nodeContext(inherit, node)

		if file := rawPlaylistFile(node.File); file != "" {
			leafCtx := ctx

			if leafCtx.Season == 0 && leafCtx.Episode == 0 {
				if s, e := numberFromPlaylistURL(file); s > 0 || e > 0 {
					if leafCtx.Season == 0 {
						leafCtx.Season = s
					}
					if leafCtx.Episode == 0 {
						leafCtx.Episode = e
					}
				}
			}
			if leafCtx.Episode == 0 {
				leafCtx.Episode = pos
			}
			onLeaf(PlaylistLeaf{Ctx: leafCtx, File: file, Subtitle: node.Subtitle})
		}

		if isContainer {
			WalkPlaylist(node.Folder, ctx, 0, onLeaf)
		}
	}
}

// rawPlaylistFile дістає File як рядок. PlayerJS дозволяє File бути
// рядком, але частина CDN кладе туди об'єкт або масив.
func rawPlaylistFile(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// CollectPlaylistLeaves збирає всі листя дерева.
func CollectPlaylistLeaves(items []playerJSPlaylistItem) []PlaylistLeaf {
	var leaves []PlaylistLeaf
	WalkPlaylist(items, PlaylistContext{}, 0, func(l PlaylistLeaf) {
		leaves = append(leaves, l)
	})
	return leaves
}

// ForEachPlaylistLeaf форма з зворотним викликом для обходу дерева.
// Окрема функція, а не додатковий аргумент CollectPlaylistLeaves:
// з optional-аргументом виклики CollectPlaylistLeaves(items, cb) та
// CollectPlaylistLeaves(items) відрізнялися б лише кількістю
// аргументів, і така помилка гірше читається в діагностиці.
func ForEachPlaylistLeaf(items []playerJSPlaylistItem, onLeaf func(PlaylistLeaf)) {
	WalkPlaylist(items, PlaylistContext{}, 0, onLeaf)
}

// ParsePlaylistJSON розбирає PlayerJS-плейлист, який прийшов рядком
// (наприклад, із file: "...").
//
// Крім звичайного JSON підтримано подвійне серіалізування
// ("[\"[{...}]\"]") та екранування лише слэшів (вони трапляються,
// коли плеєр серіалізує JSON у JSON).
func ParsePlaylistJSON(raw string) ([]playerJSPlaylistItem, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}

	var items []playerJSPlaylistItem
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		return items, len(items) > 0
	}

	var inner string
	if err := json.Unmarshal([]byte(raw), &inner); err == nil {
		var retry []playerJSPlaylistItem
		if err := json.Unmarshal([]byte(inner), &retry); err == nil {
			return retry, len(retry) > 0
		}
	}

	if unescaped := strings.ReplaceAll(raw, `\/`, "/"); unescaped != raw {
		var retry []playerJSPlaylistItem
		if err := json.Unmarshal([]byte(unescaped), &retry); err == nil {
			return retry, len(retry) > 0
		}
	}
	return nil, false
}

// filterByDub повертає копію дерева, що містить лише листки з
// вказаною студією озвучення.
func filterByDub(items []playerJSPlaylistItem, dub string) []playerJSPlaylistItem {
	var walk func(nodes []playerJSPlaylistItem, inherit PlaylistContext) []playerJSPlaylistItem
	walk = func(nodes []playerJSPlaylistItem, inherit PlaylistContext) []playerJSPlaylistItem {
		var kept []playerJSPlaylistItem
		for _, node := range nodes {
			ctx, isContainer := nodeContext(inherit, node)

			clone := playerJSPlaylistItem{Title: node.Title, File: node.File, Subtitle: node.Subtitle}

			if rawPlaylistFile(node.File) != "" && ctx.Dub == dub {
				kept = append(kept, clone)
				continue
			}
			if isContainer {
				if sub := walk(node.Folder, ctx); len(sub) > 0 {
					clone.Folder = sub
					kept = append(kept, clone)
				}
			}
		}
		return kept
	}
	return walk(items, PlaylistContext{})
}

// episodeKey — ідентифікатор серії в межах однієї студії та сезону.
type episodeKey struct {
	dub    string
	season int
	ep     int
}

type seasonKey struct {
	dub    string
	season int
}

// groupLeaves зводить листя до (студія, сезон) -> епізоди. Порядок
// визначається першою появою, щоб результат був стабільним.
func groupLeaves(items []playerJSPlaylistItem) (map[seasonKey][]domain.Episode, []seasonKey) {
	buckets := map[seasonKey][]domain.Episode{}
	seenEpisode := map[episodeKey]bool{}
	var order []seasonKey

	ForEachPlaylistLeaf(items, func(l PlaylistLeaf) {
		season := l.Ctx.Season
		if season <= 0 {
			season = 1
		}
		if l.Ctx.Episode <= 0 {
			return
		}

		ek := episodeKey{dub: l.Ctx.Dub, season: season, ep: l.Ctx.Episode}
		sk := seasonKey{dub: l.Ctx.Dub, season: season}

		if seenEpisode[ek] {
			return
		}
		seenEpisode[ek] = true

		ep := domain.Episode{Number: l.Ctx.Episode}
		if l.Ctx.Dub != "" {
			// Дуб і номери їдуть у URL, щоб GetStreams зміг
			// розібрати ref назад і віддати потрібну озвучку.
			ep.URL = EncodeVoiceEpisodeRef(l.Ctx.Dub, season, l.Ctx.Episode)
		}

		if _, ok := buckets[sk]; !ok {
			buckets[sk] = nil
			order = append(order, sk)
		}
		buckets[sk] = append(buckets[sk], ep)
	})

	return buckets, order
}

// BuildSeasonsFromPlaylist будує domain.Season з дерева плейлиста.
//
// Одна й та сама серія може зустрічатися в кількох гілках (різні
// студії), тому листя групуються за (dub, season), а не за порядком
// обходу.
func BuildSeasonsFromPlaylist(items []playerJSPlaylistItem) []domain.Season {
	buckets, order := groupLeaves(items)
	if len(order) == 0 {
		return nil
	}

	out := make([]domain.Season, 0, len(order))
	for _, sk := range order {
		list := buckets[sk]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Number < list[j].Number })
		out = append(out, domain.Season{
			Number:   sk.season,
			Title:    "Сезон " + strconv.Itoa(sk.season),
			Episodes: list,
		})
	}
	return out
}

// BuildVoiceoversFromPlaylist будує domain.Voiceover: по одному
// запису на студію озвування з повним деревом сезонів усередині.
//
// Клієнт (details_page) будує селектор озвучки саме з цього поля,
// тому воно мусить бути повним, а не «один запис на провайдера».
func BuildVoiceoversFromPlaylist(items []playerJSPlaylistItem) []domain.Voiceover {
	var order []string
	seen := map[string]bool{}

	ForEachPlaylistLeaf(items, func(l PlaylistLeaf) {
		dub := l.Ctx.Dub
		if dub == "" || seen[dub] {
			return
		}
		seen[dub] = true
		order = append(order, dub)
	})

	if len(order) == 0 {
		return nil
	}

	out := make([]domain.Voiceover, 0, len(order))
	for _, dub := range order {
		id, name, _ := ResolveVoiceoverNameNormalised(dub)
		out = append(out, domain.Voiceover{
			ID:      id,
			Name:    name,
			Seasons: BuildSeasonsFromPlaylist(filterByDub(items, dub)),
		})
	}
	SortVoiceoversByWeight(out)
	return out
}

// SelectPlaylistStream вибирає листки за запитом (сезон, серія,
// озвучка) і перетворює їх на domain.StreamSource.
//
// season<=0 або episode<=0 означає «не важливо» — беремо все, що
// пройшло фільтр. Це потрібно, бо DLE-пошук часто не знає номера
// серії, і провайдер хоче віддати всі джерела одразу.
// filterLeavesByDub повертає листи вибраної озвучки. Лист без вказаної
// озвучки не відкидається: дерева, де назва студії не розпізналася,
// не повинні зникнути через фільтр.
func filterLeavesByDub(leaves []PlaylistLeaf, voiceID string) []PlaylistLeaf {
	out := make([]PlaylistLeaf, 0, len(leaves))
	for _, l := range leaves {
		if l.Ctx.Dub == "" || strings.EqualFold(l.Ctx.Dub, voiceID) {
			out = append(out, l)
		}
	}
	return out
}

func SelectPlaylistStream(items []playerJSPlaylistItem, playerURL, playerLabel string, season, episode int, voiceID string) ([]domain.StreamSource, []domain.SubtitleSource) {
	var streams []domain.StreamSource
	var subs []domain.SubtitleSource

	matches := func(l PlaylistLeaf) bool {
		if season > 0 && l.Ctx.Season > 0 && l.Ctx.Season != season {
			return false
		}
		if episode > 0 && l.Ctx.Episode > 0 && l.Ctx.Episode != episode {
			return false
		}
		return true
	}

	// Звузити дерево до вибраної озвучки ДО вибору епізоду: інакше
	// «перший лист» був би першим листом чужої озвучки.
	leaves := CollectPlaylistLeaves(items)
	if voiceID != "" {
		if filtered := filterLeavesByDub(leaves, voiceID); len(filtered) > 0 {
			leaves = filtered
		}
	}

	// Без вибраного сезону/епізоду віддаємо лише перший лист.
	//
	// Раніше фільтр пропускав усе, тож запит із детальної сторінки
	// (season=0, episode=0) приносив усі 155 серій чотирьох сезонів
	// замість трьох якостей першої серії — клієнт показував
	// «Завантажено 155 потоків».
	if season <= 0 && episode <= 0 && len(leaves) > 0 {
		leaves = leaves[:1]
	}

	for _, l := range leaves {
		if !matches(l) {
			continue
		}
		_, dubName, _ := ResolveVoiceoverNameNormalised(l.Ctx.Dub)
		streams = append(streams, parseMultiQualityString(l.File, playerURL, dubName)...)
		if l.Subtitle != "" {
			subs = append(subs, parseSubtitlesFromPlayerHTML(l.Subtitle)...)
		}
		if extra := ApplyTortugaSubtitle(l.File); len(extra) > 0 {
			subs = append(subs, extra...)
		}
	}

	// Фолбек: віддаємо перший доступний лист, щоб деталь не виглядала
	// порожньою через один невірно розпізнаний номер.
	if len(streams) == 0 {
		if leaves := CollectPlaylistLeaves(items); len(leaves) > 0 {
			_, dubName, _ := ResolveVoiceoverNameNormalised(leaves[0].Ctx.Dub)
			streams = parseMultiQualityString(leaves[0].File, playerURL, dubName)
			if leaves[0].Subtitle != "" {
				subs = append(subs, parseSubtitlesFromPlayerHTML(leaves[0].Subtitle)...)
			}
		}
	}

	SortStreamsByDubWeight(streams)
	return streams, subs
}

// playlistLabel склеює студію озвучення та назву плеєра так, щоб у
// переліку Quality було видно і те, і те.
func playlistLabel(dub, playerLabel string) string {
	if dub == "" {
		return playerLabel
	}
	if playerLabel == "" || strings.Contains(dub, playerLabel) {
		return dub
	}
	return dub + " (" + playerLabel + ")"
}

// voiceEpisodeRefPrefix — маркер нашого формату ref. Обрано так,
// щоб його не можна було сплутати з URL плеєра.
const voiceEpisodeRefPrefix = "v:"

// EncodeVoiceEpisodeRef кодує трійку (озвучка, сезон, серія) у
// компактний ref, який GetStreams потім розбере назад.
func EncodeVoiceEpisodeRef(dub string, season, episode int) string {
	return voiceEpisodeRefPrefix + strconv.Itoa(season) + ":" + strconv.Itoa(episode) + ":" + dub
}

// DecodeVoiceEpisodeRef — зворотна операція. ok=false, якщо ref не
// нашого формату, і тоді озвучку треба брати з іншого джерела.
func DecodeVoiceEpisodeRef(ref string) (voiceID string, season, episode int, ok bool) {
	if !strings.HasPrefix(ref, voiceEpisodeRefPrefix) {
		return "", 0, 0, false
	}
	parts := strings.SplitN(ref[len(voiceEpisodeRefPrefix):], ":", 3)
	if len(parts) < 2 {
		return "", 0, 0, false
	}
	season, err := strconv.Atoi(parts[0])
	if err != nil || season <= 0 {
		return "", 0, 0, false
	}
	episode, err = strconv.Atoi(parts[1])
	if err != nil || episode <= 0 {
		return "", 0, 0, false
	}
	voiceID = ""
	if len(parts) == 3 {
		voiceID = parts[2]
	}
	return voiceID, season, episode, true
}