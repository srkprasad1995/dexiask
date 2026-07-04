package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dexiask/memory/internal/pivot"
)

func newAPI(t *testing.T) http.Handler {
	t.Helper()
	// No consolidate service needed for these read-path tests.
	api := New(t.TempDir(), pivot.Default(), nil)
	return api.Handler(http.NotFoundHandler())
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newAPI(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("healthz = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDigestRequiresWorkspace(t *testing.T) {
	h := newAPI(t)

	// No X-Workspace-Id → 400.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/memory/digest", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("without workspace = %d, want 400", rec.Code)
	}

	// With the header → 200 and a (possibly empty) digest field.
	req := httptest.NewRequest(http.MethodGet, "/v1/memory/digest", nil)
	req.Header.Set("X-Workspace-Id", "dexiask")
	req.Header.Set("X-User-Id", "u1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "digest") {
		t.Fatalf("with workspace = %d %s", rec.Code, rec.Body.String())
	}
}

func TestScopesRequiresWorkspace(t *testing.T) {
	rec := httptest.NewRecorder()
	newAPI(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/memory/scopes", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("scopes without workspace = %d, want 400", rec.Code)
	}
}
