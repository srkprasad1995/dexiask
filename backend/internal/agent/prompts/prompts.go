// Package prompts provides role-specific system prompts for the agent
// subsystem. Dexiask ships only the "ask" role prompt plus the shared
// output-format guidance. Prompt text lives in embedded .md files (never as Go
// string literals) so the binary is self-contained and prompts stay editable.
package prompts

import (
	_ "embed"
	"strings"
)

//go:embed ask.md
var ask string

//go:embed output-format.md
var outputFormat string

const fallback = "You are a helpful AI assistant for software development."

var rolePrompts = map[string]string{
	"ask": ask,
}

// ForRole returns the system prompt for the given role. The shared
// output-format guidance is appended. Unknown roles receive the generic
// fallback plus the same guidance.
func ForRole(role string) string {
	base, ok := rolePrompts[role]
	if !ok {
		base = fallback
	}
	return strings.TrimRight(base, "\n") + "\n\n" + outputFormat
}
