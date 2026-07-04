package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"go.uber.org/zap"
)

// IndexerHandler is an HTTP reverse-proxy forwarding /v1/indexer/* to the
// indexer service (DEXIASK_INDEXER_URL). The "/v1/indexer" prefix is stripped so
// the upstream sees its own routes (/v1/repos, /reindex, /v1/status, /healthz).
//
// On mutating operations it injects the caller's per-user GitHub token as
// X-Git-Token so private repos are cloned under the user's own credential, and it
// validates repo access (via the GitHub API) when a git URL is being registered.
type IndexerHandler struct {
	indexerURL string
	client     *http.Client
	github     *auth.GitHubClient
	logger     *logger.Logger
}

// NewIndexerHandler creates a new IndexerHandler. github may be nil in
// dev-fallback mode (no per-user token to inject or validate).
func NewIndexerHandler(indexerURL string, github *auth.GitHubClient, log *logger.Logger) *IndexerHandler {
	return &IndexerHandler{
		indexerURL: strings.TrimRight(indexerURL, "/"),
		client:     &http.Client{},
		github:     github,
		logger:     log,
	}
}

// ServeHTTP proxies any /v1/indexer/* request to the indexer service.
func (h *IndexerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.indexerURL == "" {
		writeError(w, http.StatusServiceUnavailable, "indexer service not configured")
		return
	}

	upstreamPath := strings.TrimPrefix(r.URL.Path, "/v1/indexer")
	if upstreamPath == "" {
		upstreamPath = "/"
	}

	principal, _ := auth.UserFromContext(r.Context())

	// Buffer the body so we can both inspect it (repo-access check) and forward it.
	var body []byte
	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		body = b
	}

	// Registering a git-URL repo: validate the caller can access it before proxying.
	if r.Method == http.MethodPost && upstreamPath == "/v1/repos" {
		if ok, status, msg := h.checkRepoAccess(r, principal, body); !ok {
			writeError(w, status, msg)
			return
		}
	}

	upstreamURL := h.indexerURL + upstreamPath
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(body))
	if err != nil {
		h.logger.Error("indexer proxy: failed to create request", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		proxyReq.Header.Set("Content-Type", ct)
	}
	// Dexiask is single-workspace: inject the fixed workspace id so the indexer
	// scopes results consistently with the engine's MCP calls.
	proxyReq.Header.Set("X-Workspace-Id", config.FixedWorkspaceID)
	// Inject the caller's GitHub token so private clones use their own credential.
	// The indexer only applies it to git operations and prefers an explicit body
	// token, so this is a no-op on reads.
	if principal.GitHubToken != "" {
		proxyReq.Header.Set("X-Git-Token", principal.GitHubToken)
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

// repoRegisterBody is the subset of POST /v1/repos we inspect for access control.
type repoRegisterBody struct {
	URL  string `json:"url"`
	Path string `json:"path"`
}

// checkRepoAccess validates that the caller can access the git URL being
// registered. Local `path` repos (the shared mounted codebase) are open to any
// authenticated user. Returns (ok, status, message).
func (h *IndexerHandler) checkRepoAccess(r *http.Request, principal auth.Principal, body []byte) (bool, int, string) {
	// No token / dev-fallback → nothing to validate against; allow.
	if h.github == nil || principal.GitHubToken == "" {
		return true, 0, ""
	}
	var b repoRegisterBody
	if err := json.Unmarshal(body, &b); err != nil {
		return false, http.StatusBadRequest, "invalid repo body"
	}
	if b.URL == "" {
		// Local path repo — no remote access to validate.
		return true, 0, ""
	}
	owner, repo, ok := parseGitHubURL(b.URL)
	if !ok {
		// Non-GitHub URL: we cannot validate access; let the clone itself gate it.
		return true, 0, ""
	}
	allowed, err := h.github.HasRepoAccess(r.Context(), principal.GitHubToken, owner, repo)
	if err != nil {
		h.logger.Warn("repo access check failed", zap.Error(err), zap.String("repo", owner+"/"+repo))
		return false, http.StatusBadGateway, "could not verify repo access"
	}
	if !allowed {
		return false, http.StatusForbidden, "you do not have access to this repository"
	}
	return true, 0, ""
}

// parseGitHubURL extracts owner/repo from an https GitHub URL. Returns ok=false
// for non-GitHub hosts.
func parseGitHubURL(url string) (owner, repo string, ok bool) {
	s := strings.TrimSpace(url)
	for _, p := range []string{"https://github.com/", "http://github.com/", "git@github.com:"} {
		if rest, found := strings.CutPrefix(s, p); found {
			rest = strings.TrimSuffix(rest, ".git")
			parts := strings.SplitN(rest, "/", 3)
			if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
				return parts[0], parts[1], true
			}
		}
	}
	return "", "", false
}
