package model

import "time"

// User is a GitHub-authenticated account. The primary key is the GitHub numeric
// user id (as a string) so product rows scope directly to a stable identity that
// never changes when the user renames their login.
//
// EncryptedToken holds the user's GitHub OAuth access token, AES-GCM encrypted at
// rest (see internal/auth/crypto.go). It is used as the git token for private-repo
// indexing and to validate repo access via the GitHub API. It is never returned
// to the browser.
type User struct {
	ID             string    `gorm:"primaryKey" json:"id"`
	Login          string    `json:"login"`
	Name           string    `json:"name"`
	AvatarURL      string    `json:"avatar_url"`
	EncryptedToken string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UpsertUserInput is the input for creating or updating a user after a GitHub
// OAuth exchange.
type UpsertUserInput struct {
	ID             string
	Login          string
	Name           string
	AvatarURL      string
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
