package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/dexiask/memory/internal/pivot"
	"github.com/dexiask/memory/internal/reqscope"
	"github.com/dexiask/memory/internal/store"
)

const viewDesc = "View consolidated memory entries. Without a path, lists the index for a scope. " +
	"With a path (e.g. 'conventions/go-errors.md'), reads the full entry content. " +
	"With a topic directory path (e.g. 'conventions'), lists entries in that topic. " +
	"Scopes: 'global', 'user', 'repo'. " +
	"scope_id is required for the 'repo' scope (the repo id). " +
	"For the 'user' scope, scope_id is your own identity and is filled in automatically — leave it blank."

const writeDesc = "Write to memory. " +
	"For chat roles: records observations to working memory (daily files). " +
	"Use 'observe' command with plain text content. " +
	"For dream role: curates consolidated memory. " +
	"Commands: 'observe' (append observation to working), " +
	"'create' (new consolidated entry in topic/entry), " +
	"'update' (existing entry), 'delete' (archive entry), " +
	"'clear_working' (mark processed working observations as consolidated — " +
	"files are retained, not deleted; future dreams only see newer observations), " +
	"'list_scopes' (discover available scopes). " +
	"scope + scope_id identify where to write (for the 'user' scope, " +
	"scope_id is your own identity and is filled in automatically — leave it blank). " +
	"For create/update/delete: topic + entry_name identify the entry. content is the body text."

const searchDesc = "Search across all consolidated memory scopes for entries matching a query string. " +
	"Returns a list of matching file paths."

func viewTool() mcp.Tool {
	return mcp.NewTool("memory_view",
		mcp.WithDescription(viewDesc),
		mcp.WithString("scope", mcp.Required()),
		mcp.WithString("scope_id"),
		mcp.WithString("path"),
	)
}

func writeTool() mcp.Tool {
	return mcp.NewTool("memory_write",
		mcp.WithDescription(writeDesc),
		mcp.WithString("command", mcp.Required()),
		mcp.WithString("scope"),
		mcp.WithString("scope_id"),
		mcp.WithString("topic"),
		mcp.WithString("entry_name"),
		mcp.WithString("content"),
		mcp.WithString("reason"),
		mcp.WithArray("filenames", mcp.Items(map[string]any{"type": "string"})),
	)
}

func searchTool() mcp.Tool {
	return mcp.NewTool("memory_search",
		mcp.WithDescription(searchDesc),
		mcp.WithString("query", mcp.Required()),
	)
}

func isDream(sc pivot.RequestScope) bool { return sc.Role == "dream" }

// writableFor returns the writable scope set: the X-Writable-Scopes header when
// present, otherwise the per-role registry default.
func writableFor(sc pivot.RequestScope) []string {
	if len(sc.Writable) > 0 {
		return sc.Writable
	}
	return pivot.WritableScopesForRole(sc.Role)
}

// resolveScopeID pins a chat role's user-scope id to its own identity; the dream
// role addresses arbitrary scopes and is left untouched.
func resolveScopeID(dream bool, scope, scopeID, userID string) string {
	if dream {
		return scopeID
	}
	if scope == "user" {
		return userID
	}
	return scopeID
}

func (s *Server) consolidated(sc pivot.RequestScope) *store.Consolidated {
	return store.NewConsolidated(s.root, sc, s.reg, writableFor(sc))
}

func (s *Server) working(sc pivot.RequestScope) *store.Working {
	return store.NewWorking(s.root, sc, s.reg)
}

func missingWorkspace() *mcp.CallToolResult {
	return mcp.NewToolResultError("missing workspace context (X-Workspace-Id required)")
}

func (s *Server) handleView(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sc, ok := reqscope.From(ctx)
	if !ok {
		return missingWorkspace(), nil
	}
	scope := req.GetString("scope", "repo")
	scopeID := resolveScopeID(isDream(sc), scope, req.GetString("scope_id", ""), sc.UserID)
	path := req.GetString("path", "")
	return mcp.NewToolResultText(s.consolidated(sc).View(scope, scopeID, path)), nil
}

func (s *Server) handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sc, ok := reqscope.From(ctx)
	if !ok {
		return missingWorkspace(), nil
	}
	q := req.GetString("query", "")
	return mcp.NewToolResultText(s.consolidated(sc).Search(q)), nil
}

func (s *Server) handleWrite(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sc, ok := reqscope.From(ctx)
	if !ok {
		return missingWorkspace(), nil
	}
	dream := isDream(sc)
	command := req.GetString("command", "")
	scope := req.GetString("scope", "repo")
	scopeID := resolveScopeID(dream, scope, req.GetString("scope_id", ""), sc.UserID)
	topic := req.GetString("topic", "")
	entryName := req.GetString("entry_name", "")
	content := req.GetString("content", "")
	reason := req.GetString("reason", "")
	var filenames []string
	if req.GetArguments()["filenames"] != nil {
		filenames = req.GetStringSlice("filenames", nil)
		if filenames == nil {
			filenames = []string{}
		}
	}

	cons := s.consolidated(sc)
	wm := s.working(sc)

	// The dream consolidates many users' memory, so a user-scope write MUST name
	// the owning user — otherwise it would fall back to the dream's own identity
	// and strand the user's memory.
	if dream && scope == "user" && scopeID == "" &&
		(command == "create" || command == "update" || command == "delete" || command == "observe" || command == "clear_working") {
		return mcp.NewToolResultText(
			"Error: user-scope writes require an explicit scope_id (the user id shown " +
				"in the pending working-memory section). Do not use your own identity for " +
				"another user's memory."), nil
	}

	var text string
	switch command {
	case "observe":
		if err := wm.Append(scope, scopeID, content, ""); err != "" {
			text = err
		} else {
			text = "Observation recorded."
		}
	case "create":
		if !dream {
			text = "Error: only the dream role can create consolidated entries. Use 'observe' to record observations."
		} else {
			text = cons.Create(scope, scopeID, topic, entryName, content)
		}
	case "update":
		if !dream {
			text = "Error: only the dream role can update consolidated entries. Use 'observe' to record observations."
		} else {
			text = cons.Update(scope, scopeID, topic, entryName, content)
		}
	case "delete":
		if !dream {
			text = "Error: only the dream role can delete consolidated entries."
		} else {
			text = cons.Delete(scope, scopeID, topic, entryName, reason)
		}
	case "clear_working":
		if !dream {
			text = "Error: only the dream role can mark working memory processed."
		} else {
			count := wm.MarkProcessed(scope, scopeID, filenames)
			text = fmt.Sprintf("Marked %d working file(s) as processed "+
				"(retained for review; future dreams see only newer observations).", count)
		}
	case "list_scopes":
		scopes := cons.ListScopes()
		if len(scopes) == 0 {
			text = "(no scopes yet)"
		} else {
			var lines []string
			for _, sinfo := range scopes {
				lines = append(lines, "- "+sinfo.Scope+"/"+sinfo.ID+" ("+sinfo.Label+")")
			}
			text = strings.Join(lines, "\n")
		}
	default:
		text = "Error: unknown command '" + command + "'. Use observe, create, update, delete, clear_working, or list_scopes."
	}
	return mcp.NewToolResultText(text), nil
}
