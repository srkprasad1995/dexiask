// Package server wires the HTTP routes and constructs the http.Server.
package server

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/handler"
	"github.com/dexiask/dexiask/internal/infrastructure/database"
	"github.com/dexiask/dexiask/internal/pkg/logger"
)

// Handlers bundles the HTTP handlers the router wires.
type Handlers struct {
	Chat         *handler.ChatHandler
	Conversation *handler.ConversationHandler
	Attachment   *handler.AttachmentHandler
	Indexer      *handler.IndexerHandler
	MCPServer    *handler.MCPServerHandler
}

// NewServer builds the http.Server with all routes registered.
func NewServer(cfg *config.Config, h Handlers, db *gorm.DB, log *logger.Logger) *http.Server {
	mux := http.NewServeMux()

	// Health.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.HealthCheck(db); err != nil {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Chat SSE.
	mux.Handle("/v1/chat/stream", h.Chat)
	mux.HandleFunc("/v1/chat/stop", h.Chat.ServeStop)

	// Conversations.
	mux.HandleFunc("/v1/conversations", h.Conversation.ListConversations)
	mux.HandleFunc("/v1/conversations/", h.Conversation.GetConversation)

	// Attachments.
	mux.HandleFunc("/v1/attachments", h.Attachment.Upload)
	mux.HandleFunc("/v1/attachments/", h.Attachment.Serve)

	// Custom MCP servers.
	mux.HandleFunc("/v1/mcp-servers", h.MCPServer.ServeCollection)
	mux.HandleFunc("/v1/mcp-servers/", h.MCPServer.ServeItem)

	// Indexer control-plane reverse proxy.
	mux.Handle("/v1/indexer/", h.Indexer)

	log.Info("http routes registered",
		zap.String("chat_stream", fmt.Sprintf(":%d/v1/chat/stream", cfg.Port)))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}
}
