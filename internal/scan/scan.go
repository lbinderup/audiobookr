// Package scan lists the input directory for the import screen and provides
// the file-ordering rules the conversion pipeline reuses: junk filtering,
// audio detection, natural sort, and multi-disc directory ordering.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// AudioExtensions are the inputs we accept. ffmpeg decodes all of these.
var AudioExtensions = map[string]bool{
	".mp3": true, ".m4a": true, ".m4b": true, ".aac": true,
	".flac": true, ".ogg": true, ".opus": true, ".wma": true, ".wav": true,
}

func IsAudioFile(name string) bool {
	return AudioExtensions[strings.ToLower(filepath.Ext(name))]
}

// IsJunk reports names that should be invisible to the app: OS metadata,
// NAS thumbnail dirs, AppleDouble files, and other hidden entries.
// (Hidden files halting bragibooks processing was issue #232.)
func IsJunk(name string) bool {
	switch name {
	case "@eaDir", ".DS_Store", "Thumbs.db", "desktop.ini", "$RECYCLE.BIN", "#recycle", ".Trash":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "._")
}

// Entry is one row in the import tree.
type Entry struct {
	Name       string
	RelPath    string // slash-separated, relative to the input root
	IsDir      bool
	ModTime    time.Time
	Size       int64 // files only
	AudioFiles int   // dirs: direct audio children; files: 1 if audio
	SubDirs    int   // dirs: direct subdirectories (after junk filtering)
}

// List returns one directory level under root/rel. Directories come first:
// at the top level newest-modified first (fresh downloads at the top), in
// nested levels natural-sorted (CD1 < CD2 < CD10). Files follow in natural
// order. Junk and non-audio files are filtered out.
func List(root, rel string) ([]Entry, error) {
	abs, err := Resolve(root, rel)
	if err != nil {
		return nil, err
	}
	dirents, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}

	var dirs, files []Entry
	for _, de := range dirents {
		name := de.Name()
		if IsJunk(name) {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue // vanished mid-listing; skip
		}
		e := Entry{
			Name:    name,
			RelPath: path.Join(rel, name),
			IsDir:   de.IsDir(),
			ModTime: info.ModTime(),
		}
		if de.IsDir() {
			e.AudioFiles, e.SubDirs = quickCount(filepath.Join(abs, name))
			if e.AudioFiles == 0 && e.SubDirs == 0 {
				continue // nothing importable below (at this level, at least one hop)
			}
			dirs = append(dirs, e)
		} else {
			if !IsAudioFile(name) {
				continue
			}
			e.Size = info.Size()
			e.AudioFiles = 1
			files = append(files, e)
		}
	}

	topLevel := rel == "" || rel == "."
	sort.Slice(dirs, func(i, j int) bool {
		if topLevel && !dirs[i].ModTime.Equal(dirs[j].ModTime) {
			return dirs[i].ModTime.After(dirs[j].ModTime)
		}
		return NaturalLess(dirs[i].Name, dirs[j].Name)
	})
	sort.Slice(files, func(i, j int) bool {
		return NaturalLess(files[i].Name, files[j].Name)
	})
	return append(dirs, files...), nil
}

// quickCount counts direct audio files and subdirectories without recursing.
func quickCount(dir string) (audio, subdirs int) {
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, de := range dirents {
		if IsJunk(de.Name()) {
			continue
		}
		if de.IsDir() {
			subdirs++
		} else if IsAudioFile(de.Name()) {
			audio++
		}
	}
	return audio, subdirs
}

