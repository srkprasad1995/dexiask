package consolidate

import (
	"fmt"
	"strings"

	"github.com/dexiask/memory/internal/store"
)

// BuildWorkingContext assembles the "Pending Working Memory" section for the
// dream agent: iterate every scope that holds working observations, read its
// unprocessed tail, and label each block with the exact (scope, scope_id) the
// dream must consolidate it into.
func BuildWorkingContext(w *store.Working) string {
	var sections []string
	for _, sp := range w.ListScopes() {
		chunk := w.ReadPending(sp.Scope, sp.ScopeID, true)
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		label := sp.Scope
		if sp.Scope != "global" {
			label = sp.Scope + "/" + sp.ScopeID
		}
		consolidate := "(Consolidate these into scope=" + sp.Scope
		if sp.ScopeID != "" {
			consolidate += ", scope_id=" + sp.ScopeID
		}
		consolidate += ")"
		sections = append(sections, fmt.Sprintf("### Scope: %s\n%s\n\n%s", label, consolidate, chunk))
	}
	if len(sections) == 0 {
		return ""
	}
	return "## Pending Working Memory\n\n" + strings.Join(sections, "\n\n")
}
