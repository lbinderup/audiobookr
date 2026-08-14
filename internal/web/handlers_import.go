package web

import (
	"net/http"

	"audioborker/internal/scan"
)

// treeData is what the shared recursive tree partial renders: one directory
// level plus the endpoint its lazily-loaded children come from, so Import and
// Library can share the markup while browsing different volumes. Recursion
// happens over HTTP, not in the template, so each handler supplies its own URL.
type treeData struct {
	Entries []scan.Entry
	TreeURL string // "/import/tree" | "/library/tree"
}

type importData struct {
	baseData
	InputDir string
	Tree     treeData
	Empty    bool
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	set := s.settings()
	entries, err := scan.List(set.InputDir, "")
	data := importData{
		baseData: s.base("Import", "import"),
		InputDir: set.InputDir,
		Tree:     treeData{Entries: entries, TreeURL: "/import/tree"},
		Empty:    err == nil && len(entries) == 0,
	}
	if err != nil {
		data.Error = "Could not read the input directory (" + set.InputDir + "): " + err.Error()
	}
	s.render.render(w, "import", data)
}

// handleImportTree returns one lazily-loaded directory level as an HTML
// fragment (htmx target).
func (s *Server) handleImportTree(w http.ResponseWriter, r *http.Request) {
	set := s.settings()
	rel := r.URL.Query().Get("path")
	entries, err := scan.List(set.InputDir, rel)
	if err != nil {
		http.Error(w, "cannot list "+rel, http.StatusBadRequest)
		return
	}
	s.render.partial(w, "import", "tree", treeData{Entries: entries, TreeURL: "/import/tree"})
}
