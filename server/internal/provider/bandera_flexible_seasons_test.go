package provider_test

import (
	"encoding/json"
	"testing"

	"github.com/edhases/oxide-server/internal/provider"
)

// The two unmarshalers below exist because the aggregator is inconsistent about
// the shape of `seasons`/`episodes`. That leniency is fine; the ORDER of the
// result is not, because GetDetails numbers seasons positionally
// (ParseSeasonNumber(s.Title, sIdx+1)) and SelectEpisodeRef looks an episode up
// by number. A non-deterministic order out of the map form therefore meant that
// season 2 could be served as season 1, and a request for season=2 could fail
// with "no matching stream found" on a title that clearly has it.

func TestFlexibleSeasons_AcceptsEveryUpstreamShape(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantLen  int
		wantTits []string // raw JSON title of each season, in output order
	}{
		{
			name:     "array of objects",
			payload:  `[{"title":1,"episodes":[{"number":1}]},{"title":2,"episodes":[{"number":1}]}]`,
			wantLen:  2,
			wantTits: []string{`1`, `2`},
		},
		{
			name:     "array of numbers",
			payload:  `[1,2,3]`,
			wantLen:  3,
			wantTits: []string{`1`, `2`, `3`},
		},
		{
			name:     "object keyed by season number",
			payload:  `{"1":{"episodes":[{"number":1}]},"2":{"episodes":[{"number":1}]}}`,
			wantLen:  2,
			wantTits: []string{`"1"`, `"2"`},
		},
		{
			name:     "scalar count carries no seasons",
			payload:  `3`,
			wantLen:  0,
			wantTits: nil,
		},
		{
			name:     "empty array",
			payload:  `[]`,
			wantLen:  0,
			wantTits: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got provider.FlexibleSeasons
			if err := json.Unmarshal([]byte(tc.payload), &got); err != nil {
				t.Fatalf("Unmarshal(%s) returned error: %v", tc.payload, err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("expected %d seasons, got %d (%+v)", tc.wantLen, len(got), got)
			}
			for i, want := range tc.wantTits {
				if string(got[i].Title) != want {
					t.Errorf("season %d: expected title %s, got %s", i, want, string(got[i].Title))
				}
			}
		})
	}
}

func TestFlexibleSeasons_NullAndEmptyBytesAreNotErrors(t *testing.T) {
	// The aggregator sends these on titles with no season list at all. A
	// non-nil error here would turn "this is a movie" into a failed lookup.
	//
	// The empty-slice case calls UnmarshalJSON directly: encoding/json rejects
	// empty input before it ever dispatches to a custom unmarshaler, so
	// json.Unmarshal would report "unexpected end of JSON input" regardless of
	// what this method does. Same convention as the sibling Flexible* tests.
	var nullSeasons provider.FlexibleSeasons
	if err := json.Unmarshal([]byte(`null`), &nullSeasons); err != nil {
		t.Errorf("Unmarshal(null) returned error: %v", err)
	}
	if nullSeasons != nil {
		t.Errorf("Unmarshal(null) expected nil, got %+v", nullSeasons)
	}

	var emptySeasons provider.FlexibleSeasons
	if err := emptySeasons.UnmarshalJSON([]byte{}); err != nil || emptySeasons != nil {
		t.Errorf("expected nil and no error on empty bytes, got %+v, err=%v", emptySeasons, err)
	}
}

