package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"go.uber.org/zap"
)

// IndexerHandler is an HTTP reverse-proxy forwarding /v1/indexer/* to the
// indexer service (DEXIASK_INDEXER_URL). The "/v1/indexer" prefix is stripped so
// the upstream sees its own routes (/v1/repos, /reindex, /v1/status, /healthz).
type IndexerHandler struct {
	indexerURL string
	client     *http.Client
	logger     *logger.Logger
}

// NewIndexerHandler creates a new IndexerHandler. indexerURL is the indexer base
// URL (e.g. "http://localhost:25054").
func NewIndexerHandler(indexerURL string, log *logger.Logger) *IndexerHandler {
	return &IndexerHandler{
		indexerURL: strings.TrimRight(indexerURL, "/"),
		client:     &http.Client{},
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
	// Dexiask is single-workspace: inject the fixed workspace id so the indexer
	// scopes results consistently with the engine's MCP calls.
	proxyReq.Header.Set("X-Workspace-Id", config.FixedWorkspaceID)

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
