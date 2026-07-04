package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dexiask/memory/internal/pivot"
)

var reservedNames = map[string]bool{"INDEX.md": true, "LOG.md": true}

var logHeaderRe = regexp.MustCompile(`^## (.+?) — (.+)$`)

// Consolidated is the guarded filesystem store for curated memory. All pivots
// live under the shared per-workspace tree <root>/<ws>/memory/memories/<pivot>/...
type Consolidated struct {
	root         string
	ws           string
	memoriesRoot string
	scope        pivot.RequestScope
	reg          *pivot.Registry
	writable     map[string]bool
}

// NewConsolidated builds a consolidated store for the request's workspace.
// writable is the set of pivot names this caller may mutate.
func NewConsolidated(root string, scope pivot.RequestScope, reg *pivot.Registry, writable []string) *Consolidated {
	ws := WorkspaceScope(scope.WorkspaceID)
	w := make(map[string]bool, len(writable))
	for _, s := range writable {
		w[s] = true
	}
	return &Consolidated{
		root:         root,
		ws:           ws,
		memoriesRoot: filepath.Join(root, ws, "memory", "memories"),
		scope:        scope,
		reg:          reg,
		writable:     w,
	}
}

func (c *Consolidated) scopeDir(scopeName, scopeID string) (string, bool) {
	p, ok := c.reg.Get(scopeName)
	if !ok {
		return "", false
	}
	id, ok := p.ResolveID(c.scope, scopeID)
	if !ok {
		return "", false
	}
	return ScopeDir(c.root, c.ws, "memories", p, id), true
}

func (c *Consolidated) checkWritable(scope string) string {
	if !c.writable[scope] {
		return fmt.Sprintf("Error: scope '%s' is read-only for this session.", scope)
	}
	return ""
}

func ensureMD(safeEntry string) string {
	if strings.HasSuffix(safeEntry, ".md") {
		return safeEntry
	}
	return safeEntry + ".md"
}

func rstrip(s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) }

// ------------------------------------------------------------------ reads

// View returns the index, a topic listing, or an entry body.
func (c *Consolidated) View(scope, scopeID, path string) string {
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s/%s'.", scope, scopeID)
	}
	if !pathExists(sd) {
		return fmt.Sprintf("No memory entries in %s/%s yet.", scope, scopeID)
	}
	if path == "" {
		index := filepath.Join(sd, "INDEX.md")
		if isFile(index) {
			return readText(index)
		}
		return c.listTopicsText(sd)
	}
	resolved, ok := SafeResolve(sd, path)
	if !ok {
		return fmt.Sprintf("Error: path '%s' is outside the memory scope. Access denied.", path)
	}
	if !pathExists(resolved) {
		return fmt.Sprintf("Error: '%s' does not exist.", path)
	}
	if isDir(resolved) {
		return listTopicEntriesText(resolved)
	}
	return readText(resolved)
}

func (c *Consolidated) listTopicsText(sd string) string {
	topics := sortedSubdirs(sd)
	if len(topics) == 0 {
		return "(empty)"
	}
	var lines []string
	for _, t := range topics {
		entries := sortedMarkdown(filepath.Join(sd, t))
		lines = append(lines, fmt.Sprintf("- %s/ (%d entries)", t, len(entries)))
	}
	return strings.Join(lines, "\n")
}

func listTopicEntriesText(topicDir string) string {
	names := sortedMarkdown(topicDir)
	if len(names) == 0 {
		return "(empty)"
	}
	var lines []string
	for _, n := range names {
		lines = append(lines, "- "+stem(n))
	}
	return strings.Join(lines, "\n")
}

// Search returns matching entry paths across the workspace's memory tree.
func (c *Consolidated) Search(query string) string {
	if !isDir(c.memoriesRoot) {
		return "No memory entries found."
	}
	var hits []string
	_ = filepath.WalkDir(c.memoriesRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".md") {
			hits = append(hits, p)
		}
		return nil
	})
	if len(hits) == 0 {
		return "No memory entries found."
	}
	sort.Strings(hits)
	lower := strings.ToLower(query)
	var results []string
	truncated := false
	for _, abs := range hits {
		if reservedNames[filepath.Base(abs)] {
			continue
		}
		rel, _ := filepath.Rel(c.memoriesRoot, abs)
		if strings.Contains(rel, ".archive") {
			continue
		}
		if strings.Contains(strings.ToLower(readText(abs)), lower) {
			results = append(results, rel)
		}
		if len(results) >= 20 {
			truncated = true
			break
		}
	}
	if truncated {
		results = append(results, "... (truncated)")
	}
	if len(results) == 0 {
		return fmt.Sprintf("No memory entries matching '%s'.", query)
	}
	return strings.Join(results, "\n")
}

