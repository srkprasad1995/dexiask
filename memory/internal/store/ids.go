package store

import (
	"path/filepath"
	"strings"
)

// WorkspaceScope returns a filesystem-safe single path segment identifying a
// workspace: any character outside [A-Za-z0-9_-] becomes '-', and an empty id
// falls back to "demo".
func WorkspaceScope(workspaceID string) string {
	s := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, workspaceID)
	if s == "" {
		return "demo"
	}
	return s
}

// SafeResolve resolves a relative path under scopeRoot, denying path traversal.
// Percent-encoded dot/slash/backslash sequences are decoded first so an attacker
// cannot smuggle ".." past the check, then the cleaned candidate must stay within
// scopeRoot. Returns ("", false) when the path escapes the scope.
func SafeResolve(scopeRoot, relPath string) (string, bool) {
	decoded := relPath
	for _, r := range []struct{ from, to string }{
		{"%2e", "."}, {"%2E", "."},
		{"%2f", "/"}, {"%2F", "/"},
		{"%5c", "\\"}, {"%5C", "\\"},
	} {
		decoded = strings.ReplaceAll(decoded, r.from, r.to)
	}
	rel := strings.TrimLeft(decoded, "/")
	root := filepath.Clean(scopeRoot)
	candidate := filepath.Clean(filepath.Join(root, rel))
	if candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		return candidate, true
	}
	return "", false
}
