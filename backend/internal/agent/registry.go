package agent

import (
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
)

// Registry maps runtime names to Runtime implementations. Populated at startup.
type Registry struct {
	runtimes map[string]Runtime
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{runtimes: make(map[string]Runtime)}
}

// Register adds a runtime to the registry.
func (r *Registry) Register(rt Runtime) { r.runtimes[rt.Name()] = rt }

// Get returns the Runtime for the given name or an error if unavailable.
func (r *Registry) Get(name string) (Runtime, error) {
	rt, ok := r.runtimes[name]
	if !ok {
		return nil, pkgerrors.InvalidArgumentf("runtime %q is not available", name)
	}
	return rt, nil
}

// Available returns the set of registered runtime names.
func (r *Registry) Available() []string {
	names := make([]string, 0, len(r.runtimes))
	for n := range r.runtimes {
		names = append(names, n)
	}
	return names
}
