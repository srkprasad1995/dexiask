// Package engineclient submits jobs to the always-on Dexiask engine and drains
// the streaming NDJSON response to a terminal result/error. The Job shape is a
// subset of the backend's Agent Job Protocol (backend/internal/agent/protocol.go).
package engineclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// MCPServerConfig is a remote MCP server attached to a Job.
type MCPServerConfig struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	URL          string            `json:"url"`
	Description  string            `json:"description,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	DeferLoading bool              `json:"deferLoading,omitempty"`
}

// Message is one conversation turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Job is the engine request body. Field names match the Dexiask engine's Job
// exactly. Note: the engine's role enum only accepts "ask" — the dream
// distinction rides the X-Role header on the memory MCP + the dream system
// prompt, not this field.
type Job struct {
	Role           string            `json:"role"`
	Model          string            `json:"model"`
	SystemPrompt   string            `json:"systemPrompt"`
	AllowedTools   []string          `json:"allowedTools"`
	PermissionMode string            `json:"permissionMode"`
	SkillsPath     string            `json:"skillsPath,omitempty"`
	WorkspacePath  string            `json:"workspacePath"`
	Messages       []Message         `json:"messages"`
	MCPServers     []MCPServerConfig `json:"mcpServers,omitempty"`
	MaxTokens      int               `json:"maxTokens,omitempty"`
}

// event is one NDJSON line from the engine; only the fields we act on.
type event struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
}

// Client posts jobs to the engine.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client pointed at the engine root (e.g. http://engine:8080).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Dream consolidation runs can be long; give them a generous ceiling.
		http: &http.Client{Timeout: 30 * time.Minute},
	}
}

// Submit posts the job and drains the NDJSON stream until the terminal
// result/error event.
func (c *Client) Submit(ctx context.Context, job Job) error {
	if c.baseURL == "" {
		return fmt.Errorf("engine URL not configured (DEXIASK_AGENT_URL)")
	}
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/jobs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach engine: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("engine returned HTTP %d", resp.StatusCode)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "error":
			if ev.Message != "" {
				return fmt.Errorf("engine error: %s", ev.Message)
			}
			return fmt.Errorf("engine error")
		case "result":
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("engine stream error: %w", err)
	}
	// Stream closed without a terminal event — treat as success (the run ended).
	return nil
}
