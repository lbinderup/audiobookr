package embedded

import (
	"reflect"
	"testing"
)

// ourTags is the ffprobe tag set of a real audioborker-produced m4b
// (The Colour of Magic). Captured from disk — re-capture rather than hand-edit.
var ourTags = map[string]string{
	"AUDIBLE_ASIN":      "B09LYZPFLZ",
	"PART":              "1",
	"SERIES":            "Discworld",
	"SUBTITLE":          "Discworld, Book 1",
	"album":             "The Colour of Magic",
	"album_artist":      "Terry Pratchett",
	"artist":            "Terry Pratchett",
	"comment":           "Over 1 million Discworld audiobooks sold",
	"composer":          "Colin Morgan, Peter Serafinowicz, Bill Nighy",
	"date":              "2022-07-07",
	"description":       "Over 1 million Discworld audiobooks sold",
	"genre":             "Science Fiction & Fantasy",
	"synopsis":          "Over 1 million Discworld audiobooks sold — discover the extraordinary universe",
	"title":             "The Colour of Magic",
	"sort_album":        "Discworld 1 - The Colour of Magic",
	"major_brand":       "M4A ",
	"compatible_brands": "M4A isomiso2",
}

// foreignTags is the tag set of a real m4b produced by another tool: the ASIN
// is under a bare key and the series position under SERIES-PART.
var foreignTags = map[string]string{
	"ASIN":         "B09LZ2HGQN",
	"EXPLICIT":     "0",
	"FORMAT":       "unabridged",
	"ISBN":         "9781473588356",
	"LANGUAGE":     "english",
	"RATING":       "4.8",
	"RELEASETIME":  "2022-04-28",
	"SERIES":       "Discworld",
	"SERIES-PART":  "13",
	"SUBTITLE":     "Discworld, Book 13",
	"WWWAUDIOFILE": "https://www.audible.com/pd/B09LZ2HGQN",
	"album":        "Small Gods",
	"album_artist": "Terry Pratchett",
	"artist":       "Terry Pratchett",
	"composer":     "Andy Serkis, Bill Nighy, Peter Serafinowicz",
	"date":         "2022",
	"genre":        "Science Fiction & Fantasy",
	"grouping":     "Discworld, Book #13",
}

func TestBookFromOurTags(t *testing.T) {
	b := Book(ourTags)
	if b.Title != "The Colour of Magic" || b.Subtitle != "Discworld, Book 1" {
		t.Errorf("title/subtitle: %+v", b)
	}
	if b.ASIN != "B09LYZPFLZ" {
		t.Errorf("asin = %q", b.ASIN)
	}
	if b.SeriesName != "Discworld" || b.SeriesPosition != "1" {
		t.Errorf("series = %q #%q", b.SeriesName, b.SeriesPosition)
	}
	if !reflect.DeepEqual(b.Authors, []string{"Terry Pratchett"}) {
		t.Errorf("authors = %v", b.Authors)
	}
	// Narrators come back from composer: ffprobe does not surface tone's ©nrt.
	want := []string{"Colin Morgan", "Peter Serafinowicz", "Bill Nighy"}
	if !reflect.DeepEqual(b.Narrators, want) {
		t.Errorf("narrators = %v, want %v", b.Narrators, want)
	}
	if b.Year != "2022" || b.ReleaseDate != "2022-07-07" {
		t.Errorf("date: year=%q release=%q", b.Year, b.ReleaseDate)
	}
	// synopsis is the longest blurb candidate, so it wins over comment.
	if b.Summary != ourTags["synopsis"] {
		t.Errorf("summary = %q", b.Summary)
	}
	if len(b.Genres) != 1 || b.Genres[0] != "Science Fiction & Fantasy" {
		t.Errorf("genres = %v", b.Genres)
	}
}

func TestBookFromForeignTags(t *testing.T) {
	b := Book(foreignTags)
	if b.ASIN != "B09LZ2HGQN" {
		t.Errorf("bare ASIN key not read: %q", b.ASIN)
	}
	if b.Title != "Small Gods" {
		t.Errorf("title should fall back to album: %q", b.Title)
	}
	// SERIES-PART normalizes to series_part.
	if b.SeriesName != "Discworld" || b.SeriesPosition != "13" {
		t.Errorf("series = %q #%q", b.SeriesName, b.SeriesPosition)
	}
	if b.Language != "english" {
		t.Errorf("language = %q", b.Language)
	}
	// A bare year has no full release date.
	if b.Year != "2022" || b.ReleaseDate != "" {
		t.Errorf("year=%q release=%q", b.Year, b.ReleaseDate)
	}
	if len(b.Narrators) != 3 || b.Narrators[0] != "Andy Serkis" {
		t.Errorf("narrators = %v", b.Narrators)
	}
}

