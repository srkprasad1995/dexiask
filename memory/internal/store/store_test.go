package store

import (
	"strings"
	"testing"

	"github.com/dexiask/memory/internal/pivot"
)

func scope() pivot.RequestScope {
	return pivot.RequestScope{WorkspaceID: "dexiask", UserID: "u1"}
}

func TestConsolidatedCreateViewSearch(t *testing.T) {
	root := t.TempDir()
	reg := pivot.Default()
	// A chat role cannot write consolidated entries; grant the dream's set here.
	c := NewConsolidated(root, scope(), reg, []string{"repo", "user", "global"})

	msg := c.Create("repo", "myrepo", "conventions", "go-errors", "Wrap errors with %w.")
	if strings.HasPrefix(msg, "Error") {
		t.Fatalf("create failed: %s", msg)
	}

	// The scope index now lists the topic.
	if idx := c.View("repo", "myrepo", ""); !strings.Contains(idx, "conventions") {
		t.Fatalf("index missing topic: %q", idx)
	}
	// Reading the entry returns its body.
	if body := c.View("repo", "myrepo", "conventions/go-errors.md"); !strings.Contains(body, "Wrap errors") {
		t.Fatalf("entry body wrong: %q", body)
	}
	// Search finds it by content.
	if hit := c.Search("wrap"); !strings.Contains(hit, "conventions/go-errors.md") {
		t.Fatalf("search missed entry: %q", hit)
	}
	// GetEntry returns the structured record.
	if e := c.GetEntry("repo", "myrepo", "conventions", "go-errors"); e == nil || e.Topic != "conventions" {
		t.Fatalf("GetEntry = %+v", e)
	}
}

func TestConsolidatedReadOnlyScope(t *testing.T) {
	root := t.TempDir()
	reg := pivot.Default()
	// Only "repo" is writable — a global write must be refused.
	c := NewConsolidated(root, scope(), reg, []string{"repo"})
	if msg := c.Create("global", "global", "t", "e", "x"); !strings.Contains(msg, "read-only") {
		t.Fatalf("expected read-only refusal, got %q", msg)
	}
}

func TestConsolidatedArchiveRoundTrip(t *testing.T) {
	root := t.TempDir()
	c := NewConsolidated(root, scope(), pivot.Default(), []string{"repo"})
	c.Create("repo", "r", "t", "e", "body")
	if msg := c.Delete("repo", "r", "t", "e", "obsolete"); !strings.HasPrefix(msg, "Archived") {
		t.Fatalf("delete = %q", msg)
	}
	// Archived entries drop out of the active listing but survive in .archive.
	if entries := c.ListEntries("repo", "r", false); len(entries) != 0 {
		t.Fatalf("active entries after delete = %d", len(entries))
	}
	if arch := c.ListEntries("repo", "r", true); len(arch) != 1 {
		t.Fatalf("archived entries = %d", len(arch))
	}
	if msg := c.Restore("repo", "r", "t", "e"); !strings.HasPrefix(msg, "Restored") {
		t.Fatalf("restore = %q", msg)
	}
	if entries := c.ListEntries("repo", "r", false); len(entries) != 1 {
		t.Fatalf("active entries after restore = %d", len(entries))
	}
}

func TestWorkingAppendReadMarkProcessed(t *testing.T) {
	root := t.TempDir()
	reg := pivot.Default()
	w := NewWorking(root, scope(), reg)

	// user scope_id blank → resolves to the request identity "u1".
	if err := w.Append("user", "", "user prefers Go error wrapping", ""); err != "" {
		t.Fatalf("append: %s", err)
	}
	pending := w.ReadPending("user", "", true)
	if !strings.Contains(pending, "error wrapping") {
		t.Fatalf("pending missing observation: %q", pending)
	}

	// ListScopes surfaces the user scope with observations.
	found := false
	for _, sp := range w.ListScopes() {
		if sp.Scope == "user" && sp.ScopeID == "u1" {
			found = true
		}
	}
	if !found {
		t.Fatal("ListScopes did not surface user/u1")
	}

	// After MarkProcessed, the same observations are no longer pending.
	if n := w.MarkProcessed("user", "", nil); n != 1 {
		t.Fatalf("marked %d files, want 1", n)
	}
	if again := w.ReadPending("user", "", true); strings.TrimSpace(again) != "" {
		t.Fatalf("expected no pending after mark, got %q", again)
	}
}
