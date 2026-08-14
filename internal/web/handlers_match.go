package web

import (
	"context"
	"net/http"
	"path"
	"time"

	"audioborker/internal/match"
	"audioborker/internal/metadata"
	"audioborker/internal/metadata/audnexus"
	"audioborker/internal/scan"
	"audioborker/internal/store"
)

type matchItem struct {
	Index    int
	RelPath  string
	Files    int
	Err      string
	Keywords string // pre-computed search guess from the folder name
	Title    string
	Author   string
}

type matchData struct {
	baseData
	Items          []matchItem
	Regions        []string
	Region         string
	CleanupDefault string
}

// handleMatch receives the import selection and renders the match screen;
// each item then auto-loads its candidates via htmx.
func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	paths := scan.DedupeSelection(r.PostForm["paths"])
	if len(paths) == 0 {
		http.Redirect(w, r, "/import", http.StatusSeeOther)
		return
	}
	set := s.settings()
	data := matchData{
		baseData:       s.base("Match", "import"),
		Regions:        store.Regions,
		Region:         set.RegionDefault,
		CleanupDefault: set.CleanupMode,
	}
	for i, p := range paths {
		item := matchItem{Index: i, RelPath: p}
		files, err := scan.CollectAudioFiles(set.InputDir, p)
		if err != nil {
			item.Err = err.Error()
		} else {
			item.Files = len(files)
		}
		normalized := match.Normalize(path.Base(p))
		if normalized == "" {
			normalized = path.Base(p) // all-digits names like "1984"
		}
		item.Keywords = normalized
		item.Author, item.Title = match.SplitAuthorTitle(normalized)
		data.Items = append(data.Items, item)
	}
	s.render.render(w, "match", data)
}

type candidatesData struct {
	RelPath string
	Region  string
	Results []metadata.SearchResult
	Err     string

	// LocalRuntimeMin is the measured length of the selected audio (0 when
	// unknown); AutoSelect marks the top result as safe to pre-tick.
	LocalRuntimeMin int
	AutoSelect      bool
}

// handleMatchCandidates searches the catalog and renders candidate cards.
func (s *Server) handleMatchCandidates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	set := s.settings()
	region := q.Get("region")
	if region == "" {
		region = set.RegionDefault
	}
	data := candidatesData{RelPath: q.Get("path"), Region: region}

	query := metadata.SearchQuery{
		Keywords: q.Get("q"),
		Title:    q.Get("title"),
		Author:   q.Get("author"),
		Region:   region,
	}
	if query.Keywords == "" && query.Title == "" && query.Author == "" {
		data.Err = "Enter a title, author or keywords to search."
		s.render.partial(w, "match", "candidates", data)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Measure the actual audio so runtime can inform the ranking. Unknown
	// (0) simply falls back to name-only matching.
	data.LocalRuntimeMin = s.LocalRuntimeMin(ctx, data.RelPath)

	results, err := s.provider().Search(ctx, query)
	if err != nil {
		data.Err = "Search failed: " + err.Error()
	} else {
		titleHint := query.Title
		if titleHint == "" {
			titleHint = query.Keywords
		}
		signals := match.Signals{
			Title:      titleHint,
			Author:     query.Author,
			RuntimeMin: data.LocalRuntimeMin,
			Language:   match.LanguageForRegion(region),
		}
		results = match.Score(results, signals)
		data.AutoSelect = match.AutoSelect(results, signals)
		if len(results) > 8 {
			results = results[:8]
		}
		data.Results = results
	}
	s.render.partial(w, "match", "candidates", data)
}

type bookCardData struct {
	RelPath string
	Book    *metadata.Book
	Err     string
}

// handleMatchLookup validates a manually entered ASIN against the provider
// and renders a confirmed book card.
func (s *Server) handleMatchLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	set := s.settings()
	region := q.Get("region")
	if region == "" {
		region = set.RegionDefault
	}
	asin := q.Get("asin")
	data := bookCardData{RelPath: q.Get("path")}

	if !audnexus.ValidASIN(asin) {
		data.Err = "An ASIN is 10 letters/digits, e.g. B08G9PRS1K."
		s.render.partial(w, "match", "book_card", data)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	book, err := s.provider().GetBook(ctx, asin, region)
	if err != nil {
		if metadata.IsNotFound(err) {
			data.Err = "No book with ASIN " + asin + " in region " + region + "."
		} else {
			data.Err = "Lookup failed: " + err.Error()
		}
	} else {
		data.Book = book
	}
	s.render.partial(w, "match", "book_card", data)
}
