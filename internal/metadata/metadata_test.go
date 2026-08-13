package metadata

import "testing"

func info(runtime int64, chapters ...Chapter) *ChapterInfo {
	return &ChapterInfo{RuntimeMs: runtime, Chapters: chapters}
}

func TestShiftedMovesStartsAndRecomputesLengths(t *testing.T) {
	in := info(60_000,
		Chapter{Title: "One", StartMs: 0, LengthMs: 20_000},
		Chapter{Title: "Two", StartMs: 20_000, LengthMs: 40_000},
	)
	got := in.Shifted(1500)
	if got.Chapters[0].StartMs != 1500 || got.Chapters[1].StartMs != 21_500 {
		t.Fatalf("starts not shifted: %+v", got.Chapters)
	}
	// Lengths follow the new boundaries; the last runs to the end.
	if got.Chapters[0].LengthMs != 20_000 {
		t.Errorf("first length = %d, want 20000", got.Chapters[0].LengthMs)
	}
	if got.Chapters[1].LengthMs != 38_500 {
		t.Errorf("last length = %d, want 38500", got.Chapters[1].LengthMs)
	}
	// The original must be untouched — jobs snapshot it.
	if in.Chapters[0].StartMs != 0 {
		t.Error("Shifted mutated the input")
	}
}

func TestShiftedClampsAtZero(t *testing.T) {
	in := info(60_000,
		Chapter{Title: "One", StartMs: 0, LengthMs: 10_000},
		Chapter{Title: "Two", StartMs: 10_000, LengthMs: 50_000},
	)
	got := in.Shifted(-15_000)
	if got.Chapters[0].StartMs != 0 || got.Chapters[1].StartMs != 0 {
		t.Fatalf("negative starts not clamped: %+v", got.Chapters)
	}
	if got.Chapters[0].LengthMs != 0 {
		t.Errorf("collapsed chapter should have zero length, got %d", got.Chapters[0].LengthMs)
	}
	if got.Chapters[1].LengthMs != 60_000 {
		t.Errorf("last length = %d, want 60000", got.Chapters[1].LengthMs)
	}
}

func TestShiftedClampsAtRuntime(t *testing.T) {
	in := info(30_000, Chapter{Title: "One", StartMs: 25_000, LengthMs: 5_000})
	got := in.Shifted(60_000)
	if got.Chapters[0].StartMs != 30_000 || got.Chapters[0].LengthMs != 0 {
		t.Errorf("start past runtime not clamped: %+v", got.Chapters[0])
	}
}

func TestShiftedNoOps(t *testing.T) {
	in := info(1000, Chapter{Title: "One"})
	if got := in.Shifted(0); got != in {
		t.Error("a zero shift should return the original")
	}
	var nilInfo *ChapterInfo
	if got := nilInfo.Shifted(1000); got != nil {
		t.Error("nil should stay nil")
	}
	empty := info(1000)
	if got := empty.Shifted(1000); got != empty {
		t.Error("an empty chapter list should return the original")
	}
}
