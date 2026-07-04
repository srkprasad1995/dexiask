package mcp

import (
	"reflect"
	"sort"
	"testing"

	"github.com/dexiask/memory/internal/pivot"
)

func TestResolveScopeID(t *testing.T) {
	// A chat role's user scope is pinned to its own identity, ignoring any given id.
	if got := resolveScopeID(false, "user", "someone-else", "u1"); got != "u1" {
		t.Fatalf("chat user scope_id = %q, want u1", got)
	}
	// The dream role addresses arbitrary scope ids verbatim.
	if got := resolveScopeID(true, "user", "u2", "system"); got != "u2" {
		t.Fatalf("dream user scope_id = %q, want u2", got)
	}
	// repo scope_id is always taken as given.
	if got := resolveScopeID(false, "repo", "myrepo", "u1"); got != "myrepo" {
		t.Fatalf("repo scope_id = %q, want myrepo", got)
	}
}

func TestWritableFor(t *testing.T) {
	// An explicit header wins.
	if got := writableFor(pivot.RequestScope{Writable: []string{"repo"}}); !reflect.DeepEqual(got, []string{"repo"}) {
		t.Fatalf("explicit writable = %v", got)
	}
	// Otherwise fall back to the role default.
	got := writableFor(pivot.RequestScope{Role: "dream"})
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"global", "repo", "user"}) {
		t.Fatalf("dream default writable = %v", got)
	}
}
