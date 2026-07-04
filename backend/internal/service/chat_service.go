package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/repository"
)

// TxRunner runs a function inside a database transaction. *transaction.TxManager
// satisfies it; tests provide a pass-through fake.
type TxRunner interface {
	InTransaction(ctx context.Context, fn func(context.Context) error) error
}

//go:generate go run go.uber.org/mock/mockgen -source=chat_service.go -destination=../../test/svcmocks/chat_service_mock.go -package=svcmocks

// The single role/runtime Dexiask runs. There is no role_config table.
const (
	askRole        = "ask"
	claudeRuntime  = "claude"
	permissionMode = "dontAsk"
	engineSkills   = "/skills"
	// idleTimeout aborts a run that produces no engine output for this long
	// (usually a provider rate-limit the SDK is silently retrying).
	idleTimeout = 120 * time.Second
)

// ChatRequest is the input to a chat turn.
type ChatRequest struct {
	// ConversationID is the existing conversation to continue. Empty starts a new one.
	ConversationID string
	// Messages holds the new user turn (single element expected). History is
	// loaded from the database; the client does not replay it.
	Messages []agent.Message
	// Attachments are files referenced in this turn.
	Attachments []agent.Attachment
	// UploadBucket is the client UUID for pending pre-conversation uploads.
	UploadBucket string
}

// ChatStartResult is returned by ChatService.Start.
type ChatStartResult struct {
	ConversationID string
	IsNew          bool
	Run            *agent.Run
}

// ChatService orchestrates a single agent run (fixed role=ask, runtime=claude).
type ChatService interface {
	// Start resolves (or creates) a conversation, persists the user turn,
	// launches a detached generation run, and returns immediately.
	Start(ctx context.Context, req ChatRequest) (*ChatStartResult, error)
	// Resume returns the live (or recently-finished) run for a conversation, if
	// one exists. Returns (nil, false, nil) when there is no active run.
	Resume(ctx context.Context, conversationID string) (*agent.Run, bool, error)
	// Stop cancels the active run for a conversation. Returns true if found.
	Stop(conversationID string) bool
}

type chatService struct {
	registry      *agent.Registry
	runMgr        *agent.RunManager
	txManager     TxRunner
	convRepo      repository.ConversationRepository
	msgRepo       repository.MessageRepository
	mcpRepo       repository.MCPServerRepository
	assembler     ContextAssembler
	attachmentSvc AttachmentService
	logger        *logger.Logger

	model          string
	maxTokens      int
	indexerServers []agent.MCPServerConfig
}

// NewChatService creates a new ChatService. indexerMCPURL, when non-empty, is
// injected into every ask Job so the agent can call the indexer's semantic
// search over the MCP protocol. Enabled user-defined MCP servers (from mcpRepo)
// are appended per turn.
func NewChatService(
	registry *agent.Registry,
	runMgr *agent.RunManager,
	txManager TxRunner,
	convRepo repository.ConversationRepository,
	msgRepo repository.MessageRepository,
	mcpRepo repository.MCPServerRepository,
	assembler ContextAssembler,
	attachmentSvc AttachmentService,
	log *logger.Logger,
	modelName string,
	maxTokens int,
	indexerMCPURL string,
) ChatService {
	var indexerServers []agent.MCPServerConfig
	if strings.TrimSpace(indexerMCPURL) != "" {
		indexerServers = []agent.MCPServerConfig{{
			Name:        "indexer",
			Type:        "http",
			URL:         indexerMCPURL,
			Description: "Semantic code search over the indexed workspace repositories (semantic_search).",
		}}
	}
	return &chatService{
		registry:       registry,
		runMgr:         runMgr,
		txManager:      txManager,
		convRepo:       convRepo,
		msgRepo:        msgRepo,
		mcpRepo:        mcpRepo,
		assembler:      assembler,
		attachmentSvc:  attachmentSvc,
		logger:         log,
		model:          modelName,
		maxTokens:      maxTokens,
		indexerServers: indexerServers,
	}
}

