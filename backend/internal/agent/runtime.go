package agent

import "context"

// Runtime is the pluggable engine interface. Dexiask ships one implementation
// (the Claude HTTP engine) but keeps the abstraction so the orchestrator stays
// engine-agnostic — adding an engine is a new adapter, no feature-code changes.
type Runtime interface {
	// Name returns the canonical runtime identifier.
	Name() string
	// Start launches the engine for the given job and returns a channel of
	// events, closed when the engine terminates. Cancelling ctx kills the run.
	Start(ctx context.Context, job Job) (<-chan Event, error)
}
