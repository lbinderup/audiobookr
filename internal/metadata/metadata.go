// Package metadata defines the provider abstraction for audiobook metadata.
// Audnexus is the shipped implementation; the interface exists so
// AudiobookDB (its announced successor) can slot in once its API launches.
package metadata

import "context"

// SearchQuery is a title/author search against the provider's catalog.
type SearchQuery struct {
	Keywords string // free-text; usually derived from the folder name
	Title    string
	Author   string
	Region   string
}

// SearchResult is one candidate match shown on the match screen.
type SearchResult struct {
	ASIN       string
	Region     string
	Title      string
	Subtitle   string
	Authors    []string
	Narrators  []string
	Year       string
	Language   string
	RuntimeMin int
	CoverURL   string
	Score      int // match confidence, higher is better (set by internal/match)
}

// Book is the full metadata snapshot used for tagging.
type Book struct {
	ASIN           string   `json:"asin"`
	Region         string   `json:"region"`
	Title          string   `json:"title"`
	Subtitle       string   `json:"subtitle"`
	Authors        []string `json:"authors"`
	Narrators      []string `json:"narrators"`
	SeriesName     string   `json:"series_name"`
	SeriesPosition string   `json:"series_position"`
	Year           string   `json:"year"`
	ReleaseDate    string   `json:"release_date"` // YYYY-MM-DD when known
	Publisher      string   `json:"publisher"`
	Language       string   `json:"language"`
	Genres         []string `json:"genres"`
	// Description is the provider's short teaser (Audnexus truncates it to
	// ~250 chars ending in "..."); Summary is the full publisher blurb,
	// converted to plain text. Prefer Summary for tagging — see Book.Blurb.
	Description string `json:"description"`
	Summary     string `json:"summary"`
	RuntimeMin  int    `json:"runtime_min"`
	CoverURL    string `json:"cover_url"`
}

// Chapter offsets are relative to the start of the complete audiobook.
type Chapter struct {
	Title    string `json:"title"`
	StartMs  int64  `json:"start_ms"`
	LengthMs int64  `json:"length_ms"`
}

// ChapterInfo is the provider's chapter listing for one edition.
type ChapterInfo struct {
	ASIN         string    `json:"asin"`
	RuntimeMs    int64     `json:"runtime_ms"`
	IsAccurate   bool      `json:"is_accurate"`
	BrandIntroMs int64     `json:"brand_intro_ms"`
	BrandOutroMs int64     `json:"brand_outro_ms"`
	Chapters     []Chapter `json:"chapters"`
}

// Blurb returns the fullest description text available: the complete
// publisher summary when the provider supplied one, otherwise the short
// teaser. This is what gets embedded in the file.
func (b Book) Blurb() string {
	if b.Summary != "" {
		return b.Summary
	}
	return b.Description
}

type Provider interface {
	// Search returns candidate matches. Implementations may consult a
	// different backend than book lookups (Audnexus has no search, so the
	// audnexus provider searches Audible's public catalog).
	Search(ctx context.Context, q SearchQuery) ([]SearchResult, error)
	// GetBook fetches the full metadata for an ASIN. Returns ErrNotFound if
	// the ASIN does not exist in the given region.
	GetBook(ctx context.Context, asin, region string) (*Book, error)
	// GetChapters fetches chapter data; may return ErrNotFound when the
	// provider has none for this edition.
	GetChapters(ctx context.Context, asin, region string) (*ChapterInfo, error)
}

// ErrNotFound is returned for unknown ASINs (including wrong-region lookups).
type notFoundError struct{ msg string }

func (e notFoundError) Error() string { return e.msg }

func NotFound(msg string) error { return notFoundError{msg} }

func IsNotFound(err error) bool {
	_, ok := err.(notFoundError)
	return ok
}
