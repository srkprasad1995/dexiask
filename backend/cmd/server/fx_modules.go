package main

import (
	"context"
	"net/http"

	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/agent/adapters"
	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/handler"
	"github.com/dexiask/dexiask/internal/infrastructure/database"
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
)

// ServiceModule provides the business-logic layer.
var ServiceModule = fx.Options(
	fx.Provide(func() service.ContextAssembler { return service.WindowAssembler{} }),
	fx.Provide(func(cfg *config.Config, repo repository.AttachmentRepository, log *logger.Logger) service.AttachmentService {
		return service.NewAttachmentService(cfg.WorkspaceMount, repo, log)
	}),
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
		log *logger.Logger,
		cfg *config.Config,
	) service.ChatService {
		return service.NewChatService(reg, rm, tx, cr, mr, mcp, as, att, log, cfg.Model, cfg.MaxTokens, cfg.IndexerMCPURL)
	}),
)

// HandlerModule provides the HTTP handlers.
var HandlerModule = fx.Options(
	fx.Provide(handler.NewChatHandler),
	fx.Provide(handler.NewConversationHandler),
	fx.Provide(handler.NewAttachmentHandler),
	fx.Provide(func(cfg *config.Config, log *logger.Logger) *handler.IndexerHandler {
		return handler.NewIndexerHandler(cfg.IndexerURL, log)
	}),
	fx.Provide(handler.NewMCPServerHandler),
	fx.Provide(func(c *handler.ChatHandler, cv *handler.ConversationHandler, a *handler.AttachmentHandler, i *handler.IndexerHandler, m *handler.MCPServerHandler) server.Handlers {
		return server.Handlers{Chat: c, Conversation: cv, Attachment: a, Indexer: i, MCPServer: m}
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
