// Package digest assembles a memory digest for injection into a system prompt.
// Scopes are ordered by Precedence (most-specific first) and trimmed to a token
// budget with per-scope minimum guarantees.
package digest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dexiask/memory/internal/pivot"
	"github.com/dexiask/memory/internal/store"
)

const charsPerToken = 4

type scopedEntry struct {
	scope   string
	id      string
	minimum int
	lines   []string
	taken   []string
}

// Build returns the markdown digest (a "## Memory" section) for the request's
// workspace, or "" when no memory exists. tokenBudget controls truncation.
func Build(root string, scope pivot.RequestScope, reg *pivot.Registry, tokenBudget int) string {
	ws := store.WorkspaceScope(scope.WorkspaceID)
	memories := filepath.Join(root, ws, "memory", "memories")

	var entries []*scopedEntry
	for _, p := range reg.DigestOrder() {
		type dir struct {
			id   string
			path string
		}
		var dirs []dir
		switch {
		case p.SingletonID != "":
			dirs = append(dirs, dir{p.SingletonID, filepath.Join(memories, p.Name)})
		case p.UserKeyed:
			if id, ok := p.ResolveID(scope, ""); ok {
				d := filepath.Join(memories, p.Name, id)
				if isDir(d) {
					dirs = append(dirs, dir{id, d})
				}
			}
		default: // enumerate (repo)
			base := filepath.Join(memories, p.Name)
			for _, name := range sortedSubdirs(base) {
				dirs = append(dirs, dir{name, filepath.Join(base, name)})
			}
		}
		for _, d := range dirs {
			index := filepath.Join(d.path, "INDEX.md")
			b, err := os.ReadFile(index)
			if err != nil {
				continue
			}
			var lines []string
			for _, ln := range strings.Split(string(b), "\n") {
				if strings.TrimSpace(ln) != "" {
					lines = append(lines, ln)
				}
			}
			if len(lines) > 0 {
				m := p.DigestMinLines
				if m == 0 {
					m = 5
				}
				entries = append(entries, &scopedEntry{scope: p.Name, id: d.id, minimum: m, lines: lines})
			}
		}
	}

	if len(entries) == 0 {
		return ""
	}

	charBudget := tokenBudget * charsPerToken
	truncated := false

	// Phase 1: guarantee per-scope minimums.
	charsUsed := 0
	for _, e := range entries {
		take := e.lines
		if len(take) > e.minimum {
			take = take[:e.minimum]
			truncated = true
		}
		e.taken = append([]string{}, take...)
		for _, ln := range take {
			charsUsed += len(ln)
		}
		charsUsed += len(take) // newlines
	}

	// Phase 2: fill remaining budget from the overflow, in scope-priority order.
	remaining := charBudget - charsUsed
	if remaining > 0 {
		type ov struct {
			idx  int
			line string
		}
		var overflow []ov
		for i, e := range entries {
			for _, ln := range e.lines[min(e.minimum, len(e.lines)):] {
				overflow = append(overflow, ov{i, ln})
			}
		}
		for _, o := range overflow {
			cost := len(o.line) + 1
			if cost > remaining {
				truncated = true
				break
			}
			entries[o.idx].taken = append(entries[o.idx].taken, o.line)
			remaining -= cost
		}
	}

	// Render.
	var sections []string
	for _, e := range entries {
		if len(e.taken) == 0 {
			continue
		}
		header := "### " + e.scope + " / " + e.id
		if e.scope == "global" {
			header = "### global"
		}
		sections = append(sections, header+"\n"+strings.Join(e.taken, "\n"))
	}
	if len(sections) == 0 {
		return ""
	}
	result := "\n\n## Memory\n\n" + strings.Join(sections, "\n\n")
	if truncated {
		result += "\n\n_More memory exists — use memory_search to find specific entries._"
	}
	return result
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func sortedSubdirs(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}
