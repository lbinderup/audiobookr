package web

import (
	"io/fs"
	"net/http"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/import", http.StatusFound)
	})

	mux.HandleFunc("GET /import", s.handleImport)
	mux.HandleFunc("GET /import/tree", s.handleImportTree)
	mux.HandleFunc("POST /match", s.handleMatch)
	mux.HandleFunc("GET /match/candidates", s.handleMatchCandidates)
	mux.HandleFunc("GET /match/lookup", s.handleMatchLookup)
	mux.HandleFunc("GET /match/preview", s.handleMatchPreview)
	mux.HandleFunc("GET /match/provider-chapters", s.handleMatchProviderChapters)
	mux.HandleFunc("GET /match/chapter-plan", s.handleMatchChapterPlan)
	mux.HandleFunc("GET /preview/audio", s.handlePreviewAudio)

	mux.HandleFunc("POST /queue", s.handleQueueCreate)
	mux.HandleFunc("GET /queue", s.handleQueue)
	mux.HandleFunc("GET /queue/table", s.handleQueueTable)
	mux.HandleFunc("POST /queue/clear-history", s.handleClearHistory)
	mux.HandleFunc("GET /jobs/{id}", s.handleJobDetail)
	mux.HandleFunc("GET /jobs/{id}/status", s.handleJobStatusPartial)
	mux.HandleFunc("GET /jobs/{id}/log", s.handleJobLog)
	mux.HandleFunc("POST /jobs/{id}/cancel", s.handleJobCancel)
	mux.HandleFunc("POST /jobs/{id}/retry", s.handleJobRetry)
	mux.HandleFunc("GET /events", s.handleEvents)

	mux.HandleFunc("GET /settings", s.handleSettingsGet)
	mux.HandleFunc("POST /settings", s.handleSettingsPost)
	mux.HandleFunc("GET /settings/path-preview", s.handlePathPreview)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	staticRoot, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(http.FileServerFS(staticRoot))))

	return mux
}

func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}
