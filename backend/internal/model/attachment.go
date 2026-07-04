package model

import "time"

// Attachment represents a file attached to a conversation message. Files are
// stored under .dexiask/conversations/<conversationID>/attachments/<fileID>-<name>.
type Attachment struct {
	// ID is the fileID UUID — also used as the storage file prefix.
	ID string `gorm:"primaryKey" json:"id"`
	// ConversationID is empty while a first-turn upload is pending reconciliation.
	ConversationID string `gorm:"index;not null" json:"conversation_id"`
	// MessageID is set after the turn that introduced the attachment completes.
	MessageID *string `gorm:"index" json:"message_id"`
	// UploadBucket groups uploads that arrive before the conversation exists.
	UploadBucket string `gorm:"index" json:"-"`
	Filename     string `gorm:"not null" json:"filename"`
	MediaType    string `gorm:"not null" json:"media_type"`
	Size         int64  `gorm:"not null" json:"size"`
	// RelPath is the path relative to the workspace mount root.
	RelPath   string    `gorm:"not null" json:"rel_path"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreAttachmentInput is the input for creating an attachment record.
type StoreAttachmentInput struct {
	ID             string
	ConversationID string
	UploadBucket   string
	Filename       string
	MediaType      string
	Size           int64
	RelPath        string
}

// Validate validates the input.
func (i *StoreAttachmentInput) Validate() error {
	if i.ID == "" {
		return ErrInvalidInput("id is required")
	}
	if i.Filename == "" {
		return ErrInvalidInput("filename is required")
	}
	if i.MediaType == "" {
		return ErrInvalidInput("media_type is required")
	}
	if i.RelPath == "" {
		return ErrInvalidInput("rel_path is required")
	}
	if i.ConversationID == "" && i.UploadBucket == "" {
		return ErrInvalidInput("conversation_id or upload_bucket is required")
	}
	return nil
}
