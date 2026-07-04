package agent

import (
	"context"
	"sync"
	"time"

	"github.com/dexiask/dexiask/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	// MaxRunDuration is the maximum time a run may take before its context is cancelled.
	MaxRunDuration = 10 * time.Minute
	// RunEvictionTTL is how long a completed run remains in memory after it
	// finishes, so late reconnects can still replay the full event buffer.
	RunEvictionTTL = 60 * time.Second
)

// Run holds the state of an active or recently-completed agent generation.
// It is keyed by conversation ID inside RunManager.
type Run struct {
	mu     sync.Mutex
	events []Event // append-only; read under lock via slice header snapshot
	done   bool
	ready  chan struct{} // closed-and-replaced to broadcast new events or done
	cancel context.CancelFunc
}

// appendAndBroadcast appends ev to the event buffer and wakes all subscribers.
func (r *Run) appendAndBroadcast(ev Event) {
	r.mu.Lock()
	r.events = append(r.events, ev)
	old := r.ready
	r.ready = make(chan struct{})
	r.mu.Unlock()
	close(old)
}

// markDoneAndBroadcast marks the run as finished and wakes all subscribers.
func (r *Run) markDoneAndBroadcast() {
	r.mu.Lock()
	r.done = true
	old := r.ready
	r.ready = make(chan struct{})
	r.mu.Unlock()
	close(old)
}

// Subscribe returns a channel of events starting at fromIndex. The channel is
// closed when the run is finished and all events from fromIndex onwards have
// been delivered. Cancelling ctx unsubscribes without stopping the run.
func (r *Run) Subscribe(ctx context.Context, fromIndex int) <-chan Event {
	out := make(chan Event, 32)
	go func() {
		defer close(out)
		cursor := fromIndex
		for {
			r.mu.Lock()
			events := r.events
			done := r.done
			ready := r.ready
			r.mu.Unlock()

			for cursor < len(events) {
				select {
				case out <- events[cursor]:
					cursor++
				case <-ctx.Done():
					return
				}
			}

			if done {
				r.mu.Lock()
				total := len(r.events)
				r.mu.Unlock()
				if cursor >= total {
					return
				}
				continue
			}

			select {
			case <-ready:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// IsDone returns true if the run has completed.
func (r *Run) IsDone() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

// EventCount returns the number of events buffered so far.
func (r *Run) EventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// Cancel cancels the run context (stops generation).
func (r *Run) Cancel() { r.cancel() }

// RunManager manages active agent runs, keyed by conversation ID. Chat is
// sequential per thread, so at most one live run exists per conversation.
type RunManager struct {
	mu     sync.RWMutex
	runs   map[string]*Run
	logger *logger.Logger
}

// NewRunManager creates a new RunManager.
func NewRunManager(log *logger.Logger) *RunManager {
	return &RunManager{
		runs:   make(map[string]*Run),
		logger: log,
	}
}

// Start launches a new run for convID and returns it. If a live (not-done) run
// already exists it is cancelled and removed first.
//
// runFn is called in a goroutine with a run-scoped context (never the HTTP
// request context) and an emit func. runFn drains the engine channel and calls
// emit for each Event. When runFn returns, the run is marked done.
func (m *RunManager) Start(convID string, runFn func(ctx context.Context, emit func(Event))) *Run {
	m.mu.Lock()
	if existing, ok := m.runs[convID]; ok && !existing.IsDone() {
		existing.cancel()
		delete(m.runs, convID)
		m.logger.Warn("cancelled orphaned run to allow new turn",
			zap.String("conversation_id", convID))
	}

	ctx, cancel := context.WithTimeout(context.Background(), MaxRunDuration)
	run := &Run{
		cancel: cancel,
		ready:  make(chan struct{}),
	}
	m.runs[convID] = run
	m.mu.Unlock()

	m.logger.Info("run started", zap.String("conversation_id", convID))

	go func() {
		defer func() {
			cancel()
			run.markDoneAndBroadcast()
			m.logger.Info("run finished", zap.String("conversation_id", convID))

			time.AfterFunc(RunEvictionTTL, func() {
				m.mu.Lock()
				if r, ok := m.runs[convID]; ok && r == run {
					delete(m.runs, convID)
				}
				m.mu.Unlock()
			})
		}()

		runFn(ctx, run.appendAndBroadcast)
	}()

	return run
}

// Get returns the run for convID if it exists (live or within eviction TTL).
func (m *RunManager) Get(convID string) (*Run, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.runs[convID]
	return r, ok
}

// Stop cancels the run for convID. Returns true if a run was found.
func (m *RunManager) Stop(convID string) bool {
	m.mu.RLock()
	run, ok := m.runs[convID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	run.cancel()
	m.logger.Info("run stopped by request", zap.String("conversation_id", convID))
	return true
}
