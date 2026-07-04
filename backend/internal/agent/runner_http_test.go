package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dexiask/dexiask/internal/pkg/logger"
)

// TestHTTPRunner_StreamsNDJSON verifies the runner POSTs the Job to /v1/jobs and
// parses the NDJSON response body into an ordered event channel.
func TestHTTPRunner_StreamsNDJSON(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"type":"text.delta","text":"hi"}`+"\n")
		io.WriteString(w, "\n") // blank line should be skipped
		io.WriteString(w, `{"type":"result","status":"ok","sessionId":"s-9"}`+"\n")
	}))
	defer srv.Close()

	runner := NewHTTPRunner(srv.URL, srv.Client(), logger.NewNop())
	events, err := runner.Run(context.Background(), Job{Role: "ask", Model: "m", SessionID: "prev"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var got []Event
	for ev := range events {
		got = append(got, ev)
	}
	if gotPath != "/v1/jobs" {
		t.Errorf("posted to %q, want /v1/jobs", gotPath)
	}
	if !strings.Contains(gotBody, `"sessionId":"prev"`) {
		t.Errorf("job body did not carry sessionId: %s", gotBody)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Type != "text.delta" || got[0].Text != "hi" {
		t.Errorf("event[0] = %+v", got[0])
	}
	if got[1].Type != "result" || got[1].SessionID != "s-9" {
		t.Errorf("event[1] = %+v", got[1])
	}
}

// TestHTTPRunner_NonOKStatus surfaces a non-200 engine response as an error.
func TestHTTPRunner_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	runner := NewHTTPRunner(srv.URL, srv.Client(), logger.NewNop())
	if _, err := runner.Run(context.Background(), Job{}); err == nil {
		t.Fatal("expected error on HTTP 500, got nil")
	}
}

// TestHTTPRunner_ContextCancel returns the context error (not an engine error)
// when the request is cancelled.
func TestHTTPRunner_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := NewHTTPRunner(srv.URL, srv.Client(), logger.NewNop())
	_, err := runner.Run(ctx, Job{})
	if err != context.Canceled {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
