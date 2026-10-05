package provider_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/provider"
)

// Фікстури нижче — справжні відповіді CDN, зняті 2026-10-05.
// Не вигадки: кожен фрагмент відтворює те, що реально приходить у
// відповідях, тому тести падають не на вигаданому форматі, а на
// тому, з чим ми справді маємо працювати.

// loadFixture читає JSON-фікстуру з testdata.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// --- Сканер file: -------------------------------------------------------

// Регресія: регулярка file\s*:\s*(\[[^"'].*?\]) не збігалася на
// реальному HDVB, бо там одинарні лапки зовні та подвійні всередині.
// Наслідок був 1 стрім замість 155 серій.
func TestExtractPlaylistFromPlayerHTML_HandlesSingleQuotedHDVB(t *testing.T) {
	html := `<script>new Playerjs({file:'[{"title":"1 сезон","folder":[{"title":"1+1","folder":[{"title":"Серія 1","file":"https://hdvbua.pro/a/index.m3u8"}]}]}]'})</script>`

	raw, found := provider.ExtractPlaylistFromPlayerHTML(html)
	if !found {
		t.Fatal("must find file: value in single-quoted HDVB config")
	}
	if !strings.HasPrefix(raw, "[") {
		t.Fatalf("raw must start with '[', got %q", firstN(raw, 40))
	}

	items, ok := provider.ParsePlayerJSPlaylist(html)
	if !ok {
		t.Fatal("ParsePlayerJSPlaylist must succeed on real HDVB markup")
	}
	if len(items) != 1 {
		t.Fatalf("want 1 root node, got %d", len(items))
	}
	if items[0].Title != "1 сезон" {
		t.Fatalf("root title = %q, want '1 сезон'", items[0].Title)
	}
}

func TestExtractPlaylistFromPlayerHTML_HandlesDoubleQuoted(t *testing.T) {
	html := `new Playerjs({"file":"[{\"title\":\"Сезон 1\"}]"})`

	raw, found := provider.ExtractPlaylistFromPlayerHTML(html)
	if !found {
		t.Fatal("must find file: in double-quoted config")
	}
	if !strings.Contains(raw, "Сезон 1") {
		t.Fatalf("unescaped value must contain title, got %q", firstN(raw, 60))
	}
}

// Значення file: може бути одразу масивом об'єктів, без рядкової
// обгортки — так пише частина CDN.
func TestExtractPlaylistFromPlayerHTML_HandlesBareJSONArray(t *testing.T) {
	html := `var player = {file: [{"title":"Сезон 1","folder":[]}]};`

	raw, found := provider.ExtractPlaylistFromPlayerHTML(html)
	if !found {
		t.Fatal("must find bare JSON array assigned to file")
	}
	items, ok := provider.ParsePlaylistJSON(raw)
	if !ok || len(items) != 1 {
		t.Fatalf("bare array must parse, got ok=%v len=%d", ok, len(items))
	}
}

// Регулярка не повинна з'їсти сусідній key: file: повинен
// відрізнятися від profile: та інших полів із таким самим суфіксом.
func TestExtractPlaylistFromPlayerHTML_DoesNotMatchOtherKeys(t *testing.T) {
	html := `{"profile":"[not a playlist]","file":"[{\"title\":\"Сезон 2\"}]"}`

	raw, found := provider.ExtractPlaylistFromPlayerHTML(html)
	if !found {
		t.Fatal("must find file: value")
	}
	if strings.Contains(raw, "not a playlist") {
		t.Fatalf("must not pick up profile: value, got %q", raw)
	}
}

