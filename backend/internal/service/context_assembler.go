package service

import (
	"path"
	"strings"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/model"
)

// ContextAssembler converts a stored conversation history into the message
// slice sent to the engine. Each message carries its file attachments so a file
// attached on turn 1 stays referenceable by the engine on every later turn.
type ContextAssembler interface {
	Assemble(history []*model.MessageWithAttachments) []agent.Message
}

// WindowAssembler keeps only the most recent MaxMessages messages (0 = all).
//
// Messages are filtered before windowing:
//   - empty content with no attachments → skipped (the model API rejects them)
//   - status "error" or "running" → skipped (failed/in-flight turns)
type WindowAssembler struct {
	// MaxMessages is the maximum number of messages to include (0 = unlimited).
	MaxMessages int
}

// Assemble implements ContextAssembler.
func (a WindowAssembler) Assemble(history []*model.MessageWithAttachments) []agent.Message {
	filtered := make([]*model.MessageWithAttachments, 0, len(history))
	for _, m := range history {
		if m.Status == model.MessageStatusError || m.Status == model.MessageStatusRunning {
			continue
		}
		if m.Content == "" && len(m.Attachments) == 0 {
			continue
		}
		filtered = append(filtered, m)
	}

	if a.MaxMessages > 0 && len(filtered) > a.MaxMessages {
		filtered = filtered[len(filtered)-a.MaxMessages:]
	}

	out := make([]agent.Message, 0, len(filtered))
	for _, m := range filtered {
		msg := agent.Message{Role: m.Role, Content: m.Content}
		for _, att := range m.Attachments {
			kind := "file"
			if strings.HasPrefix(att.MediaType, "image/") {
				kind = "image"
			}
			// RelPath is relative to the workspace mount (".dexiask/..."); joining
			// with the engine mount root yields the container-side path inside the
			// Job's WorkspacePath jail.
			msg.Attachments = append(msg.Attachments, agent.Attachment{
				Kind:      kind,
				Path:      path.Join(agent.WorkspaceMount, att.RelPath),
				MediaType: att.MediaType,
				Filename:  att.Filename,
			})
		}
		out = append(out, msg)
	}
	return out
}
