// Package mcp exposes the memory tools (memory_view / memory_write /
// memory_search) over a stateless streamable-HTTP MCP server. Every call is
// scoped to the request's workspace via the shared reqscope headers.
package mcp

import (
	"context"
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/dexiask/memory/internal/pivot"
	"github.com/dexiask/memory/internal/reqscope"
)

// Server holds the dependencies the tool handlers need.
type Server struct {
	root string
	reg  *pivot.Registry
}

// NewHandler builds the streamable-HTTP MCP handler. root is the memory volume
// mount; reg is the pivot registry.
func NewHandler(root string, reg *pivot.Registry) http.Handler {
	s := &Server{root: root, reg: reg}

	m := server.NewMCPServer("dexiask-memory", "1.0.0")
	m.AddTool(viewTool(), s.handleView)
	m.AddTool(writeTool(), s.handleWrite)
	m.AddTool(searchTool(), s.handleSearch)

	return server.NewStreamableHTTPServer(m,
		server.WithStateLess(true),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			if sc, ok := reqscope.Parse(r); ok {
				return reqscope.With(ctx, sc)
			}
			return ctx
		}),
	)
}
