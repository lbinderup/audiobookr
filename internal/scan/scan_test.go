package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixture builds:
//
//	root/
//	  Fresh Book/            (newest)  b.mp3, a.mp3, cover.jpg, .DS_Store
//	  Old Book/              (oldest)  file_1.mp3 file_2.mp3 file_10.mp3
//	  Disc Book/             CD1/x.mp3 CD2/x.mp3 CD10/x.mp3 (+ intro.mp3 in root)
//	  @eaDir/                junk dir
//	  Empty Dir/             nothing importable
//	  loose.m4b, notes.txt, ._junk.mp3
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel string, mod time.Time) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !mod.IsZero() {
			os.Chtimes(p, mod, mod)
		}
	}
	now := time.Now()
	mk("Fresh Book/b.mp3", time.Time{})
	mk("Fresh Book/a.mp3", time.Time{})
	mk("Fresh Book/cover.jpg", time.Time{})
	mk("Fresh Book/.DS_Store", time.Time{})
	mk("Old Book/file_1.mp3", time.Time{})
	mk("Old Book/file_2.mp3", time.Time{})
	mk("Old Book/file_10.mp3", time.Time{})
	mk("Disc Book/intro.mp3", time.Time{})
	mk("Disc Book/CD1/x.mp3", time.Time{})
	mk("Disc Book/CD2/x.mp3", time.Time{})
	mk("Disc Book/CD10/x.mp3", time.Time{})
	mk("@eaDir/thumb.mp3", time.Time{})
	mk("loose.m4b", time.Time{})
	mk("notes.txt", time.Time{})
	mk("._junk.mp3", time.Time{})
	if err := os.MkdirAll(filepath.Join(root, "Empty Dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Fix directory mtimes: Fresh newest, Old oldest.
	os.Chtimes(filepath.Join(root, "Fresh Book"), now, now)
	os.Chtimes(filepath.Join(root, "Old Book"), now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	os.Chtimes(filepath.Join(root, "Disc Book"), now.Add(-24*time.Hour), now.Add(-24*time.Hour))
	return root
}

func names(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func TestListRoot(t *testing.T) {
	root := fixture(t)
	entries, err := List(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Fresh Book", "Disc Book", "Old Book", "loose.m4b"}
	got := names(entries)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	// Junk, non-audio and empty dirs must be gone.
	for _, e := range entries {
		if e.Name == "@eaDir" || e.Name == "notes.txt" || e.Name == "Empty Dir" || e.Name == "._junk.mp3" {
			t.Errorf("junk entry %q leaked through", e.Name)
		}
	}
	if entries[0].AudioFiles != 2 {
		t.Errorf("Fresh Book audio count = %d, want 2 (cover.jpg and .DS_Store excluded)", entries[0].AudioFiles)
	}
	if entries[1].SubDirs != 3 {
		t.Errorf("Disc Book subdirs = %d, want 3", entries[1].SubDirs)
	}
}

func TestListNestedSortsNaturally(t *testing.T) {
	root := fixture(t)
	entries, err := List(root, "Disc Book")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CD1", "CD2", "CD10", "intro.mp3"}
	got := names(entries)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("nested order got %v, want %v", got, want)
		}
	}
}

func TestResolveRejectsEscape(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"../outside", "..", "a/../../b", "/abs", `C:\abs`, `a\b`} {
		if _, err := Resolve(root, bad); err == nil {
			t.Errorf("Resolve accepted %q", bad)
		}
	}
	if _, err := Resolve(root, "a/b c/d"); err != nil {
		t.Errorf("Resolve rejected a normal path: %v", err)
	}
}

func TestCollectAudioFilesOrdering(t *testing.T) {
	root := fixture(t)

	// Natural sort within one flat book.
	files, err := CollectAudioFiles(root, "Old Book")
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{"file_1.mp3", "file_2.mp3", "file_10.mp3"}
	for i, f := range files {
		if filepath.Base(f) != wantOrder[i] {
			t.Fatalf("flat order = %v, want %v", files, wantOrder)
		}
	}

	// Disc dirs: root files first, then CD1 < CD2 < CD10.
	files, err = CollectAudioFiles(root, "Disc Book")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("disc book file count = %d, want 4 (%v)", len(files), files)
	}
	if filepath.Base(files[0]) != "intro.mp3" {
		t.Errorf("root-level file should come first, got %v", files)
	}
	wantDirs := []string{"CD1", "CD2", "CD10"}
	for i, f := range files[1:] {
		if filepath.Base(filepath.Dir(f)) != wantDirs[i] {
			t.Fatalf("disc order wrong: %v", files)
		}
	}

	// Single file selection.
	files, err = CollectAudioFiles(root, "loose.m4b")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("single file = %v", files)
	}

	// Junk-only dir errors.
	if _, err := CollectAudioFiles(root, "Empty Dir"); err == nil {
		t.Error("empty dir should error")
	}
}

func TestDedupeSelection(t *testing.T) {
	got := DedupeSelection([]string{
		"Terry Pratchett",
		"Terry Pratchett/The Colour of Magic.m4b", // covered by the folder
		"Other Book",
		"Other Book",                      // exact duplicate
		"Deep/Nested/Dir/file.mp3",        // no ancestor selected: kept
		"Terry Pratchett/CD1/track_1.mp3", // covered transitively
	})
	want := []string{"Terry Pratchett", "Other Book", "Deep/Nested/Dir/file.mp3"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDiscNumber(t *testing.T) {
	cases := map[string]struct {
		n  int
		ok bool
	}{
		"CD1": {1, true}, "cd 2": {2, true}, "Disc 3": {3, true}, "DISK_10": {10, true},
		"Part.4": {4, true}, "Chapter 1": {0, false}, "CDX": {0, false}, "Book CD": {0, false},
	}
	for in, want := range cases {
		n, ok := DiscNumber(in)
		if n != want.n || ok != want.ok {
			t.Errorf("DiscNumber(%q) = %d,%v want %d,%v", in, n, ok, want.n, want.ok)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	ordered := [][2]string{
		{"file_2", "file_10"},
		{"a", "b"},
		{"a1", "a2"},
		{"a2b", "a10a"},
		{"Chapter 9", "chapter 10"},
		{"01 intro", "2 body"},
		{"a", "a1"},
	}
	for _, p := range ordered {
		if !NaturalLess(p[0], p[1]) {
			t.Errorf("NaturalLess(%q, %q) = false, want true", p[0], p[1])
		}
		if NaturalLess(p[1], p[0]) {
			t.Errorf("NaturalLess(%q, %q) = true, want false", p[1], p[0])
		}
	}
	if NaturalLess("same", "same") {
		t.Error("NaturalLess(x, x) must be false")
	}
	// Equal numeric value, different padding: must be deterministic, not equal-both-ways.
	if NaturalLess("a01", "a1") == NaturalLess("a1", "a01") {
		t.Error("padded vs unpadded must order deterministically")
	}
}
