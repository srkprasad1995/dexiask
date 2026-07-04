package model

import "time"

// Session is a server-side login session. The signed session cookie carries only
// the opaque Session ID; the row maps it to a user and an expiry so logout and
// expiry are enforced server-side.
type Session struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"index" json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}
