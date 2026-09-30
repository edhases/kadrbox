package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	Source   string          `json:"source"`
	Title    string          `json:"title"`
	TitleEn  FlexibleString  `json:"title_en"`
	Poster   FlexibleString  `json:"poster"`
	Type     FlexibleString  `json:"type"`
	Year     json.RawMessage `json:"year"`
	Ref      json.RawMessage `json:"ref"`
	GroupKey FlexibleString  `json:"group_key"`
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
	OK      bool                `json:"ok"`
	Source  string              `json:"source"`
	Type    string              `json:"type"`
	Info    *BanderaContentInfo `json:"info"`
	Streams []BanderaStreamItem `json:"streams"`
	Voices  []BanderaVoice      `json:"voices"`
}

type BanderaContentInfo struct {
	Title       FlexibleString  `json:"title"`
	TitleEn     FlexibleString  `json:"title_en"`
	Description FlexibleString  `json:"description"`
	Image       FlexibleString  `json:"image"`
	ReleaseDate FlexibleString  `json:"release_date"`
	EpisodeTime FlexibleString  `json:"episode_time"`
	Trailer     FlexibleString  `json:"trailer"`
	Genres      []string        `json:"genres"`
	Rating      FlexibleFloat   `json:"rating"`
	Year        json.RawMessage `json:"year"`
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
	Seasons     []BanderaSeason  `json:"seasons"`
	Episodes    []BanderaEpisode `json:"episodes"`
}

type BanderaSeason struct {
	Title    json.RawMessage  `json:"title"` // Може бути int або string ("1" або 1)
	Episodes []BanderaEpisode `json:"episodes"`
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
