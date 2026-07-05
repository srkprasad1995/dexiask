package model

import "time"

// MCPServer is a user-defined remote MCP server injected into every ask Job
// alongside the built-in indexer server.
//
// Servers are scoped per user (UserID == GitHub id): Headers may hold auth
// secrets, so one user's servers must never be injected into another user's
// turn. WorkspaceID stays the fixed single workspace. Headers are stored as JSON
// in Postgres (plaintext-at-rest is acceptable for this local tool).
type MCPServer struct {
	ID          string            `gorm:"primaryKey" json:"id"`
	WorkspaceID string            `json:"workspace_id"`
	UserID      string            `gorm:"index" json:"user_id"`
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
	UserID      string
	Name        string
	Type        string
	URL         string
	Headers     map[string]string
	Enabled     bool
}

// Validate validates the create input. UserID is recorded for audit (the admin
// who created the server) but is optional.
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

// ListMCPServersFilter represents filters for listing MCP servers. MCP servers
// are workspace-wide (admin-managed) and injected into every user's turn, so the
// list is not user-scoped.
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
