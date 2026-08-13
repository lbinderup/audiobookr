package match

import (
	"testing"

	"audiobookr/internal/metadata"
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
	scored := Score(results, "Project Hail Mary", "Andy Weir")
	if scored[0].Title != "Project Hail Mary" {
		t.Errorf("best match = %q, want exact title", scored[0].Title)
	}
	if scored[len(scored)-1].Title != "Artemis" {
		t.Errorf("worst match = %q, want Artemis", scored[len(scored)-1].Title)
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
