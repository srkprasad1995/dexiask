package model

import "time"

// SlackThread maps a Slack thread (channel + thread timestamp) to a Dexiask
// conversation, so the bot reuses one conversation per Slack thread across turns.
type SlackThread struct {
	// ThreadKey is "<channelID>:<threadTS>" — the primary key.
	ThreadKey string `gorm:"primaryKey" json:"thread_key"`
	// ConversationID is the Dexiask conversation backing this thread.
	ConversationID string    `gorm:"not null" json:"conversation_id"`
	CreatedAt      time.Time `json:"created_at"`
}
