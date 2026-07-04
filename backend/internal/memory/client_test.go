package memory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dexiask/dexiask/internal/config"
)

func TestDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/memory/digest" ||
			r.Header.Get("X-Workspace-Id") != config.FixedWorkspaceID ||
			r.Header.Get("X-User-Id") != "42" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"digest":"## Memory\n- x"}`))
	}))
	defer srv.Close()

	got := NewClient(srv.URL).Digest(context.Background(), "42")
	if got != "## Memory\n- x" {
		t.Fatalf("digest = %q", got)
	}
}

func TestDigestDisabledWhenNoURL(t *testing.T) {
	if got := NewClient("").Digest(context.Background(), "42"); got != "" {
		t.Fatalf("expected empty digest when disabled, got %q", got)
	}
}

func TestDigestBestEffortOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if got := NewClient(srv.URL).Digest(context.Background(), "42"); got != "" {
		t.Fatalf("expected empty digest on upstream error, got %q", got)
	}
}
