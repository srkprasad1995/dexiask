package pivot

import (
	"reflect"
	"sort"
	"testing"
)

func TestDefaultRegistryHasThreeScopes(t *testing.T) {
	r := Default()
	got := make([]string, 0, 3)
	for _, p := range r.All() {
		got = append(got, p.Name)
	}
	want := []string{"global", "user", "repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
}

func TestDigestOrderMostSpecificFirst(t *testing.T) {
	r := Default()
	var got []string
	for _, p := range r.DigestOrder() {
		got = append(got, p.Name)
	}
	want := []string{"repo", "user", "global"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("digest order = %v, want %v", got, want)
	}
}

func TestResolveIDs(t *testing.T) {
	r := Default()
	sc := RequestScope{UserID: "u1"}

	// global is a singleton.
	global, _ := r.Get("global")
	if id, ok := global.ResolveID(sc, ""); !ok || id != "global" {
		t.Fatalf("global resolve = %q,%v", id, ok)
	}
	// user defaults to the request identity when scope_id is blank.
	user, _ := r.Get("user")
	if id, ok := user.ResolveID(sc, ""); !ok || id != "u1" {
		t.Fatalf("user resolve = %q,%v", id, ok)
	}
	// repo requires an explicit id.
	repo, _ := r.Get("repo")
	if _, ok := repo.ResolveID(sc, ""); ok {
		t.Fatal("repo must not resolve without an id")
	}
	if id, ok := repo.ResolveID(sc, "myrepo"); !ok || id != "myrepo" {
		t.Fatalf("repo resolve = %q,%v", id, ok)
	}
}

func TestWritableScopesForRole(t *testing.T) {
	chat := WritableScopesForRole("ask")
	sort.Strings(chat)
	if !reflect.DeepEqual(chat, []string{"repo", "user"}) {
		t.Fatalf("chat writable = %v", chat)
	}
	dream := WritableScopesForRole("dream")
	sort.Strings(dream)
	if !reflect.DeepEqual(dream, []string{"global", "repo", "user"}) {
		t.Fatalf("dream writable = %v", dream)
	}
}

func TestSanitizeID(t *testing.T) {
	// Separators and ".." are neutralised; empty and dotfile ids are rejected.
	if got, ok := SanitizeID("a/b"); !ok || got != "a_b" {
		t.Fatalf("SanitizeID(a/b) = %q,%v", got, ok)
	}
	if _, ok := SanitizeID(""); ok {
		t.Fatal("empty id must be rejected")
	}
	if _, ok := SanitizeID(".hidden"); ok {
		t.Fatal("dotfile id must be rejected")
	}
	if got, _ := SanitizeID("a/../b"); containsSep(got) {
		t.Fatalf("sanitized id still has a separator: %q", got)
	}
}

func containsSep(s string) bool {
	for _, r := range s {
		if r == '/' || r == '\\' {
			return true
		}
	}
	return false
}
