package web

import (
	"context"
	"fmt"
	"os"
	"sync"

	"audioborker/internal/pipeline"
	"audioborker/internal/scan"
)

// durationCache memoizes the measured length of a selection. Probing costs
// one ffprobe per file, and the match screen asks repeatedly (every search,
// every re-render), so results are kept until the files change.
type durationCache struct {
	mu sync.Mutex
	m  map[string]durationEntry
}

type durationEntry struct {
	sig     string // changes when the underlying files do
	totalMs int64
}

func newDurationCache() *durationCache {
	return &durationCache{m: map[string]durationEntry{}}
}

// LocalRuntimeMs returns the combined duration of everything that would be
// merged for this selection. It returns 0 when the duration can't be
// determined — callers treat that as "unknown" rather than failing.
func (s *Server) LocalRuntimeMs(ctx context.Context, rel string) int64 {
	set := s.settings()
	files, err := scan.CollectAudioFiles(set.InputDir, rel)
	if err != nil || len(files) == 0 {
		return 0
	}

	sig := filesSignature(files)
	s.durations.mu.Lock()
	entry, ok := s.durations.m[rel]
	s.durations.mu.Unlock()
	if ok && entry.sig == sig {
		return entry.totalMs
	}

	var totalMs int64
	for _, f := range files {
		info, err := pipeline.ProbeFile(ctx, s.cfg.FFprobePath, f)
		if err != nil {
			return 0 // partial totals would mis-rank candidates; better to skip
		}
		totalMs += info.DurationMs
	}

	s.durations.mu.Lock()
	s.durations.m[rel] = durationEntry{sig: sig, totalMs: totalMs}
	s.durations.mu.Unlock()
	return totalMs
}

// LocalRuntimeMin is LocalRuntimeMs rounded to the nearest minute, for
// candidate scoring.
func (s *Server) LocalRuntimeMin(ctx context.Context, rel string) int {
	return int((s.LocalRuntimeMs(ctx, rel) + 30_000) / 60_000)
}

// filesSignature cheaply identifies a set of files by count, size and mtime,
// so edits or replacements invalidate the cached duration without re-probing.
func filesSignature(files []string) string {
	var sig string
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return "stat-error"
		}
		sig += fmt.Sprintf("%s:%d:%d|", f, info.Size(), info.ModTime().UnixNano())
	}
	return sig
}
