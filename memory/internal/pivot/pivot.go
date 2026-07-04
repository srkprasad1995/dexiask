// Package pivot defines the memory scope registry. Dexiask has three scopes:
// global (shared), user (per GitHub user), and repo (per indexed repo). Adding a
// scope is a one-line registry entry; every part of the service that enumerates
// or resolves scopes iterates the registry.
package pivot

import (
	"sort"
	"strings"
)

// RequestScope carries the per-request identity parsed from the inbound headers
// (X-Workspace-Id / X-User-Id / X-Role / X-Writable-Scopes).
type RequestScope struct {
	WorkspaceID string   // raw workspace id (sanitized to a path segment by store.WorkspaceScope)
	UserID      string   // X-User-Id, defaults to "default"
	Role        string   // X-Role, drives dream routing + writable-scope defaults
	Writable    []string // X-Writable-Scopes (CSV); nil when the header is absent
}

// Pivot is one scope dimension. Precedence orders pivots most-specific-first for
// the digest (lower = higher priority). DigestMinLines is the per-scope minimum
// guaranteed before budget trimming. SingletonID is non-empty for fixed scopes
// (global) that live at <memories>/<name> with no id subdirectory. UserKeyed
// marks the scope whose id comes from the request identity.
type Pivot struct {
	Name           string
	Precedence     int
	DigestMinLines int
	SingletonID    string
	UserKeyed      bool
	ResolveID      func(scope RequestScope, given string) (id string, ok bool)
}

// Registry is the ordered set of pivots.
type Registry struct {
	order  []*Pivot
	byName map[string]*Pivot
}

// Get returns the pivot with the given name.
func (r *Registry) Get(name string) (*Pivot, bool) {
	p, ok := r.byName[name]
	return p, ok
}

// All returns the pivots in listing order (global, user, repo).
func (r *Registry) All() []*Pivot { return r.order }

// DigestOrder returns the pivots sorted by Precedence ascending (most-specific
// first): repo, user, global.
func (r *Registry) DigestOrder() []*Pivot {
	out := make([]*Pivot, len(r.order))
	copy(out, r.order)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Precedence < out[j].Precedence })
	return out
}

// SanitizeID strips whitespace, replaces '/', '\\' and '..' with '_', then
// rejects empty or dotfile-leading ids.
func SanitizeID(raw string) (string, bool) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")
	cleaned = strings.ReplaceAll(cleaned, "..", "_")
	if cleaned == "" || strings.HasPrefix(cleaned, ".") {
		return "", false
	}
	return cleaned, true
}

func resolveGlobal(_ RequestScope, _ string) (string, bool) { return "global", true }

func resolveUser(s RequestScope, given string) (string, bool) {
	id := given
	if id == "" {
		id = s.UserID
	}
	return SanitizeID(id)
}

func resolvePlain(_ RequestScope, given string) (string, bool) {
	return SanitizeID(given)
}

// Default returns the registry seeded with the three built-in pivots.
func Default() *Registry {
	pivots := []*Pivot{
		{Name: "global", Precedence: 2, DigestMinLines: 5, SingletonID: "global", ResolveID: resolveGlobal},
		{Name: "user", Precedence: 1, DigestMinLines: 10, UserKeyed: true, ResolveID: resolveUser},
		{Name: "repo", Precedence: 0, DigestMinLines: 20, ResolveID: resolvePlain},
	}
	r := &Registry{order: pivots, byName: make(map[string]*Pivot, len(pivots))}
	for _, p := range pivots {
		r.byName[p.Name] = p
	}
	return r
}

// WritableScopesForRole returns the default writable scope set for a role, used
// when the X-Writable-Scopes header is absent. Chat roles may only record
// user/repo observations; the dream role curates every scope.
func WritableScopesForRole(role string) []string {
	if role == "dream" {
		return []string{"repo", "user", "global"}
	}
	return []string{"repo", "user"}
}
