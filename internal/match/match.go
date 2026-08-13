// Package match turns messy folder/file names into search queries and scores
// candidate results, so the match screen can pre-select the right book.
// Pure functions only — no I/O.
package match

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"audiobookr/internal/metadata"
)

var (
	bracketRe = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|\{[^}]*\}`)
	leadNumRe = regexp.MustCompile(`^\s*\d+[\s.\-_]*`)
	junkWords = regexp.MustCompile(`(?i)\b(official|audiobook|unabridged|abridged|complete|retail|mp3|m4b|m4a|flac|64k|128k|320k|kbps|web|rip)\b`)
	nonWordRe = regexp.MustCompile(`[_.]+`)
	spaceRe   = regexp.MustCompile(`\s{2,}`)
)

// asciiFold maps common Latin diacritics to ASCII so Levenshtein comparisons
// against Audible's mostly-ASCII catalog behave.
var asciiFold = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n", "æ", "ae", "œ", "oe", "ß", "ss",
	"Á", "A", "À", "A", "Â", "A", "Ä", "A", "Ã", "A", "Å", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Ö", "O", "Õ", "O", "Ø", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N", "Æ", "AE", "Œ", "OE",
)

// Normalize cleans a folder or file name into search keywords:
// strips the extension, bracketed blocks, leading track numbers, quality/
// format words, and separator noise.
func Normalize(name string) string {
	s := path.Base(strings.ReplaceAll(name, "\\", "/"))
	if ext := path.Ext(s); len(ext) > 1 && len(ext) <= 6 {
		s = strings.TrimSuffix(s, ext)
	}
	s = bracketRe.ReplaceAllString(s, " ")
	s = nonWordRe.ReplaceAllString(s, " ") // before junk words: "_64k" must become " 64k"
	s = leadNumRe.ReplaceAllString(s, "")
	s = junkWords.ReplaceAllString(s, " ")
	s = asciiFold.Replace(s)
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// SplitAuthorTitle guesses (author, title) from "Author - Title" style names.
// Returns empty author when there's no clear separator.
func SplitAuthorTitle(normalized string) (author, title string) {
	parts := strings.SplitN(normalized, " - ", 2)
	if len(parts) == 2 && len(strings.Fields(parts[0])) <= 4 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", normalized
}

// Score ranks results against the query hint (bragibooks' weighting: title
// distance ×2, author distance ×10, catalog relevance as index penalty) and
// sorts best-first.
func Score(results []metadata.SearchResult, titleHint, authorHint string) []metadata.SearchResult {
	titleHint = strings.ToLower(asciiFold.Replace(titleHint))
	authorHint = strings.ToLower(asciiFold.Replace(authorHint))
	for i := range results {
		r := &results[i]
		score := 100
		if titleHint != "" {
			score -= 2 * fuzzyDistance(titleHint, strings.ToLower(asciiFold.Replace(r.Title)))
		}
		if authorHint != "" && len(r.Authors) > 0 {
			best := 1 << 30
			for _, a := range r.Authors {
				if d := levenshtein(authorHint, strings.ToLower(asciiFold.Replace(a))); d < best {
					best = d
				}
			}
			score -= 10 * best
		}
		score -= i // preserve some of the catalog's relevance ordering
		r.Score = score
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

// fuzzyDistance is Levenshtein softened for subtitles: a candidate that
// contains the whole hint ("Project Hail Mary: Deluxe Edition" for hint
// "project hail mary") is nearly exact — raw edit distance would rank it
// below unrelated short titles.
func fuzzyDistance(hint, candidate string) int {
	d := levenshtein(hint, candidate)
	if len(hint) >= 4 && strings.Contains(candidate, hint) {
		extra := (len(candidate) - len(hint)) / 8
		if extra+1 < d {
			return extra + 1
		}
	}
	return d
}

// levenshtein is the classic two-row edit distance over runes.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}
