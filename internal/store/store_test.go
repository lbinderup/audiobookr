package store

import (
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s2, err := Open(path) // second open re-runs migrate; must be a no-op
	if err != nil {
		t.Fatal(err)
	}
	s2.Close()
}

func TestSettingsRoundTrip(t *testing.T) {
	s := openTest(t)
	def := Defaults("/input", "/output")

	got, err := s.GetSettings(def)
	if err != nil {
		t.Fatal(err)
	}
	if got != def {
		t.Errorf("empty db should return defaults, got %+v", got)
	}

	want := def
	want.OutputDir = "/mnt/audiobooks"
	want.CleanupMode = "move"
	want.CompletedDir = "/mnt/processed"
	want.WorkerConcurrency = 2
	want.BitrateKbps = 128
	want.WriteChaptersTxt = false
	if err := s.SaveSettings(want); err != nil {
		t.Fatal(err)
	}

	got, err = s.GetSettings(def)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestSaveSettingsRejectsInvalid(t *testing.T) {
	s := openTest(t)
	bad := Defaults("/input", "/output")
	bad.CleanupMode = "move"
	bad.CompletedDir = "/input/done" // inside input dir — the bragibooks trap
	if err := s.SaveSettings(bad); err == nil {
		t.Error("completed dir inside input dir was accepted")
	}

	bad2 := Defaults("/input", "/output")
	bad2.PathTemplate = "{nope}"
	if err := s.SaveSettings(bad2); err == nil {
		t.Error("unknown template token was accepted")
	}
}

func TestJobLifecycle(t *testing.T) {
	s := openTest(t)

	job := &Job{InputPath: "Some Book", ASIN: "B000000001", Region: "us"}
	if err := s.CreateJob(job); err != nil {
		t.Fatal(err)
	}

	// Claim flips pending -> running.
	claimed, err := s.ClaimNextPending()
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if claimed.ID != job.ID || claimed.Status != StatusRunning || claimed.StartedAt.IsZero() {
		t.Errorf("claimed = %+v", claimed)
	}
	// Nothing else pending.
	if again, _ := s.ClaimNextPending(); again != nil {
		t.Errorf("claimed a second job: %+v", again)
	}

	// Boot reconciliation flips running -> error.
	n, err := s.MarkInterrupted()
	if err != nil || n != 1 {
		t.Fatalf("MarkInterrupted = %d, %v", n, err)
	}
	got, _ := s.GetJob(job.ID)
	if got.Status != StatusError || got.Error != "interrupted by restart" || got.FinishedAt.IsZero() {
		t.Errorf("after reconcile: %+v", got)
	}

	// Retry clones to a fresh pending job.
	clone, err := s.CloneForRetry(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == job.ID || clone.RetriedFrom != job.ID {
		t.Errorf("clone = %+v", clone)
	}
	if ok, _ := s.HasActiveJobForPath("Some Book"); !ok {
		t.Error("clone should count as active for its path")
	}

	// Pending jobs cancel directly.
	if ok, err := s.CancelPending(clone.ID); err != nil || !ok {
		t.Fatalf("CancelPending = %v, %v", ok, err)
	}
	// Terminal jobs clear from history.
	logs, err := s.ClearHistory()
	if err != nil {
		t.Fatal(err)
	}
	_ = logs
	if _, total, _ := s.ListJobs(0, 10); total != 0 {
		t.Errorf("history not cleared, %d jobs left", total)
	}
}

func TestValidateCollectsAllErrors(t *testing.T) {
	set := Settings{} // everything empty/invalid
	errs := set.Validate()
	if len(errs) < 5 {
		t.Errorf("expected many errors for zero-value settings, got %d: %v", len(errs), errs)
	}
}
