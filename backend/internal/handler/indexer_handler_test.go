package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/handler"
	"github.com/dexiask/dexiask/internal/pkg/logger"
)

func TestIndexerHandler_InjectsGitTokenAndWorkspace(t *testing.T) {
	var gotToken, gotWorkspace string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Git-Token")
		gotWorkspace = r.Header.Get("X-Workspace-Id")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := handler.NewIndexerHandler(upstream.URL, auth.NewGitHubClient(0), logger.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/indexer/v1/index/r", strings.NewReader(`{}`)).
		WithContext(auth.WithUser(context.Background(), auth.Principal{UserID: "42", GitHubToken: "gho_tok"}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if gotToken != "gho_tok" {
		t.Fatalf("X-Git-Token = %q, want gho_tok", gotToken)
	}
	if gotWorkspace == "" {
		t.Fatalf("X-Workspace-Id must be injected")
	}
}

func TestIndexerHandler_RepoAccessDenied(t *testing.T) {
	// GitHub API stub: the user cannot see octocat/secret.
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer gh.Close()

	// Upstream must never be reached.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be called when access is denied")
	}))
	defer upstream.Close()

	h := handler.NewIndexerHandler(upstream.URL, auth.NewGitHubClientWithBase(gh.URL, time.Minute), logger.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/indexer/v1/repos",
		strings.NewReader(`{"id":"r","url":"https://github.com/octocat/secret.git"}`)).
		WithContext(auth.WithUser(context.Background(), auth.Principal{UserID: "42", GitHubToken: "gho_tok"}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
