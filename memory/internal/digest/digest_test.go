package digest

import (
	"strings"
	"testing"

	"github.com/dexiask/memory/internal/pivot"
	"github.com/dexiask/memory/internal/store"
)

func TestBuildDigest(t *testing.T) {
	root := t.TempDir()
	reg := pivot.Default()
	sc := pivot.RequestScope{WorkspaceID: "dexiask", UserID: "u1"}

	// Seed a consolidated entry in the user scope so an INDEX.md exists.
	c := store.NewConsolidated(root, sc, reg, []string{"user"})
	c.Create("user", "", "prefs", "style", "Prefers concise answers.")

	d := Build(root, sc, reg, 2000)
	if !strings.Contains(d, "## Memory") {
		t.Fatalf("digest missing header: %q", d)
	}
	if !strings.Contains(d, "user / u1") {
		t.Fatalf("digest missing user scope: %q", d)
	}
}

func TestBuildDigestEmpty(t *testing.T) {
	sc := pivot.RequestScope{WorkspaceID: "dexiask", UserID: "u1"}
	if d := Build(t.TempDir(), sc, pivot.Default(), 2000); d != "" {
		t.Fatalf("expected empty digest, got %q", d)
	}
}
