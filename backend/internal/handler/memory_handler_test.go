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

func TestMemoryHandler_InjectsScopeHeaders(t *testing.T) {
	var gotWorkspace, gotUser, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotWorkspace = r.Header.Get("X-Workspace-Id")
		gotUser = r.Header.Get("X-User-Id")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	h := handler.NewMemoryHandler(upstream.URL, logger.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/memory/scopes", nil).
		WithContext(auth.WithUser(context.Background(), auth.Principal{UserID: "42"}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if gotWorkspace == "" || gotUser != "42" {
		t.Fatalf("headers wrong: ws=%q user=%q", gotWorkspace, gotUser)
	}
	if gotPath != "/v1/memory/scopes" {
		t.Fatalf("upstream path = %q", gotPath)
	}
}

func TestMemoryHandler_ConsolidateMapping(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()

	h := handler.NewMemoryHandler(upstream.URL, logger.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/memory/consolidate", strings.NewReader(`{}`)).
		WithContext(auth.WithUser(context.Background(), auth.Principal{UserID: "42"}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// /v1/memory/consolidate maps to the service's /v1/consolidate.
	if gotPath != "/v1/consolidate" {
		t.Fatalf("consolidate mapped to %q, want /v1/consolidate", gotPath)
	}
}

func TestMemoryHandler_Unauthenticated(t *testing.T) {
	h := handler.NewMemoryHandler("http://memory:8080", logger.NewNop())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/memory/scopes", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without principal, got %d", rec.Code)
	}
}
