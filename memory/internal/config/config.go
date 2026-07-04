// Package config loads the memory service configuration from the environment.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config is the resolved service configuration.
type Config struct {
	// Root is the memory volume mount; memory lives under <root>/<ws>/memory.
	Root string // DEXIASK_MEMORY_ROOT
	Host string // DEXIASK_MEMORY_HOST — listen host.
	Port string // DEXIASK_MEMORY_PORT — listen port (container side).

	// MCPSelfURL is the engine-reachable /mcp URL of THIS service, stamped into
	// dream jobs so the engine writes consolidated memory back through our MCP.
	MCPSelfURL string // DEXIASK_MEMORY_MCP_SELF_URL

	// AgentURL is the always-on engine /v1/jobs base URL.
	AgentURL string // DEXIASK_AGENT_URL

	// Dream job shape (Dexiask engine manages its own API key, so no key here).
	DreamModel    string // DEXIASK_DREAM_MODEL (falls back to DEXIASK_MODEL).
	WorkspacePath string // DEXIASK_MEMORY_WORKSPACE_PATH — engine cwd for the dream job.
	MaxTokens     int    // DEXIASK_MAX_TOKENS (0 = engine default).

	// Dream scheduler.
	DreamInterval time.Duration // DEXIASK_DREAM_INTERVAL (default 1h; 0 disables).
}

// FixedWorkspaceID is the single workspace this deployment consolidates. Mirrors
// the backend's config.FixedWorkspaceID.
const FixedWorkspaceID = "dexiask"

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getdur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return def
}

// Load reads the configuration from the process environment.
func Load() *Config {
	maxTokens := 0
	if v := os.Getenv("DEXIASK_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxTokens = n
		}
	}
	return &Config{
		Root:          getenv("DEXIASK_MEMORY_ROOT", "/memory"),
		Host:          getenv("DEXIASK_MEMORY_HOST", "0.0.0.0"),
		Port:          getenv("DEXIASK_MEMORY_PORT", "8080"),
		MCPSelfURL:    getenv("DEXIASK_MEMORY_MCP_SELF_URL", "http://memory:8080/mcp"),
		AgentURL:      getenv("DEXIASK_AGENT_URL", ""),
		DreamModel:    getenv("DEXIASK_DREAM_MODEL", getenv("DEXIASK_MODEL", "claude-sonnet-5")),
		WorkspacePath: getenv("DEXIASK_MEMORY_WORKSPACE_PATH", "/workspace/.dexiask"),
		MaxTokens:     maxTokens,
		DreamInterval: getdur("DEXIASK_DREAM_INTERVAL", time.Hour),
	}
}

// Addr returns the host:port the HTTP server binds to.
func (c *Config) Addr() string { return c.Host + ":" + c.Port }
