// Package web is the HTTP layer: routes, handlers, templates and static
// assets. Pages are server-rendered html/template; htmx swaps partial
// fragments for interactivity.
package web

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"audiobookr/internal/config"
	"audiobookr/internal/metadata"
	"audiobookr/internal/metadata/audnexus"
	"audiobookr/internal/queue"
	"audiobookr/internal/store"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	render *renderer
	queue  *queue.Manager
	broker *queue.Broker

	provMu  sync.Mutex
	prov    metadata.Provider
	provURL string

	durations *durationCache
}

func New(cfg config.Config, st *store.Store, mgr *queue.Manager, broker *queue.Broker) *http.Server {
	s := &Server{
		cfg:       cfg,
		store:     st,
		render:    newRenderer(cfg.Dev),
		queue:     mgr,
		broker:    broker,
		durations: newDurationCache(),
	}
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// defaults returns the settings defaults seeded from process config.
func (s *Server) defaults() store.Settings {
	return store.Defaults(s.cfg.InputDir, s.cfg.OutputDir)
}

// settings loads current settings or falls back to defaults on DB error.
func (s *Server) settings() store.Settings {
	set, err := s.store.GetSettings(s.defaults())
	if err != nil {
		return s.defaults()
	}
	return set
}

// provider returns the metadata provider for the current settings, rebuilding
// it only when the configured base URL changes.
func (s *Server) provider() metadata.Provider {
	set := s.settings()
	s.provMu.Lock()
	defer s.provMu.Unlock()
	if s.prov == nil || s.provURL != set.AudnexusURL {
		s.prov = audnexus.New(set.AudnexusURL, "audiobookr/"+s.cfg.Version)
		s.provURL = set.AudnexusURL
	}
	return s.prov
}

// baseData is embedded in every page's template data.
type baseData struct {
	Title   string
	Active  string // nav highlight: import|queue|settings
	Version string
	Flash   string
	Error   string
}

func (s *Server) base(title, active string) baseData {
	return baseData{Title: title, Active: active, Version: s.cfg.Version}
}
