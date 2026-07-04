package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/agent/adapters"
	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/handler"
	"github.com/dexiask/dexiask/internal/infrastructure/database"
	"github.com/dexiask/dexiask/internal/memory"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/pkg/transaction"
	"github.com/dexiask/dexiask/internal/repository"
	"github.com/dexiask/dexiask/internal/server"
	"github.com/dexiask/dexiask/internal/service"
	slackbot "github.com/dexiask/dexiask/internal/slack"
)

// ConfigModule provides the resolved configuration.
var ConfigModule = fx.Options(
	fx.Provide(config.Load),
)

// InfrastructureModule provides the logger and database.
var InfrastructureModule = fx.Options(
	fx.Provide(func(cfg *config.Config) (*logger.Logger, error) {
		return logger.New(logger.Config{Level: cfg.LogLevel, Env: cfg.Env})
	}),
	fx.Provide(func(cfg *config.Config, log *logger.Logger, lc fx.Lifecycle) (*gorm.DB, error) {
		db, err := database.NewPostgresDB(cfg.DBDSN, log)
		if err != nil {
			return nil, err
		}
		if err := database.MigrateDB(db, log); err != nil {
			return nil, err
		}
		lc.Append(fx.Hook{
			OnStop: func(context.Context) error { return database.CloseDB(db, log) },
		})
		return db, nil
	}),
	fx.Provide(transaction.NewTxManager),
)

// AgentModule provides the engine registry + run manager and registers the
// Claude HTTP runtime.
var AgentModule = fx.Options(
	fx.Provide(agent.NewRunManager),
	fx.Provide(func(cfg *config.Config, log *logger.Logger) *agent.Registry {
		reg := agent.NewRegistry()
		if cfg.AgentURL != "" {
			reg.Register(adapters.NewClaudeHTTPRuntime(cfg.AgentURL, log))
			log.Info("registered claude runtime", zap.String("agent_url", cfg.AgentURL))
		} else {
			log.Warn("DEXIASK_AGENT_URL not set — chat will fail until configured")
		}
		return reg
	}),
)

// RepositoryModule provides the data-access layer.
var RepositoryModule = fx.Options(
	fx.Provide(repository.NewConversationRepository),
	fx.Provide(repository.NewMessageRepository),
	fx.Provide(repository.NewAttachmentRepository),
	fx.Provide(repository.NewSlackThreadRepository),
	fx.Provide(repository.NewMCPServerRepository),
	fx.Provide(repository.NewUserRepository),
	fx.Provide(repository.NewSessionRepository),
)

// AuthModule provides GitHub OAuth, sessions, and the request-auth middleware.
// In dev-fallback mode (RequireAuth false) the OAuth/cipher/signer providers
// return nil — they are only exercised in auth mode.
var AuthModule = fx.Options(
	fx.Provide(func(cfg *config.Config) (*auth.TokenCipher, error) {
		if !cfg.RequireAuth {
			return nil, nil
		}
		if cfg.TokenEncKey == "" {
			return nil, fmt.Errorf("DEXIASK_TOKEN_ENC_KEY is required when auth is enabled")
		}
		return auth.NewTokenCipher(cfg.TokenEncKey)
	}),
	fx.Provide(func(cfg *config.Config) (*auth.CookieSigner, error) {
		if !cfg.RequireAuth {
			return nil, nil
		}
		if cfg.SessionSecret == "" {
			return nil, fmt.Errorf("DEXIASK_SESSION_SECRET is required when auth is enabled")
		}
		return auth.NewCookieSigner(cfg.SessionSecret, strings.HasPrefix(cfg.OAuthCallbackURL, "https://")), nil
	}),
	fx.Provide(func(cfg *config.Config) *auth.OAuth {
		if !cfg.RequireAuth {
			return nil
		}
		return auth.NewOAuth(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.OAuthCallbackURL)
	}),
	fx.Provide(func() *auth.GitHubClient { return auth.NewGitHubClient(5 * time.Minute) }),
	fx.Provide(func(o *auth.OAuth, gh *auth.GitHubClient, c *auth.TokenCipher, ur repository.UserRepository, sr repository.SessionRepository, log *logger.Logger) service.AuthService {
		return service.NewAuthService(o, gh, c, ur, sr, log)
	}),
	fx.Provide(func(cfg *config.Config, s *auth.CookieSigner, c *auth.TokenCipher, sr repository.SessionRepository, ur repository.UserRepository, log *logger.Logger) *auth.Authenticator {
		return auth.NewAuthenticator(cfg.RequireAuth, config.FixedUserID, s, c, sr, ur, log)
	}),
)