// This is the regression test for the real defect: the map form used to be
// iterated with `for _, s := range seasonMap`, whose order Go randomises per
// process. The test runs the same payload many times; under the old code it
// failed intermittently, and a single pass could easily have passed.
func TestFlexibleSeasons_MapFormIsOrderedAndNumbered(t *testing.T) {
	// 10 seasons, written in an order that is neither ascending nor descending,
	// with no `title` field so the key is the only source of the season number.
	payload := `{
		"10":{"episodes":[]},
		"2":{"episodes":[]},
		"1":{"episodes":[]},
		"7":{"episodes":[]},
		"3":{"episodes":[]}
	}`

	for attempt := 0; attempt < 200; attempt++ {
		var got provider.FlexibleSeasons
		if err := json.Unmarshal([]byte(payload), &got); err != nil {
			t.Fatalf("Unmarshal returned error: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("expected 5 seasons, got %d", len(got))
		}
		for i, want := range []string{`"1"`, `"2"`, `"3"`, `"7"`, `"10"`} {
			if string(got[i].Title) != want {
				t.Fatalf("attempt %d: season %d expected title %s, got %s (order is not stable)",
					attempt, i, want, string(got[i].Title))
			}
		}
		// The number GetDetails will actually use must follow from the title,
		// which is the whole point: ParseSeasonNumber reads Title first.
		for i, wantNum := range []int{1, 2, 3, 7, 10} {
			if got := provider.ParseSeasonNumber(got[i].Title, i+1); got != wantNum {
				t.Fatalf("attempt %d: season %d parses to %d, expected %d", attempt, i, got, wantNum)
			}
		}
	}
}

func TestFlexibleEpisodes_AcceptsEveryUpstreamShape(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantLen   int
		wantNums  []int
		wantTitle string
	}{
		{
			name:     "array of objects",
			payload:  `[{"number":1,"title":"Пілот"},{"number":2,"title":"Хвиля"}]`,
			wantLen:  2,
			wantNums: []int{1, 2},
		},
		{
			name:     "array of numbers",
			payload:  `[1,2,3]`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
		},
		{
			name:     "object keyed by episode number",
			payload:  `{"2":{"title":"Хвиля"},"1":{"title":"Пілот"}}`,
			wantLen:  2,
			wantNums: []int{1, 2},
		},
		{
			name:      "object value without number takes it from the key",
			payload:   `{"5":{"title":"Фінал"}}`,
			wantLen:   1,
			wantNums:  []int{5},
			wantTitle: "Фінал",
		},
		{
			name:     "scalar count carries no episodes",
			payload:  `12`,
			wantLen:  0,
			wantNums: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got provider.FlexibleEpisodes
			if err := json.Unmarshal([]byte(tc.payload), &got); err != nil {
				t.Fatalf("Unmarshal(%s) returned error: %v", tc.payload, err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("expected %d episodes, got %d (%+v)", tc.wantLen, len(got), got)
			}
			for i, want := range tc.wantNums {
				if got[i].Number.Int() != want {
					t.Errorf("episode %d: expected number %d, got %d", i, want, got[i].Number.Int())
				}
			}
			if tc.wantTitle != "" && got[0].Title.String() != tc.wantTitle {
				t.Errorf("expected title %q, got %q", tc.wantTitle, got[0].Title.String())
			}
		})
	}
}

func TestFlexibleEpisodes_MapFormIsOrdered(t *testing.T) {
	payload := `{
		"10":{"title":"Фінал"},
		"2":{"title":"Хвиля"},
		"1":{"title":"Пілот"},
		"3":{"title":"Тиша"}
	}`

	for attempt := 0; attempt < 200; attempt++ {
		var got provider.FlexibleEpisodes
		if err := json.Unmarshal([]byte(payload), &got); err != nil {
			t.Fatalf("Unmarshal returned error: %v", err)
		}
		for i, want := range []int{1, 2, 3, 10} {
			if got[i].Number.Int() != want {
				t.Fatalf("attempt %d: episode %d expected number %d, got %d (order is not stable)",
					attempt, i, want, got[i].Number.Int())
			}
		}
	}
}

func TestFlexibleEpisodes_NullAndEmptyBytesAreNotErrors(t *testing.T) {
	// See TestFlexibleSeasons_NullAndEmptyBytesAreNotErrors for why the empty
	// slice goes through UnmarshalJSON directly.
	var nullEpisodes provider.FlexibleEpisodes
	if err := json.Unmarshal([]byte(`null`), &nullEpisodes); err != nil {
		t.Errorf("Unmarshal(null) returned error: %v", err)
	}
	if nullEpisodes != nil {
		t.Errorf("Unmarshal(null) expected nil, got %+v", nullEpisodes)
	}

	var emptyEpisodes provider.FlexibleEpisodes
	if err := emptyEpisodes.UnmarshalJSON([]byte{}); err != nil || emptyEpisodes != nil {
		t.Errorf("expected nil and no error on empty bytes, got %+v, err=%v", emptyEpisodes, err)
	}
}
