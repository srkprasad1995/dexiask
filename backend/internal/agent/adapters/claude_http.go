// Package adapters contains Runtime implementations, one per engine.
package adapters

import (
	"context"
	"net/http"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/pkg/logger"
)

const claudeHTTPRuntimeName = "claude"

// ClaudeHTTPRuntime implements agent.Runtime by calling the always-on Claude
// engine HTTP service. The engine manages its own API keys, session storage,
// and workspace mounts; the backend only needs the base URL.
type ClaudeHTTPRuntime struct {
	runner *agent.HTTPRunner
	logger *logger.Logger
}

// NewClaudeHTTPRuntime creates a ClaudeHTTPRuntime pointed at agentURL
// (DEXIASK_AGENT_URL), e.g. "http://localhost:8080".
func NewClaudeHTTPRuntime(agentURL string, log *logger.Logger) *ClaudeHTTPRuntime {
	runner := agent.NewHTTPRunner(agentURL, &http.Client{}, log)
	return &ClaudeHTTPRuntime{runner: runner, logger: log}
}

// Name implements agent.Runtime.
func (c *ClaudeHTTPRuntime) Name() string { return claudeHTTPRuntimeName }

// Start implements agent.Runtime.
func (c *ClaudeHTTPRuntime) Start(ctx context.Context, job agent.Job) (<-chan agent.Event, error) {
	return c.runner.Run(ctx, job)
}
