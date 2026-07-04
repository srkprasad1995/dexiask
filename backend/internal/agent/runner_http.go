package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"go.uber.org/zap"
)

// HTTPRunner calls a long-running engine HTTP service.
//
// Protocol: POST <baseURL>/v1/jobs with the Job JSON body; the engine responds
// with a streaming application/x-ndjson body — one Event JSON per line,
// terminated by a "result" or "error" event. Cancelling ctx aborts the request.
type HTTPRunner struct {
	client  *http.Client
	baseURL string
	logger  *logger.Logger
}

// NewHTTPRunner returns an HTTPRunner pointed at the engine at baseURL.
func NewHTTPRunner(baseURL string, client *http.Client, log *logger.Logger) *HTTPRunner {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPRunner{
		client:  client,
		baseURL: strings.TrimRight(baseURL, "/"),
		logger:  log,
	}
}

// Run posts the Job to the engine and returns a channel of Events. The channel
// is closed when the engine response body closes. Cancelling ctx aborts the
// HTTP request.
func (r *HTTPRunner) Run(ctx context.Context, job Job) (<-chan Event, error) {
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return nil, pkgerrors.Internal("failed to marshal job", err)
	}

	endpoint := r.baseURL + "/v1/jobs"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(jobJSON))
	if err != nil {
		return nil, pkgerrors.Internal("failed to build HTTP request", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")

	r.logger.Info("starting agent HTTP job",
		zap.String("endpoint", endpoint),
		zap.String("role", job.Role),
		zap.String("model", job.Model),
		zap.String("session_id", job.SessionID),
	)

	resp, err := r.client.Do(req)
	if err != nil {
		// A canceled context (the run was stopped by the user) is a deliberate
		// abort, not an engine failure — return the context error unwrapped so
		// callers can detect it and stay quiet.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, pkgerrors.Internal("failed to reach engine", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, pkgerrors.Internal(fmt.Sprintf("engine returned HTTP %d", resp.StatusCode), nil)
	}

	events := make(chan Event, 32)

	// Parse NDJSON response body → events channel.
	go func() {
		defer close(events)
		defer resp.Body.Close()

		sc := bufio.NewScanner(resp.Body)
		// Engine event lines can be large (tool inputs/results); grow the buffer.
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				r.logger.Warn("failed to parse agent event",
					zap.String("line", line), zap.Error(err))
				continue
			}
			select {
			case events <- ev:
			case <-ctx.Done():
				return
			}
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			r.logger.Warn("engine response stream error", zap.Error(err))
		}
	}()

	return events, nil
}
