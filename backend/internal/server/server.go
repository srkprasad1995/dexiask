// Package server wires the HTTP routes and constructs the http.Server.
package server

import (
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/dexiask/dexiask/internal/auth"
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
	Memory       *handler.MemoryHandler
	MCPServer    *handler.MCPServerHandler
	Auth         *handler.AuthHandler
}

// NewServer builds the http.Server with all routes registered. Product routes are
// wrapped with the auth middleware (which injects the request principal); the
// login/callback/logout endpoints and health are left unauthenticated.
func NewServer(cfg *config.Config, h Handlers, authn *auth.Authenticator, db *gorm.DB, log *logger.Logger) *http.Server {
	// Protected app routes — served behind the auth middleware.
	app := http.NewServeMux()
	app.Handle("/v1/chat/stream", h.Chat)
	app.HandleFunc("/v1/chat/stop", h.Chat.ServeStop)
	app.HandleFunc("/v1/conversations", h.Conversation.ListConversations)
	app.HandleFunc("/v1/conversations/", h.Conversation.GetConversation)
	app.HandleFunc("/v1/attachments", h.Attachment.Upload)
	app.HandleFunc("/v1/attachments/", h.Attachment.Serve)
	app.HandleFunc("/v1/mcp-servers", h.MCPServer.ServeCollection)
	app.HandleFunc("/v1/mcp-servers/", h.MCPServer.ServeItem)
	app.Handle("/v1/indexer/", h.Indexer)
	app.Handle("/v1/memory/", h.Memory)
	app.HandleFunc("/v1/auth/me", h.Auth.Me)
	// Admin: invites + users (the handlers enforce the admin role).
	app.HandleFunc("/v1/invites", h.Auth.Invites)
	app.HandleFunc("/v1/invites/", h.Auth.DeleteInvite)
	app.HandleFunc("/v1/users", h.Auth.Users)
	protected := authn.Middleware(app)

	// Root mux — health + unauthenticated auth endpoints + the protected app.
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.HealthCheck(db); err != nil {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/auth/config", h.Auth.Config)
	mux.HandleFunc("/v1/auth/login", h.Auth.Login)
	mux.HandleFunc("/v1/auth/callback", h.Auth.Callback)
	mux.HandleFunc("/v1/auth/token-login", h.Auth.TokenLogin)
	mux.HandleFunc("/v1/auth/logout", h.Auth.Logout)
	mux.Handle("/", protected)

	log.Info("http routes registered",
		zap.String("chat_stream", fmt.Sprintf(":%d/v1/chat/stream", cfg.Port)),
		zap.Bool("require_auth", cfg.RequireAuth))

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}
}
