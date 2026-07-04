package engineclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubmitDrainsToResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"type":"text.delta","text":"working"}` + "\n"))
		_, _ = w.Write([]byte(`{"type":"result","status":"ok"}` + "\n"))
	}))
	defer srv.Close()

	if err := New(srv.URL).Submit(context.Background(), Job{Role: "ask"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

func TestSubmitReturnsEngineError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"type":"error","message":"boom"}` + "\n"))
	}))
	defer srv.Close()

	err := New(srv.URL).Submit(context.Background(), Job{Role: "ask"})
	if err == nil || err.Error() != "engine error: boom" {
		t.Fatalf("expected engine error, got %v", err)
	}
}

func TestSubmitNoURL(t *testing.T) {
	if err := New("").Submit(context.Background(), Job{}); err == nil {
		t.Fatal("expected error when engine URL is unset")
	}
}
