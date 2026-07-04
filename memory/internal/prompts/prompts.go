// Package prompts provides the embedded agent prompts for the memory service.
// Prompt text lives in .md files (never as Go string literals).
package prompts

import (
	_ "embed"
	"strings"
)

//go:embed dream.md
var dream string

// Dream returns the dream/consolidation system prompt.
func Dream() string { return strings.TrimRight(dream, "\n") }