// Start implements ChatService.
func (s *chatService) Start(ctx context.Context, req ChatRequest) (*ChatStartResult, error) {
	var userContent string
	if len(req.Messages) > 0 {
		userContent = req.Messages[len(req.Messages)-1].Content
	}
	hasAttachments := len(req.Attachments) > 0 || req.UploadBucket != ""
	if strings.TrimSpace(userContent) == "" && !hasAttachments {
		return nil, pkgerrors.InvalidArgument("no user message provided")
	}

	// Resolve the runtime before the transaction so we fail fast if unavailable.
	rt, err := s.registry.Get(claudeRuntime)
	if err != nil {
		return nil, pkgerrors.InvalidArgumentf("runtime %q is not available", claudeRuntime)
	}

	// Use a detached context so a client disconnect doesn't abort the DB write.
	dbCtx := context.WithoutCancel(ctx)

	var (
		convID         string
		convSessionID  string
		isNew          bool
		history        []*model.MessageWithAttachments
		assistantMsgID string
	)

	err = s.txManager.InTransaction(dbCtx, func(txCtx context.Context) error {
		if req.ConversationID == "" {
			conv, err := s.convRepo.Create(txCtx, &model.CreateConversationInput{
				WorkspaceID: config.FixedWorkspaceID,
				UserID:      config.FixedUserID,
				Title:       deriveTitle(userContent),
			})
			if err != nil {
				return err
			}
			convID = conv.ID
			isNew = true
		} else {
			conv, err := s.convRepo.GetByID(txCtx, req.ConversationID)
			if err != nil {
				return err
			}
			convID = conv.ID
			convSessionID = conv.SessionID
		}

		userMsg, err := s.msgRepo.Append(txCtx, &model.AppendMessageInput{
			ConversationID: convID,
			Role:           model.MessageRoleUser,
			Content:        userContent,
			Status:         model.MessageStatusComplete,
		})
		if err != nil {
			return err
		}

		assistantMsg, err := s.msgRepo.Append(txCtx, &model.AppendMessageInput{
			ConversationID: convID,
			Role:           model.MessageRoleAssistant,
			Content:        "",
			Status:         model.MessageStatusRunning,
			Model:          s.model,
		})
		if err != nil {
			return err
		}
		assistantMsgID = assistantMsg.ID

		// Reconcile uploaded attachments: move pending files to the conversation
		// directory and link them to this user message.
		if req.UploadBucket != "" || len(req.Attachments) > 0 {
			if err := s.attachmentSvc.Reconcile(txCtx, req.UploadBucket, convID, userMsg.ID); err != nil {
				s.logger.Warn("failed to reconcile attachments",
					zap.Error(err), zap.String("conversation_id", convID))
				// Non-fatal: proceed even if reconcile fails.
			}
		}

		if err := s.convRepo.Touch(txCtx, convID); err != nil {
			return err
		}

		msgs, err := s.msgRepo.ListByConversation(txCtx, convID)
		if err != nil {
			return err
		}
		atts, err := s.attachmentSvc.ListByConversation(txCtx, convID)
		if err != nil {
			s.logger.Warn("failed to load attachments for assembly",
				zap.Error(err), zap.String("conversation_id", convID))
			atts = nil
		}
		attsByMsg := make(map[string][]*model.Attachment, len(atts))
		for _, a := range atts {
			if a.MessageID != nil {
				attsByMsg[*a.MessageID] = append(attsByMsg[*a.MessageID], a)
			}
		}
		history = make([]*model.MessageWithAttachments, 0, len(msgs))
		for _, m := range msgs {
			history = append(history, &model.MessageWithAttachments{
				Message:     *m,
				Attachments: attsByMsg[m.ID],
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	jobMessages := s.assembler.Assemble(history)
	mcpServers := s.resolveMCPServers(dbCtx)

	job := agent.Job{
		Role:             askRole,
		Model:            s.model,
		SystemPrompt:     agent.SystemPromptForRole(askRole),
		AllowedTools:     agent.AllowedToolsForRole(askRole),
		PermissionMode:   permissionMode,
		SkillsPath:       engineSkills,
		WorkspacePath:    agent.WorkspacePath(),
		Messages:         jobMessages,
		SessionID:        convSessionID, // non-empty → engine resumes prior SDK session
		SessionStorePath: agent.SessionStorePathFor(convID),
		MCPServers:       mcpServers,
		MaxTokens:        s.maxTokens,
	}

	s.logger.Info("starting chat run",
		zap.String("conversation_id", convID),
		zap.String("model", s.model),
		zap.Int("history_messages", len(jobMessages)),
	)

	run := s.runMgr.Start(convID, func(runCtx context.Context, emit func(agent.Event)) {
		s.drain(runCtx, rt, job, convID, assistantMsgID, emit)
	})

	return &ChatStartResult{ConversationID: convID, IsNew: isNew, Run: run}, nil
}

// resolveMCPServers returns the MCP servers attached to a turn: the built-in
// indexer server followed by every enabled user-defined server. A failure to
// load the custom servers is logged and degrades to indexer-only rather than
// failing the turn.
func (s *chatService) resolveMCPServers(ctx context.Context) []agent.MCPServerConfig {
	servers := make([]agent.MCPServerConfig, 0, len(s.indexerServers)+1)
	servers = append(servers, s.indexerServers...)

	if s.mcpRepo == nil {
		return servers
	}
	custom, err := s.mcpRepo.List(ctx, &model.ListMCPServersFilter{
		WorkspaceID: config.FixedWorkspaceID,
		EnabledOnly: true,
	})
	if err != nil {
		s.logger.Warn("failed to load custom mcp servers — using indexer only", zap.Error(err))
		return servers
	}
	for _, c := range custom {
		servers = append(servers, agent.MCPServerConfig{
			Name:    c.Name,
			Type:    c.Type,
			URL:     c.URL,
			Headers: c.Headers,
		})
	}
	return servers
}

// drain runs the engine, forwards every event to emit, accumulates the reply
// text, and persists the assistant message + returned sessionId when finished.
func (s *chatService) drain(runCtx context.Context, rt agent.Runtime, job agent.Job, convID, assistantMsgID string, emit func(agent.Event)) {
	engineCtx, cancelEngine := context.WithCancel(runCtx)
	defer cancelEngine()

	events, err := rt.Start(engineCtx, job)
	if err != nil {
		if engineCtx.Err() != nil {
			// Deliberately stopped by the user — persist as partial, stay quiet.
			s.persistAssistant(assistantMsgID, "", model.MessageStatusPartial, s.model, nil)
			return
		}
		s.logger.Error("failed to start engine", zap.Error(err), zap.String("conversation_id", convID))
		emit(agent.Event{Type: "error", Message: err.Error()})
		s.persistAssistant(assistantMsgID, "", model.MessageStatusError, s.model, nil)
		return
	}

	var sb strings.Builder
	finalStatus := model.MessageStatusPartial
	sawResult := false
	var newSessionID string
	var askChoiceInputs []interface{}

	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()

drainLoop:
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				break drainLoop
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleTimeout)

			emit(ev)
			switch ev.Type {
			case "text.delta":
				sb.WriteString(ev.Text)
			case "tool.input_done":
				if isAskChoiceTool(ev.Name) {
					askChoiceInputs = append(askChoiceInputs, ev.Input)
				}
			case "result":
				sawResult = true
				newSessionID = ev.SessionID
				if ev.Status == "error" {
					finalStatus = model.MessageStatusError
				} else {
					finalStatus = model.MessageStatusComplete
				}
			case "error":
				finalStatus = model.MessageStatusError
			}
		case <-idle.C:
			cancelEngine()
			go func() {
				for range events {
				}
			}()
			s.logger.Warn("run stalled with no engine output — aborting",
				zap.String("conversation_id", convID))
			emit(agent.Event{Type: "error", Message: "The engine stopped responding (no output for 2 minutes) — it may be rate-limited or stalled. Try again."})
			finalStatus = model.MessageStatusError
			sawResult = true
			break drainLoop
		}
	}

	if !sawResult && finalStatus == model.MessageStatusPartial {
		s.logger.Warn("run ended without result event", zap.String("conversation_id", convID))
		emit(agent.Event{Type: "error", Message: "The run ended without a result — the engine may have crashed or hit a provider error. Try again."})
		finalStatus = model.MessageStatusError
	}

	var questionsJSON json.RawMessage
	if len(askChoiceInputs) > 0 {
		if b, err := json.Marshal(askChoiceInputs); err == nil {
			questionsJSON = b
		}
	}

	s.persistAssistant(assistantMsgID, sb.String(), finalStatus, s.model, questionsJSON)

	// Persist the SDK session_id so the next turn can resume the conversation.
	if newSessionID != "" {
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.convRepo.UpdateSessionID(persistCtx, convID, newSessionID); err != nil {
			s.logger.Error("failed to persist session_id",
				zap.String("conversation_id", convID), zap.Error(err))
		}
	}
}

// Resume implements ChatService.
func (s *chatService) Resume(ctx context.Context, conversationID string) (*agent.Run, bool, error) {
	// Verify the conversation exists (authorization is trivial in single-user mode).
	if _, err := s.convRepo.GetByID(ctx, conversationID); err != nil {
		return nil, false, err
	}
	run, ok := s.runMgr.Get(conversationID)
	// A completed run means the response is already persisted; the client should
	// use its persisted history instead of a stale replay.
	if ok && run.IsDone() {
		return nil, false, nil
	}
	return run, ok, nil
}

// Stop implements ChatService.
func (s *chatService) Stop(conversationID string) bool {
	return s.runMgr.Stop(conversationID)
}

// persistAssistant updates the assistant placeholder with the final content and
// status. Uses a background context so a client disconnect doesn't abort it.
func (s *chatService) persistAssistant(msgID, content, status, modelName string, questions json.RawMessage) {
	persistCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.msgRepo.UpdateMessage(persistCtx, &model.UpdateMessageInput{
		ID:        msgID,
		Content:   content,
		Status:    status,
		Model:     modelName,
		Questions: questions,
	}); err != nil {
		s.logger.Error("failed to persist assistant message",
			zap.String("message_id", msgID), zap.String("status", status), zap.Error(err))
	}
}

// isAskChoiceTool reports whether a tool name refers to the interactive
// AskChoice tool (it arrives MCP-namespaced as mcp__interactive__AskChoice).
func isAskChoiceTool(name string) bool {
	return name == "AskChoice" || strings.HasSuffix(name, "__AskChoice")
}

// deriveTitle returns a short title derived from the user's first message.
func deriveTitle(content string) string {
	title := content
	if idx := strings.IndexByte(title, '\n'); idx >= 0 {
		title = title[:idx]
	}
	title = strings.TrimSpace(title)
	// Truncate on a rune boundary — slicing by byte can split a multibyte rune
	// and yield an invalid-UTF-8 title.
	if utf8.RuneCountInString(title) > 80 {
		title = string([]rune(title)[:77]) + "..."
	}
	if title == "" {
		title = "New conversation"
	}
	return title
}
