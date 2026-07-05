package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
	mocks "github.com/dexiask/dexiask/test/mocks"
	svcmocks "github.com/dexiask/dexiask/test/svcmocks"
)

// fakeTx is a pass-through service.TxRunner: it runs fn with the given context.
type fakeTx struct{}

func (fakeTx) InTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// fakeRuntime records the last Job and replays a preset event sequence.
type fakeRuntime struct {
	mu       sync.Mutex
	lastJob  agent.Job
	events   []agent.Event
	startErr error
}

func (f *fakeRuntime) Name() string { return "claude" }

func (f *fakeRuntime) Start(_ context.Context, job agent.Job) (<-chan agent.Event, error) {
	f.mu.Lock()
	f.lastJob = job
	f.mu.Unlock()
	if f.startErr != nil {
		return nil, f.startErr
	}
	ch := make(chan agent.Event, len(f.events)+1)
	for _, e := range f.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (f *fakeRuntime) job() agent.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastJob
}

type chatFixture struct {
	svc      service.ChatService
	rt       *fakeRuntime
	convRepo *mocks.MockConversationRepository
	msgRepo  *mocks.MockMessageRepository
	mcpRepo  *mocks.MockMCPServerRepository
	attSvc   *svcmocks.MockAttachmentService
}

func newChatFixture(t *testing.T, events []agent.Event) *chatFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	convRepo := mocks.NewMockConversationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)
	mcpRepo := mocks.NewMockMCPServerRepository(ctrl)
	attSvc := svcmocks.NewMockAttachmentService(ctrl)

	// Default: no custom MCP servers. Tests that assert injection override this.
	mcpRepo.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	rt := &fakeRuntime{events: events}
	reg := agent.NewRegistry()
	reg.Register(rt)
	rm := agent.NewRunManager(logger.NewNop())

	svc := service.NewChatService(reg, rm, fakeTx{}, convRepo, msgRepo, mcpRepo, service.WindowAssembler{}, attSvc,
		nil, logger.NewNop(), "claude-test", 4096, "http://indexer:8080/mcp", "", "")

	return &chatFixture{svc: svc, rt: rt, convRepo: convRepo, msgRepo: msgRepo, mcpRepo: mcpRepo, attSvc: attSvc}
}

// appendReturnsIDs makes msgRepo.Append return distinct ids per role.
func appendReturnsIDs(msgRepo *mocks.MockMessageRepository) {
	msgRepo.EXPECT().Append(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.AppendMessageInput) (*model.Message, error) {
			id := "u1"
			if in.Role == model.MessageRoleAssistant {
				id = "a1"
			}
			return &model.Message{ID: id, ConversationID: in.ConversationID, Role: in.Role, Content: in.Content, Status: in.Status}, nil
		}).Times(2)
}

func collect(run *agent.Run) []agent.Event {
	sub := run.Subscribe(context.Background(), 0)
	var got []agent.Event
	for ev := range sub {
		got = append(got, ev)
	}
	return got
}

