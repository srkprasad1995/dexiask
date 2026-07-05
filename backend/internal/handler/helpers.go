package handler

import (
	"encoding/json"
	"net/http"

	"github.com/dexiask/dexiask/internal/auth"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
)

// requirePrincipal returns the authenticated principal the auth middleware
// injected. When absent (should not happen behind the middleware) it writes a
// 401 and returns ok=false.
func requirePrincipal(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	p, ok := auth.UserFromContext(r.Context())
	if !ok || p.UserID == "" {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return auth.Principal{}, false
	}
	return p, true
}

// requireAdmin is requirePrincipal plus an admin-role check. Non-admins get 403.
func requireAdmin(w http.ResponseWriter, r *http.Request) (auth.Principal, bool) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return auth.Principal{}, false
	}
	if !p.IsAdmin() {
		writeError(w, http.StatusForbidden, "admin only")
		return auth.Principal{}, false
	}
	return p, true
}

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error envelope with the given status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeServiceError maps an application error to an HTTP status + message.
func writeServiceError(w http.ResponseWriter, err error) {
	writeError(w, pkgerrors.HTTPStatus(err), err.Error())
}
