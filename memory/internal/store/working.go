package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dexiask/memory/internal/pivot"
)

const processedFile = ".processed.json"

var workingHeaderRe = regexp.MustCompile(`(?m)^## \d{2}:\d{2}:\d{2}`)

// Working is the append-only daily-observation store, under the shared
// per-workspace layout <root>/<ws>/memory/working/<pivot>/<pivot_id>/YYYY-MM-DD.md.
type Working struct {
	root        string
	ws          string
	workingRoot string
	scope       pivot.RequestScope
	reg         *pivot.Registry
}

// NewWorking builds a working store for the request's workspace.
func NewWorking(root string, scope pivot.RequestScope, reg *pivot.Registry) *Working {
	ws := WorkspaceScope(scope.WorkspaceID)
	return &Working{
		root:        root,
		ws:          ws,
		workingRoot: filepath.Join(root, ws, "memory", "working"),
		scope:       scope,
		reg:         reg,
	}
}

func (w *Working) scopeDir(scopeName, scopeID string) (string, bool) {
	p, ok := w.reg.Get(scopeName)
	if !ok {
		return "", false
	}
	id, ok := p.ResolveID(w.scope, scopeID)
	if !ok {
		return "", false
	}
	return ScopeDir(w.root, w.ws, "working", p, id), true
}

// Append records an observation to today's daily file. Returns an error string on
// failure, "" on success.
func (w *Working) Append(scope, scopeID, observation, conversationID string) string {
	sdir, ok := w.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s' or scope_id.", scope)
	}
	now := time.Now().UTC()
	filename := now.Format("2006-01-02") + ".md"
	timeStr := now.Format("15:04:05")
	convTag := ""
	if conversationID != "" {
		convTag = " — " + conversationID
	}
	block := fmt.Sprintf("\n## %s%s\n%s\n", timeStr, convTag, rstrip(observation))
	if err := appendLocked(filepath.Join(sdir, filename), block); err != nil {
		return "Error: " + err.Error()
	}
	return ""
}

// ReadPending concatenates daily files chronologically. When onlyUnprocessed is
// true, each file is read only from its recorded processed offset onward.
func (w *Working) ReadPending(scope, scopeID string, onlyUnprocessed bool) string {
	sdir, ok := w.scopeDir(scope, scopeID)
	if !ok || !isDir(sdir) {
		return ""
	}
	var marks map[string]int
	if onlyUnprocessed {
		marks = w.readMarks(sdir)
	}
	var parts []string
	for _, name := range sortedMarkdown(sdir) {
		text := readText(filepath.Join(sdir, name))
		start := marks[name]
		if start >= len(text) {
			continue
		}
		chunk := text[start:]
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("# %s\n%s", stem(name), chunk))
	}
	return strings.Join(parts, "\n")
}

// ScopePair identifies a (scope, scope_id) that holds working files.
type ScopePair struct {
	Scope   string
	ScopeID string
}

// ListScopes enumerates (scope, scope_id) pairs with working observations.
func (w *Working) ListScopes() []ScopePair {
	out := []ScopePair{}
	for _, p := range w.reg.All() {
		base := filepath.Join(w.workingRoot, p.Name)
		if !isDir(base) {
			continue
		}
		if p.SingletonID != "" {
			if len(sortedMarkdown(base)) > 0 {
				out = append(out, ScopePair{Scope: p.Name, ScopeID: ""})
			}
			continue
		}
		for _, name := range sortedSubdirs(base) {
			if len(sortedMarkdown(filepath.Join(base, name))) > 0 {
				out = append(out, ScopePair{Scope: p.Name, ScopeID: name})
			}
		}
	}
	return out
}

// ListFiles returns metadata for each daily file, newest-first.
func (w *Working) ListFiles(scope, scopeID string) []WorkingFileInfo {
	out := []WorkingFileInfo{}
	sdir, ok := w.scopeDir(scope, scopeID)
	if !ok || !isDir(sdir) {
		return out
	}
	names := sortedMarkdown(sdir)
	for i := len(names) - 1; i >= 0; i-- {
		name := names[i]
		text := readText(filepath.Join(sdir, name))
		count := len(workingHeaderRe.FindAllString(text, -1))
		preview := ""
		for _, line := range strings.Split(text, "\n") {
			s := strings.TrimSpace(line)
			if s != "" && !strings.HasPrefix(line, "#") {
				preview = s
				break
			}
		}
		if len(preview) > 120 {
			preview = preview[:120]
		}
		out = append(out, WorkingFileInfo{Filename: name, Date: stem(name), EntryCount: count, Preview: preview})
	}
	return out
}

// GetFile returns the content of a single daily file, or ("", false) if absent or
// outside the scope.
func (w *Working) GetFile(scope, scopeID, filename string) (string, bool) {
	sdir, ok := w.scopeDir(scope, scopeID)
	if !ok {
		return "", false
	}
	resolved, ok := SafeResolve(sdir, filename)
	if !ok {
		return "", false
	}
	if !isFile(resolved) {
		return "", false
	}
	return readText(resolved), true
}

// MarkProcessed records each working file as consolidated up to its current
// length, without deleting it. filenames=nil marks every daily file.
func (w *Working) MarkProcessed(scope, scopeID string, filenames []string) int {
	sdir, ok := w.scopeDir(scope, scopeID)
	if !ok || !isDir(sdir) {
		return 0
	}
	marks := w.readMarks(sdir)
	var names []string
	if filenames == nil {
		names = sortedMarkdown(sdir)
	} else {
		names = filenames
	}
	count := 0
	for _, n := range names {
		resolved, ok := SafeResolve(sdir, n)
		if !ok || !isFile(resolved) {
			continue
		}
		marks[filepath.Base(resolved)] = len(readText(resolved))
		count++
	}
	w.writeMarks(sdir, marks)
	return count
}

func (w *Working) readMarks(sdir string) map[string]int {
	p := filepath.Join(sdir, processedFile)
	b, err := os.ReadFile(p)
	if err != nil {
		return map[string]int{}
	}
	var raw map[string]json.Number
	if json.Unmarshal(b, &raw) != nil {
		return map[string]int{}
	}
	marks := make(map[string]int, len(raw))
	for k, v := range raw {
		if iv, err := v.Int64(); err == nil {
			marks[k] = int(iv)
		}
	}
	return marks
}

func (w *Working) writeMarks(sdir string, marks map[string]int) {
	if marks == nil {
		marks = map[string]int{}
	}
	b, err := json.MarshalIndent(marks, "", "  ")
	if err != nil {
		return
	}
	tmp := filepath.Join(sdir, processedFile+".tmp")
	if os.WriteFile(tmp, b, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, filepath.Join(sdir, processedFile))
}
