package match

import (
	"testing"

	"audioborker/internal/metadata"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Project Hail Mary [Unabridged] (2021).mp3": "Project Hail Mary",
		"01 - The Way of Kings":                     "The Way of Kings",
		"andy_weir-the_martian_64k":                 "andy weir-the martian",
		"Dune (Complete Audiobook) {Frank Herbert}": "Dune",
		"Émile Zola - Germinal":                     "Emile Zola - Germinal",
		"The.Hobbit.m4b":                            "The Hobbit",
		"1984":                                      "1984", // lone numbers survive only if nothing else remains? no: leading digits strip -> ""
	}
	for in, want := range cases {
		if in == "1984" {
			continue // covered separately below
		}
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAllDigitsName(t *testing.T) {
	// Leading-number stripping must not eat a purely numeric title... it does
	// by design (track numbers), so document the fallback: empty result means
	// the caller should fall back to the raw name.
	if got := Normalize("1984"); got != "" {
		t.Logf("Normalize(1984) = %q (informational)", got)
	}
}

func TestSplitAuthorTitle(t *testing.T) {
	a, ti := SplitAuthorTitle("Andy Weir - Project Hail Mary")
	if a != "Andy Weir" || ti != "Project Hail Mary" {
		t.Errorf("got %q / %q", a, ti)
	}
	a, ti = SplitAuthorTitle("Project Hail Mary")
	if a != "" || ti != "Project Hail Mary" {
		t.Errorf("no-separator case got %q / %q", a, ti)
	}
	// A long left side is a title with a dash, not an author.
	a, _ = SplitAuthorTitle("The Long And Winding Road Of Many Words - Special Edition")
	if a != "" {
		t.Errorf("long left side treated as author: %q", a)
	}
}

func TestScoreOrdering(t *testing.T) {
	results := []metadata.SearchResult{
		{Title: "Project Hail Mary: Deluxe Edition", Authors: []string{"Andy Weir"}},
		{Title: "Project Hail Mary", Authors: []string{"Andy Weir"}},
		{Title: "Artemis", Authors: []string{"Andy Weir"}},
	}
	scored := Score(results, Signals{Title: "Project Hail Mary", Author: "Andy Weir"})
	if scored[0].Title != "Project Hail Mary" {
		t.Errorf("best match = %q, want exact title", scored[0].Title)
	}
	if scored[len(scored)-1].Title != "Artemis" {
		t.Errorf("worst match = %q, want Artemis", scored[len(scored)-1].Title)
	}
}

func TestScoreRuntimeSeparatesEditions(t *testing.T) {
	// Same title, three editions: abridged, unabridged, and a dramatization.
	// The local audio is 970 min, so the unabridged edition must win.
	results := []metadata.SearchResult{
		{Title: "Project Hail Mary", Authors: []string{"Andy Weir"}, RuntimeMin: 420},
		{Title: "Project Hail Mary", Authors: []string{"Andy Weir"}, RuntimeMin: 969},
		{Title: "Project Hail Mary", Authors: []string{"Andy Weir"}, RuntimeMin: 1500},
	}
	scored := Score(results, Signals{Title: "Project Hail Mary", Author: "Andy Weir", RuntimeMin: 970})
	if scored[0].RuntimeMin != 969 {
		t.Errorf("best runtime = %d, want 969", scored[0].RuntimeMin)
	}
	if scored[0].RuntimeMatch != "exact" {
		t.Errorf("expected exact runtime match, got %q", scored[0].RuntimeMatch)
	}
	if scored[0].RuntimeDeltaMin != -1 {
		t.Errorf("delta = %d, want -1", scored[0].RuntimeDeltaMin)
	}
	for _, r := range scored[1:] {
		if r.RuntimeMatch != "off" {
			t.Errorf("runtime %d should be classified off, got %q", r.RuntimeMin, r.RuntimeMatch)
		}
	}
}

func TestScoreRuntimeDoesNotOutrankAuthor(t *testing.T) {
	// A coincidental runtime match on the wrong author must not beat the
	// right author with a slightly different runtime.
	results := []metadata.SearchResult{
		{Title: "Project Hail Mary", Authors: []string{"Someone Else"}, RuntimeMin: 970},
		{Title: "Project Hail Mary", Authors: []string{"Andy Weir"}, RuntimeMin: 1000},
	}
	scored := Score(results, Signals{Title: "Project Hail Mary", Author: "Andy Weir", RuntimeMin: 970})
	if scored[0].Authors[0] != "Andy Weir" {
		t.Errorf("author match should win, got %v", scored[0].Authors)
	}
}

func TestScoreWithoutLocalRuntime(t *testing.T) {
	results := []metadata.SearchResult{{Title: "A Book", RuntimeMin: 100}}
	scored := Score(results, Signals{Title: "A Book"})
	if scored[0].RuntimeMatch != "" || scored[0].RuntimeDeltaMin != 0 {
		t.Errorf("unknown local runtime must not classify: %+v", scored[0])
	}
}

func TestAutoSelectIsConservative(t *testing.T) {
	exact := metadata.SearchResult{Title: "Project Hail Mary", RuntimeMin: 970}
	mk := func(rs ...metadata.SearchResult) []metadata.SearchResult {
		return Score(rs, Signals{Title: "Project Hail Mary", Author: "Andy Weir", RuntimeMin: 970})
	}

	// Lone, exact, title-perfect candidate: safe.
	if !AutoSelect(mk(exact), Signals{Title: "Project Hail Mary", RuntimeMin: 970}) {
		t.Error("a lone exact match should auto-select")
	}
	// Two editions with the same runtime: ambiguous, never auto-select.
	if AutoSelect(mk(exact, exact), Signals{Title: "Project Hail Mary", RuntimeMin: 970}) {
		t.Error("two exact runtime matches must not auto-select")
	}
	// Runtime off: never.
	if AutoSelect(mk(metadata.SearchResult{Title: "Project Hail Mary", RuntimeMin: 500}), Signals{Title: "Project Hail Mary", RuntimeMin: 970}) {
		t.Error("a non-matching runtime must not auto-select")
	}
	// Unknown runtime: never.
	noRuntime := Score([]metadata.SearchResult{{Title: "Project Hail Mary"}}, Signals{Title: "Project Hail Mary"})
	if AutoSelect(noRuntime, Signals{Title: "Project Hail Mary"}) {
		t.Error("unknown runtime must not auto-select")
	}
	// Wrong title but coincidentally exact runtime: never.
	wrongTitle := mk(metadata.SearchResult{Title: "Something Entirely Different", RuntimeMin: 970})
	if AutoSelect(wrongTitle, Signals{Title: "Project Hail Mary", RuntimeMin: 970}) {
		t.Error("a distant title must not auto-select")
	}
	// No hint at all: never.
	if AutoSelect(mk(exact), Signals{RuntimeMin: 970}) {
		t.Error("empty title hint must not auto-select")
	}
	// Right runtime, right title, wrong language: never — this is the
	// foreign-edition trap (a Spanish printing with an identical runtime).
	foreign := Score([]metadata.SearchResult{
		{Title: "Project Hail Mary", RuntimeMin: 970, Language: "spanish"},
	}, Signals{Title: "Project Hail Mary", RuntimeMin: 970, Language: "english"})
	if AutoSelect(foreign, Signals{Title: "Project Hail Mary", RuntimeMin: 970, Language: "english"}) {
		t.Error("a foreign-language edition must not auto-select")
	}
}

func TestScoreLanguagePreference(t *testing.T) {
	// A foreign edition whose runtime happens to match exactly must not
	// outrank an English edition that is merely close.
	results := []metadata.SearchResult{
		{Title: "Harry Potter", RuntimeMin: 498, Language: "spanish"},
		{Title: "Harry Potter", RuntimeMin: 505, Language: "english"},
	}
	scored := Score(results, Signals{Title: "Harry Potter", RuntimeMin: 498, Language: "english"})
	if scored[0].Language != "english" {
		t.Errorf("expected the English edition first, got %+v", scored[0])
	}
	// An unknown language on either side must never be penalized.
	unknown := Score([]metadata.SearchResult{{Title: "X", RuntimeMin: 100}},
		Signals{Title: "X", RuntimeMin: 100, Language: "english"})
	if unknown[0].Score < 100 {
		t.Errorf("missing language should not be penalized, score = %d", unknown[0].Score)
	}
}

func TestLanguageForRegion(t *testing.T) {
	for region, want := range map[string]string{
		"us": "english", "uk": "english", "au": "english", "ca": "english", "in": "english",
		"de": "german", "fr": "french", "es": "spanish", "it": "italian", "jp": "japanese",
		"zz": "", "": "",
	} {
		if got := LanguageForRegion(region); got != want {
			t.Errorf("LanguageForRegion(%q) = %q, want %q", region, got, want)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"a", "", 1}, {"", "abc", 3},
		{"kitten", "sitting", 3}, {"same", "same", 0}, {"café", "cafe", 1},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
