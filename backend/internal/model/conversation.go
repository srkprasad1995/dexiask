package model

import "time"

// Conversation represents a persistent chat thread.
//
// Trimmed for Dexiask: no workspace/project/stage/task/memory/repo fields.
// WorkspaceID and UserID are retained (stamped with the fixed identity) so the
// data model and repository queries mirror upstream, but they are never derived
// from client input.
type Conversation struct {
	ID          string `gorm:"primaryKey" json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Title       string `json:"title"`
	// SessionID is the Claude Agent SDK session identifier (Agent Job Protocol).
	// Populated after the first completed run; passed back to the engine on
	// subsequent turns so it can resume the conversation natively (resume=<id>).
	SessionID string    `json:"session_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateConversationInput is the input for creating a new conversation.
type CreateConversationInput struct {
	WorkspaceID string
	UserID      string
	Title       string
}

// Validate validates the create input.
func (i *CreateConversationInput) Validate() error {
	if i.WorkspaceID == "" {
		return ErrInvalidInput("workspace_id is required")
	}
	if i.UserID == "" {
		return ErrInvalidInput("user_id is required")
	}
	return nil
}

// ListConversationsFilter represents filters for listing conversations.
type ListConversationsFilter struct {
	WorkspaceID string
	UserID      string
	PageSize    int
	PageToken   string
}

// Validate validates the list filter.
func (f *ListConversationsFilter) Validate() error {
	if f.WorkspaceID == "" {
		return ErrInvalidInput("workspace_id is required")
	}
	if f.UserID == "" {
		return ErrInvalidInput("user_id is required")
	}
	if f.PageSize < 0 {
		return ErrInvalidInput("page_size must be non-negative")
	}
	if f.PageSize > 100 {
		return ErrInvalidInput("page_size must not exceed 100")
	}
	return nil
}
