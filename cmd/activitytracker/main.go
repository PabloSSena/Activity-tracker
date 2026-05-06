package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/user/activitytracker/internal/autostart"
	"github.com/user/activitytracker/internal/config"
	"github.com/user/activitytracker/internal/monitor/collector"
	"github.com/user/activitytracker/internal/monitor/idle"
	"github.com/user/activitytracker/internal/monitor/window"
	"github.com/user/activitytracker/internal/report"
	"github.com/user/activitytracker/internal/storage"
	"github.com/user/activitytracker/internal/ui"
	"github.com/user/activitytracker/internal/vscode"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("config: load error (using defaults): %v", err)
	}

	dataDir, err := config.DataDir()
	if err != nil {
		log.Fatalf("config: resolve data dir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatalf("config: create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, "data.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("storage: open: %v", err)
	}
	defer db.Close()

	bgCtx := context.Background()

	if err := db.Migrate(bgCtx); err != nil {
		log.Fatalf("storage: migrate: %v", err)
	}
	if err := db.RecoverCheckpoints(bgCtx, cfg.Monitoring.MinSessionSecs); err != nil {
		log.Printf("storage: recover checkpoints: %v", err)
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	if n, err := db.PurgeOldRawEvents(bgCtx, cutoff); err != nil {
		log.Printf("storage: purge raw events: %v", err)
	} else if n > 0 {
		log.Printf("storage: purged %d old raw events", n)
	}

	if cfg.Autostart.Enabled {
		exePath, err := os.Executable()
		if err != nil {
			log.Printf("autostart: resolve exe path: %v", err)
		} else {
			as := autostart.New("ActivityTracker", exePath)
			if !as.IsEnabled() {
				if err := as.Enable(); err != nil {
					log.Printf("autostart: enable: %v", err)
				}
			}
		}
	}

	pollInterval := time.Duration(cfg.Monitoring.PollIntervalSecs) * time.Second
	windowMon := window.New(pollInterval)
	idleDet := idle.New()

	opts := collector.Options{
		MinSessionSecs:  cfg.Monitoring.MinSessionSecs,
		CheckpointSecs:  cfg.Monitoring.CheckpointSecs,
		IdleTimeoutMins: cfg.Monitoring.IdleTimeoutMins,
	}
	coll := collector.New(windowMon, idleDet, &storeAdapter{db: db, min: cfg.Monitoring.MinSessionSecs}, opts)

	ctx, cancel := context.WithCancel(bgCtx)
	go coll.Run(ctx)

	workspaces := vscode.Discover()
	if len(workspaces) > 0 {
		log.Printf("vscode: discovered %d workspaces", len(workspaces))
	} else {
		log.Printf("vscode: no workspaces discovered — file change list will be empty")
	}
	resolver := func(name string) string { return workspaces[name] }

	gen := report.NewGenerator(report.NewGrouper(cfg.Grouping.BrowserAdjacencyMins)).
		WithWorkspaceResolver(resolver)
	fmtr := report.NewFormatter()
	liveFn := func() *ui.LiveSession {
		s := coll.CurrentSession()
		if s == nil {
			return nil
		}
		return &ui.LiveSession{
			ContextType:  s.ContextType,
			ContextLabel: s.ContextLabel,
			StartUTC:     s.StartUTC,
		}
	}
	srv := ui.NewReportServer(db, gen, fmtr, liveFn)
	if err := srv.Start(ctx); err != nil {
		log.Fatalf("ui: start report server: %v", err)
	}

	app := ui.New(srv)
	app.Run(func() {
		cancel()
	})
}

// storeAdapter bridges collector.Store to storage.Storage.
type storeAdapter struct {
	db  storage.Storage
	min int
}

func (a *storeAdapter) OpenSession(contextType, label string) int64 {
	now := time.Now().UTC()
	s := storage.Session{
		DateLocal:    now.In(time.Local).Format("2006-01-02"),
		ContextType:  contextType,
		ContextLabel: label,
		StartUTC:     now,
	}
	id, err := a.db.OpenSession(bgCtx(), s)
	if err != nil {
		log.Printf("storage: open session: %v", err)
		return 0
	}
	return id
}

func (a *storeAdapter) CloseSession(id int64, durationSecs int) {
	now := time.Now().UTC()
	if err := a.db.CloseSession(bgCtx(), id, now, durationSecs, a.min); err != nil {
		log.Printf("storage: close session %d: %v", id, err)
	}
}

func bgCtx() context.Context { return context.Background() }
