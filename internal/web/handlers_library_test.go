package web

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// seedLibrary builds a small library laid out the way the default path
// template writes one.
func seedLibrary(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := []string{
		"Terry Pratchett/Discworld/The Colour of Magic/The Colour of Magic [B09LYZPFLZ].m4b",
		"Terry Pratchett/Discworld/Small Gods/Small Gods [B09LZ2HGQN].m4b",
		"Andy Weir/Project Hail Mary/Project Hail Mary [B08G9PRS1K].m4b",
		"Andy Weir/Project Hail Mary/Project Hail Mary [B08G9PRS1K].chapters.txt", // not an m4b
		"Neil Gaiman/Neverwhere/Neverwhere.mp3",                                   // not an m4b
	}
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func relPaths(t *testing.T, root, query string) []string {
	t.Helper()
	entries, _, err := searchLibrary(context.Background(), root, query, searchRowLimit)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.RelPath
	}
	return out
}

func TestSearchLibraryMatchesAcrossThePath(t *testing.T) {
	root := seedLibrary(t)

	// The author and series live in directory names, so a query spanning both
	// a folder and the filename has to match the whole relative path.
	got := relPaths(t, root, "pratchett magic")
	if len(got) != 1 || got[0] != "Terry Pratchett/Discworld/The Colour of Magic/The Colour of Magic [B09LYZPFLZ].m4b" {
		t.Errorf("multi-term across path segments: %v", got)
	}

	if got := relPaths(t, root, "discworld"); len(got) != 2 {
		t.Errorf("series should match both books: %v", got)
	}
	if got := relPaths(t, root, "PRATCHETT"); len(got) != 2 {
		t.Errorf("matching should be case-insensitive: %v", got)
	}
	if got := relPaths(t, root, "b08g9prs1k"); len(got) != 1 {
		t.Errorf("ASIN in the filename should match: %v", got)
	}
	if got := relPaths(t, root, "nothing here"); len(got) != 0 {
		t.Errorf("expected no matches, got %v", got)
	}
}

func TestSearchLibraryOnlyReturnsM4b(t *testing.T) {
	root := seedLibrary(t)
	// A sidecar and an mp3 share these terms but neither can be retagged.
	if got := relPaths(t, root, "project hail mary"); len(got) != 1 {
		t.Errorf("sidecars must not be listed: %v", got)
	}
	if got := relPaths(t, root, "neverwhere"); len(got) != 0 {
		t.Errorf("non-m4b audio must not be listed: %v", got)
	}
}

func TestSearchLibraryReportsTruncation(t *testing.T) {
	root := seedLibrary(t)
	entries, total, err := searchLibrary(context.Background(), root, "m", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("row cap not applied: %d rows", len(entries))
	}
	if total <= len(entries) {
		t.Errorf("the true match count must be reported so a capped list never reads as complete: total=%d", total)
	}
}

func TestLibraryFilesForExpandsFolders(t *testing.T) {
	root := seedLibrary(t)

	// A folder stands for every book under it, one retag each.
	got := libraryFilesFor(root, []string{"Terry Pratchett"})
	if len(got) != 2 {
		t.Fatalf("folder should expand to both books: %v", got)
	}

	// Selecting a folder and a file inside it must not queue the file twice.
	got = libraryFilesFor(root, []string{
		"Terry Pratchett",
		"Terry Pratchett/Discworld/Small Gods/Small Gods [B09LZ2HGQN].m4b",
	})
	if len(got) != 2 {
		t.Errorf("overlapping selection should dedupe: %v", got)
	}

	// Non-m4b selections resolve to nothing rather than to a job that fails.
	if got := libraryFilesFor(root, []string{"Neil Gaiman"}); len(got) != 0 {
		t.Errorf("mp3-only folder should yield nothing: %v", got)
	}
}