// DedupeSelection removes selections that are already covered by another:
// exact duplicates, and any path whose ancestor directory is also selected
// (picking a folder plus a file inside it must not create two jobs for the
// same audio). Order is preserved.
func DedupeSelection(paths []string) []string {
	selected := make(map[string]bool, len(paths))
	for _, p := range paths {
		selected[path.Clean(strings.ReplaceAll(p, "\\", "/"))] = true
	}
	var keep []string
	seen := map[string]bool{}
	for _, p := range paths {
		c := path.Clean(strings.ReplaceAll(p, "\\", "/"))
		if seen[c] {
			continue
		}
		covered := false
		for dir := path.Dir(c); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if selected[dir] {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		seen[c] = true
		keep = append(keep, c)
	}
	return keep
}

// Resolve joins root with the user-supplied relative path and guarantees the
// result stays inside root (rejects traversal and absolute paths).
func Resolve(root, rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return filepath.Clean(root), nil
	}
	if path.IsAbs(rel) || filepath.IsAbs(rel) || strings.Contains(rel, "\\") {
		return "", fmt.Errorf("invalid path %q", rel)
	}
	cleaned := path.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q escapes the input directory", rel)
	}
	return filepath.Join(filepath.Clean(root), filepath.FromSlash(cleaned)), nil
}

// CollectAudioFiles returns every audio file under root/rel (a file or a
// directory tree) in conversion order: disc directories sorted by disc
// number then name, files natural-sorted within each directory.
// This is the pipeline's merge-order source of truth.
func CollectAudioFiles(root, rel string) ([]string, error) {
	abs, err := Resolve(root, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !IsAudioFile(abs) {
			return nil, fmt.Errorf("%s is not a supported audio file", abs)
		}
		return []string{abs}, nil
	}

	type dirGroup struct {
		path  string
		disc  int
		found bool
		files []string
	}
	groups := map[string]*dirGroup{}
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if IsJunk(d.Name()) && p != abs {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			disc, ok := DiscNumber(d.Name())
			groups[p] = &dirGroup{path: p, disc: disc, found: ok}
			return nil
		}
		if !IsAudioFile(d.Name()) {
			return nil
		}
		dir := filepath.Dir(p)
		g, ok := groups[dir]
		if !ok {
			g = &dirGroup{path: dir}
			groups[dir] = g
		}
		g.files = append(g.files, p)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var ordered []*dirGroup
	for _, g := range groups {
		if len(g.files) > 0 {
			ordered = append(ordered, g)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.found != b.found {
			return !a.found // non-disc dirs (usually the root) first
		}
		if a.found && b.found && a.disc != b.disc {
			return a.disc < b.disc
		}
		return NaturalLess(a.path, b.path)
	})

	var out []string
	for _, g := range ordered {
		sort.Slice(g.files, func(i, j int) bool {
			return NaturalLess(filepath.Base(g.files[i]), filepath.Base(g.files[j]))
		})
		out = append(out, g.files...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no supported audio files under %s", abs)
	}
	return out, nil
}

var discRe = regexp.MustCompile(`(?i)^(?:cd|disc|disk|part)[\s_.-]*(\d+)$`)

// DiscNumber parses directory names like "CD1", "Disc 2", "disk_10", "Part 3".
func DiscNumber(name string) (int, bool) {
	m := discRe.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return 0, false
	}
	n := 0
	for _, r := range m[1] {
		n = n*10 + int(r-'0')
	}
	return n, true
}

// NaturalLess compares strings so embedded numbers sort numerically:
// "file_2" < "file_10". Case-insensitive.
func NaturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ac, bc := a[ai], b[bi]
		if isDigit(ac) && isDigit(bc) {
			// Compare whole number runs.
			aj, bj := ai, bi
			for aj < len(a) && isDigit(a[aj]) {
				aj++
			}
			for bj < len(b) && isDigit(b[bj]) {
				bj++
			}
			an := strings.TrimLeft(a[ai:aj], "0")
			bn := strings.TrimLeft(b[bi:bj], "0")
			if len(an) != len(bn) {
				return len(an) < len(bn)
			}
			if an != bn {
				return an < bn
			}
			ai, bi = aj, bj
			continue
		}
		al, bl := lower(ac), lower(bc)
		if al != bl {
			return al < bl
		}
		ai++
		bi++
	}
	if la, lb := len(a)-ai, len(b)-bi; la != lb {
		return la < lb
	}
	// Numerically/case-insensitively equal but not identical ("a01" vs "a1"):
	// tiebreak on the raw string so ordering is total and deterministic.
	return a < b
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}