// ServiceModule provides the business-logic layer.
var ServiceModule = fx.Options(
	fx.Provide(func() service.ContextAssembler { return service.WindowAssembler{} }),
	fx.Provide(func(cfg *config.Config, repo repository.AttachmentRepository, log *logger.Logger) service.AttachmentService {
		return service.NewAttachmentService(cfg.WorkspaceMount, repo, log)
	}),
	fx.Provide(func(cfg *config.Config) *memory.Client { return memory.NewClient(cfg.MemoryURL) }),
	fx.Provide(service.NewConversationService),
	fx.Provide(func(
		reg *agent.Registry,
		rm *agent.RunManager,
		tx *transaction.TxManager,
		cr repository.ConversationRepository,
		mr repository.MessageRepository,
		mcp repository.MCPServerRepository,
		as service.ContextAssembler,
		att service.AttachmentService,
		mem *memory.Client,
		log *logger.Logger,
		cfg *config.Config,
	) service.ChatService {
		return service.NewChatService(reg, rm, tx, cr, mr, mcp, as, att, mem, log, cfg.Model, cfg.MaxTokens, cfg.IndexerMCPURL, cfg.MemoryMCPURL)
	}),
)

// HandlerModule provides the HTTP handlers.
var HandlerModule = fx.Options(
	fx.Provide(handler.NewChatHandler),
	fx.Provide(handler.NewConversationHandler),
	fx.Provide(handler.NewAttachmentHandler),
	fx.Provide(func(cfg *config.Config, gh *auth.GitHubClient, log *logger.Logger) *handler.IndexerHandler {
		return handler.NewIndexerHandler(cfg.IndexerURL, gh, log)
	}),
	fx.Provide(func(cfg *config.Config, log *logger.Logger) *handler.MemoryHandler {
		return handler.NewMemoryHandler(cfg.MemoryURL, log)
	}),
	fx.Provide(handler.NewMCPServerHandler),
	fx.Provide(func(cfg *config.Config, svc service.AuthService, signer *auth.CookieSigner, log *logger.Logger) *handler.AuthHandler {
		secure := strings.HasPrefix(cfg.OAuthCallbackURL, "https://")
		return handler.NewAuthHandler(svc, signer, cfg.WebBaseURL, cfg.RequireAuth, secure, config.FixedUserID, log)
	}),
	fx.Provide(func(c *handler.ChatHandler, cv *handler.ConversationHandler, a *handler.AttachmentHandler, i *handler.IndexerHandler, mem *handler.MemoryHandler, m *handler.MCPServerHandler, au *handler.AuthHandler) server.Handlers {
		return server.Handlers{Chat: c, Conversation: cv, Attachment: a, Indexer: i, Memory: mem, MCPServer: m, Auth: au}
	}),
)

// SlackModule provides the Slack bot and runs it as a lifecycle goroutine.
var SlackModule = fx.Options(
	fx.Provide(func(chatSvc service.ChatService, threadRepo repository.SlackThreadRepository, cfg *config.Config, log *logger.Logger) *slackbot.Bot {
		return slackbot.NewBot(chatSvc, threadRepo, cfg.SlackAppToken, cfg.SlackBotToken, log)
	}),
	fx.Invoke(func(lc fx.Lifecycle, bot *slackbot.Bot, log *logger.Logger) {
		botCtx, cancel := context.WithCancel(context.Background())
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go func() {
					if err := bot.Run(botCtx); err != nil {
						log.Error("slack bot exited", zap.Error(err))
					}
				}()
				return nil
			},
			OnStop: func(context.Context) error {
				cancel()
				return nil
			},
		})
	}),
)

// ServerModule builds the HTTP server and manages its lifecycle.
var ServerModule = fx.Options(
	fx.Provide(server.NewServer),
	fx.Invoke(func(lc fx.Lifecycle, srv *http.Server, log *logger.Logger) {
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				go func() {
					log.Info("http server listening", zap.String("addr", srv.Addr))
					if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						log.Error("http server error", zap.Error(err))
					}
				}()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				return srv.Shutdown(ctx)
			},
		})
	}),
)