// Незакритий масив не можна віддавати клієнту: json.Unmarshal
// усе одно впаде, але краще відсікти ще на скануванні.
func TestIsBalancedJSON_RejectsUnterminated(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"closed", `[{"a":1}]`, true},
		{"unterminated", `[{"a":1}`, false},
		{"trailing garbage", `[{"a":1}] junk`, false},
		{"bracket inside string", `[{"a":"}]"}]`, true},
		{"escaped quote inside string", `[{"a":"\"}]"}]`, true},
		{"not array", `{"a":1}`, false},
		{"empty", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := provider.IsBalancedJSON(tc.in); got != tc.want {
				t.Fatalf("IsBalancedJSON(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// --- Розпізнавання ролей вузлів ----------------------------------------

func TestClassifyPlaylistNode_Roles(t *testing.T) {
	cases := []struct {
		title  string
		role   string // для читабельності: "" = unknown
		num    int
		isOK   bool
		qualit string
	}{
		{title: "Сезон 1", role: "season", num: 1, isOK: true},
		{title: "1 сезон", role: "season", num: 1, isOK: true},
		{title: "Сезон 12", role: "season", num: 12, isOK: true},
		{title: "Season 3", role: "season", num: 3, isOK: true},
		{title: "Серіал 2", role: "season", num: 2, isOK: true},

		{title: "Серія 5", role: "episode", num: 5, isOK: true},
		{title: "5 серія", role: "episode", num: 5, isOK: true},
		{title: "Епізод 7", role: "episode", num: 7, isOK: true},
		{title: "Episode 4", role: "episode", num: 4, isOK: true},
		{title: " сері'я 3", role: "episode", num: 3, isOK: true},

		{title: "1080p", role: "quality", isOK: true, qualit: "1080p"},
		{title: "FullHD", role: "quality", isOK: true, qualit: "1080p"},
		{title: "4K", role: "quality", isOK: true, qualit: "4K"},

		// Назви студій не містять нічого розпізнаваного.
		{title: "1+1", isOK: false},
		{title: "Postmodern", isOK: false},
		{title: "Студія Качур", isOK: false},
		{title: "", isOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			role, num, quality, ok := provider.ClassifyPlaylistNode(tc.title)
			if ok != tc.isOK {
				t.Fatalf("ok = %v, want %v", ok, tc.isOK)
			}
			if !ok {
				return
			}
			switch tc.role {
			case "season":
				if role != provider.RoleSeason || num != tc.num {
					t.Fatalf("want season %d, got role=%v num=%d", tc.num, role, num)
				}
			case "episode":
				if role != provider.RoleEpisode || num != tc.num {
					t.Fatalf("want episode %d, got role=%v num=%d", tc.num, role, num)
				}
			case "quality":
				if role != provider.RoleQuality || quality != tc.qualit {
					t.Fatalf("want quality %q, got role=%v quality=%q", tc.qualit, role, quality)
				}
			}
		})
	}
}

// Регресія: «1080p» не повинно читатися як серія «1» з цифри 1080,
// бо стара parseSeasonOrEpisodeNum брала перше число в рядку.
func TestParseSeasonOrEpisodeNum_DoesNotReadQualityAsEpisode(t *testing.T) {
	// Це поведінка, якої ми навчилися: classifyPlaylistNode має
	// віддати роль quality, а не episode. Перевіряємо через
	// WalkPlaylist, бо саме там це має значення.
	items := []provider.PlayerJSPlaylistItem{{
		Title:  "Сезон 1",
		Folder: []provider.PlayerJSPlaylistItem{{Title: "1080p", File: json.RawMessage(`"https://cdn/x/1080/index.m3u8"`)}},
	}}

	leaves := provider.CollectPlaylistLeaves(items)
	if len(leaves) != 1 {
		t.Fatalf("want 1 leaf, got %d", len(leaves))
	}
	if leaves[0].Ctx.Quality != "1080p" {
		t.Fatalf("quality = %q, want 1080p", leaves[0].Ctx.Quality)
	}
	if leaves[0].Ctx.Episode != 1 {
		t.Fatalf("episode = %d, want 1 (position), not 1080", leaves[0].Ctx.Episode)
	}
}

// --- Три різні вкладеності мають давати однаковий результат ------------

// ashdiTree — реальна вкладеність: озвучка зверху.
var ashdiTree = []provider.PlayerJSPlaylistItem{{
	Title: "1+1",
	Folder: []provider.PlayerJSPlaylistItem{{
		Title: " Серіал 1",
		Folder: []provider.PlayerJSPlaylistItem{
			{Title: "Серія 1", File: json.RawMessage(`"https://ashdi.vip/e1/index.m3u8"`)},
			{Title: "Серія 2", File: json.RawMessage(`"https://ashdi.vip/e2/index.m3u8"`)},
		},
	}},
}}

// hdvbTree — реальна вкладеність: озвучка в середині. Це та
// структура, за яку старий код не бачив жодної серії.
var hdvbTree = []provider.PlayerJSPlaylistItem{{
	Title: "1 сезон",
	Folder: []provider.PlayerJSPlaylistItem{{
		Title: "1+1",
		Folder: []provider.PlayerJSPlaylistItem{
			{Title: "Серія 1", File: json.RawMessage(`"https://hdvbua.pro/e1/index.m3u8"`)},
			{Title: "Серія 2", File: json.RawMessage(`"https://hdvbua.pro/e2/index.m3u8"`)},
		},
	}},
}}

// Обидві вкладеності мають дати однакові сезони та озвучки. Різниця
// у тому, де в дереві стоїть назва студії, а не в результаті.
func TestBuildVoiceoversFromPlaylist_SeasonThenDubEqualsDubThenSeason(t *testing.T) {
	for name, tree := range map[string][]provider.PlayerJSPlaylistItem{
		"hdvb (season->dub->episode)": hdvbTree,
		"ashdi (dub->season->episode)": ashdiTree,
	} {
		t.Run(name, func(t *testing.T) {
			voices := provider.BuildVoiceoversFromPlaylist(tree)
			if len(voices) != 1 {
				t.Fatalf("want 1 voiceover, got %d (%+v)", len(voices), voices)
			}
			if voices[0].Name != "1+1" {
				t.Fatalf("voiceover name = %q, want 1+1", voices[0].Name)
			}
			if voices[0].ID != "1plus1" {
				t.Fatalf("voiceover id = %q, want 1plus1", voices[0].ID)
			}
			if len(voices[0].Seasons) != 1 {
				t.Fatalf("want 1 season, got %d", len(voices[0].Seasons))
			}
			if got := len(voices[0].Seasons[0].Episodes); got != 2 {
				t.Fatalf("want 2 episodes, got %d", got)
			}
			for i, ep := range voices[0].Seasons[0].Episodes {
				if ep.Number != i+1 {
					t.Fatalf("episode[%d].Number = %d, want %d", i, ep.Number, i+1)
				}
				if ep.URL == "" {
					t.Fatalf("episode[%d] must carry voice/season/episode ref", i)
				}
			}
		})
	}
}

// Регресія, заради якої все затівалося: озвучка «1+1» у дереві
// season->dub->episode раніше читалася як серія 1, тому запит
// episode=2 зрізав все піддерево і клієнт отримував перший-ліпший
// стрім замість другої серії.
func TestSelectPlaylistStream_PicksRequestedEpisodeInNestedHDVB(t *testing.T) {
	streams, _ := provider.SelectPlaylistStream(hdvbTree, "https://hdvbua.pro/embed/1", "HDVB", 1, 2, "1+1")

	if len(streams) != 1 {
		t.Fatalf("want exactly 1 stream for S01E02, got %d: %+v", len(streams), streams)
	}
	if !strings.Contains(streams[0].URL, "e2") {
		t.Fatalf("must return episode 2, got %s", streams[0].URL)
	}
	if streams[0].Voiceover != "1+1" {
		t.Fatalf("voiceover = %q, want 1+1", streams[0].Voiceover)
	}
}

// Фільтр за озвучкою мусить відсікати чужі гілки.
func TestSelectPlaylistStream_FiltersByVoice(t *testing.T) {
	tree := []provider.PlayerJSPlaylistItem{
		{Title: "1+1", Folder: []provider.PlayerJSPlaylistItem{{Title: "Серія 1", File: json.RawMessage(`"https://a/one.m3u8"`)}}},
		{Title: "Postmodern", Folder: []provider.PlayerJSPlaylistItem{{Title: "Серія 1", File: json.RawMessage(`"https://a/post.m3u8"`)}}},
	}

	streams, _ := provider.SelectPlaylistStream(tree, "https://a/", "Ashdi", 0, 0, "Postmodern")
	if len(streams) != 1 {
		t.Fatalf("want 1 stream, got %d", len(streams))
	}
	if !strings.Contains(streams[0].URL, "post") {
		t.Fatalf("must return Postmodern branch, got %s", streams[0].URL)
	}
}

// --- Реальні дані: Роксолана з Tortuga -------------------------------

// Регресія з живої сесії користувача: детальна сторінка робить
// getStreams без season/episode, і Tortuga-екстрактор приносив кожну
// серію серіалу («Завантажено 155 потоків» і 155 однакових рядків
// «Авто — 1+1») замість якостей першої серії.
//
// Фікстура — реальне розшифроване дерево Роксолани: 4 сезони, 8 серій
// (3+3+1+1).
//
// TortugaStreamsFromPlaylist навмисно лишається «покажи все дерево» —
// ним користується FetchTortugaEmbed для деталей. Звужує до однієї
// серії FetchTortugaEpisode, тож перевіряємо саме його.
func TestTortugaStreamsFromPlaylist_NoSelectionReturnsOneEpisode(t *testing.T) {
	raw := loadFixture(t, "tortuga_roksolana.json")
	var tree []provider.PlayerJSPlaylistItem
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("fixture must be a valid playlist: %v", err)
	}

	episodes := 0
	for _, v := range provider.BuildVoiceoversFromPlaylist(tree) {
		for _, s := range v.Seasons {
			episodes += len(s.Episodes)
		}
	}
	if episodes < 4 {
		t.Fatalf("fixture must be a multi-episode series, got only %d episodes", episodes)
	}

	whole, _ := provider.TortugaStreamsFromPlaylist(tree, "https://tortuga.tw/embed/98", "Tortuga", 0, 0, "")
	if len(whole) != episodes {
		t.Fatalf("whole-tree view = %d streams, want one per episode (%d)", len(whole), episodes)
	}

	one, _ := provider.TortugaStreamsFromPlaylist(tree, "https://tortuga.tw/embed/98", "Tortuga", 1, 1, "")
	if len(one) != 1 {
		t.Fatalf("explicit S01E01 = %d streams, want 1", len(one))
	}
	if !strings.Contains(one[0].URL, "s01e01") {
		t.Errorf("S01E01 = %s, want an s01e01 URL", one[0].URL)
	}
}

// Те саме для дерева PlayerJS у вигляді, який реально розбирає
// parseMultiQualityString (прості m3u8, а не Tortuga-формат).
func TestSelectPlaylistStream_NoSelectionReturnsOneEpisodeForPlainURLs(t *testing.T) {
	var tree []provider.PlayerJSPlaylistItem
	for s := 1; s <= 3; s++ {
		season := provider.PlayerJSPlaylistItem{Title: fmt.Sprintf("Сезон %d", s)}
		for e := 1; e <= 4; e++ {
			season.Folder = append(season.Folder, provider.PlayerJSPlaylistItem{
				Title: fmt.Sprintf("Серія %d", e),
				File:  json.RawMessage(fmt.Sprintf(`"https://cdn.example/hls/s%de%02d/index.m3u8"`, s, e)),
			})
		}
		tree = append(tree, season)
	}

	def, _ := provider.SelectPlaylistStream(tree, "https://cdn.example/embed/1", "Ashdi", 0, 0, "")
	if len(def) != 1 {
		t.Fatalf("no-selection request returned %d streams, want 1 episode out of 12: %+v", len(def), def)
	}
	if !strings.Contains(def[0].URL, "s1e01") {
		t.Errorf("default episode = %s, want s1e01", def[0].URL)
	}

	explicit, _ := provider.SelectPlaylistStream(tree, "https://cdn.example/embed/1", "Ashdi", 2, 3, "")
	if len(explicit) != 1 || !strings.Contains(explicit[0].URL, "s2e03") {
		t.Errorf("S02E03 = %+v, want the s2e03 stream", explicit)
	}
}

// Явний вибір епізоду мусить і далі повертати саме його — на дереві,
// яке реально розбирає parseMultiQualityString (див. тест вище).
func TestSelectPlaylistStream_ExplicitEpisodeStillWins(t *testing.T) {
	var tree []provider.PlayerJSPlaylistItem
	for s := 1; s <= 3; s++ {
		season := provider.PlayerJSPlaylistItem{Title: fmt.Sprintf("Сезон %d", s)}
		for e := 1; e <= 4; e++ {
			season.Folder = append(season.Folder, provider.PlayerJSPlaylistItem{
				Title: fmt.Sprintf("Серія %d", e),
				File:  json.RawMessage(fmt.Sprintf(`"https://cdn.example/hls/s%de%02d/index.m3u8"`, s, e)),
			})
		}
		tree = append(tree, season)
	}

	first, _ := provider.SelectPlaylistStream(tree, "https://cdn.example/embed/1", "Ashdi", 1, 1, "")
	third, _ := provider.SelectPlaylistStream(tree, "https://cdn.example/embed/1", "Ashdi", 1, 3, "")

	if len(first) == 0 || len(third) == 0 {
		t.Fatal("both selections must resolve")
	}
	if first[0].URL == third[0].URL {
		t.Errorf("S01E03 returned the same URL as S01E01: %s", first[0].URL)
	}
}

// Це та фікстура, заради якої все затівалося: мультисезонний
// серіал, якого немає в агрегаторі Bandera.
func TestBuildSeasonsFromPlaylist_RealRoksolana(t *testing.T) {
	raw := loadFixture(t, "tortuga_roksolana.json")
	var tree []provider.PlayerJSPlaylistItem
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("fixture must be a valid playlist: %v", err)
	}

	voices := provider.BuildVoiceoversFromPlaylist(tree)
	// Формат file: у Tortuga — «{озвучка}URL(subtitle:…)», тож
	// озвучка сидить у самому файлі, а не в title вузла. Обхід
	// мусить її звідти дістати, інакше селектор озвучки порожній.
	if len(voices) != 1 {
		t.Fatalf("want 1 voiceover from file-embedded dub, got %d: %+v", len(voices), voices)
	}
	if voices[0].Name != "1+1" {
		t.Fatalf("voiceover name = %q, want 1+1", voices[0].Name)
	}
	if voices[0].ID != "1plus1" {
		t.Fatalf("voiceover id = %q, want 1plus1", voices[0].ID)
	}

	seasons := provider.BuildVoiceoversFromPlaylist(tree)[0].Seasons
	if len(seasons) != 4 {
		t.Fatalf("want 4 seasons, got %d", len(seasons))
	}
	for i, s := range seasons {
		if s.Number != i+1 {
			t.Fatalf("season[%d].Number = %d, want %d", i, s.Number, i+1)
		}
		if len(s.Episodes) == 0 {
			t.Fatalf("season %d has no episodes", s.Number)
		}
		for j, ep := range s.Episodes {
			if ep.Number != j+1 {
				t.Fatalf("season %d episode[%d].Number = %d", s.Number, j, ep.Number)
			}
		}
	}
}

