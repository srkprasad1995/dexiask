// Package consolidate owns the dream/consolidation flow: it builds the dream
// context from working memory, submits a job to the agent engine, and lets the
// engine's LLM write consolidated memory back through this service's own MCP. It
// also owns the periodic scheduler.
package consolidate

import (
	"context"

	"github.com/dexiask/memory/internal/config"
	"github.com/dexiask/memory/internal/engineclient"
	"github.com/dexiask/memory/internal/pivot"
	"github.com/dexiask/memory/internal/prompts"
	"github.com/dexiask/memory/internal/store"
)

// Service runs a single consolidation for a workspace.
type Service struct {
	cfg    *config.Config
	reg    *pivot.Registry
	engine *engineclient.Client
}

// NewService wires a consolidation service.
func NewService(cfg *config.Config, reg *pivot.Registry, engine *engineclient.Client) *Service {
	return &Service{cfg: cfg, reg: reg, engine: engine}
}

// Run executes one consolidation for workspaceID. It builds the dream context
// from working memory, constructs the dream Job (whose memory writes flow back
// through this service's MCP), and drains the engine stream. A workspace with no
// pending working memory is a no-op.
func (s *Service) Run(ctx context.Context, workspaceID string) error {
	scope := pivot.RequestScope{WorkspaceID: workspaceID, UserID: "system", Role: "dream"}
	w := store.NewWorking(s.cfg.Root, scope, s.reg)
	pending := BuildWorkingContext(w)
	if pending == "" {
		return nil // nothing to consolidate
	}

	// The engine role enum only accepts "ask"; the dream distinction is carried
	// by the X-Role header on the memory MCP + the dream system prompt.
	job := engineclient.Job{
		Role:           "ask",
		Model:          s.cfg.DreamModel,
		SystemPrompt:   prompts.Dream() + "\n\n" + pending,
		AllowedTools:   []string{},
		PermissionMode: "dontAsk",
		WorkspacePath:  s.cfg.WorkspacePath,
		Messages: []engineclient.Message{{
			Role:    "user",
			Content: "Consolidate the pending working memory into long-term memory now.",
		}},
		MCPServers: []engineclient.MCPServerConfig{{
			Name:        "memory",
			Type:        "http",
			URL:         s.cfg.MCPSelfURL,
			Description: "Dexiask long-term memory: view, search, and consolidate scoped memory.",
			Headers: map[string]string{
				"X-Workspace-Id":    workspaceID,
				"X-User-Id":         "system",
				"X-Role":            "dream",
				"X-Writable-Scopes": "global,user,repo",
			},
		}},
		MaxTokens: s.cfg.MaxTokens,
	}
	return s.engine.Submit(ctx, job)
}
