package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"audiobookr/internal/metadata"
	"audiobookr/internal/pathtmpl"
	"audiobookr/internal/store"
)

// FakeConverter simulates a conversion for UI development: it walks the real
// stages with plausible timing, emits log lines and progress, honors
// cancellation, and fails deterministically when the input path contains
// "fail" (so error UX is testable).
type FakeConverter struct {
	// StageDelay scales the simulation; default ~8s total.
	StageDelay time.Duration
}

func (f *FakeConverter) delay() time.Duration {
	if f.StageDelay > 0 {
		return f.StageDelay
	}
	if ms, err := strconv.Atoi(os.Getenv("FAKE_STAGE_MS")); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 700 * time.Millisecond
}

func (f *FakeConverter) Run(ctx context.Context, job *store.Job, report ProgressFunc, logf LogFunc) (*Result, error) {
	logf("fake converter: starting %s (asin %s)", job.InputPath, job.ASIN)

	type step struct {
		stage string
		until float64
		ticks int
	}
	steps := []step{
		{"probe", 0.05, 1}, {"plan", 0.10, 1}, {"merge", 0.90, 10},
		{"chapters", 0.93, 1}, {"tag", 0.96, 1}, {"move", 0.98, 1},
		{"verify", 0.99, 1}, {"cleanup", 1.0, 1},
	}
	last := 0.0
	for _, st := range steps {
		logf("stage: %s", st.stage)
		for i := 1; i <= st.ticks; i++ {
			select {
			case <-ctx.Done():
				logf("cancelled during %s", st.stage)
				return nil, ctx.Err()
			case <-time.After(f.delay() / time.Duration(st.ticks)):
			}
			p := last + (st.until-last)*float64(i)/float64(st.ticks)
			report(st.stage, p)
			if st.stage == "merge" {
				logf("  time=00:%02d:00.00 bitrate=128.0kbits/s", i*7)
			}
		}
		last = st.until
		if st.stage == "merge" && strings.Contains(strings.ToLower(job.InputPath), "fail") {
			logf("simulated ffmpeg failure")
			return nil, fmt.Errorf("simulated merge failure (input path contains \"fail\")")
		}
	}

	rel, err := pathtmpl.Render(job.Options.PathTemplate, varsFor(job))
	if err != nil {
		return nil, err
	}
	out := path.Join(job.Options.OutputDir, rel+".m4b")
	chapters, _ := json.Marshal(map[string]any{
		"source": "fake",
		"chapters": []metadata.Chapter{
			{Title: "Opening Credits", StartMs: 0, LengthMs: 11264},
			{Title: "Chapter 1", StartMs: 11264, LengthMs: 2203838},
		},
	})
	logf("fake converter: wrote %s", out)
	return &Result{OutputPath: out, ChaptersJSON: string(chapters)}, nil
}

// varsFor maps a job's metadata snapshot to path-template variables.
// Shared by real and fake converters.
func varsFor(job *store.Job) pathtmpl.Vars {
	v := pathtmpl.Vars{
		Title:          job.Metadata.Title,
		Subtitle:       job.Metadata.Subtitle,
		SeriesName:     job.Metadata.SeriesName,
		SeriesPosition: job.Metadata.SeriesPosition,
		Year:           job.Metadata.Year,
		ASIN:           job.Metadata.ASIN,
	}
	if len(job.Metadata.Authors) > 0 {
		v.Author = job.Metadata.Authors[0]
	}
	if len(job.Metadata.Narrators) > 0 {
		v.Narrator = job.Metadata.Narrators[0]
	}
	return v
}