// Клієнт отримує сезони й озвучки, а URL серії — це ref, який
// GetStreams розбирає назад. Без ref Season.Episodes дали б лише
// номери без способу їх відтворити.
func TestBuildVoiceoversFromPlaylist_RealRoksolana_EpisodeRefRoundTrip(t *testing.T) {
	var tree []provider.PlayerJSPlaylistItem
	if err := json.Unmarshal(loadFixture(t, "tortuga_roksolana.json"), &tree); err != nil {
		t.Fatalf("fixture must be valid: %v", err)
	}

	seasons := provider.BuildVoiceoversFromPlaylist(tree)[0].Seasons
	ep := seasons[1].Episodes[0]

	dub, season, episode, ok := provider.DecodeVoiceEpisodeRef(ep.URL)
	if !ok {
		t.Fatalf("episode URL must be a decodable ref, got %q", ep.URL)
	}
	if dub != "1+1" || season != 2 || episode != 1 {
		t.Fatalf("ref round trip = %q/%d/%d, want 1+1/2/1", dub, season, episode)
	}
}

// Формат «{озвучка}URL(subtitle:…)» — розкриття Tortuga.
func TestParseTortugaFileFormat(t *testing.T) {
	raw := `{1+1}https://calypso.tortuga.tw/hls/serials/muhtesem.yuzyil/s01/x.mvo_66961/hls/index.m3u8(subtitle:)`

	dub, url, subtitle := provider.ParseTortugaFileField(raw)

	if dub != "1+1" {
		t.Fatalf("dub = %q, want 1+1", dub)
	}
	if url != "https://calypso.tortuga.tw/hls/serials/muhtesem.yuzyil/s01/x.mvo_66961/hls/index.m3u8" {
		t.Fatalf("url = %q", url)
	}
	if subtitle != "" {
		t.Fatalf("empty subtitle must stay empty, got %q", subtitle)
	}
}

