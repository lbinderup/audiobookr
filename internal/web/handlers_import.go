package web

import (
	"net/http"

	"audioborker/internal/scan"
)

type importData struct {
	baseData
	InputDir string
	Entries  []scan.Entry
	Empty    bool
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	set := s.settings()
	entries, err := scan.List(set.InputDir, "")
	data := importData{
		baseData: s.base("Import", "import"),
		InputDir: set.InputDir,
		Entries:  entries,
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
	s.render.partial(w, "import", "tree", entries)
}
