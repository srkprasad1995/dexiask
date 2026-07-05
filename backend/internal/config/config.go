// Package config loads Dexiask backend configuration from the environment.
//
// Dexiask is a single-workspace service (one mounted codebase == one workspace),
// so WorkspaceID stays fixed. Users, however, are real: they sign in with GitHub
// and every product row is stamped with the resolved GitHub user id. When no
// OAuth app is configured the backend runs in dev-fallback mode and stamps the
// fixed dev user id so the zero-config `docker compose up` experience is kept.
package config

import (
	"os"
	"strconv"
)

// FixedWorkspaceID is the single workspace every row is scoped to (one mounted
// codebase). FixedUserID is the dev-fallback user id used only when auth is not
// configured (RequireAuth == false).
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
	// MemoryURL is the REST base URL of the memory service (proxy + digest target).
	// Inert when empty (memory disabled).
	MemoryURL string
	// MemoryMCPURL is the engine-reachable memory MCP endpoint. Injected per turn
	// with per-user headers so the agent can view/record memory. Inert when empty.
	MemoryMCPURL string
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

	// --- GitHub OAuth / auth (Phase 1) ---
	// GitHubClientID / GitHubClientSecret configure the GitHub OAuth app. When
	// either is empty the backend runs in dev-fallback mode (RequireAuth false).
	GitHubClientID     string
	GitHubClientSecret string
	// OAuthCallbackURL is the backend callback the GitHub app redirects to
	// (e.g. http://localhost:25052/v1/auth/callback).
	OAuthCallbackURL string
	// SessionSecret signs session cookies (HMAC). Required in auth mode.
	SessionSecret string
	// TokenEncKey is the AES key (hex, 32/48/64 chars) encrypting stored OAuth
	// tokens at rest. Required in auth mode.
	TokenEncKey string
	// WebBaseURL is the web app origin the callback redirects the browser back
	// to after login (e.g. http://localhost:25051).
	WebBaseURL string
	// OAuthEnabled is true when a GitHub OAuth app is configured (adds the
	// authorize/callback login flow on top of token-login).
	OAuthEnabled bool
	// RequireAuth is true when login is enabled: either an OAuth app is configured
	// or the session infra (SessionSecret + TokenEncKey) is set for token-login.
	// When false the backend injects the fixed dev-fallback principal.
	RequireAuth bool
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
	githubClientID := os.Getenv("DEXIASK_GITHUB_CLIENT_ID")
	githubClientSecret := os.Getenv("DEXIASK_GITHUB_CLIENT_SECRET")
	sessionSecret := os.Getenv("DEXIASK_SESSION_SECRET")
	tokenEncKey := os.Getenv("DEXIASK_TOKEN_ENC_KEY")
	oauthEnabled := githubClientID != "" && githubClientSecret != ""
	// Login is enabled when the session infra is present (token-login) or an OAuth
	// app is configured. Both paths need the session secret + token-encryption key.
	requireAuth := oauthEnabled || (sessionSecret != "" && tokenEncKey != "")

	return &Config{
		DBDSN:          os.Getenv("DEXIASK_DB_DSN"),
		AgentURL:       os.Getenv("DEXIASK_AGENT_URL"),
		IndexerURL:     os.Getenv("DEXIASK_INDEXER_URL"),
		IndexerMCPURL:  os.Getenv("DEXIASK_INDEXER_MCP_URL"),
		MemoryURL:      os.Getenv("DEXIASK_MEMORY_URL"),
		MemoryMCPURL:   os.Getenv("DEXIASK_MEMORY_MCP_URL"),
		Model:          getEnv("DEXIASK_MODEL", "claude-sonnet-5"),
		MaxTokens:      maxTokens,
		WorkspaceMount: getEnv("DEXIASK_WORKSPACE_MOUNT", "/workspace"),
		SlackAppToken:  os.Getenv("SLACK_APP_TOKEN"),
		SlackBotToken:  os.Getenv("SLACK_BOT_TOKEN"),
		Port:           port,
		LogLevel:       getEnv("DEXIASK_LOG_LEVEL", "info"),
		Env:            getEnv("DEXIASK_ENV", "development"),

		GitHubClientID:     githubClientID,
		GitHubClientSecret: githubClientSecret,
		OAuthCallbackURL:   os.Getenv("DEXIASK_OAUTH_CALLBACK_URL"),
		SessionSecret:      sessionSecret,
		TokenEncKey:        tokenEncKey,
		WebBaseURL:         getEnv("DEXIASK_WEB_BASE_URL", "http://localhost:25051"),
		OAuthEnabled:       oauthEnabled,
		RequireAuth:        requireAuth,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