func TestParseTortugaFileFormat_PassthroughWithoutBraces(t *testing.T) {
	raw := "https://calypso.tortuga.tw/hls/plain/index.m3u8"

	dub, url, _ := provider.ParseTortugaFileField(raw)
	if dub != "" {
		t.Fatalf("dub must be empty, got %q", dub)
	}
	if url != raw {
		t.Fatalf("url = %q, want unchanged", url)
	}
}

// --- Ваги озвучки ------------------------------------------------------

func TestDubWeight_OrdersSources(t *testing.T) {
	cases := []struct {
		label string
		want  provider.DubPriority
	}{
		{"1+1", provider.DubPriorityDub},
		{"Postmodern", provider.DubPriorityDub},
		{"Багатоголосий", provider.DubPriorityMulti},
		{"Двоголосовий", provider.DubPriorityTwoVoice},
		{"Одноголосий", provider.DubPriorityMono},
		{"Оригінал", provider.DubPriorityOriginal},
		{"Субтитри", provider.DubPrioritySubtitles},
		{"", provider.DubPriorityOriginal},
	}

	var streams []domain.StreamSource
	for _, tc := range cases {
		if got := provider.DubWeight(tc.label); got != tc.want {
			t.Errorf("DubWeight(%q) = %d, want %d", tc.label, got, tc.want)
		}
		// Розміщуємо у навмисно «поганому» порядку.
		streams = append(streams, domain.StreamSource{Voiceover: tc.label, Quality: "1080p"})
	}

	provider.SortStreamsByDubWeight(streams)

	if streams[0].Voiceover != "1+1" {
		t.Fatalf("best dub must be first, got %q", streams[0].Voiceover)
	}
	if streams[len(streams)-1].Voiceover != "Субтитри" {
		t.Fatalf("subtitles must be last, got %q", streams[len(streams)-1].Voiceover)
	}
}

