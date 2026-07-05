package model

import "time"

// Role names. The first user to sign in bootstraps as RoleAdmin; everyone else
// must be invited and joins as RoleMember. Admins manage indexing, MCP servers,
// memory consolidation, and invites; members chat + browse.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// User is a GitHub-authenticated account. The primary key is the GitHub numeric
// user id (as a string) so product rows scope directly to a stable identity that
// never changes when the user renames their login.
//
// EncryptedToken holds the user's GitHub token (OAuth or PAT), AES-GCM encrypted
// at rest (see internal/auth/crypto.go). It identifies the user; it is never
// returned to the browser.
type User struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	Login          string    `json:"login"`
	Name           string    `json:"name"`
	AvatarURL      string    `json:"avatar_url"`
	Role           string    `json:"role"`
	EncryptedToken string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UpsertUserInput is the input for creating or updating a user after login.
type UpsertUserInput struct {
	ID             string
	Login          string
	Name           string
	AvatarURL      string
	Role           string
	EncryptedToken string
}

// Validate validates the upsert input.
func (i *UpsertUserInput) Validate() error {
	if i.ID == "" {
		return ErrInvalidInput("id is required")
	}
	if i.Login == "" {
		return ErrInvalidInput("login is required")
	}
	return nil
}
