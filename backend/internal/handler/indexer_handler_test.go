package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/handler"
	"github.com/dexiask/dexiask/internal/pkg/logger"
)

func adminCtx() context.Context {
	return auth.WithUser(context.Background(), auth.Principal{UserID: "42", Login: "admin", Role: "admin"})
}

func memberCtx() context.Context {
	return auth.WithUser(context.Background(), auth.Principal{UserID: "7", Login: "member", Role: "member"})
}

func TestIndexerHandler_GetInjectsWorkspace(t *testing.T) {
	var gotWorkspace string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWorkspace = r.Header.Get("X-Workspace-Id")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := handler.NewIndexerHandler(upstream.URL, "", logger.NewNop())
	// A member may read (GET).
	req := httptest.NewRequest(http.MethodGet, "/v1/indexer/v1/repos", nil).WithContext(memberCtx())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || gotWorkspace == "" {
		t.Fatalf("status=%d workspace=%q", rec.Code, gotWorkspace)
	}
}

func TestIndexerHandler_ForwardsGatingHeaders(t *testing.T) {
	var admin, member map[string]string
	capture := func(dst *map[string]string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			*dst = map[string]string{
				"X-Internal-Token": r.Header.Get("X-Internal-Token"),
				"X-Repo-Access":    r.Header.Get("X-Repo-Access"),
				"X-User-Token":     r.Header.Get("X-User-Token"),
			}
			w.WriteHeader(http.StatusOK)
		}
	}
	up := httptest.NewServer(nil)
	defer up.Close()

	h := handler.NewIndexerHandler(up.URL, "s3cret", logger.NewNop())

	// Admin → unrestricted assertion, no user token.
	up.Config.Handler = capture(&admin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/indexer/v1/repos", nil).WithContext(adminCtx()))
	if admin["X-Internal-Token"] != "s3cret" || admin["X-Repo-Access"] != "all" || admin["X-User-Token"] != "" {
		t.Fatalf("admin forwarded headers = %+v", admin)
	}

	// Member → forwards their token, not unrestricted. Client-supplied gating
	// headers must be overwritten, not trusted.
	up.Config.Handler = capture(&member)
	req := httptest.NewRequest(http.MethodGet, "/v1/indexer/v1/repos", nil).
		WithContext(auth.WithUser(context.Background(), auth.Principal{UserID: "7", Role: "member", GitHubToken: "gho_m"}))
	req.Header.Set("X-Repo-Access", "all") // spoof attempt
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if member["X-User-Token"] != "gho_m" || member["X-Repo-Access"] != "" {
		t.Fatalf("member forwarded headers = %+v (spoofed X-Repo-Access must be dropped)", member)
	}
}

func TestIndexerHandler_MutationRequiresAdmin(t *testing.T) {
	// Upstream must never be reached for a non-admin mutation.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be called for a member mutation")
	}))
	defer upstream.Close()

	h := handler.NewIndexerHandler(upstream.URL, "", logger.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/indexer/reindex", strings.NewReader(`{"repo":"r"}`)).
		WithContext(memberCtx())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member reindex, got %d", rec.Code)
	}
}

func TestIndexerHandler_AdminMutationProxied(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := handler.NewIndexerHandler(upstream.URL, "", logger.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/indexer/reindex", strings.NewReader(`{"repo":"r"}`)).
		WithContext(adminCtx())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || gotPath != "/reindex" {
		t.Fatalf("status=%d path=%q", rec.Code, gotPath)
	}
}
