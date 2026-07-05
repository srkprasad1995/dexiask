// Package auth implements GitHub OAuth login, server-side sessions, and the
// request-scoped identity (Principal) that the rest of the backend reads instead
// of a fixed user id. When no GitHub OAuth app is configured the middleware runs
// in dev-fallback mode and injects a single fixed principal, preserving the
// zero-config local experience.
package auth

import "context"

// Principal is the authenticated identity resolved for a request. UserID is the
// GitHub numeric user id (as a string); GitHubToken is the decrypted GitHub
// token; Role is "admin" or "member".
type Principal struct {
	UserID      string
	Login       string
	Role        string
	GitHubToken string
}

// IsAdmin reports whether the principal has the admin role.
func (p Principal) IsAdmin() bool { return p.Role == "admin" }

type ctxKey struct{}

// WithUser returns a copy of ctx carrying the principal.
func WithUser(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// UserFromContext returns the principal on ctx, if any.
func UserFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// MustUserID returns the principal's user id, falling back to the dev id when no
// principal is present. It exists so call sites that cannot fail (e.g. the Slack
// bot, which has no HTTP request) still resolve a stable id.
func MustUserID(ctx context.Context, fallback string) string {
	if p, ok := UserFromContext(ctx); ok && p.UserID != "" {
		return p.UserID
	}
	return fallback
}
