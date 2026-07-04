package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"go.uber.org/zap"
)

// MemoryHandler is an HTTP reverse-proxy forwarding /v1/memory/* to the memory
// service (DEXIASK_MEMORY_URL). It injects the fixed workspace id and the
// authenticated user id so the memory service scopes results per user.
//
// POST /v1/memory/consolidate triggers a manual dream; the memory service reads
// the workspace from the injected header.
type MemoryHandler struct {
	memoryURL string
	client    *http.Client
	logger    *logger.Logger
}

// NewMemoryHandler creates a new MemoryHandler.
func NewMemoryHandler(memoryURL string, log *logger.Logger) *MemoryHandler {
	return &MemoryHandler{
		memoryURL: strings.TrimRight(memoryURL, "/"),
		client:    &http.Client{},
		logger:    log,
	}
}

// ServeHTTP proxies any /v1/memory/* request to the memory service.
func (h *MemoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.memoryURL == "" {
		writeError(w, http.StatusServiceUnavailable, "memory service not configured")
		return
	}
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}

	upstreamPath := strings.TrimPrefix(r.URL.Path, "/v1/memory")
	// /v1/memory/consolidate maps to the service's /v1/consolidate; everything
	// else keeps its /v1/memory/... path.
	var upstreamURL string
	if upstreamPath == "/consolidate" {
		upstreamURL = h.memoryURL + "/v1/consolidate"
	} else {
		upstreamURL = h.memoryURL + "/v1/memory" + upstreamPath
	}
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, r.Body)
	if err != nil {
		h.logger.Error("memory proxy: failed to create request", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		proxyReq.Header.Set("Content-Type", ct)
	}
	proxyReq.Header.Set("X-Workspace-Id", config.FixedWorkspaceID)
	proxyReq.Header.Set("X-User-Id", p.UserID)

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		h.logger.Error("memory proxy: request failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "memory service unreachable")
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
