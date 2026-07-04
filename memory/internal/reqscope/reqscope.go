// Package reqscope parses the per-request scoping headers (shared by the REST
// and MCP entrypoints) into a pivot.RequestScope and threads it through the
// request context. A request without X-Workspace-Id is refused by the caller.
package reqscope

import (
	"context"
	"net/http"
	"strings"

	"github.com/dexiask/memory/internal/pivot"
)

type ctxKey struct{}

// Parse reads the scoping headers. ok is false when X-Workspace-Id is missing.
//
//	X-Workspace-Id    required — the tenant boundary
//	X-User-Id         default "default" — user-scoped memory identity
//	X-Role            drives dream routing + writable-scope defaults
//	X-Writable-Scopes CSV — the writable set; nil falls back to the role default
func Parse(r *http.Request) (pivot.RequestScope, bool) {
	ws := strings.TrimSpace(r.Header.Get("X-Workspace-Id"))
	if ws == "" {
		return pivot.RequestScope{}, false
	}
	user := strings.TrimSpace(r.Header.Get("X-User-Id"))
	if user == "" {
		user = "default"
	}
	var writable []string
	if csv := strings.TrimSpace(r.Header.Get("X-Writable-Scopes")); csv != "" {
		for _, s := range strings.Split(csv, ",") {
			if s = strings.TrimSpace(s); s != "" {
				writable = append(writable, s)
			}
		}
	}
	return pivot.RequestScope{
		WorkspaceID: ws,
		UserID:      user,
		Role:        strings.TrimSpace(r.Header.Get("X-Role")),
		Writable:    writable,
	}, true
}

// With stores the scope on the context.
func With(ctx context.Context, scope pivot.RequestScope) context.Context {
	return context.WithValue(ctx, ctxKey{}, scope)
}

// From retrieves the scope from the context.
func From(ctx context.Context) (pivot.RequestScope, bool) {
	scope, ok := ctx.Value(ctxKey{}).(pivot.RequestScope)
	return scope, ok
}