// ListScopes enumerates scopes that exist on disk.
func (c *Consolidated) ListScopes() []ScopeInfo {
	out := []ScopeInfo{}
	for _, p := range c.reg.All() {
		base := filepath.Join(c.memoriesRoot, p.Name)
		if !isDir(base) {
			continue
		}
		if p.SingletonID != "" {
			out = append(out, ScopeInfo{Scope: p.Name, ID: p.SingletonID, Label: titleCase(p.SingletonID)})
			continue
		}
		for _, name := range sortedSubdirs(base) {
			out = append(out, ScopeInfo{Scope: p.Name, ID: name, Label: name})
		}
	}
	return out
}

// ListTopics returns the topic directories with entry counts.
func (c *Consolidated) ListTopics(scope, scopeID string) []TopicInfo {
	out := []TopicInfo{}
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok || !isDir(sd) {
		return out
	}
	for _, t := range sortedSubdirs(sd) {
		names := sortedMarkdown(filepath.Join(sd, t))
		stems := make([]string, 0, len(names))
		for _, n := range names {
			stems = append(stems, stem(n))
		}
		out = append(out, TopicInfo{Topic: t, EntryCount: len(stems), Entries: stems})
	}
	return out
}

// ListEntries returns the listing view for active or archived entries.
func (c *Consolidated) ListEntries(scope, scopeID string, archived bool) []EntryMeta {
	out := []EntryMeta{}
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok || !isDir(sd) {
		return out
	}
	base := sd
	if archived {
		base = filepath.Join(sd, ".archive")
	}
	if !isDir(base) {
		return out
	}
	for _, t := range sortedSubdirs(base) {
		topicDir := filepath.Join(base, t)
		for _, n := range sortedMarkdown(topicDir) {
			preview := firstPreview(readText(filepath.Join(topicDir, n)))
			if preview == "" {
				preview = "(no content)"
			}
			out = append(out, EntryMeta{Topic: t, Entry: stem(n), Scope: scope, ScopeID: scopeID, Preview: preview})
		}
	}
	return out
}

// GetEntry returns a full entry (active or archived), or nil if absent.
func (c *Consolidated) GetEntry(scope, scopeID, topic, entryName string) *EntryFull {
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return nil
	}
	safeTopic, okt := pivot.SanitizeID(topic)
	safeEntry, oke := pivot.SanitizeID(entryName)
	if !okt || !oke {
		return nil
	}
	filename := ensureMD(safeEntry)
	entryPath := filepath.Join(sd, safeTopic, filename)
	if !isFile(entryPath) {
		archived := filepath.Join(sd, ".archive", safeTopic, filename)
		if isFile(archived) {
			entryPath = archived
		} else {
			return nil
		}
	}
	text := readText(entryPath)
	preview := firstPreview(text)
	if preview == "" {
		preview = "(no content)"
	}
	return &EntryFull{
		Topic:   safeTopic,
		Entry:   strings.TrimSuffix(safeEntry, ".md"),
		Scope:   scope,
		ScopeID: scopeID,
		Preview: preview,
		Body:    strings.TrimSpace(text),
	}
}

// ------------------------------------------------------------------ writes

// Create writes a new consolidated entry.
func (c *Consolidated) Create(scope, scopeID, topic, entryName, content string) string {
	if err := c.checkWritable(scope); err != "" {
		return err
	}
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s/%s'.", scope, scopeID)
	}
	safeTopic, okt := pivot.SanitizeID(topic)
	safeEntry, oke := pivot.SanitizeID(entryName)
	if !okt || !oke {
		return "Error: invalid topic or entry name."
	}
	filename := ensureMD(safeEntry)
	topicDir := filepath.Join(sd, safeTopic)
	entryPath := filepath.Join(topicDir, filename)
	if pathExists(entryPath) {
		return fmt.Sprintf("Error: '%s/%s' already exists. Use update instead.", topic, filename)
	}
	if err := os.MkdirAll(topicDir, 0o755); err != nil {
		return "Error: " + err.Error()
	}
	if err := AtomicWrite(entryPath, rstrip(content)+"\n"); err != nil {
		return "Error: " + err.Error()
	}
	c.rebuildIndex(sd)
	return fmt.Sprintf("Created %s/%s/%s/%s.", scope, scopeID, topic, filename)
}