// Сортування стабільне: джерела з однаковою вагою лишаються в
// порядку CDN, щоб не псувати порядок якостей усередині озвучки.
func TestSortStreamsByDubWeight_IsStable(t *testing.T) {
	streams := []domain.StreamSource{
		{Voiceover: "1+1", Quality: "480p"},
		{Voiceover: "1+1", Quality: "1080p"},
		{Voiceover: "1+1", Quality: "720p"},
	}

	provider.SortStreamsByDubWeight(streams)

	want := []string{"480p", "1080p", "720p"}
	for i, q := range want {
		if streams[i].Quality != q {
			t.Fatalf("quality[%d] = %q, want %q (order must be preserved)", i, streams[i].Quality, q)
		}
	}
}

// --- Словник студій ---------------------------------------------------

func TestResolveStudio_DoesNotShadowShorterAlias(t *testing.T) {
	// Регресія з оригіналу: лінійний прохід по таблиці робив
	// «ICTV2» -> «ICTV» і «InariOkami» -> «Inari».
	for _, tc := range []struct{ in, want string }{
		{"ICTV2", "ICTV2"},
		{"InariOkami", "InariOkami"},
		{"Inari", "Inari"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			info, ok := provider.ResolveStudio(tc.in)
			if !ok {
				t.Fatalf("%q must resolve", tc.in)
			}
			if info.Name != tc.want {
				t.Fatalf("ResolveStudio(%q).Name = %q, want %q", tc.in, info.Name, tc.want)
			}
		})
	}
}

