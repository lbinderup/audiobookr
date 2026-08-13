package web

import (
	"context"
	"fmt"
	"os"
	"sync"

	"audiobookr/internal/pipeline"
	"audiobookr/internal/scan"
)

// durationCache memoizes the measured length of a selection. Probing costs
// one ffprobe per file, and the match screen asks repeatedly (every search,
// every re-render), so results are kept until the files change.
type durationCache struct {
	mu sync.Mutex
	m  map[string]durationEntry
}

type durationEntry struct {
	sig    string // changes when the underlying files do
	totalM int    // minutes
}

func newDurationCache() *durationCache {
	return &durationCache{m: map[string]durationEntry{}}
}

// LocalRuntimeMin returns the combined duration of everything that would be
// merged for this selection, in minutes. It returns 0 when the duration
// can't be determined — callers treat that as "unknown" and fall back to
// name-only matching rather than failing.
func (s *Server) LocalRuntimeMin(ctx context.Context, rel string) int {
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
		return entry.totalM
	}

	var totalMs int64
	for _, f := range files {
		info, err := pipeline.ProbeFile(ctx, s.cfg.FFprobePath, f)
		if err != nil {
			return 0 // partial totals would mis-rank candidates; better to skip
		}
		totalMs += info.DurationMs
	}
	minutes := int((totalMs + 30_000) / 60_000) // round to nearest minute

	s.durations.mu.Lock()
	s.durations.m[rel] = durationEntry{sig: sig, totalM: minutes}
	s.durations.mu.Unlock()
	return minutes
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
