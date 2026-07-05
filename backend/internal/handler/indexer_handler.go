package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"go.uber.org/zap"
)

// IndexerHandler is an HTTP reverse-proxy forwarding /v1/indexer/* to the indexer
// service (DEXIASK_INDEXER_URL). The "/v1/indexer" prefix is stripped so the
// upstream sees its own routes.
//
// Indexing uses a single central git token configured by an admin (the indexer's
// global token, set via PUT /v1/indexer/v1/git-token). Mutating operations
// (reindex, register/remove repos, set the git token) require the admin role;
// reads (list repos, status, search, docs) are open to any member.
type IndexerHandler struct {
	indexerURL    string
	internalToken string
	client        *http.Client
	logger        *logger.Logger
}

// NewIndexerHandler creates a new IndexerHandler. internalToken (when set) enables
// per-user repo gating: the backend forwards the caller's identity to the indexer,
// which validates access itself.
func NewIndexerHandler(indexerURL, internalToken string, log *logger.Logger) *IndexerHandler {
	return &IndexerHandler{
		indexerURL:    strings.TrimRight(indexerURL, "/"),
		internalToken: internalToken,
		client:        &http.Client{},
		logger:        log,
	}
}

// ServeHTTP proxies any /v1/indexer/* request to the indexer service.
func (h *IndexerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.indexerURL == "" {
		writeError(w, http.StatusServiceUnavailable, "indexer service not configured")
		return
	}
	// Reads are open to any member; anything mutating (indexing, repo registry,
	// git token) is admin-only.
	var p auth.Principal
	var ok bool
	if r.Method == http.MethodGet {
		p, ok = requirePrincipal(w, r)
	} else {
		p, ok = requireAdmin(w, r)
	}
	if !ok {
		return
	}

	upstreamPath := strings.TrimPrefix(r.URL.Path, "/v1/indexer")
	if upstreamPath == "" {
		upstreamPath = "/"
	}
	upstreamURL := h.indexerURL + upstreamPath
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		h.logger.Error("indexer proxy: failed to create request", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		proxyReq.Header.Set("Content-Type", ct)
	}
	// Single-workspace: inject the fixed workspace id so the indexer scopes results
	// consistently with the engine's MCP calls.
	proxyReq.Header.Set("X-Workspace-Id", config.FixedWorkspaceID)
	// Per-user repo gating: forward the caller's identity so the indexer can
	// validate access itself. proxyReq is built fresh (only Content-Type copied),
	// so client-supplied gating headers are never forwarded.
	for k, v := range auth.IndexerAuthHeaders(h.internalToken, p.IsAdmin(), p.GitHubToken) {
		proxyReq.Header.Set(k, v)
	}

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		h.logger.Error("indexer proxy: request failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "indexer unreachable")
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