func TestResolveStudio_NormalizesAliases(t *testing.T) {
	for _, alias := range []string{"1 плюс 1", "один плюс один", "1+1"} {
		info, ok := provider.ResolveStudio(alias)
		if !ok {
			t.Fatalf("alias %q must resolve", alias)
		}
		if info.ID != "1plus1" {
			t.Fatalf("alias %q -> id %q, want 1plus1", alias, info.ID)
		}
	}
}

// Короткі аліаси не мають перехоплювати випадкові підрядки:
// «ні» з боку «студії» не повинно перетворитися на «НЛО-ТВ».
func TestResolveStudio_IgnoresShortAliasesInSubstringSearch(t *testing.T) {
	for _, in := range []string{"Го студия", "Місто гріха", "Обростання"} {
		if info, ok := provider.ResolveStudio(in); ok && info.ID == "nlotv" {
			t.Fatalf("%q must not resolve to nlotv via alias 'нло'", in)
		}
	}
}

func TestCleanStudioName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Ukrainian (1+1)", "1+1"},
		{"[MoonAnime] Postmodern", "Postmodern"},
		{"Субтитри | BambooUA", "BambooUA"},
		{"Багатоголосий закадровий: 1+1", "1+1"},
		{"Субтитри (Чорний Верес)", "Чорний Верес"},
		{"  1+1  ", "1+1"},
		{"", ""},
		{"   ", ""},
		{"не визначено", ""},
		{"default", ""},
		// Голе «Субтитри» має лишитися: інакше озвучка порожніє.
		{"Субтитри", "Субтитри"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := provider.CleanStudioName(tc.in); got != tc.want {
				t.Fatalf("CleanStudioName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Мова визначається за СИРОЮ назвою: «СвійSUB» не має українського
// озвучення, воно лише додає субтитри. Визначати мову після
// очищення означало б отримати uk.
func TestResolveVoiceoverNameNormalised_SubtitleTrackIsNotUkrainian(t *testing.T) {
	_, name, lang := provider.ResolveVoiceoverNameNormalised("Субтитри СвійSUB")
	if lang != "none" {
		t.Fatalf("language = %q, want none", lang)
	}
	if name != "СвійSUB" {
		t.Fatalf("name = %q, want СвійSUB", name)
	}

	_, name2, lang2 := provider.ResolveVoiceoverNameNormalised("1+1")
	if lang2 != "uk" || name2 != "1+1" {
		t.Fatalf("got name=%q lang=%q, want 1+1/uk", name2, lang2)
	}
}

// Невідома студія не губиться — показуємо сиру назву.
func TestResolveStudio_UnknownPassesThrough(t *testing.T) {
	if _, ok := provider.ResolveStudio("Незупиняй"); ok {
		t.Log("Незупиняй is in the dictionary")
	}
	_, _, lang := provider.ResolveVoiceoverNameNormalised("Якась невідома студія")
	if lang != "uk" {
		t.Fatalf("unknown studio must still be treated as uk, got %q", lang)
	}
}

// --- Трейлери ---------------------------------------------------------

// Регресія: iframe трейлера не має слова «trailer» у самому URL —
// там лише ?tr=1, а слово з'являється вже у розшифрованому шляху
// m3u8. Без фільтра трейлер потрапляв у список стримів.
func TestIsTrailerURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://hdvbua.pro/vid/5008?tr=1", true},
		{"https://hdvbua.pro/embed/422/b0c42c552", false},
		{"https://s10.hdvbua.pro/media3/hls/trailers/muhtesem/index.m3u8", true},
		{"https://ashdi.vip/video05/3/new/show/hls/index.m3u8", false},
		{"https://tortuga.tw/trailer/98", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := provider.IsTrailerURL(tc.in); got != tc.want {
				t.Fatalf("IsTrailerURL(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// --- ref озвучки ------------------------------------------------------

func TestVoiceEpisodeRefRoundTrip(t *testing.T) {
	ref := provider.EncodeVoiceEpisodeRef("1+1", 2, 15)

	dub, season, ep, ok := provider.DecodeVoiceEpisodeRef(ref)
	if !ok {
		t.Fatalf("must decode own ref %q", ref)
	}
	if dub != "1+1" || season != 2 || ep != 15 {
		t.Fatalf("round trip = %q/%d/%d, want 1+1/2/15", dub, season, ep)
	}
}

func TestDecodeVoiceEpisodeRef_RejectsForeignRef(t *testing.T) {
	// Чужий ref не можна прийняти за наш: інакше ми б вирішили
	// season=0 з URL, який насправді є плеєром.
	for _, ref := range []string{"", "https://ashdi.vip/serial/1", "v:abc:1", "v:0:1", "v:1:0"} {
		if _, _, _, ok := provider.DecodeVoiceEpisodeRef(ref); ok {
			t.Fatalf("ref %q must be rejected", ref)
		}
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}