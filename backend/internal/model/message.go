package model

import (
	"encoding/json"
	"time"
)

// Message role constants.
const (
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
)

// Message status constants.
const (
	MessageStatusRunning  = "running"
	MessageStatusComplete = "complete"
	MessageStatusPartial  = "partial"
	MessageStatusError    = "error"
)

// Message represents a single message in a conversation transcript.
type Message struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	ConversationID string    `gorm:"index;uniqueIndex:ux_messages_conv_seq,priority:1" json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	Seq            int       `gorm:"uniqueIndex:ux_messages_conv_seq,priority:2" json:"seq"`
	Status         string    `json:"status"`
	Model          string    `json:"model"`
	CreatedAt      time.Time `json:"created_at"`
	// Questions holds the JSON inputs of any AskChoice tool calls this message
	// made, so the interactive question card can be reconstructed on reload.
	Questions json.RawMessage `gorm:"type:text;serializer:json" json:"questions,omitempty"`
}

// AppendMessageInput is the input for appending a message to a conversation.
type AppendMessageInput struct {
	ConversationID string
	Role           string
	Content        string
	Status         string
	Model          string
}

// Validate validates the append input.
func (i *AppendMessageInput) Validate() error {
	if i.ConversationID == "" {
		return ErrInvalidInput("conversation_id is required")
	}
	if i.Role == "" {
		return ErrInvalidInput("role is required")
	}
	if i.Status == "" {
		return ErrInvalidInput("status is required")
	}
	return nil
}

// UpdateMessageInput is the input for updating a message's content and status.
type UpdateMessageInput struct {
	ID        string
	Content   string
	Status    string
	Model     string
	Questions json.RawMessage
}

// Validate validates the update input.
func (i *UpdateMessageInput) Validate() error {
	if i.ID == "" {
		return ErrInvalidInput("id is required")
	}
	if i.Status == "" {
		return ErrInvalidInput("status is required")
	}
	return nil
}

// MessageWithAttachments is a message enriched with its file attachments.
type MessageWithAttachments struct {
	Message
	Attachments []*Attachment `json:"attachments"`
}
