// Package httpapi serves the memory REST surface (/v1/memory/*), the digest
// endpoint, the manual /v1/consolidate trigger, /healthz, and mounts the MCP
// handler at /mcp.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/dexiask/memory/internal/consolidate"
	"github.com/dexiask/memory/internal/pivot"
)

// restWritable is the writable scope set for the REST CRUD surface (the FE memory
// browser can edit every scope).
var restWritable = []string{"repo", "user", "global"}

// API holds the REST handlers' dependencies.
type API struct {
	root string
	reg  *pivot.Registry
	svc  *consolidate.Service
}

// New builds the API.
func New(root string, reg *pivot.Registry, svc *consolidate.Service) *API {
	return &API{root: root, reg: reg, svc: svc}
}

// Handler returns the mux wiring every route plus the MCP handler.
func (a *API) Handler(mcpHandler http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", healthz)

	mux.HandleFunc("GET /v1/memory/scopes", a.listScopes)
	mux.HandleFunc("GET /v1/memory/entries", a.listEntries)
	mux.HandleFunc("POST /v1/memory/entries", a.createEntry)
	mux.HandleFunc("GET /v1/memory/entries/{scope}/{scope_id}/{topic}/{entry_name}", a.getEntry)
	mux.HandleFunc("PUT /v1/memory/entries/{scope}/{scope_id}/{topic}/{entry_name}", a.updateEntry)
	mux.HandleFunc("DELETE /v1/memory/entries/{scope}/{scope_id}/{topic}/{entry_name}", a.deleteEntry)
	mux.HandleFunc("POST /v1/memory/entries/{scope}/{scope_id}/{topic}/{entry_name}/restore", a.restoreEntry)
	mux.HandleFunc("GET /v1/memory/topics", a.listTopics)
	mux.HandleFunc("POST /v1/memory/purge", a.purge)
	mux.HandleFunc("POST /v1/memory/archive-stale", a.archiveStale)
	mux.HandleFunc("GET /v1/memory/log", a.getLog)
	mux.HandleFunc("GET /v1/memory/working", a.listWorking)
	mux.HandleFunc("GET /v1/memory/working/{filename}", a.getWorking)
	mux.HandleFunc("GET /v1/memory/digest", a.digest)
	mux.HandleFunc("POST /v1/consolidate", a.consolidate)

	mux.Handle("/mcp", mcpHandler)

	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
