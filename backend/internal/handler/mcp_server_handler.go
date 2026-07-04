package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/repository"
	"go.uber.org/zap"
)

// MCPServerHandler serves the custom MCP server REST endpoints. These are the
// user-defined MCP servers injected into every ask Job alongside the indexer.
//
//	GET    /v1/mcp-servers        — list servers
//	POST   /v1/mcp-servers        — create a server
//	PUT    /v1/mcp-servers/{id}   — update a server (incl. enabled toggle)
//	DELETE /v1/mcp-servers/{id}   — delete a server
type MCPServerHandler struct {
	repo   repository.MCPServerRepository
	logger *logger.Logger
}

// NewMCPServerHandler creates a new MCPServerHandler.
func NewMCPServerHandler(repo repository.MCPServerRepository, log *logger.Logger) *MCPServerHandler {
	return &MCPServerHandler{repo: repo, logger: log}
}

// mcpServerBody is the create/update request payload.
type mcpServerBody struct {
	Name    *string            `json:"name"`
	Type    *string            `json:"type"`
	URL     *string            `json:"url"`
	Headers *map[string]string `json:"headers"`
	Enabled *bool              `json:"enabled"`
}

// ServeCollection handles GET (list) and POST (create) on /v1/mcp-servers.
func (h *MCPServerHandler) ServeCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ServeItem handles PUT (update) and DELETE on /v1/mcp-servers/{id}.
func (h *MCPServerHandler) ServeItem(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/mcp-servers/"), "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "mcp server id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		h.update(w, r, id)
	case http.MethodDelete:
		h.delete(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *MCPServerHandler) list(w http.ResponseWriter, r *http.Request) {
	servers, err := h.repo.List(r.Context(), &model.ListMCPServersFilter{
		WorkspaceID: config.FixedWorkspaceID,
	})
	if err != nil {
		h.logger.Error("list mcp servers failed", zap.Error(err))
		writeServiceError(w, err)
		return
	}
	if servers == nil {
		servers = []*model.MCPServer{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"mcpServers": servers})
}

func (h *MCPServerHandler) create(w http.ResponseWriter, r *http.Request) {
	var body mcpServerBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	input := &model.CreateMCPServerInput{
		WorkspaceID: config.FixedWorkspaceID,
		Enabled:     true,
	}
	if body.Name != nil {
		input.Name = strings.TrimSpace(*body.Name)
	}
	if body.Type != nil {
		input.Type = strings.TrimSpace(*body.Type)
	}
	if body.URL != nil {
		input.URL = strings.TrimSpace(*body.URL)
	}
	if body.Headers != nil {
		input.Headers = *body.Headers
	}
	if body.Enabled != nil {
		input.Enabled = *body.Enabled
	}
	srv, err := h.repo.Create(r.Context(), input)
	if err != nil {
		h.logger.Error("create mcp server failed", zap.Error(err))
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, srv)
}

func (h *MCPServerHandler) update(w http.ResponseWriter, r *http.Request, id string) {
	var body mcpServerBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	input := &model.UpdateMCPServerInput{
		ID:      id,
		Headers: body.Headers,
		Enabled: body.Enabled,
	}
	if body.Name != nil {
		trimmed := strings.TrimSpace(*body.Name)
		input.Name = &trimmed
	}
	if body.Type != nil {
		trimmed := strings.TrimSpace(*body.Type)
		input.Type = &trimmed
	}
	if body.URL != nil {
		trimmed := strings.TrimSpace(*body.URL)
		input.URL = &trimmed
	}
	srv, err := h.repo.Update(r.Context(), input)
	if err != nil {
		h.logger.Error("update mcp server failed", zap.Error(err), zap.String("id", id))
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (h *MCPServerHandler) delete(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.repo.Delete(r.Context(), id); err != nil {
		h.logger.Error("delete mcp server failed", zap.Error(err), zap.String("id", id))
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
