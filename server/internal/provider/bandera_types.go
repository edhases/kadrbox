package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FlexibleString декодує будь-яке скалярне JSON-значення (рядок, число, bool) у рядок.
type FlexibleString string

func (f *FlexibleString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = FlexibleString(strings.TrimSpace(s))
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*f = FlexibleString(n.String())
		return nil
	}
	var fl float64
	if err := json.Unmarshal(b, &fl); err == nil {
		*f = FlexibleString(fmt.Sprintf("%v", fl))
		return nil
	}
	var bl bool
	if err := json.Unmarshal(b, &bl); err == nil {
		*f = FlexibleString(strconv.FormatBool(bl))
		return nil
	}
	*f = FlexibleString(strings.Trim(string(b), "\""))
	return nil
}

func (f FlexibleString) String() string {
	return string(f)
}

// FlexibleFloat декодує число або рядок у float64.
type FlexibleFloat float64

func (f *FlexibleFloat) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	var num float64
	if err := json.Unmarshal(b, &num); err == nil {
		*f = FlexibleFloat(num)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			*f = 0
			return nil
		}
		if parsed, err := strconv.ParseFloat(s, 64); err == nil {
			*f = FlexibleFloat(parsed)
			return nil
		}
	}
	*f = 0
	return nil
}

func (f FlexibleFloat) Float64() float64 {
	return float64(f)
}

// FlexibleInt декодує int, float або рядок у int.
type FlexibleInt int

func (f *FlexibleInt) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	var num int
	if err := json.Unmarshal(b, &num); err == nil {
		*f = FlexibleInt(num)
		return nil
	}
	var fl float64
	if err := json.Unmarshal(b, &fl); err == nil {
		*f = FlexibleInt(int(fl))
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			*f = 0
			return nil
		}
		if parsed, err := strconv.Atoi(s); err == nil {
			*f = FlexibleInt(parsed)
			return nil
		}
	}
	*f = 0
	return nil
}

func (f FlexibleInt) Int() int {
	return int(f)
}

var reYearPattern = regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)

// ParseFlexibleYear парсить рік за правилом bo.js: slice(0, 4) або regexp з перевіркою діапазону 1900..2100.
func ParseFlexibleYear(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var num int
	if err := json.Unmarshal(raw, &num); err == nil {
		if num >= 1900 && num <= 2100 {
			return num
		}
		return 0
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		str = strings.TrimSpace(str)
		if len(str) >= 4 {
			if y, err := strconv.Atoi(str[:4]); err == nil && y >= 1900 && y <= 2100 {
				return y
			}
		}
		if m := reYearPattern.FindString(str); m != "" {
			if y, err := strconv.Atoi(m); err == nil && y >= 1900 && y <= 2100 {
				return y
			}
		}
	}
	return 0
}

// ---- /sources API DTO ----

type BanderaSourcesResponse struct {
	OK      bool                 `json:"ok"`
	Sources []BanderaSourceEntry `json:"sources"`
}

type BanderaSourceEntry struct {
	Key          string                    `json:"key"`
	Name         string                    `json:"name"`
	Enabled      bool                      `json:"enabled"`
	Capabilities BanderaSourceCapabilities `json:"capabilities"`
	Inputs       BanderaSourceInputs       `json:"inputs"`
}

type BanderaSourceCapabilities struct {
	Search  bool `json:"search"`
	Content bool `json:"content"`
	Stream  bool `json:"stream"`
}

type BanderaSourceInputs struct {
	Search  []string `json:"search"`
	Content []string `json:"content"`
	Stream  []string `json:"stream"`
}

// SourceMeta зберігає розпарсений стан одного джерела в пам'яті клієнта
type SourceMeta struct {
	Key         string
	Name        string
	Enabled     bool
	ContentKeys []string
	StreamKeys  []string
	CanSearch   bool
	CanContent  bool
	CanStream   bool
}

// ---- Search API DTO ----

type BanderaSearchResponse struct {
	OK    bool                       `json:"ok"`
	Error FlexibleString             `json:"error,omitempty"`
	Items []BanderaSearchItem        `json:"items"`
	Meta  *BanderaSearchMetaResponse `json:"meta,omitempty"`
}

type BanderaSearchMetaResponse struct {
	Statuses map[string]BanderaSourceStatus `json:"statuses,omitempty"`
}

