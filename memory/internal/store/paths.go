package store

import (
	"path/filepath"

	"github.com/dexiask/memory/internal/pivot"
)

// ScopeDir returns the on-disk directory for a pivot's memory of the given kind
// ("memories" or "working"), under the shared per-workspace tree
// <root>/<ws>/memory/<kind>/<name>[/<id>]. The global (singleton) pivot has no id
// subdirectory.
func ScopeDir(root, ws, kind string, p *pivot.Pivot, id string) string {
	base := filepath.Join(root, ws, "memory", kind, p.Name)
	if p.SingletonID != "" {
		return base
	}
	return filepath.Join(base, id)
}
