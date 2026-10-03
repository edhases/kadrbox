package provider

// parseYear used to compile `\b(19\d\d|20\d\d)\b` on EVERY call, and it is called
// once per card on every catalogue page (uakino/lavakino/eneyida) plus several
// times per details page. These tests pin the semantics after the hoist to the
// already-compiled package-level reYearPattern: the result must be identical to a
// locally compiled copy of the SAME pattern for every input, and safe to call from
// many goroutines at once (regexp.Regexp is concurrency-safe, MustCompile is not
// needed per call).

import (
	"regexp"
	"strconv"
	"sync"
	"testing"
)

// referenceParseYear is the pre-hoist implementation, verbatim.
func referenceParseYear(text string) int {
	re := regexp.MustCompile(`\b(19\d\d|20\d\d)\b`)
	match := re.FindString(text)
	if match != "" {
		if yr, err := strconv.Atoi(match); err == nil {
			return yr
		}
	}
	return 0
}

func TestPerfParseYearMatchesPreviousImplementation(t *testing.T) {
	cases := []string{
		"1999",
		"2000 рік",
		"Movie (1984)",
		"без року",
		"",
		"1899",
		"20245",
		"  2024  ",
		"1917-2019",
		"Release: 2099",
		"1 999",
		"movie_2012_part2",
		"../../../2007/film",
		"19999",
		"92019",
		"1900",
		"2100",
		"abc2000def",
	}

	for _, in := range cases {
		if got, want := parseYear(in), referenceParseYear(in); got != want {
			t.Errorf("parseYear(%q) = %d, previous implementation gave %d", in, got, want)
		}
	}
}

// TestPerfParseYearConcurrent hammers the hoisted regexp from many goroutines; a
// per-call MustCompile would show up here as heavy garbage even where it cannot
// tear (MustCompile returns a fresh value), and this is the call shape the
// catalogue parsers actually use.
func TestPerfParseYearConcurrent(t *testing.T) {
	const (
		goroutines = 16
		calls      = 500
	)
	inputs := []string{"1999", "без року", "Movie (2021)", "20245", "серіал 2016 рік"}

	var wg sync.WaitGroup
	failures := make(chan string, goroutines*calls)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < calls; i++ {
				in := inputs[(g+i)%len(inputs)]
				if got, want := parseYear(in), referenceParseYear(in); got != want {
					failures <- in + ": got " + strconv.Itoa(got) + " want " + strconv.Itoa(want)
				}
			}
		}(g)
	}
	wg.Wait()
	close(failures)

	for msg := range failures {
		t.Error(msg)
	}
}

// TestPerfReRatingNumIsShared documents that the rating regexp is one package-level
// value shared by uakino and eneyida instead of one MustCompile per GetDetails call.
func TestPerfReRatingNumIsShared(t *testing.T) {
	want := regexp.MustCompile(`([\d.]+)`)
	for _, in := range []string{"8.5", "IMDB: 7.9/10", "", "—", "10"} {
		if got, expect := reRatingNum.FindString(in), want.FindString(in); got != expect {
			t.Errorf("reRatingNum.FindString(%q) = %q, want %q", in, got, expect)
		}
	}
}
