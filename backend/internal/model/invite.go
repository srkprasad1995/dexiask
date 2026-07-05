package model

import (
	"strings"
	"time"
)

// Invite authorizes a GitHub login to join the workspace as a member. Admins
// create invites; a matching invite is consumed the first time that user signs
// in. Keyed by lowercased GitHub login (the id isn't known until they log in).
type Invite struct {
	Login     string    `gorm:"primaryKey" json:"login"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// NormalizeLogin lowercases + trims a GitHub login for stable invite matching.
func NormalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}
