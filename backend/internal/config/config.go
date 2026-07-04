// Package config loads Dexiask backend configuration from the environment.
//
// Dexiask is a single-user, single-workspace service: there is
// no auth, no per-workspace scoping. A fixed identity is used everywhere the
// upstream code threads a resolved (workspace, user) pair.
package config

import (
	"os"
	"strconv"
)

// Fixed identity for the single-user build. Every product row is stamped with
// these so the data model is unchanged from upstream, but nothing is derived
// from a client-supplied header.
const (
	FixedWorkspaceID = "dexiask"
	FixedUserID      = "dexiask"
)

// Config holds the resolved backend configuration.
type Config struct {
	// DBDSN is the Postgres DSN (URL or key=value form). Required.
	DBDSN string
	// AgentURL is the base URL of the always-on Claude engine HTTP service.
	AgentURL string
	// IndexerURL is the base URL of the indexer service (REST proxy target).
	IndexerURL string
	// IndexerMCPURL is the engine-reachable indexer MCP endpoint. Injected into
	// every ask Job so the agent can call semantic_search. Inert when empty.
	IndexerMCPURL string
	// Model is the Claude model the ask agent runs on.
	Model string
	// MaxTokens optionally caps output tokens per turn (0 = engine default).
	MaxTokens int
	// WorkspaceMount is the host path mounted at /workspace (attachment jail root).
	WorkspaceMount string
	// SlackAppToken / SlackBotToken enable the Slack Socket Mode bot when both set.
	SlackAppToken string
	SlackBotToken string
	// Port is the HTTP listen port.
	Port int
	// LogLevel / Env control logging.
	LogLevel string
	Env      string
}

// Load reads configuration from the environment.
func Load() *Config {
	maxTokens := 0
	if v := os.Getenv("DEXIASK_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			maxTokens = n
		}
	}
	port := 8080
	if v := os.Getenv("DEXIASK_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			port = n
		}
	}
	return &Config{
		DBDSN:          os.Getenv("DEXIASK_DB_DSN"),
		AgentURL:       os.Getenv("DEXIASK_AGENT_URL"),
		IndexerURL:     os.Getenv("DEXIASK_INDEXER_URL"),
		IndexerMCPURL:  os.Getenv("DEXIASK_INDEXER_MCP_URL"),
		Model:          getEnv("DEXIASK_MODEL", "claude-sonnet-5"),
		MaxTokens:      maxTokens,
		WorkspaceMount: getEnv("DEXIASK_WORKSPACE_MOUNT", "/workspace"),
		SlackAppToken:  os.Getenv("SLACK_APP_TOKEN"),
		SlackBotToken:  os.Getenv("SLACK_BOT_TOKEN"),
		Port:           port,
		LogLevel:       getEnv("DEXIASK_LOG_LEVEL", "info"),
		Env:            getEnv("DEXIASK_ENV", "development"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
