package agent

import (
	"context"
	"testing"
	"time"

	"github.com/dexiask/dexiask/internal/pkg/logger"
)

func newTestRunManager() *RunManager {
	return NewRunManager(logger.NewNop())
}

// TestRunManager_BufferAndReplay verifies a late subscriber replays the full
// event buffer from index 0.
func TestRunManager_BufferAndReplay(t *testing.T) {
	m := newTestRunManager()
	done := make(chan struct{})
	run := m.Start("conv-1", func(ctx context.Context, emit func(Event)) {
		emit(Event{Type: "text.delta", Text: "a"})
		emit(Event{Type: "text.delta", Text: "b"})
		emit(Event{Type: "result", Status: "ok", SessionID: "s1"})
		close(done)
	})
	<-done
	// Give the run goroutine a moment to mark done.
	waitFor(t, func() bool { return run.IsDone() })

	// Subscribe after completion: should replay all 3 buffered events then close.
	sub := run.Subscribe(context.Background(), 0)
	var got []Event
	for ev := range sub {
		got = append(got, ev)
	}
	if len(got) != 3 {
		t.Fatalf("replay got %d events, want 3: %+v", len(got), got)
	}
	if got[0].Text != "a" || got[2].SessionID != "s1" {
		t.Fatalf("unexpected replay: %+v", got)
	}
}

// TestRunManager_ResumeFromIndex verifies Last-Event-ID style resume skips
// already-delivered events.
func TestRunManager_ResumeFromIndex(t *testing.T) {
	m := newTestRunManager()
	done := make(chan struct{})
	run := m.Start("conv-2", func(ctx context.Context, emit func(Event)) {
		for i := 0; i < 5; i++ {
			emit(Event{Type: "text.delta", Text: string(rune('0' + i))})
		}
		close(done)
	})
	<-done
	waitFor(t, func() bool { return run.IsDone() })

	sub := run.Subscribe(context.Background(), 2) // resume after index 1
	var got []Event
	for ev := range sub {
		got = append(got, ev)
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	if got[0].Text != "2" {
		t.Fatalf("resume started at %q, want '2'", got[0].Text)
	}
}

// TestRunManager_LiveStreaming verifies a subscriber attached before completion
// receives events as they are emitted.
func TestRunManager_LiveStreaming(t *testing.T) {
	m := newTestRunManager()
	release := make(chan struct{})
	run := m.Start("conv-3", func(ctx context.Context, emit func(Event)) {
		emit(Event{Type: "text.delta", Text: "first"})
		<-release
		emit(Event{Type: "result", Status: "ok"})
	})

	sub := run.Subscribe(context.Background(), 0)
	first := <-sub
	if first.Text != "first" {
		t.Fatalf("first event = %+v", first)
	}
	close(release)
	second := <-sub
	if second.Type != "result" {
		t.Fatalf("second event = %+v", second)
	}
}

// TestRunManager_Stop cancels an in-flight run's context.
func TestRunManager_Stop(t *testing.T) {
	m := newTestRunManager()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	m.Start("conv-4", func(ctx context.Context, emit func(Event)) {
		close(started)
		<-ctx.Done()
		close(cancelled)
	})
	<-started
	if !m.Stop("conv-4") {
		t.Fatal("Stop returned false for live run")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("run context was not cancelled by Stop")
	}
	if m.Stop("no-such-conv") {
		t.Fatal("Stop returned true for unknown conversation")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}
