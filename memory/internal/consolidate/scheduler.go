package consolidate

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/dexiask/memory/internal/config"
)

// Scheduler periodically consolidates the single workspace. An FS flock guards
// each tick so overlapping or multi-process runs are skipped rather than queued.
type Scheduler struct {
	cfg      *config.Config
	svc      *Service
	lockPath string
}

// NewScheduler wires the dream scheduler.
func NewScheduler(cfg *config.Config, svc *Service) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		svc:      svc,
		lockPath: filepath.Join(cfg.Root, ".locks", "dream.lock"),
	}
}

// Run blocks, firing one consolidation sweep every DreamInterval until ctx is
// cancelled. A zero/absent interval or missing engine URL disables the loop.
func (s *Scheduler) Run(ctx context.Context) {
	if s.cfg.DreamInterval <= 0 || s.cfg.AgentURL == "" {
		log.Printf("dream scheduler disabled (interval=%s, agent_url=%q)", s.cfg.DreamInterval, s.cfg.AgentURL)
		return
	}
	log.Printf("dream scheduler started: interval=%s", s.cfg.DreamInterval)
	ticker := time.NewTicker(s.cfg.DreamInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	if err := os.MkdirAll(filepath.Dir(s.lockPath), 0o755); err != nil {
		log.Printf("dream tick: cannot create lock dir: %v", err)
		return
	}
	fl := flock.New(s.lockPath)
	locked, err := fl.TryLock()
	if err != nil {
		log.Printf("dream tick: lock error: %v", err)
		return
	}
	if !locked {
		return // another run holds the lock — skip this tick
	}
	defer func() { _ = fl.Unlock() }()

	if err := s.svc.Run(ctx, config.FixedWorkspaceID); err != nil {
		log.Printf("dream consolidation failed: %v", err)
	}
}
