package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/dexiask/memory/internal/digest"
	"github.com/dexiask/memory/internal/pivot"
	"github.com/dexiask/memory/internal/reqscope"
	"github.com/dexiask/memory/internal/store"
)

func (a *API) consolidated(sc pivot.RequestScope) *store.Consolidated {
	return store.NewConsolidated(a.root, sc, a.reg, restWritable)
}

func (a *API) working(sc pivot.RequestScope) *store.Working {
	return store.NewWorking(a.root, sc, a.reg)
}

// requireScope parses the scoping headers; ok is false (and the caller 400s) when
// X-Workspace-Id is absent — there is no unscoped path.
func requireScope(w http.ResponseWriter, r *http.Request) (pivot.RequestScope, bool) {
	sc, ok := reqscope.Parse(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "missing workspace context (X-Workspace-Id header required)")
	}
	return sc, ok
}

func qInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (a *API) listScopes(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	cons := a.consolidated(sc)
	wm := a.working(sc)
	type key struct{ scope, id string }
	seen := map[key]bool{}
	out := []store.ScopeInfo{}
	for _, s := range cons.ListScopes() {
		k := key{s.Scope, s.ID}
		if !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	for _, sp := range wm.ListScopes() {
		idLabel := sp.ScopeID
		if sp.Scope == "global" {
			idLabel = "global"
		}
		k := key{sp.Scope, idLabel}
		if !seen[k] {
			seen[k] = true
			out = append(out, store.ScopeInfo{Scope: sp.Scope, ID: idLabel, Label: idLabel})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) listEntries(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	archived := q.Get("state") == "archived"
	out := a.consolidated(sc).ListEntries(q.Get("scope"), q.Get("id"), archived)
	writeJSON(w, http.StatusOK, out)
}

func (a *API) listTopics(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	out := a.consolidated(sc).ListTopics(q.Get("scope"), q.Get("id"))
	writeJSON(w, http.StatusOK, out)
}

type createEntryRequest struct {
	Scope     string `json:"scope"`
	ScopeID   string `json:"scope_id"`
	Topic     string `json:"topic"`
	EntryName string `json:"entry_name"`
	Content   string `json:"content"`
}

func (a *API) createEntry(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	var body createEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result := a.consolidated(sc).Create(body.Scope, body.ScopeID, body.Topic, body.EntryName, body.Content)
	if strings.HasPrefix(result, "Error") {
		writeError(w, http.StatusBadRequest, result)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": result})
}

func (a *API) getEntry(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	entry := a.consolidated(sc).GetEntry(r.PathValue("scope"), r.PathValue("scope_id"), r.PathValue("topic"), r.PathValue("entry_name"))
	if entry == nil {
		writeError(w, http.StatusNotFound, "Entry not found.")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (a *API) updateEntry(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result := a.consolidated(sc).Update(r.PathValue("scope"), r.PathValue("scope_id"), r.PathValue("topic"), r.PathValue("entry_name"), body.Content)
	if strings.HasPrefix(result, "Error") {
		writeError(w, http.StatusBadRequest, result)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": result})
}

func (a *API) deleteEntry(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "manual forget"
	}
	result := a.consolidated(sc).Delete(r.PathValue("scope"), r.PathValue("scope_id"), r.PathValue("topic"), r.PathValue("entry_name"), reason)
	if strings.HasPrefix(result, "Error") {
		writeError(w, http.StatusBadRequest, result)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": result})
}

func (a *API) restoreEntry(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	result := a.consolidated(sc).Restore(r.PathValue("scope"), r.PathValue("scope_id"), r.PathValue("topic"), r.PathValue("entry_name"))
	if strings.HasPrefix(result, "Error") {
		writeError(w, http.StatusBadRequest, result)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": result})
}

func (a *API) purge(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	result := a.consolidated(sc).PurgeArchive(q.Get("scope"), q.Get("id"), qInt(r, "retention_days", 30))
	if strings.HasPrefix(result, "Error") {
		writeError(w, http.StatusBadRequest, result)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": result})
}

func (a *API) archiveStale(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	result := a.consolidated(sc).ArchiveStale(q.Get("scope"), q.Get("id"), qInt(r, "days", 60))
	writeJSON(w, http.StatusOK, map[string]string{"message": result})
}

func (a *API) getLog(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	out := a.consolidated(sc).ReadLog(q.Get("scope"), q.Get("id"), qInt(r, "limit", 50))
	writeJSON(w, http.StatusOK, out)
}

func (a *API) listWorking(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	out := a.working(sc).ListFiles(q.Get("scope"), q.Get("id"))
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getWorking(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	filename := r.PathValue("filename")
	content, found := a.working(sc).GetFile(q.Get("scope"), q.Get("id"), filename)
	if !found {
		writeError(w, http.StatusNotFound, "Working file not found.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"filename": filename, "content": content})
}

func (a *API) digest(w http.ResponseWriter, r *http.Request) {
	sc, ok := requireScope(w, r)
	if !ok {
		return
	}
	budget := qInt(r, "token_budget", 2000)
	d := digest.Build(a.root, sc, a.reg, budget)
	writeJSON(w, http.StatusOK, map[string]string{"digest": d})
}

type consolidateRequest struct {
	WorkspaceID string `json:"workspaceId"`
}

func (a *API) consolidate(w http.ResponseWriter, r *http.Request) {
	var body consolidateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ws := strings.TrimSpace(body.WorkspaceID)
	if ws == "" {
		// Default to the header workspace (the backend proxy injects it).
		if sc, ok := reqscope.Parse(r); ok {
			ws = sc.WorkspaceID
		}
	}
	if ws == "" {
		writeError(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	// Run in the background so the caller gets a prompt 202.
	go func(ws string) {
		_ = a.svc.Run(context.Background(), ws)
	}(ws)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "workspaceId": ws})
}
