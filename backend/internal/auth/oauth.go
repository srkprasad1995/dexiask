package auth

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

// OAuth wraps the GitHub OAuth2 authorization-code flow.
type OAuth struct {
	cfg *oauth2.Config
}

// NewOAuth builds the GitHub OAuth config. The "repo" scope is requested so the
// issued token can clone private repositories for indexing and validate
// repo access; "read:user" resolves the account identity.
func NewOAuth(clientID, clientSecret, callbackURL string) *OAuth {
	return &OAuth{cfg: &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  callbackURL,
		Endpoint:     github.Endpoint,
		Scopes:       []string{"read:user", "repo"},
	}}
}

// AuthCodeURL returns the GitHub authorize URL for the given anti-CSRF state.
func (o *OAuth) AuthCodeURL(state string) string {
	return o.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

// Exchange trades an authorization code for an access token.
func (o *OAuth) Exchange(ctx context.Context, code string) (string, error) {
	tok, err := o.cfg.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}
