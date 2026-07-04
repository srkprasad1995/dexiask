package consolidate

import (
	"strings"
	"testing"

	"github.com/dexiask/memory/internal/pivot"
	"github.com/dexiask/memory/internal/store"
)

func TestBuildWorkingContext(t *testing.T) {
	root := t.TempDir()
	reg := pivot.Default()
	sc := pivot.RequestScope{WorkspaceID: "dexiask", UserID: "u1"}
	w := store.NewWorking(root, sc, reg)
	w.Append("user", "u1", "user likes terse answers", "")
	w.Append("repo", "myrepo", "the build uses make", "")

	ctx := BuildWorkingContext(w)
	if !strings.Contains(ctx, "## Pending Working Memory") {
		t.Fatalf("missing header: %q", ctx)
	}
	// Each block names the exact (scope, scope_id) the dream must consolidate into.
	if !strings.Contains(ctx, "scope=user, scope_id=u1") {
		t.Fatalf("missing user consolidate hint: %q", ctx)
	}
	if !strings.Contains(ctx, "scope=repo, scope_id=myrepo") {
		t.Fatalf("missing repo consolidate hint: %q", ctx)
	}
}

func TestBuildWorkingContextEmpty(t *testing.T) {
	sc := pivot.RequestScope{WorkspaceID: "dexiask", UserID: "u1"}
	w := store.NewWorking(t.TempDir(), sc, pivot.Default())
	if ctx := BuildWorkingContext(w); ctx != "" {
		t.Fatalf("expected empty context, got %q", ctx)
	}
}
