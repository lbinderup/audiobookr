// audiobookr — self-hosted audiobook converter.
// Copyright (C) 2026 Laurids Binderup
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
// or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License
// for more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <https://www.gnu.org/licenses/>.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sync"

	"audiobookr/internal/config"
	"audiobookr/internal/metadata"
	"audiobookr/internal/metadata/audnexus"
	"audiobookr/internal/pipeline"
	"audiobookr/internal/queue"
	"audiobookr/internal/store"
	"audiobookr/internal/web"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

// chapterSourceFactory builds per-URL chapter sources that consult the
// metadata cache before hitting the provider.
func chapterSourceFactory(st *store.Store, version string) func(url string) pipeline.ChapterSource {
	var mu sync.Mutex
	providers := map[string]*audnexus.Provider{}
	return func(url string) pipeline.ChapterSource {
		mu.Lock()
		p, ok := providers[url]
		if !ok {
			p = audnexus.New(url, "audiobookr/"+version)
			providers[url] = p
		}
		mu.Unlock()
		return cachedChapters{st: st, prov: p}
	}
}

type cachedChapters struct {
	st   *store.Store
	prov *audnexus.Provider
}

func (c cachedChapters) GetChapters(ctx context.Context, asin, region string) (*metadata.ChapterInfo, error) {
	if ch, err := c.st.CachedChapters(asin, region); err == nil && ch != nil {
		return ch, nil
	}
	ch, err := c.prov.GetChapters(ctx, asin, region)
	if err != nil {
		return nil, err
	}
	c.st.CacheChapters(asin, region, ch)
	return ch, nil
}

func main() {
	cfg := config.Load(version)
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	for _, dir := range []string{cfg.ConfigDir, cfg.LogsDir(), cfg.WorkDir(), cfg.InputDir, cfg.OutputDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Error("create directory", "dir", dir, "err", err)
			os.Exit(1)
		}
	}

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Error("open database", "path", cfg.DBPath(), "err", err)
		os.Exit(1)
	}
	defer st.Close()

	var conv pipeline.Converter
	if cfg.Converter == "fake" {
		log.Warn("using the FAKE converter — no real audio is produced (set CONVERTER=real)")
		conv = &pipeline.FakeConverter{}
	} else {
		conv = &pipeline.RealConverter{
			FFmpeg:   cfg.FFmpegPath,
			FFprobe:  cfg.FFprobePath,
			Tone:     cfg.TonePath,
			Fdkaac:   cfg.FdkaacPath,
			WorkRoot: cfg.WorkDir(),
			Chapters: chapterSourceFactory(st, version),
		}
	}

	set, err := st.GetSettings(store.Defaults(cfg.InputDir, cfg.OutputDir))
	if err != nil {
		log.Error("load settings", "err", err)
		os.Exit(1)
	}
	broker := queue.NewBroker()
	mgr := queue.NewManager(st, conv, broker, cfg.LogsDir(), set.WorkerConcurrency, log)

	workCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	mgr.Start(workCtx)

	srv := web.New(cfg, st, mgr, broker)

	go func() {
		log.Info("audiobookr listening", "addr", srv.Addr, "version", version, "dev", cfg.Dev)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
	stopWorkers()
	mgr.Wait()
}