func TestBookFromEmptyTags(t *testing.T) {
	b := Book(nil)
	if b.Title != "" || b.ASIN != "" || len(b.Authors) != 0 || b.Year != "" {
		t.Errorf("empty tags should yield an empty book: %+v", b)
	}
}

func TestASINSpellingsAndValidation(t *testing.T) {
	cases := []struct {
		name string
		tags map[string]string
		want string
	}{
		{"ours", map[string]string{"AUDIBLE_ASIN": "B09LYZPFLZ"}, "B09LYZPFLZ"},
		{"bare", map[string]string{"ASIN": "B09LZ2HGQN"}, "B09LZ2HGQN"},
		{"lowercase key", map[string]string{"audible_asin": "b09lyzpflz"}, "B09LYZPFLZ"},
		{"namespaced freeform", map[string]string{"----:com.pilabor.tone:AUDIBLE_ASIN": "B09LYZPFLZ"}, "B09LYZPFLZ"},
		{"unknown key ending in asin", map[string]string{"com.foo.myASIN": "B09LYZPFLZ"}, "B09LYZPFLZ"},
		{"too short", map[string]string{"ASIN": "B09LYZPFL"}, ""},
		{"too long", map[string]string{"ASIN": "B09LYZPFLZ1"}, ""},
		{"punctuated", map[string]string{"ASIN": "B09-YZPFLZ"}, ""},
		{"free text", map[string]string{"ASIN": "see the website"}, ""},
		{"blank", map[string]string{"ASIN": "   "}, ""},
		{"none", map[string]string{"album": "x"}, ""},
		// AUDIBLE_ASIN is preferred over a bare ASIN when both are present.
		{"prefers audible_asin", map[string]string{"ASIN": "1111111111", "AUDIBLE_ASIN": "B09LYZPFLZ"}, "B09LYZPFLZ"},
	}
	for _, c := range cases {
		if got := ASIN(c.tags); got != c.want {
			t.Errorf("%s: ASIN = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestASINIsDeterministic(t *testing.T) {
	// Two unknown keys both holding valid-shaped ASINs: map iteration order is
	// random, so the sorted scan must always pick the same one.
	tags := map[string]string{"zzz_asin": "B000000002", "aaa_asin": "B000000001"}
	first := ASIN(tags)
	for i := 0; i < 50; i++ {
		if got := ASIN(tags); got != first {
			t.Fatalf("nondeterministic: got %q then %q", first, got)
		}
	}
	if first != "B000000001" {
		t.Errorf("expected the lexicographically first key to win, got %q", first)
	}
}

func TestSplitPeople(t *testing.T) {
	cases := map[string][]string{
		"":                      nil,
		"Terry Pratchett":       {"Terry Pratchett"},
		"A, B, C":               {"A", "B", "C"},
		"A; B":                  {"A", "B"},
		"Neil Gaiman & Terry P": {"Neil Gaiman", "Terry P"},
		"  Padded  ":            {"Padded"},
	}
	for in, want := range cases {
		if got := splitPeople(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitPeople(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNormalizeFoldsSpellings(t *testing.T) {
	n := normalize(map[string]string{
		"----:com.pilabor.tone:AUDIBLE_ASIN": "B09LYZPFLZ",
		"SERIES-PART":                        "13",
		"  Title  ":                          "T",
		"blank":                              "   ",
	})
	if n["audible_asin"] != "B09LYZPFLZ" {
		t.Errorf("namespace prefix not stripped: %v", n)
	}
	if n["series_part"] != "13" {
		t.Errorf("dash not folded to underscore: %v", n)
	}
	if n["title"] != "T" {
		t.Errorf("key not trimmed/lowered: %v", n)
	}
	if _, ok := n["blank"]; ok {
		t.Errorf("blank value should be dropped: %v", n)
	}
}
