// Package agent implements the orchestrator side of the Agent Job Protocol.
// See the Dexiask engine's PROTOCOL.md for the full specification.
//
// This is the Dexiask (single-user, ask-only) orchestrator: one role (ask),
// one runtime (claude), one fixed
// workspace subtree. Event type names are kept byte-for-byte identical to the
// engine output and the web SSE consumer, so the backend re-frames without
// translation.
package agent

import (
	"path"

	"github.com/dexiask/dexiask/internal/agent/prompts"
)

// WorkspaceMount is the engine-container mount root. All agent working files
// live under a single fixed subtree beneath it (no per-workspace scoping).
const WorkspaceMount = "/workspace"

// workspaceSubdir is the single fixed workspace root under the mount. Dexiask
// has no per-tenant isolation, so everything lives under /workspace/.dexiask.
const workspaceSubdir = ".dexiask"

// WorkspacePath returns the engine-side working directory (agent cwd + the
// attachment jail): "/workspace/.dexiask".
func WorkspacePath() string {
	return path.Join(WorkspaceMount, workspaceSubdir)
}

// SessionStorePathFor returns the per-conversation directory the engine points
// its session store at, so each conversation resumes its own SDK session.
func SessionStorePathFor(conversationID string) string {
	if conversationID == "" {
		return ""
	}
	return path.Join(WorkspacePath(), "conversations", conversationID, "session")
}

// Attachment describes a file attached to a message turn.
// Kind is "image" when MediaType starts with "image/", else "file".
type Attachment struct {
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	MediaType string `json:"mediaType"`
	Filename  string `json:"filename"`
}

// Message is a single turn in a conversation history.
type Message struct {
	Role        string       `json:"role"`
	Content     string       `json:"content"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// MCPServerConfig is a remote MCP server attached to a Job. The engine merges
// these into its mcp_servers map alongside its in-process workspace server.
type MCPServerConfig struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"` // "http" | "sse"
	URL          string            `json:"url"`
	Description  string            `json:"description,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	DeferLoading bool              `json:"deferLoading,omitempty"`
}

// Job is the payload sent to the engine (HTTP request body).
// Field names match PROTOCOL.md exactly.
type Job struct {
	Role             string            `json:"role"`
	Model            string            `json:"model"`
	SystemPrompt     string            `json:"systemPrompt"`
	AllowedTools     []string          `json:"allowedTools"`
	PermissionMode   string            `json:"permissionMode"`
	SkillsPath       string            `json:"skillsPath"`
	WorkspacePath    string            `json:"workspacePath"`
	Messages         []Message         `json:"messages"`
	SessionID        string            `json:"sessionId,omitempty"`
	SessionStorePath string            `json:"sessionStorePath,omitempty"`
	MCPServers       []MCPServerConfig `json:"mcpServers,omitempty"`
	MaxTokens        int               `json:"maxTokens,omitempty"`
}

// Event is a single NDJSON event emitted by the engine on its streaming
// response. The Type field names match the engine output and the web SSE
// consumer exactly so the orchestrator re-frames them with zero translation.
type Event struct {
	// Common
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`

	// text.* / thinking.*
	Text string `json:"text,omitempty"`

	// tool.*
	Name   string      `json:"name,omitempty"`
	JSON   string      `json:"json,omitempty"`
	Input  interface{} `json:"input,omitempty"`
	Result interface{} `json:"result,omitempty"`

	// agent.step
	Step interface{} `json:"step,omitempty"`

	// result
	Status string `json:"status,omitempty"`
	Model  string `json:"model,omitempty"`
	// SessionID is the SDK session identifier returned in the "result" event.
	// Used by ChatService to persist and forward on the next turn.
	SessionID string `json:"sessionId,omitempty"`

	// error
	Message string `json:"message,omitempty"`
}

// SystemPromptForRole returns the system prompt for the given role.
// Dexiask ships only the "ask" prompt (plus the shared output-format guidance).
func SystemPromptForRole(role string) string {
	return prompts.ForRole(role)
}

// AllowedToolsForRole returns the tool whitelist for the given role. Dexiask
// runs only the read-only ask role; the engine additionally attaches the
// indexer MCP tools (semantic_search) named via the mcp__indexer prefix.
//
// WebSearch/WebFetch are sensitive built-in tools the engine denies in dontAsk
// mode unless explicitly named, so they are listed here. AskChoice lets the
// agent ask structured multiple-choice questions instead of prose.
func AllowedToolsForRole(role string) []string {
	return []string{"Read", "Glob", "Grep", "WebSearch", "WebFetch", "AskChoice"}
}