// TestChatService_NewConversation_ForwardsEventsAndPersistsSession covers a
// fresh conversation: events are forwarded to subscribers and the returned
// sessionId is persisted for the next turn.
func TestChatService_NewConversation_ForwardsEventsAndPersistsSession(t *testing.T) {
	fx := newChatFixture(t, []agent.Event{
		{Type: "text.delta", Text: "Hello "},
		{Type: "text.delta", Text: "world"},
		{Type: "result", Status: "ok", SessionID: "sess-1"},
	})

	fx.convRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(&model.Conversation{ID: "conv-1", WorkspaceID: "dexiask", UserID: "dexiask"}, nil)
	appendReturnsIDs(fx.msgRepo)
	fx.convRepo.EXPECT().Touch(gomock.Any(), "conv-1").Return(nil)
	fx.msgRepo.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(
		[]*model.Message{{ID: "u1", Role: model.MessageRoleUser, Content: "hi", Status: model.MessageStatusComplete}}, nil)
	fx.attSvc.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(nil, nil)

	var persistedContent string
	fx.msgRepo.EXPECT().UpdateMessage(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.UpdateMessageInput) error {
			persistedContent = in.Content
			return nil
		})
	sessionCh := make(chan string, 1)
	fx.convRepo.EXPECT().UpdateSessionID(gomock.Any(), "conv-1", "sess-1").DoAndReturn(
		func(_ context.Context, _, sid string) error {
			sessionCh <- sid
			return nil
		})

	res, err := fx.svc.Start(context.Background(), service.ChatRequest{
		Messages: []agent.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !res.IsNew || res.ConversationID != "conv-1" {
		t.Fatalf("unexpected result: %+v", res)
	}

	got := collect(res.Run)
	// Forwarded events: two deltas + result.
	if len(got) != 3 || got[0].Text != "Hello " || got[2].Type != "result" {
		t.Fatalf("forwarded events wrong: %+v", got)
	}

	select {
	case sid := <-sessionCh:
		if sid != "sess-1" {
			t.Fatalf("persisted session = %q", sid)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("session id was not persisted")
	}
	if persistedContent != "Hello world" {
		t.Fatalf("persisted assistant content = %q", persistedContent)
	}

	// The Job carried the ask role, the configured model, and the indexer MCP server.
	job := fx.rt.job()
	if job.Role != "ask" || job.Model != "claude-test" {
		t.Fatalf("job role/model = %q/%q", job.Role, job.Model)
	}
	if job.SessionID != "" {
		t.Fatalf("new conversation should not resume a session, got %q", job.SessionID)
	}
	if len(job.MCPServers) != 1 || job.MCPServers[0].Name != "indexer" {
		t.Fatalf("indexer MCP server not injected: %+v", job.MCPServers)
	}
}

// TestChatService_InjectsEnabledCustomMCPServers verifies the Job carries the
// indexer server plus every enabled user-defined MCP server. The repository is
// queried with EnabledOnly=true, so disabled rows never reach the Job.
func TestChatService_InjectsEnabledCustomMCPServers(t *testing.T) {
	ctrl := gomock.NewController(t)
	convRepo := mocks.NewMockConversationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)
	mcpRepo := mocks.NewMockMCPServerRepository(ctrl)
	attSvc := svcmocks.NewMockAttachmentService(ctrl)

	rt := &fakeRuntime{events: []agent.Event{{Type: "result", Status: "ok", SessionID: "s"}}}
	reg := agent.NewRegistry()
	reg.Register(rt)
	rm := agent.NewRunManager(logger.NewNop())
	svc := service.NewChatService(reg, rm, fakeTx{}, convRepo, msgRepo, mcpRepo, service.WindowAssembler{}, attSvc,
		nil, logger.NewNop(), "claude-test", 4096, "http://indexer:8080/mcp", "", "")

	// The service must ask for enabled servers only; return a single enabled one.
	var gotFilter *model.ListMCPServersFilter
	mcpRepo.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, f *model.ListMCPServersFilter) ([]*model.MCPServer, error) {
			gotFilter = f
			return []*model.MCPServer{{
				ID: "m1", Name: "github", Type: "http", URL: "http://gh/mcp",
				Headers: map[string]string{"Authorization": "Bearer x"}, Enabled: true,
			}}, nil
		})

	var gotCreate *model.CreateConversationInput
	convRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.CreateConversationInput) (*model.Conversation, error) {
			gotCreate = in
			return &model.Conversation{ID: "conv-1", UserID: in.UserID}, nil
		})
	appendReturnsIDs(msgRepo)
	convRepo.EXPECT().Touch(gomock.Any(), "conv-1").Return(nil)
	msgRepo.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(
		[]*model.Message{{ID: "u1", Role: model.MessageRoleUser, Content: "hi", Status: model.MessageStatusComplete}}, nil)
	attSvc.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(nil, nil)
	msgRepo.EXPECT().UpdateMessage(gomock.Any(), gomock.Any()).Return(nil)
	convRepo.EXPECT().UpdateSessionID(gomock.Any(), "conv-1", "s").Return(nil)

	res, err := svc.Start(context.Background(), service.ChatRequest{
		UserID:   "42",
		Messages: []agent.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	collect(res.Run)

	if gotFilter == nil || !gotFilter.EnabledOnly {
		t.Fatalf("expected EnabledOnly filter, got %+v", gotFilter)
	}
	// The conversation is scoped to the caller (MCP servers are workspace-wide).
	if gotCreate == nil || gotCreate.UserID != "42" {
		t.Fatalf("conversation created with user %v, want 42", gotCreate)
	}
	job := rt.job()
	if len(job.MCPServers) != 2 {
		t.Fatalf("expected indexer + 1 custom server, got %+v", job.MCPServers)
	}
	if job.MCPServers[0].Name != "indexer" {
		t.Fatalf("first server should be indexer, got %q", job.MCPServers[0].Name)
	}
	custom := job.MCPServers[1]
	if custom.Name != "github" || custom.Type != "http" || custom.URL != "http://gh/mcp" ||
		custom.Headers["Authorization"] != "Bearer x" {
		t.Fatalf("custom server mapped wrong: %+v", custom)
	}
}

// TestChatService_ResumesPriorSession verifies an existing conversation's
// persisted sessionId is sent back to the engine on the next turn.
func TestChatService_ResumesPriorSession(t *testing.T) {
	fx := newChatFixture(t, []agent.Event{{Type: "result", Status: "ok", SessionID: "sess-2"}})

	fx.convRepo.EXPECT().GetByID(gomock.Any(), "conv-1").Return(
		&model.Conversation{ID: "conv-1", WorkspaceID: "dexiask", UserID: "dexiask", SessionID: "sess-1"}, nil)
	appendReturnsIDs(fx.msgRepo)
	fx.convRepo.EXPECT().Touch(gomock.Any(), "conv-1").Return(nil)
	fx.msgRepo.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(
		[]*model.Message{{ID: "u1", Role: model.MessageRoleUser, Content: "again", Status: model.MessageStatusComplete}}, nil)
	fx.attSvc.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(nil, nil)
	fx.msgRepo.EXPECT().UpdateMessage(gomock.Any(), gomock.Any()).Return(nil)
	fx.convRepo.EXPECT().UpdateSessionID(gomock.Any(), "conv-1", "sess-2").Return(nil)

	res, err := fx.svc.Start(context.Background(), service.ChatRequest{
		ConversationID: "conv-1",
		Messages:       []agent.Message{{Role: "user", Content: "again"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	collect(res.Run) // drain to completion

	if job := fx.rt.job(); job.SessionID != "sess-1" {
		t.Fatalf("resume sessionId = %q, want sess-1", job.SessionID)
	}
}

// TestChatService_AttachmentWiring verifies uploaded attachments are reconciled
// onto the user message and projected into the Job's message list.
func TestChatService_AttachmentWiring(t *testing.T) {
	fx := newChatFixture(t, []agent.Event{{Type: "result", Status: "ok", SessionID: "s"}})

	fx.convRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(&model.Conversation{ID: "conv-1"}, nil)
	appendReturnsIDs(fx.msgRepo)

	var reconciledBucket, reconciledConv, reconciledMsg string
	fx.attSvc.EXPECT().Reconcile(gomock.Any(), "bkt-9", "conv-1", "u1").DoAndReturn(
		func(_ context.Context, bucket, conv, msg string) error {
			reconciledBucket, reconciledConv, reconciledMsg = bucket, conv, msg
			return nil
		})
	fx.convRepo.EXPECT().Touch(gomock.Any(), "conv-1").Return(nil)
	fx.msgRepo.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(
		[]*model.Message{{ID: "u1", Role: model.MessageRoleUser, Content: "see file", Status: model.MessageStatusComplete}}, nil)
	msgID := "u1"
	fx.attSvc.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(
		[]*model.Attachment{{ID: "att1", MessageID: &msgID, Filename: "a.png", MediaType: "image/png",
			RelPath: ".dexiask/conversations/conv-1/attachments/att1-a.png"}}, nil)
	fx.msgRepo.EXPECT().UpdateMessage(gomock.Any(), gomock.Any()).Return(nil)
	fx.convRepo.EXPECT().UpdateSessionID(gomock.Any(), "conv-1", "s").Return(nil)

	res, err := fx.svc.Start(context.Background(), service.ChatRequest{
		Messages:     []agent.Message{{Role: "user", Content: "see file"}},
		UploadBucket: "bkt-9",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	collect(res.Run)

	if reconciledBucket != "bkt-9" || reconciledConv != "conv-1" || reconciledMsg != "u1" {
		t.Fatalf("reconcile args = %q/%q/%q", reconciledBucket, reconciledConv, reconciledMsg)
	}
	job := fx.rt.job()
	if len(job.Messages) != 1 || len(job.Messages[0].Attachments) != 1 {
		t.Fatalf("job messages/attachments wrong: %+v", job.Messages)
	}
	att := job.Messages[0].Attachments[0]
	if att.Kind != "image" || att.Path != "/workspace/.dexiask/conversations/conv-1/attachments/att1-a.png" {
		t.Fatalf("attachment projection wrong: %+v", att)
	}
}

// TestChatService_RejectsEmptyTurn guards the no-content precondition.
func TestChatService_RejectsEmptyTurn(t *testing.T) {
	fx := newChatFixture(t, nil)
	if _, err := fx.svc.Start(context.Background(), service.ChatRequest{
		Messages: []agent.Message{{Role: "user", Content: "   "}},
	}); err == nil {
		t.Fatal("expected error for empty turn")
	}
}

// fakeDigester records the user it was asked about and returns a canned digest.
type fakeDigester struct {
	seenUser string
	digest   string
}

func (f *fakeDigester) Digest(_ context.Context, userID string) string {
	f.seenUser = userID
	return f.digest
}

// TestChatService_InjectsMemory verifies the per-user memory MCP server is
// attached (with the caller's headers) and the memory digest is appended to the
// system prompt.
func TestChatService_InjectsMemory(t *testing.T) {
	ctrl := gomock.NewController(t)
	convRepo := mocks.NewMockConversationRepository(ctrl)
	msgRepo := mocks.NewMockMessageRepository(ctrl)
	mcpRepo := mocks.NewMockMCPServerRepository(ctrl)
	attSvc := svcmocks.NewMockAttachmentService(ctrl)

	rt := &fakeRuntime{events: []agent.Event{{Type: "result", Status: "ok", SessionID: "s"}}}
	reg := agent.NewRegistry()
	reg.Register(rt)
	rm := agent.NewRunManager(logger.NewNop())

	digester := &fakeDigester{digest: "\n\n## Memory\n\n### user / 42\n- prefers terse answers"}
	svc := service.NewChatService(reg, rm, fakeTx{}, convRepo, msgRepo, mcpRepo, service.WindowAssembler{}, attSvc,
		digester, logger.NewNop(), "claude-test", 4096, "http://indexer:8080/mcp", "http://memory:8080/mcp", "")

	mcpRepo.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil)
	convRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(&model.Conversation{ID: "conv-1", UserID: "42"}, nil)
	appendReturnsIDs(msgRepo)
	convRepo.EXPECT().Touch(gomock.Any(), "conv-1").Return(nil)
	msgRepo.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(
		[]*model.Message{{ID: "u1", Role: model.MessageRoleUser, Content: "hi", Status: model.MessageStatusComplete}}, nil)
	attSvc.EXPECT().ListByConversation(gomock.Any(), "conv-1").Return(nil, nil)
	msgRepo.EXPECT().UpdateMessage(gomock.Any(), gomock.Any()).Return(nil)
	convRepo.EXPECT().UpdateSessionID(gomock.Any(), "conv-1", "s").Return(nil)

	res, err := svc.Start(context.Background(), service.ChatRequest{
		UserID:   "42",
		Messages: []agent.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	collect(res.Run)

	job := rt.job()
	// The memory server sits after the indexer server, with per-user headers.
	var mem *agent.MCPServerConfig
	for i := range job.MCPServers {
		if job.MCPServers[i].Name == "memory" {
			mem = &job.MCPServers[i]
		}
	}
	if mem == nil {
		t.Fatalf("memory MCP server not injected: %+v", job.MCPServers)
	}
	if mem.Headers["X-User-Id"] != "42" || mem.Headers["X-Role"] != "ask" ||
		mem.Headers["X-Writable-Scopes"] != "user,repo" {
		t.Fatalf("memory headers wrong: %+v", mem.Headers)
	}
	// The digest is fetched for the caller and appended to the system prompt.
	if digester.seenUser != "42" {
		t.Fatalf("digest fetched for %q, want 42", digester.seenUser)
	}
	if !strings.Contains(job.SystemPrompt, "## Memory") {
		t.Fatalf("system prompt missing memory digest: %q", job.SystemPrompt)
	}
}