// Update overwrites an existing consolidated entry.
func (c *Consolidated) Update(scope, scopeID, topic, entryName, content string) string {
	if err := c.checkWritable(scope); err != "" {
		return err
	}
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s/%s'.", scope, scopeID)
	}
	safeTopic, okt := pivot.SanitizeID(topic)
	safeEntry, oke := pivot.SanitizeID(entryName)
	if !okt || !oke {
		return "Error: invalid topic or entry name."
	}
	filename := ensureMD(safeEntry)
	entryPath := filepath.Join(sd, safeTopic, filename)
	if !pathExists(entryPath) {
		return fmt.Sprintf("Error: '%s/%s' does not exist. Use create instead.", topic, filename)
	}
	if err := AtomicWrite(entryPath, rstrip(content)+"\n"); err != nil {
		return "Error: " + err.Error()
	}
	c.rebuildIndex(sd)
	return fmt.Sprintf("Updated %s/%s/%s/%s.", scope, scopeID, topic, filename)
}

// Delete archives an entry (soft delete).
func (c *Consolidated) Delete(scope, scopeID, topic, entryName, reason string) string {
	if reason == "" {
		reason = "manual forget"
	}
	return c.Archive(scope, scopeID, topic, entryName, reason)
}

// Archive moves an entry into the .archive subtree.
func (c *Consolidated) Archive(scope, scopeID, topic, entryName, reason string) string {
	if err := c.checkWritable(scope); err != "" {
		return err
	}
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s/%s'.", scope, scopeID)
	}
	safeTopic, okt := pivot.SanitizeID(topic)
	safeEntry, oke := pivot.SanitizeID(entryName)
	if !okt || !oke {
		return "Error: invalid topic or entry name."
	}
	filename := ensureMD(safeEntry)
	entryPath := filepath.Join(sd, safeTopic, filename)
	if !pathExists(entryPath) {
		return fmt.Sprintf("Error: '%s/%s' does not exist.", topic, filename)
	}
	archiveTopicDir := filepath.Join(sd, ".archive", safeTopic)
	if err := os.MkdirAll(archiveTopicDir, 0o755); err != nil {
		return "Error: " + err.Error()
	}
	dest := filepath.Join(archiveTopicDir, filename)
	if err := AtomicWrite(dest, readText(entryPath)); err != nil {
		return "Error: " + err.Error()
	}
	_ = os.Remove(entryPath)
	topicDir := filepath.Join(sd, safeTopic)
	if dirEmpty(topicDir) {
		_ = os.Remove(topicDir)
	}
	c.rebuildIndex(sd)
	detail := "Archived " + topic + "/" + filename
	if reason != "" {
		detail += fmt.Sprintf(" (reason: %s)", reason)
	}
	c.AppendLog(scope, scopeID, "archive", []string{detail})
	r := reason
	if r == "" {
		r = "none"
	}
	return fmt.Sprintf("Archived %s/%s (reason: %s).", topic, filename, r)
}

// Restore moves an archived entry back into the active topic.
func (c *Consolidated) Restore(scope, scopeID, topic, entryName string) string {
	if err := c.checkWritable(scope); err != "" {
		return err
	}
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s/%s'.", scope, scopeID)
	}
	safeTopic, okt := pivot.SanitizeID(topic)
	safeEntry, oke := pivot.SanitizeID(entryName)
	if !okt || !oke {
		return "Error: invalid topic or entry name."
	}
	filename := ensureMD(safeEntry)
	archivedPath := filepath.Join(sd, ".archive", safeTopic, filename)
	if !pathExists(archivedPath) {
		return fmt.Sprintf("Error: '%s/%s' not found in archive.", topic, filename)
	}
	topicDir := filepath.Join(sd, safeTopic)
	if err := os.MkdirAll(topicDir, 0o755); err != nil {
		return "Error: " + err.Error()
	}
	dest := filepath.Join(topicDir, filename)
	if err := AtomicWrite(dest, readText(archivedPath)); err != nil {
		return "Error: " + err.Error()
	}
	_ = os.Remove(archivedPath)
	archiveTopic := filepath.Join(sd, ".archive", safeTopic)
	if dirEmpty(archiveTopic) {
		_ = os.Remove(archiveTopic)
	}
	c.rebuildIndex(sd)
	c.AppendLog(scope, scopeID, "restore", []string{"Restored " + topic + "/" + filename})
	return fmt.Sprintf("Restored %s/%s.", topic, filename)
}

