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

	h := handler.NewIndexerHandler(upstream.URL, logger.NewNop())
	// A member may read (GET).
	req := httptest.NewRequest(http.MethodGet, "/v1/indexer/v1/repos", nil).WithContext(memberCtx())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || gotWorkspace == "" {
		t.Fatalf("status=%d workspace=%q", rec.Code, gotWorkspace)
	}
}

func TestIndexerHandler_MutationRequiresAdmin(t *testing.T) {
	// Upstream must never be reached for a non-admin mutation.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be called for a member mutation")
	}))
	defer upstream.Close()

	h := handler.NewIndexerHandler(upstream.URL, logger.NewNop())
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

	h := handler.NewIndexerHandler(upstream.URL, logger.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/indexer/reindex", strings.NewReader(`{"repo":"r"}`)).
		WithContext(adminCtx())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || gotPath != "/reindex" {
		t.Fatalf("status=%d path=%q", rec.Code, gotPath)
	}
}
