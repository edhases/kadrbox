package provider

// Експорт внутрішніх деталей для зовнішніх тестів.
//
// Тести лежать у package provider_test, щоб перевіряти публічну
// поведінку, але кілька перевірок мають дістатися до внутрішніх
// функцій: саме вони є предметом регресії (сканер file:, ролі
// вузлів дерева). Go дозволяє це лише через файл у самому
// package — тому експорт живе тут, а не в робочому коді.

// Test hooks. Вони існують тільки під тести, тому ix не видно
// жодному споживачу.

var (
	IsBalancedJSON       = isBalancedJSON
	ClassifyPlaylistNode = classifyPlaylistNode
	CleanStudioName      = cleanStudioName
	// Перевірка контрактів Quality/Voiceover: ці функції є внутрішніми,
	// але саме на них тримається UI, тому тести мають діставатися
	// безпосередньо.
	QualityFromURL          = qualityFromURL
	NormalizeQualityLabel   = normalizeQualityLabel
	CanonicalSubtitleLang   = canonicalSubtitleLang
	ParseSubtitlesFromHTML  = parseSubtitlesFromPlayerHTML
)

// Ролі вузлів — експортуємо константи, щоб тест міг їх порівняти.
const (
	RoleSeason  = roleSeason
	RoleEpisode = roleEpisode
	RoleQuality = roleQuality
)

// PlayerJSPlaylistItem — вузол дерева плейлиста.
type PlayerJSPlaylistItem = playerJSPlaylistItem

// CSVStub існує лише для сумісності зовнішніх тестів, які
// використовують raw JSON. Реальні тести користуються
// encoding/json, тому цей тип не має бути частиною API.