type BanderaSourceStatus struct {
	Status    string `json:"status"`
	Count     int    `json:"count,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms,omitempty"`
	Elapsed   int64  `json:"elapsed,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s BanderaSourceStatus) GetElapsedMs() int64 {
	if s.ElapsedMs > 0 {
		return s.ElapsedMs
	}
	return s.Elapsed
}

type BanderaSearchItem struct {
	Source      string          `json:"source"`
	Title       string          `json:"title"`
	TitleEn     FlexibleString  `json:"title_en"`
	Poster      FlexibleString  `json:"poster"`
	Type        FlexibleString  `json:"type"`
	Year        json.RawMessage `json:"year"`
	Ref         json.RawMessage `json:"ref"`
	GroupKey    FlexibleString  `json:"group_key"`
	IMDbID      FlexibleString  `json:"imdb_id,omitempty"`
	TMDBID      FlexibleInt     `json:"tmdb_id,omitempty"`
	KinopoiskID FlexibleString  `json:"kinopoisk_id,omitempty"`
	MalID       FlexibleInt     `json:"mal_id,omitempty"`
	Serial      FlexibleInt     `json:"serial,omitempty"`
}

// BanderaItemPayload серіалізується в MediaItem.URL та повертається в клієнт
type BanderaItemPayload struct {
	ID        string          `json:"id,omitempty"` // Стабільний ID (збігається між Search та Details)
	Source    string          `json:"source"`
	Ref       json.RawMessage `json:"ref"`
	Type      string          `json:"type,omitempty"`
	Title     string          `json:"title,omitempty"`
	Poster    string          `json:"poster,omitempty"`
	Year      int             `json:"year,omitempty"`
	IsItemRef bool            `json:"is_item_ref,omitempty"` // Явний маркер, що це контентне посилання
}

// BanderaStreamRef зберігається в Episode.URL або передається в GetStreams
type BanderaStreamRef struct {
	Source      string          `json:"source"`
	Ref         json.RawMessage `json:"ref"`
	IsStreamRef bool            `json:"is_stream_ref,omitempty"`
}

// ---- /content API DTO ----

type BanderaContentRequest struct {
	Source string          `json:"source"`
	Ref    json.RawMessage `json:"ref"`
	Full   bool            `json:"full"`
}

type BanderaContentResponse struct {
	OK       bool                `json:"ok"`
	Error    FlexibleString      `json:"error,omitempty"`
	Source   string              `json:"source"`
	Type     string              `json:"type"`
	Info     *BanderaContentInfo `json:"info"`
	Streams  []BanderaStreamItem `json:"streams"`
	Voices   []BanderaVoice      `json:"voices"`
	Seasons  FlexibleSeasons     `json:"seasons,omitempty"`
	Episodes FlexibleEpisodes    `json:"episodes,omitempty"`
}

type FlexibleGenres []string

func (f *FlexibleGenres) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = nil
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err == nil {
		var clean []string
		for _, s := range list {
			s = strings.TrimSpace(s)
			if s != "" {
				clean = append(clean, s)
			}
		}
		*f = clean
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		var clean []string
		for _, part := range strings.Split(str, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				clean = append(clean, part)
			}
		}
		*f = clean
		return nil
	}
	var objList []struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(b, &objList); err == nil {
		var clean []string
		for _, obj := range objList {
			n := strings.TrimSpace(obj.Name)
			if n == "" {
				n = strings.TrimSpace(obj.Title)
			}
			if n != "" {
				clean = append(clean, n)
			}
		}
		*f = clean
		return nil
	}
	*f = nil
	return nil
}

type BanderaContentInfo struct {
	Title       FlexibleString  `json:"title"`
	TitleEn     FlexibleString  `json:"title_en"`
	Description FlexibleString  `json:"description"`
	Image       FlexibleString  `json:"image"`
	Poster      FlexibleString  `json:"poster,omitempty"`
	ReleaseDate FlexibleString  `json:"release_date"`
	EpisodeTime FlexibleString  `json:"episode_time"`
	Duration    FlexibleString  `json:"duration,omitempty"`
	Trailer     FlexibleString  `json:"trailer"`
	Genres      FlexibleGenres  `json:"genres"`
	Rating      FlexibleFloat   `json:"rating"`
	Year        json.RawMessage `json:"year"`
	Actors      FlexibleGenres  `json:"actors,omitempty"`
	Director    FlexibleString  `json:"director,omitempty"`
	Country     FlexibleString  `json:"country,omitempty"`
}

type BanderaStreamItem struct {
	Title     FlexibleString        `json:"title"`
	URL       FlexibleString        `json:"url"`
	Quality   FlexibleString        `json:"quality"`
	Ref       json.RawMessage       `json:"ref"`
	Subtitles []BanderaSubtitleItem `json:"subtitles"`
}

type BanderaVoice struct {
	ID          FlexibleString   `json:"id"`
	DisplayName FlexibleString   `json:"display_name"`
	Seasons     FlexibleSeasons  `json:"seasons"`
	Episodes    FlexibleEpisodes `json:"episodes"`
}

type BanderaSeason struct {
	Title    json.RawMessage  `json:"title"` // Може бути int або string ("1" або 1)
	Episodes FlexibleEpisodes `json:"episodes"`
}

type FlexibleSeasons []BanderaSeason

// UnmarshalJSON приймає seasons у чотирьох форматах, які трапляються в
// відповідях агрегатора: масив об'єктів, масив чисел, об'єкт-ключ->сезон, або
// скаляр (кількість сезонів без корисного навантаження).
//
// Інваріант вихідного порядку: seasons мають бути ВІДСОРТОВАНІ за номером.
// GetDetails нумерує сезони позиційно (ParseSeasonNumber(s.Title, sIdx+1)), тому
// недетермінований порядок із map-форми означав би, що season 2 може отримати
// номер 1, а запит season=2 — "no matching stream found". Ключі map-форми
// одночасно є номером сезону, тому вони не лише сортуються, а й підставляються
// у Title, коли той відсутній.
func (f *FlexibleSeasons) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = nil
		return nil
	}

	// 1. Масив об'єктів — канонічна форма. Перевіряється першим, бо жодна
	// інша форма не має розмірності масиву об'єктів.
	var seasons []BanderaSeason
	if err := json.Unmarshal(b, &seasons); err == nil {
		*f = normaliseSeasonOrder(seasons)
		return nil
	}

	// 2. Масив чисел [1, 2, 3] — номер сезону без об'єкта.
	var nums []int
	if err := json.Unmarshal(b, &nums); err == nil {
		res := make([]BanderaSeason, 0, len(nums))
		for _, n := range nums {
			res = append(res, BanderaSeason{Title: rawInt(n)})
		}
		*f = normaliseSeasonOrder(res)
		return nil
	}

	// 3. Об'єкт-ключ->сезон. Ключ = номер сезону, значення = сам сезон.
	var seasonMap map[string]json.RawMessage
	if err := json.Unmarshal(b, &seasonMap); err == nil {
		keys := make([]string, 0, len(seasonMap))
		for k := range seasonMap {
			keys = append(keys, k)
		}
		sortNumericStrings(keys)
		res := make([]BanderaSeason, 0, len(seasonMap))
		for _, k := range keys {
			var season BanderaSeason
			if err := json.Unmarshal(seasonMap[k], &season); err != nil {
				// Значення, яке не є об'єктом сезону, все одно дає корисний
				// seasons: номер береться з ключа.
				season = BanderaSeason{}
			}
			if len(season.Title) == 0 || string(season.Title) == "null" {
				season.Title = rawSeasonKey(k)
			}
			res = append(res, season)
		}
		*f = normaliseSeasonOrder(res)
		return nil
	}

	// 4. Скаляр (число або рядок) — агрегатор не віддав жодного сезону.
	// Не помилка: seasons просто немає.
	*f = nil
	return nil
}

// normaliseSeasonOrder сортує сезони за номером, щоб позиційна нумерація в
// GetDetails/SelectEpisodeRef була стабільною між запитами.
func normaliseSeasonOrder(seasons []BanderaSeason) []BanderaSeason {
	if len(seasons) < 2 {
		return seasons
	}
	sort.SliceStable(seasons, func(i, j int) bool {
		return ParseSeasonNumber(seasons[i].Title, i+1) < ParseSeasonNumber(seasons[j].Title, j+1)
	})
	return seasons
}

// rawInt серіалізує число у raw JSON, щоб заповнити Title тим самим типом, який
// приходить від агрегатора (int), а не рядком.
func rawInt(n int) json.RawMessage {
	b, err := json.Marshal(n)
	if err != nil {
		return nil
	}
	return b
}

// rawSeasonKey перетворює ключ об'єкта на raw JSON-номер. Ключ, який не є
// числом, повертається як nil — тоді ParseSeasonNumber коректно відкатиться на
// позиційний індекс.
func rawSeasonKey(k string) json.RawMessage {
	if _, err := strconv.Atoi(strings.TrimSpace(k)); err != nil {
		return nil
	}
	b, err := json.Marshal(strings.TrimSpace(k))
	if err != nil {
		return nil
	}
	return b
}

// sortNumericStrings сортує рядки як числа, коли це можливо ("2" перед "10"),
// а не-числові ключі — лексикографічно після. Порядок стабільний.
func sortNumericStrings(keys []string) {
	sort.SliceStable(keys, func(i, j int) bool {
		a, aErr := strconv.Atoi(strings.TrimSpace(keys[i]))
		b, bErr := strconv.Atoi(strings.TrimSpace(keys[j]))
		switch {
		case aErr == nil && bErr == nil:
			return a < b
		case aErr == nil:
			return true
		case bErr == nil:
			return false
		default:
			return keys[i] < keys[j]
		}
	})
}

type FlexibleEpisodes []BanderaEpisode

func (f *FlexibleEpisodes) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = nil
		return nil
	}
	// 1. Масив об'єктів — канонічна форма.
	var episodes []BanderaEpisode
	if err := json.Unmarshal(b, &episodes); err == nil {
		*f = normaliseEpisodeOrder(episodes)
		return nil
	}

	// 2. Масив чисел [1, 2, 3] — номер епізоду без об'єкта.
	var nums []int
	if err := json.Unmarshal(b, &nums); err == nil {
		res := make([]BanderaEpisode, 0, len(nums))
		for _, n := range nums {
			res = append(res, BanderaEpisode{Number: FlexibleInt(n)})
		}
		*f = normaliseEpisodeOrder(res)
		return nil
	}

	// 3. Об'єкт-ключ->епізод. Ключ = номер епізоду, значення = сам епізод.
	var epMap map[string]json.RawMessage
	if err := json.Unmarshal(b, &epMap); err == nil {
		keys := make([]string, 0, len(epMap))
		for k := range epMap {
			keys = append(keys, k)
		}
		sortNumericStrings(keys)
		res := make([]BanderaEpisode, 0, len(epMap))
		for _, k := range keys {
			var ep BanderaEpisode
			if err := json.Unmarshal(epMap[k], &ep); err != nil {
				// Значення, яке не є об'єктом епізоду, все одно дає корисний
				// результат: номер береться з ключа.
				ep = BanderaEpisode{}
			}
			if ep.Number == 0 {
				if n, convErr := strconv.Atoi(strings.TrimSpace(k)); convErr == nil {
					ep.Number = FlexibleInt(n)
				}
			}
			res = append(res, ep)
		}
		*f = normaliseEpisodeOrder(res)
		return nil
	}

	// 4. Скаляр — епізодів немає, але це не помилка.
	*f = nil
	return nil
}

// normaliseEpisodeOrder сортує епізоди за номером, щоб порядок у details-відповіді
// та в клієнті не залежав від порядку ключів у відповіді агрегатора.
func normaliseEpisodeOrder(episodes []BanderaEpisode) []BanderaEpisode {
	if len(episodes) < 2 {
		return episodes
	}
	sort.SliceStable(episodes, func(i, j int) bool {
		return episodes[i].Number.Int() < episodes[j].Number.Int()
	})
	return episodes
}

type BanderaEpisode struct {
	Number    FlexibleInt           `json:"number"`
	Title     FlexibleString        `json:"title"`
	Ref       json.RawMessage       `json:"ref"`
	Subtitles []BanderaSubtitleItem `json:"subtitles"`
}

type BanderaSubtitleItem struct {
	URL   FlexibleString `json:"url"`
	Lang  FlexibleString `json:"lang"`
	Label FlexibleString `json:"label"`
}

// ---- /stream API DTO ----

type BanderaStreamRequest struct {
	Source string          `json:"source"`
	Ref    json.RawMessage `json:"ref"`
}

type BanderaStreamResponse struct {
	OK        bool                  `json:"ok"`
	Source    string                `json:"source"`
	Streams   []BanderaStreamEntry  `json:"streams"`
	Subtitles []BanderaSubtitleItem `json:"subtitles"`
	ErrorCode FlexibleString        `json:"error_code"`
	Error     FlexibleString        `json:"error"`
}

type BanderaStreamEntry struct {
	URL       FlexibleString        `json:"url"`
	Quality   FlexibleString        `json:"quality"`
	Subtitles []BanderaSubtitleItem `json:"subtitles"`
}
