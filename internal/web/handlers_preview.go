package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"audiobookr/internal/metadata"
	"audiobookr/internal/pipeline"
	"audiobookr/internal/scan"
)

// audioContentTypes maps extensions the browser can typically play natively.
// wma is deliberately absent — no browser decodes it.
var audioContentTypes = map[string]string{
	".m4b": "audio/mp4", ".m4a": "audio/mp4", ".aac": "audio/aac",
	".mp3": "audio/mpeg", ".flac": "audio/flac",
	".ogg": "audio/ogg", ".opus": "audio/ogg", ".wav": "audio/wav",
}

// handlePreviewAudio streams one input file. http.ServeContent handles HTTP
// Range requests, so browser <audio> seeking works — including remotely
// through a reverse proxy.
func (s *Server) handlePreviewAudio(w http.ResponseWriter, r *http.Request) {
	set := s.settings()
	abs, err := scan.Resolve(set.InputDir, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() || !scan.IsAudioFile(abs) {
		http.NotFound(w, r)
		return
	}
	ct, ok := audioContentTypes[strings.ToLower(filepath.Ext(abs))]
	if !ok {
		http.Error(w, "this format cannot be played in a browser", http.StatusUnsupportedMediaType)
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		http.Error(w, "cannot open file", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ct)
	http.ServeContent(w, r, filepath.Base(abs), info.ModTime(), f)
}

type previewChapter struct {
	Title   string
	StartMs int64
	Stamp   string // HH:MM:SS
}

// previewFile is one source file in a multi-file selection: its start offset
// in the merged book is where a boundary chapter would begin.
type previewFile struct {
	Index     int
	Name      string
	Rel       string // input-relative, for the streaming URL
	StartMs   int64
	Stamp     string
	Duration  string
	EndSeekMs int64 // a few seconds before the file ends
	Playable  bool
}

type previewData struct {
	RelPath    string
	Multi      bool
	Files      []previewFile
	FilesJSON  string // [{"rel","start","end"}] for the seek-across-files JS
	Playable   bool   // first file is browser-playable
	DurationMs int64
	Duration   string
	Chapters   []previewChapter // single-file only: chapters already embedded
	Err        string
}

// handleMatchPreview renders the chapter-preview panel for one match item:
// an audio player plus seekable chapter/file lists. Single-file selections
// list the file's embedded chapters; multi-file selections list each file's
// start (the would-be boundary chapters), so the user can hear whether files
// begin at natural breaks or were split arbitrarily mid-sentence.
func (s *Server) handleMatchPreview(w http.ResponseWriter, r *http.Request) {
	set := s.settings()
	rel := r.URL.Query().Get("path")
	data := previewData{RelPath: rel}

	fail := func(msg string) {
		data.Err = msg
		s.render.partial(w, "match", "chapter_preview", data)
	}

	files, err := scan.CollectAudioFiles(set.InputDir, rel)
	if err != nil {
		fail(err.Error())
		return
	}
	if len(files) > 200 {
		fail(fmt.Sprintf("This selection has %d files — too many to preview individually.", len(files)))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	type boundary struct {
		Rel   string `json:"rel"`
		Start int64  `json:"start"`
		End   int64  `json:"end"`
	}
	inputRoot := filepath.Clean(set.InputDir)
	var bounds []boundary
	var offset int64
	for i, f := range files {
		info, err := pipeline.ProbeFile(ctx, s.cfg.FFprobePath, f)
		if err != nil {
			fail("ffprobe failed: " + err.Error())
			return
		}
		fileRel, err := filepath.Rel(inputRoot, f)
		if err != nil {
			fail("cannot resolve file path")
			return
		}
		frel := filepath.ToSlash(fileRel)
		_, playable := audioContentTypes[strings.ToLower(filepath.Ext(f))]

		endSeek := offset + info.DurationMs - 6000
		if endSeek < offset {
			endSeek = offset
		}
		data.Files = append(data.Files, previewFile{
			Index:     i + 1,
			Name:      filepath.Base(f),
			Rel:       frel,
			StartMs:   offset,
			Stamp:     msClock(offset),
			Duration:  msClock(info.DurationMs),
			EndSeekMs: endSeek,
			Playable:  playable,
		})
		bounds = append(bounds, boundary{Rel: frel, Start: offset, End: offset + info.DurationMs})

		if len(files) == 1 {
			for _, c := range info.Chapters {
				data.Chapters = append(data.Chapters, previewChapter{
					Title: c.Title, StartMs: c.StartMs, Stamp: msClock(c.StartMs),
				})
			}
		}
		offset += info.DurationMs
	}

	data.Multi = len(files) > 1
	data.DurationMs = offset
	data.Duration = msClock(offset)
	data.Playable = data.Files[0].Playable
	if data.Multi {
		raw, _ := json.Marshal(bounds)
		data.FilesJSON = string(raw)
	}
	s.render.partial(w, "match", "chapter_preview", data)
}

type providerChaptersData struct {
	Path      string // input-relative selection path, echoed for the retry button
	Chapters  []previewChapter
	RuntimeMs int64
	Runtime   string
	Accurate  bool
	ShiftMs   int64 // current per-book offset, preserved across reloads
	Err       string
}

// handleMatchProviderChapters fetches the selected candidate's chapter
// timings so the user can seek the local audio to them for comparison.
func (s *Server) handleMatchProviderChapters(w http.ResponseWriter, r *http.Request) {
	choice := r.URL.Query().Get("choice") // "ASIN|region" from the selected radio
	data := providerChaptersData{
		Path:    r.URL.Query().Get("path"),
		ShiftMs: parseShiftMs(r.URL.Query().Get("shift")),
	}
	asin, region, ok := strings.Cut(choice, "|")
	if !ok || asin == "" {
		data.Err = "Select a match first — then its official chapters can be loaded here."
		s.render.partial(w, "match", "provider_chapters", data)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	info, err := s.provider().GetChapters(ctx, asin, region)
	if err != nil {
		if metadata.IsNotFound(err) {
			data.Err = "No chapter data available for " + asin + " (" + region + ")."
		} else {
			data.Err = "Chapter lookup failed: " + err.Error()
		}
		s.render.partial(w, "match", "provider_chapters", data)
		return
	}
	s.store.CacheChapters(asin, region, info)
	data.RuntimeMs = info.RuntimeMs
	data.Runtime = msClock(info.RuntimeMs)
	data.Accurate = info.IsAccurate
	for _, c := range info.Chapters {
		data.Chapters = append(data.Chapters, previewChapter{
			Title: c.Title, StartMs: c.StartMs, Stamp: msClock(c.StartMs),
		})
	}
	s.render.partial(w, "match", "provider_chapters", data)
}

type chapterPlanData struct {
	Source   string // audnexus|existing|files|single
	Icon     string
	Verdict  string
	Reason   string
	Warnings []string
	Err      string
}

// handleMatchChapterPlan runs the pipeline's actual chapter decision against
// the selected match and reports the verdict, so "auto" is never opaque.
func (s *Server) handleMatchChapterPlan(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	set := s.settings()
	mode := q.Get("mode")
	if mode == "" {
		mode = pipeline.ChapterModeAuto
	}
	data := chapterPlanData{}

	files, err := scan.CollectAudioFiles(set.InputDir, q.Get("path"))
	if err != nil {
		data.Err = err.Error()
		s.render.partial(w, "match", "chapter_plan", data)
		return
	}
	if len(files) > 300 {
		data.Err = "Too many files to preview the chapter decision; it will be made during conversion."
		s.render.partial(w, "match", "chapter_plan", data)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	infos := make([]*pipeline.FileInfo, 0, len(files))
	var totalMs int64
	for _, f := range files {
		info, err := pipeline.ProbeFile(ctx, s.cfg.FFprobePath, f)
		if err != nil {
			data.Err = "ffprobe failed: " + err.Error()
			s.render.partial(w, "match", "chapter_plan", data)
			return
		}
		infos = append(infos, info)
		totalMs += info.DurationMs
	}

	var provider *metadata.ChapterInfo
	if asin, region, ok := strings.Cut(q.Get("choice"), "|"); ok && asin != "" {
		if provider, _ = s.store.CachedChapters(asin, region); provider == nil {
			if ch, err := s.provider().GetChapters(ctx, asin, region); err == nil {
				provider = ch
				s.store.CacheChapters(asin, region, ch)
			} else if !metadata.IsNotFound(err) {
				data.Warnings = append(data.Warnings, "Chapter lookup failed ("+err.Error()+") — decision shown without provider data.")
			}
		}
	}
	// The verdict must reflect the same shift the conversion will apply.
	if shift := parseShiftMs(q.Get("shift")); shift != 0 && provider != nil {
		provider = provider.Shifted(shift)
	}

	plan := pipeline.PlanChapters(mode, provider, infos, totalMs, "")
	data.Source = plan.Source
	data.Warnings = append(data.Warnings, plan.Warnings...)

	n := len(plan.Chapters)
	switch plan.Source {
	case "audnexus":
		data.Icon, data.Verdict = "🌐", fmt.Sprintf("Will embed Audible's %d chapters.", n)
		if provider != nil {
			data.Reason = fmt.Sprintf("Official runtime %s vs your audio %s.", msClock(provider.RuntimeMs), msClock(totalMs))
		}
	case "existing":
		data.Icon, data.Verdict = "📼", fmt.Sprintf("Will keep the file's own %d chapters.", n)
		if mode == pipeline.ChapterModeExisting {
			data.Reason = "You chose to keep them."
		} else if provider == nil {
			data.Reason = "No usable Audible chapter data" + choiceHint(q.Get("choice")) + "."
		}
	case "files":
		data.Icon, data.Verdict = "🧩", fmt.Sprintf("Will generate %d chapters from the file boundaries.", n)
		if provider == nil && mode != pipeline.ChapterModeExisting {
			data.Reason = "No usable Audible chapter data" + choiceHint(q.Get("choice")) + "."
		}
	case "single":
		data.Icon, data.Verdict = "▭", "Will embed one whole-book chapter."
		data.Reason = "No chapter data anywhere: not from Audible, none embedded in the file."
	}
	s.render.partial(w, "match", "chapter_plan", data)
}

func choiceHint(choice string) string {
	if choice == "" {
		return " (no match selected yet)"
	}
	return ""
}

// parseShiftMs reads a user-supplied chapter offset, ignoring junk and
// capping at ±1 hour — a larger offset means the wrong edition, not a rip
// that drifted.
func parseShiftMs(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	const limit = 3_600_000
	if n > limit {
		return limit
	}
	if n < -limit {
		return -limit
	}
	return n
}

func msClock(ms int64) string {
	h := ms / 3_600_000
	m := ms % 3_600_000 / 60_000
	sec := ms % 60_000 / 1000
	if h > 0 {
		return fmtClock(h, m, sec)
	}
	return fmtClock(0, m, sec)[3:]
}

func fmtClock(h, m, s int64) string {
	const digits = "0123456789"
	b := []byte{0, 0, ':', 0, 0, ':', 0, 0}
	b[0], b[1] = digits[h/10%10], digits[h%10]
	b[3], b[4] = digits[m/10], digits[m%10]
	b[6], b[7] = digits[s/10], digits[s%10]
	return string(b)
}
