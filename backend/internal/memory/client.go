// Package memory is a thin client for the standalone memory service. The backend
// uses it to fetch the per-user memory digest injected into the ask system prompt.
package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/dexiask/dexiask/internal/config"
)

// Client fetches the memory digest from the memory service. A zero-value base URL
// disables it (Digest returns "").
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a memory client. An empty baseURL yields an inert client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Digest is fetched inline at job-build time; keep it snappy and best-effort.
		http: &http.Client{Timeout: 3 * time.Second},
	}
}

// Digest returns the read-time memory digest for the given user, scoped to the
// fixed workspace. It is best-effort: any failure yields "" so the turn proceeds
// without a digest.
func (c *Client) Digest(ctx context.Context, userID string) string {
	if c.baseURL == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/memory/digest", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-Workspace-Id", config.FixedWorkspaceID)
	req.Header.Set("X-User-Id", userID)
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var body struct {
		Digest string `json:"digest"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return ""
	}
	return body.Digest
}
