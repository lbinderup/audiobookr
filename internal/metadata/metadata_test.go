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

func TestShiftedByInterpolates(t *testing.T) {
	// Anchors: chapter 1 (t=0) shift 0, chapter 5 (t=40000) shift +2000.
	in := info(60_000,
		Chapter{Title: "1", StartMs: 0},
		Chapter{Title: "2", StartMs: 10_000},
		Chapter{Title: "3", StartMs: 20_000},
		Chapter{Title: "4", StartMs: 30_000},
		Chapter{Title: "5", StartMs: 40_000},
		Chapter{Title: "6", StartMs: 50_000},
	)
	got := in.ShiftedBy(ShiftSpec{Mode: "interp", FromIdx: 1, FromMs: 0, ToIdx: 5, ToMs: 2000})
	want := []int64{0, 10_500, 21_000, 31_500, 42_000, 52_000} // +0,+500,+1000,+1500,+2000, then held at +2000
	for i, w := range want {
		if got.Chapters[i].StartMs != w {
			t.Errorf("chapter %d start = %d, want %d", i+1, got.Chapters[i].StartMs, w)
		}
	}
	// Lengths follow the new boundaries.
	if got.Chapters[0].LengthMs != 10_500 || got.Chapters[5].LengthMs != 8_000 {
		t.Errorf("lengths wrong: first=%d last=%d", got.Chapters[0].LengthMs, got.Chapters[5].LengthMs)
	}
}

func TestShiftedByClampsOutsideAnchors(t *testing.T) {
	// Interior anchors: chapters before X get X's shift, after Y get Y's.
	in := info(50_000,
		Chapter{Title: "1", StartMs: 0},
		Chapter{Title: "2", StartMs: 10_000},
		Chapter{Title: "3", StartMs: 20_000},
		Chapter{Title: "4", StartMs: 30_000},
		Chapter{Title: "5", StartMs: 40_000},
	)
	got := in.ShiftedBy(ShiftSpec{Mode: "interp", FromIdx: 2, FromMs: 1000, ToIdx: 4, ToMs: 3000})
	want := []int64{1000, 11_000, 22_000, 33_000, 43_000}
	for i, w := range want {
		if got.Chapters[i].StartMs != w {
			t.Errorf("chapter %d start = %d, want %d", i+1, got.Chapters[i].StartMs, w)
		}
	}
}

func TestShiftedBySanitizesAnchors(t *testing.T) {
	in := info(30_000,
		Chapter{Title: "1", StartMs: 0},
		Chapter{Title: "2", StartMs: 10_000},
		Chapter{Title: "3", StartMs: 20_000},
	)
	// Reversed anchors behave the same as ordered ones.
	fwd := in.ShiftedBy(ShiftSpec{Mode: "interp", FromIdx: 1, FromMs: 0, ToIdx: 3, ToMs: 1000})
	rev := in.ShiftedBy(ShiftSpec{Mode: "interp", FromIdx: 3, FromMs: 1000, ToIdx: 1, ToMs: 0})
	for i := range fwd.Chapters {
		if fwd.Chapters[i].StartMs != rev.Chapters[i].StartMs {
			t.Errorf("reversed anchors diverge at %d: %d vs %d", i, fwd.Chapters[i].StartMs, rev.Chapters[i].StartMs)
		}
	}
	// Out-of-range indices clamp; equal anchors degrade to a fixed shift.
	same := in.ShiftedBy(ShiftSpec{Mode: "interp", FromIdx: -5, FromMs: 500, ToIdx: -9, ToMs: 9999})
	if same.Chapters[0].StartMs != 500 && same.Chapters[0].StartMs != 0 {
		t.Errorf("degenerate anchors: %+v", same.Chapters)
	}
	// Zero spec is a no-op returning the original.
	if got := in.ShiftedBy(ShiftSpec{}); got != in {
		t.Error("empty spec should return the original")
	}
	if got := in.ShiftedBy(ShiftSpec{Mode: "interp"}); got != in {
		t.Error("all-zero interp spec should return the original")
	}
}

func TestShiftedByKeepsOrderingUnderNegativeGradient(t *testing.T) {
	// A shift going strongly negative could reorder chapters; starts must
	// stay non-decreasing regardless.
	in := info(20_000,
		Chapter{Title: "1", StartMs: 0},
		Chapter{Title: "2", StartMs: 1000},
		Chapter{Title: "3", StartMs: 2000},
	)
	got := in.ShiftedBy(ShiftSpec{Mode: "interp", FromIdx: 1, FromMs: 0, ToIdx: 3, ToMs: -1900})
	var prev int64 = -1
	for i, ch := range got.Chapters {
		if ch.StartMs < prev {
			t.Fatalf("chapter %d start %d before previous %d", i+1, ch.StartMs, prev)
		}
		prev = ch.StartMs
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
