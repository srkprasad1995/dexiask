package handler

import (
	"encoding/json"
	"net/http"

	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
)

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