// PurgeArchive permanently deletes archived entries older than retentionDays.
func (c *Consolidated) PurgeArchive(scope, scopeID string, retentionDays int) string {
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s/%s'.", scope, scopeID)
	}
	archiveDir := filepath.Join(sd, ".archive")
	if !isDir(archiveDir) {
		return "No archive to purge."
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	purged := 0
	ents, _ := os.ReadDir(archiveDir)
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		topicDir := filepath.Join(archiveDir, e.Name())
		for _, n := range sortedMarkdown(topicDir) {
			p := filepath.Join(topicDir, n)
			fi, err := os.Stat(p)
			if err != nil {
				continue
			}
			if fi.ModTime().Before(cutoff) {
				_ = os.Remove(p)
				purged++
			}
		}
		if dirEmpty(topicDir) {
			_ = os.Remove(topicDir)
		}
	}
	return fmt.Sprintf("Purged %d archived entries older than %d days.", purged, retentionDays)
}

// ArchiveStale archives entries whose mtime is older than retentionDays.
func (c *Consolidated) ArchiveStale(scope, scopeID string, retentionDays int) string {
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return fmt.Sprintf("Error: invalid scope '%s/%s'.", scope, scopeID)
	}
	if !isDir(sd) {
		return "No entries to archive."
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	var archived []string
	for _, t := range sortedSubdirs(sd) {
		topicDir := filepath.Join(sd, t)
		for _, n := range sortedMarkdown(topicDir) {
			fi, err := os.Stat(filepath.Join(topicDir, n))
			if err != nil {
				continue
			}
			if fi.ModTime().Before(cutoff) {
				c.Archive(scope, scopeID, t, stem(n), fmt.Sprintf("stale (%d+ days)", retentionDays))
				archived = append(archived, t+"/"+n)
			}
		}
	}
	if len(archived) == 0 {
		return "No stale entries found."
	}
	return fmt.Sprintf("Archived %d stale entries: %s", len(archived), strings.Join(archived, ", "))
}

// ------------------------------------------------------------------ index

func (c *Consolidated) rebuildIndex(scopeRoot string) {
	if !isDir(scopeRoot) {
		return
	}
	var lines []string
	for _, t := range sortedSubdirs(scopeRoot) {
		topicDir := filepath.Join(scopeRoot, t)
		var stems []string
		for _, n := range sortedMarkdown(topicDir) {
			stems = append(stems, stem(n))
		}
		if len(stems) == 0 {
			continue
		}
		hooks := make([]string, 0, len(stems))
		for _, s := range stems {
			hooks = append(hooks, hookText(s))
		}
		label := titleCase(hookText(t))
		lines = append(lines, fmt.Sprintf("- [%s](%s/) — %s", label, t, strings.Join(hooks, ", ")))
	}
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	_ = AtomicWrite(filepath.Join(scopeRoot, "INDEX.md"), content)
}

// ------------------------------------------------------------------ LOG.md

// AppendLog appends an audit-trail block to the scope's LOG.md.
func (c *Consolidated) AppendLog(scope, scopeID, action string, details []string) {
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return
	}
	if err := os.MkdirAll(sd, 0o755); err != nil {
		return
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n## %s — %s\n", now, action))
	for _, d := range details {
		b.WriteString("- " + d + "\n")
	}
	_ = appendLocked(filepath.Join(sd, "LOG.md"), b.String())
}

// ReadLog parses LOG.md and returns recent entries newest-first, capped at limit.
func (c *Consolidated) ReadLog(scope, scopeID string, limit int) []LogEntry {
	out := []LogEntry{}
	sd, ok := c.scopeDir(scope, scopeID)
	if !ok {
		return out
	}
	logPath := filepath.Join(sd, "LOG.md")
	if !isFile(logPath) {
		return out
	}
	var entries []LogEntry
	var current *LogEntry
	for _, line := range strings.Split(readText(logPath), "\n") {
		if m := logHeaderRe.FindStringSubmatch(line); m != nil {
			if current != nil {
				entries = append(entries, *current)
			}
			current = &LogEntry{Timestamp: m[1], Action: m[2], Details: []string{}}
		} else if current != nil && strings.HasPrefix(line, "- ") {
			current.Details = append(current.Details, line[2:])
		}
	}
	if current != nil {
		entries = append(entries, *current)
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if limit >= 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	if entries == nil {
		return out
	}
	return entries
}
