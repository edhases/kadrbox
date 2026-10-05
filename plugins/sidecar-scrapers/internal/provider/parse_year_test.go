package provider

import "testing"

func TestCovParseYear(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{input: "1999", want: 1999},
		{input: "2000 рік", want: 2000},
		{input: "Movie (1984)", want: 1984},
		{input: "без року", want: 0},
		{input: "", want: 0},
		{input: "1899", want: 0},
		{input: "20245", want: 0},
	}

	for _, tc := range cases {
		if got := parseYear(tc.input); got != tc.want {
			t.Errorf("parseYear(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
