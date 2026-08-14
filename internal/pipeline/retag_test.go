package pipeline

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"audioborker/internal/metadata"
	"audioborker/internal/store"
)

func retagOpts(rename bool) store.JobOptions {
	return store.JobOptions{
		Kind:         store.KindRetag,
		OutputDir:    filepath.FromSlash("/library"),
		PathTemplate: "{author}/{series_name}/{title}/{title} [{asin}]",
		Rename:       rename,
	}
}

var retagBook = metadata.Book{
	ASIN: "B000000001", Title: "The Colour of Magic", Authors: []string{"Terry Pratchett"},
	SeriesName: "Discworld", SeriesPosition: "1",
}

// missing reports that nothing is at the destination.
func missing(target, src string) (bool, bool) { return false, false }

func TestRetagTargetWithoutRename(t *testing.T) {
	src := filepath.FromSlash("/library/Somewhere/Old Name.m4b")
	got, renamed, err := retagTarget(src, retagOpts(false), retagBook, missing)
	if err != nil || got != src || renamed {
		t.Errorf("in-place retag: got %q renamed=%v err=%v", got, renamed, err)
	}
}

func TestRetagTargetRenames(t *testing.T) {
	src := filepath.FromSlash("/library/Somewhere/Old Name.m4b")
	got, renamed, err := retagTarget(src, retagOpts(true), retagBook, missing)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.FromSlash("/library/Terry Pratchett/Discworld/The Colour of Magic/The Colour of Magic [B000000001].m4b")
	if got != want || !renamed {
		t.Errorf("got %q renamed=%v, want %q renamed=true", got, renamed, want)
	}
}

func TestRetagTargetRenderingItsOwnPathIsNotARename(t *testing.T) {
	// The file already sits where the template puts it: replace in place, and
	// crucially do NOT then delete "the old file" — it is this one.
	src := filepath.FromSlash("/library/Terry Pratchett/Discworld/The Colour of Magic/The Colour of Magic [B000000001].m4b")
	got, renamed, err := retagTarget(src, retagOpts(true), retagBook, missing)
	if err != nil || got != src || renamed {
		t.Errorf("got %q renamed=%v err=%v, want the source and renamed=false", got, renamed, err)
	}
}

func TestRetagTargetCaseOnlyRenameIsAllowed(t *testing.T) {
	// On NTFS/SMB "the colour of magic" and "The Colour of Magic" stat as the
	// same file. Refusing that would make the rename toggle useless for
	// exactly the mistakes people want to fix.
	src := filepath.FromSlash("/library/terry pratchett/discworld/the colour of magic/the colour of magic [B000000001].m4b")
	sameFile := func(target, src string) (bool, bool) { return true, true }

	got, renamed, err := retagTarget(src, retagOpts(true), retagBook, sameFile)
	if err != nil {
		t.Fatalf("case-only rename refused: %v", err)
	}
	if renamed {
		t.Error("a case-only rename has no old file to remove, so renamed must be false")
	}
	want := filepath.FromSlash("/library/Terry Pratchett/Discworld/The Colour of Magic/The Colour of Magic [B000000001].m4b")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRetagTargetRefusesToOverwriteAnotherFile(t *testing.T) {
	src := filepath.FromSlash("/library/Somewhere/Old Name.m4b")
	occupied := func(target, src string) (bool, bool) { return true, false }
	if _, _, err := retagTarget(src, retagOpts(true), retagBook, occupied); err == nil {
		t.Fatal("renaming onto a different existing file must be refused")
	}
}

func TestRetagTargetTemplateError(t *testing.T) {
	opts := retagOpts(true)
	opts.PathTemplate = "{nope}"
	if _, _, err := retagTarget("/x.m4b", opts, retagBook, missing); err == nil {
		t.Fatal("an invalid template must fail the job, not write somewhere arbitrary")
	}
}

func TestVerifyRetag(t *testing.T) {
	before := &FileInfo{DurationMs: 3_600_000}
	ok := &FileInfo{
		DurationMs: 3_600_000,
		Chapters:   []ProbedChapter{{Title: "One"}, {Title: "Two"}},
		Tags:       map[string]string{"title": "The Colour of Magic"},
	}
	if err := verifyRetag(before, ok, 2, "B000000001"); err != nil {
		t.Errorf("a good copy was rejected: %v", err)
	}

	// A few ms of remux drift is expected and must not fail.
	drift := *ok
	drift.DurationMs = 3_600_040
	if err := verifyRetag(before, &drift, 2, ""); err != nil {
		t.Errorf("small remux drift should be tolerated: %v", err)
	}

	long := *ok
	long.DurationMs = 4_000_000
	if err := verifyRetag(before, &long, 2, ""); err == nil {
		t.Error("a duration change must block the replace")
	}

	short := *ok
	short.Chapters = []ProbedChapter{{Title: "One"}}
	if err := verifyRetag(before, &short, 2, ""); err == nil {
		t.Error("tone dropping chapters must block the replace")
	}

	untitled := *ok
	untitled.Tags = map[string]string{"title": "  "}
	if err := verifyRetag(before, &untitled, 2, ""); err == nil {
		t.Error("a copy with no title tag must block the replace")
	}
}

func TestPruneEmptyParents(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "Author", "Series", "Book")
	if err := os.MkdirAll(deep, 0o777); err != nil {
		t.Fatal(err)
	}
	pruneEmptyParents(deep, root, func(string, ...any) {})

	if _, err := os.Stat(filepath.Join(root, "Author")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("empty ancestors should have been pruned")
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("the library root itself must never be removed: %v", err)
	}
}

func TestPruneEmptyParentsStopsAtRealContent(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "Author", "Series", "Book")
	if err := os.MkdirAll(deep, 0o777); err != nil {
		t.Fatal(err)
	}
	// Another book by the same author must keep the author folder alive.
	sibling := filepath.Join(root, "Author", "Other.m4b")
	if err := os.WriteFile(sibling, []byte("x"), 0o666); err != nil {
		t.Fatal(err)
	}
	pruneEmptyParents(deep, root, func(string, ...any) {})

	if _, err := os.Stat(filepath.Join(root, "Author", "Series")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the emptied series folder should have been pruned")
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Errorf("a folder holding another book must survive: %v", err)
	}
}

func TestPruneEmptyParentsRefusesOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "keep")
	if err := os.MkdirAll(victim, 0o777); err != nil {
		t.Fatal(err)
	}
	pruneEmptyParents(victim, root, func(string, ...any) {})
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("a directory outside the library must not be touched: %v", err)
	}
}

func TestSidecarFor(t *testing.T) {
	got := sidecarFor(filepath.FromSlash("/library/A/B [X].m4b"))
	want := filepath.FromSlash("/library/A/B [X].chapters.txt")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
