package model

import "time"

// MCPServer is a user-defined remote MCP server injected into every ask Job
// alongside the built-in indexer server.
//
// Dexiask is single-user/single-workspace: WorkspaceID is stamped with the
// fixed identity so the data model mirrors upstream, but it is never derived
// from client input. Headers may hold auth secrets; they are stored as JSON in
// Postgres (plaintext-at-rest is acceptable for this single-user local tool).
type MCPServer struct {
	ID          string            `gorm:"primaryKey" json:"id"`
	WorkspaceID string            `json:"workspace_id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "http" | "sse"
	URL         string            `json:"url"`
	Headers     map[string]string `gorm:"serializer:json" json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// validMCPType reports whether t is an accepted MCP transport type.
func validMCPType(t string) bool {
	return t == "http" || t == "sse"
}

// CreateMCPServerInput is the input for creating a new MCP server.
type CreateMCPServerInput struct {
	WorkspaceID string
	Name        string
	Type        string
	URL         string
	Headers     map[string]string
	Enabled     bool
}

// Validate validates the create input.
func (i *CreateMCPServerInput) Validate() error {
	if i.WorkspaceID == "" {
		return ErrInvalidInput("workspace_id is required")
	}
	if i.Name == "" {
		return ErrInvalidInput("name is required")
	}
	if i.URL == "" {
		return ErrInvalidInput("url is required")
	}
	if !validMCPType(i.Type) {
		return ErrInvalidInput("type must be one of: http, sse")
	}
	return nil
}

// UpdateMCPServerInput is the input for updating an existing MCP server. Only
// non-nil fields are applied.
type UpdateMCPServerInput struct {
	ID      string
	Name    *string
	Type    *string
	URL     *string
	Headers *map[string]string
	Enabled *bool
}

// Validate validates the update input.
func (i *UpdateMCPServerInput) Validate() error {
	if i.ID == "" {
		return ErrInvalidInput("id is required")
	}
	if i.Name != nil && *i.Name == "" {
		return ErrInvalidInput("name must not be empty")
	}
	if i.URL != nil && *i.URL == "" {
		return ErrInvalidInput("url must not be empty")
	}
	if i.Type != nil && !validMCPType(*i.Type) {
		return ErrInvalidInput("type must be one of: http, sse")
	}
	return nil
}

// ListMCPServersFilter represents filters for listing MCP servers.
type ListMCPServersFilter struct {
	WorkspaceID string
	// EnabledOnly restricts the result to enabled servers (used at injection time).
	EnabledOnly bool
}

// Validate validates the list filter.
func (f *ListMCPServersFilter) Validate() error {
	if f.WorkspaceID == "" {
		return ErrInvalidInput("workspace_id is required")
	}
	return nil
}
