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

// Signals are what a candidate is judged against.
type Signals struct {
	Title  string // expected title (from the folder name or user's search)
	Author string // expected author, may be empty
	// RuntimeMin is the measured duration of the files to be converted;
	// 0 when unknown.
	RuntimeMin int
	// Language expected of the edition, e.g. "english". Derived from the
	// search region; empty disables the check.
	Language string
}

// LanguageForRegion maps an Audible storefront to the language its listeners
// normally want, so a foreign-language edition of the right book doesn't
// outrank the edition actually being converted.
func LanguageForRegion(region string) string {
	switch strings.ToLower(region) {
	case "us", "uk", "au", "ca", "in":
		return "english"
	case "de":
		return "german"
	case "fr":
		return "french"
	case "es":
		return "spanish"
	case "it":
		return "italian"
	case "jp":
		return "japanese"
	}
	return ""
}

// Score ranks results against the signals (bragibooks' weighting: title
// distance ×2, author distance ×10, catalog relevance as index penalty),
// adjusts for how closely each candidate's published runtime matches the
// local audio, penalizes the wrong language, and sorts best-first.
//
// Runtime is a strong signal — it separates abridged from unabridged editions
// and dramatizations from straight readings — but it is deliberately weighted
// below the author match, since a coincidental runtime should never outrank
// the right author.
func Score(results []metadata.SearchResult, sig Signals) []metadata.SearchResult {
	titleHint := strings.ToLower(asciiFold.Replace(sig.Title))
	authorHint := strings.ToLower(asciiFold.Replace(sig.Author))
	localMin := sig.RuntimeMin
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
		score += classifyRuntime(r, localMin)
		if !languageMatches(sig.Language, r.Language) {
			score -= 25
		}
		r.Score = score
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results
}

// RuntimeBadge classifies how a published runtime compares to the local
// audio, for display outside candidate scoring (e.g. the row summary).
func RuntimeBadge(officialMin, localMin int) (deltaMin int, class string) {
	r := metadata.SearchResult{RuntimeMin: officialMin}
	classifyRuntime(&r, localMin)
	return r.RuntimeDeltaMin, r.RuntimeMatch
}

// languageMatches is permissive: unknown on either side is not a mismatch,
// so a provider that omits the field never causes a penalty.
func languageMatches(want, got string) bool {
	if want == "" || got == "" {
		return true
	}
	return strings.EqualFold(want, got)
}

// classifyRuntime records how far the candidate's runtime is from the local
// audio and returns the score adjustment.
func classifyRuntime(r *metadata.SearchResult, localMin int) int {
	if localMin <= 0 || r.RuntimeMin <= 0 {
		r.RuntimeMatch, r.RuntimeDeltaMin = "", 0
		return 0
	}
	delta := r.RuntimeMin - localMin
	r.RuntimeDeltaMin = delta
	if delta < 0 {
		delta = -delta
	}
	pct := float64(delta) / float64(localMin)

	switch {
	case delta <= 1 || pct <= 0.005:
		r.RuntimeMatch = "exact"
		return 25
	case pct <= 0.02:
		r.RuntimeMatch = "close"
		return 12
	case pct <= 0.05:
		r.RuntimeMatch = "near"
		return 3
	case pct <= 0.15:
		r.RuntimeMatch = "off"
		return -12
	default:
		// A different edition, an abridgement, or the wrong book entirely.
		r.RuntimeMatch = "off"
		return -30
	}
}

// AutoSelect reports whether the top result is safe to pre-select for the
// user. It is intentionally strict: the runtime must match almost exactly,
// the title must be a near-perfect hit, and the runner-up must be clearly
// worse. Anything less and the user chooses, since a silently wrong
// pre-selection is far more costly than one extra click.
func AutoSelect(results []metadata.SearchResult, sig Signals) bool {
	if len(results) == 0 || sig.Title == "" {
		return false
	}
	best := results[0]
	if best.RuntimeMatch != "exact" {
		return false
	}
	// Never auto-select an edition in a language the user probably can't
	// listen to, however well the runtime happens to line up.
	if sig.Language != "" && !strings.EqualFold(sig.Language, best.Language) {
		return false
	}
	hint := strings.ToLower(asciiFold.Replace(sig.Title))
	if fuzzyDistance(hint, strings.ToLower(asciiFold.Replace(best.Title))) > 2 {
		return false
	}
	// Ambiguous when a rival scores nearly as well (e.g. two editions with
	// the same runtime), or when another candidate also matches exactly.
	if len(results) > 1 {
		if best.Score-results[1].Score < 15 || results[1].RuntimeMatch == "exact" {
			return false
		}
	}
	return true
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
