package pipeline

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"audioborker/internal/metadata"
)

// ResolvedChapters is what actually gets embedded, plus provenance for the
// job record and UI.
type ResolvedChapters struct {
	Source   string             `json:"source"` // "audnexus" | "files" | "existing" | "single"
	Chapters []metadata.Chapter `json:"chapters"`
	Warnings []string           `json:"-"`
}

// runtimeTolerance returns the allowed |provider - actual| runtime delta.
func runtimeTolerance(actualMs int64) int64 {
	onePct := actualMs / 100
	if onePct < 10_000 {
		return 10_000 // at least 10 seconds
	}
	return onePct
}

// Chapter source modes a job can request.
const (
	ChapterModeAuto     = "auto"     // provider if runtime matches → existing → boundaries
	ChapterModeExisting = "existing" // keep the input file's own chapters
	ChapterModeProvider = "provider" // force provider chapters even on runtime mismatch
)

// PlanChapters previews exactly what a conversion would embed, without
// converting: the match screen calls this so the "auto" decision is visible
// before queueing. totalMs is the summed source duration (a close stand-in
// for the merged duration the pipeline validates against).
func PlanChapters(mode string, provider *metadata.ChapterInfo, files []*FileInfo, totalMs int64, bookTitle string) ResolvedChapters {
	if mode == "" {
		mode = ChapterModeAuto
	}
	return resolveChapters(mode, provider, files, totalMs, bookTitle)
}

// resolveChapters decides what chapters to embed. In "auto" mode the
// priority is:
//  1. Provider chapters whose runtime matches the merged audio.
//  2. Chapters already present in a single input file.
//  3. One chapter per input file (cumulative boundaries).
//  4. A single whole-book chapter (single-file input with no data at all).
//
// Multi-file input NEVER produces zero chapters (bragibooks/m4b-merge gap).
func resolveChapters(mode string, provider *metadata.ChapterInfo, files []*FileInfo, actualMs int64, bookTitle string) ResolvedChapters {
	var out ResolvedChapters

	useProvider := mode != ChapterModeExisting && provider != nil && len(provider.Chapters) > 0
	if useProvider {
		delta := provider.RuntimeMs - actualMs
		if delta < 0 {
			delta = -delta
		}
		mismatch := delta > runtimeTolerance(actualMs)
		if !mismatch || mode == ChapterModeProvider {
			out.Source = "audnexus"
			out.Chapters = provider.Chapters
			if mismatch {
				out.Warnings = append(out.Warnings, fmt.Sprintf(
					"Provider chapters were forced but expect a runtime of %s vs the actual %s — timings may be misaligned.",
					fmtDuration(provider.RuntimeMs), fmtDuration(actualMs)))
			}
			if !provider.IsAccurate {
				out.Warnings = append(out.Warnings,
					"The chapter source flags these timings as possibly inaccurate (isAccurate=false).")
			}
			return out
		}
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"Provider chapters expect a runtime of %s but the merged audio is %s — fell back to file-based chapters. (Different edition or incomplete rip?)",
			fmtDuration(provider.RuntimeMs), fmtDuration(actualMs)))
	}
	if mode == ChapterModeProvider && !useProvider {
		out.Warnings = append(out.Warnings,
			"Provider chapters were requested but none were available — used file-based chapters instead.")
	}

	// Single input file that already carries chapters: keep them.
	if len(files) == 1 && len(files[0].Chapters) > 0 {
		out.Source = "existing"
		for _, c := range files[0].Chapters {
			out.Chapters = append(out.Chapters, metadata.Chapter{
				Title: c.Title, StartMs: c.StartMs, LengthMs: c.EndMs - c.StartMs,
			})
		}
		return out
	}

	if len(files) > 1 {
		out.Source = "files"
		out.Chapters = chaptersFromBoundaries(files)
		return out
	}

	// Single file, no chapters anywhere: one chapter spanning the book.
	title := bookTitle
	if title == "" {
		title = "Chapter 01"
	}
	out.Source = "single"
	out.Chapters = []metadata.Chapter{{Title: title, StartMs: 0, LengthMs: actualMs}}
	return out
}

// chaptersFromBoundaries builds one chapter per input file from cumulative
// durations. Titles come from cleaned filenames when they carry signal,
// otherwise "Chapter NN".
func chaptersFromBoundaries(files []*FileInfo) []metadata.Chapter {
	titles := filenameTitles(files)
	var chapters []metadata.Chapter
	var offset int64
	for i, f := range files {
		chapters = append(chapters, metadata.Chapter{
			Title:    titles[i],
			StartMs:  offset,
			LengthMs: f.DurationMs,
		})
		offset += f.DurationMs
	}
	return chapters
}

var trackNumPrefix = regexp.MustCompile(`(?i)^\s*(?:track|chapter|part|ch|pt)?[\s._-]*\d+[\s._-]*`)

// filenameTitles derives chapter titles from filenames. If cleaning leaves
// nothing meaningful (pure track numbers) or every name collapses to the
// same string, fall back to "Chapter NN".
func filenameTitles(files []*FileInfo) []string {
	titles := make([]string, len(files))
	distinct := map[string]bool{}
	useful := true
	for i, f := range files {
		base := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
		clean := trackNumPrefix.ReplaceAllString(base, "")
		clean = strings.Trim(strings.ReplaceAll(clean, "_", " "), " -–.")
		if len([]rune(clean)) < 3 || !containsLetter(clean) {
			useful = false
			break
		}
		titles[i] = clean
		distinct[clean] = true
	}
	if !useful || len(distinct) < len(files) {
		for i := range titles {
			titles[i] = fmt.Sprintf("Chapter %02d", i+1)
		}
	}
	return titles
}

func containsLetter(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 127 {
			return true
		}
	}
	return false
}

// chaptersTxt renders the classic m4b-tool/mp4chaps sidecar format:
// "HH:MM:SS.mmm Title" per line. tone imports the same format.
func chaptersTxt(chapters []metadata.Chapter) string {
	var b strings.Builder
	var end int64
	for _, c := range chapters {
		fmt.Fprintf(&b, "%s %s\n", msToTimestamp(c.StartMs), strings.TrimSpace(c.Title))
		if e := c.StartMs + c.LengthMs; e > end {
			end = e
		}
	}
	if end > 0 {
		fmt.Fprintf(&b, "## total-duration: %s\n", msToTimestamp(end))
	}
	return b.String()
}

func msToTimestamp(ms int64) string {
	h := ms / 3_600_000
	m := ms % 3_600_000 / 60_000
	s := ms % 60_000 / 1000
	frac := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, frac)
}

func fmtDuration(ms int64) string {
	h := ms / 3_600_000
	m := ms % 3_600_000 / 60_000
	return fmt.Sprintf("%dh%02dm", h, m)
}

func (r ResolvedChapters) json() string {
	raw, _ := json.Marshal(r)
	return string(raw)
}
