package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"audioborker/internal/store"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// renderer parses layout + one page + all partials into a template set per
// page. In dev mode it re-reads from disk on every render so template edits
// show up on refresh.
type renderer struct {
	dev  bool
	root string // disk path to internal/web, dev mode only

	mu    sync.Mutex
	cache map[string]*template.Template
}

func newRenderer(dev bool) *renderer {
	r := &renderer{dev: dev, cache: map[string]*template.Template{}}
	if dev {
		r.root = filepath.Join("internal", "web")
	}
	return r
}

func (r *renderer) fs() fs.FS {
	if r.dev {
		return os.DirFS(r.root)
	}
	return templatesFS
}

func (r *renderer) page(name string) (*template.Template, error) {
	if !r.dev {
		r.mu.Lock()
		if t, ok := r.cache[name]; ok {
			r.mu.Unlock()
			return t, nil
		}
		r.mu.Unlock()
	}
	t, err := template.New("layout.html").Funcs(funcMap).ParseFS(r.fs(),
		"templates/layout.html",
		"templates/pages/"+name+".html",
		"templates/partials/*.html",
	)
	if err != nil {
		return nil, fmt.Errorf("parse templates for %s: %w", name, err)
	}
	if !r.dev {
		r.mu.Lock()
		r.cache[name] = t
		r.mu.Unlock()
	}
	return t, nil
}

// render writes a full page (layout + page content).
func (r *renderer) render(w http.ResponseWriter, name string, data any) {
	t, err := r.page(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout.html", data); err != nil {
		// Headers are gone; log to the response tail is useless. Best effort.
		fmt.Fprintf(w, "\n<!-- template error: %v -->", err)
	}
}

// partial writes a single named partial (htmx fragment responses).
func (r *renderer) partial(w http.ResponseWriter, page, block string, data any) {
	t, err := r.page(page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, block, data); err != nil {
		fmt.Fprintf(w, "\n<!-- template error: %v -->", err)
	}
}

var funcMap = template.FuncMap{
	"humanSize": humanSize,
	"runtime":   humanRuntime,
	"add":       func(a, b int) int { return a + b },
	"sub":       func(a, b int) int { return a - b },
	"deltaMin":  deltaMinutes,
	"progressOf": func(j *store.Job) progressData {
		return progressData{Stage: j.Stage, Percent: int(j.Progress * 100)}
	},
}

// deltaMinutes renders a signed runtime difference as "12 min longer" or
// "1 hr 5 min shorter", from the candidate's point of view.
func deltaMinutes(delta int) string {
	word := "longer"
	if delta < 0 {
		delta, word = -delta, "shorter"
	}
	if delta == 0 {
		return "same length"
	}
	return humanRuntime(delta) + " " + word
}

// humanRuntime renders minutes as "10 hrs 12 min".
func humanRuntime(min int) string {
	h, m := min/60, min%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d hrs", h)
	}
	return fmt.Sprintf("%d hrs %d min", h, m)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
