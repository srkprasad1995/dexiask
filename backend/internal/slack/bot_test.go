package slack

import (
	"context"
	"strings"
	"sync"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
	mocks "github.com/dexiask/dexiask/test/mocks"
	svcmocks "github.com/dexiask/dexiask/test/svcmocks"
)

// fakeMessenger records Slack post/update calls.
type fakeMessenger struct {
	mu       sync.Mutex
	postTS   string
	posted   int
	updates  []string
	postedTo string
}

func (f *fakeMessenger) Post(_ context.Context, channel, _ string, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posted++
	f.postedTo = channel
	f.postTS = "p1"
	return f.postTS, nil
}

func (f *fakeMessenger) Update(_ context.Context, _, _ string, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, text)
	return nil
}

func (f *fakeMessenger) lastUpdate() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.updates) == 0 {
		return ""
	}
	return f.updates[len(f.updates)-1]
}

// runWith builds a completed *agent.Run replaying the given events.
func runWith(events ...agent.Event) *agent.Run {
	rm := agent.NewRunManager(logger.NewNop())
	done := make(chan struct{})
	run := rm.Start("conv-1", func(_ context.Context, emit func(agent.Event)) {
		for _, e := range events {
			emit(e)
		}
		close(done)
	})
	<-done
	return run
}

func TestBot_Disabled(t *testing.T) {
	bot := NewBot(nil, nil, "", "", logger.NewNop())
	if bot.Enabled() {
		t.Fatal("bot should be disabled with empty tokens")
	}
	if err := bot.Run(context.Background()); err != nil {
		t.Fatalf("disabled Run should be a no-op, got %v", err)
	}

	if NewBot(nil, nil, "xapp-1", "", logger.NewNop()).Enabled() {
		t.Fatal("bot should be disabled when only app token set")
	}
}

// TestBot_HandleMessage_NewThread verifies an inbound message with no existing
// mapping starts a new conversation, stores the mapping, and streams the reply
// into a chat.update.
func TestBot_HandleMessage_NewThread(t *testing.T) {
	ctrl := gomock.NewController(t)
	chatSvc := svcmocks.NewMockChatService(ctrl)
	threadRepo := mocks.NewMockSlackThreadRepository(ctrl)

	threadRepo.EXPECT().Get(gomock.Any(), "C1:T1").Return("", nil)

	run := runWith(
		agent.Event{Type: "text.delta", Text: "Hi "},
		agent.Event{Type: "text.delta", Text: "there"},
		agent.Event{Type: "result", Status: "ok", SessionID: "s"},
	)

	var gotReq service.ChatRequest
	chatSvc.EXPECT().Start(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req service.ChatRequest) (*service.ChatStartResult, error) {
			gotReq = req
			return &service.ChatStartResult{ConversationID: "conv-1", IsNew: true, Run: run}, nil
		})
	threadRepo.EXPECT().Put(gomock.Any(), "C1:T1", "conv-1").Return(nil)

	bot := NewBot(chatSvc, threadRepo, "xapp-1", "xoxb-1", logger.NewNop())
	msg := &fakeMessenger{}
	bot.handleMessage(context.Background(), msg, "C1", "T1", "hello world")

	if gotReq.ConversationID != "" {
		t.Errorf("new thread should start a new conversation, got convID %q", gotReq.ConversationID)
	}
	if len(gotReq.Messages) != 1 || gotReq.Messages[0].Content != "hello world" {
		t.Errorf("chat request messages = %+v", gotReq.Messages)
	}
	if msg.posted != 1 {
		t.Errorf("expected one placeholder post, got %d", msg.posted)
	}
	if final := msg.lastUpdate(); !strings.Contains(final, "Hi there") {
		t.Errorf("final chat.update = %q, want to contain reply", final)
	}
}

// TestBot_HandleMessage_ExistingThread verifies a mapped thread reuses its
// conversation and does not re-store the mapping.
func TestBot_HandleMessage_ExistingThread(t *testing.T) {
	ctrl := gomock.NewController(t)
	chatSvc := svcmocks.NewMockChatService(ctrl)
	threadRepo := mocks.NewMockSlackThreadRepository(ctrl)

	threadRepo.EXPECT().Get(gomock.Any(), "C1:T1").Return("conv-existing", nil)

	run := runWith(agent.Event{Type: "text.delta", Text: "ok"}, agent.Event{Type: "result", Status: "ok"})

	var gotConvID string
	chatSvc.EXPECT().Start(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req service.ChatRequest) (*service.ChatStartResult, error) {
			gotConvID = req.ConversationID
			return &service.ChatStartResult{ConversationID: "conv-existing", Run: run}, nil
		})
	// Put must NOT be called for an existing mapping.

	bot := NewBot(chatSvc, threadRepo, "xapp-1", "xoxb-1", logger.NewNop())
	bot.handleMessage(context.Background(), &fakeMessenger{}, "C1", "T1", "follow up")

	if gotConvID != "conv-existing" {
		t.Errorf("existing thread should reuse conversation, got %q", gotConvID)
	}
}

// TestBot_HandleMessage_StartError edits the placeholder with the error.
func TestBot_HandleMessage_StartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	chatSvc := svcmocks.NewMockChatService(ctrl)
	threadRepo := mocks.NewMockSlackThreadRepository(ctrl)

	threadRepo.EXPECT().Get(gomock.Any(), "C1:T1").Return("", nil)
	chatSvc.EXPECT().Start(gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)

	bot := NewBot(chatSvc, threadRepo, "xapp-1", "xoxb-1", logger.NewNop())
	msg := &fakeMessenger{}
	bot.handleMessage(context.Background(), msg, "C1", "T1", "hi")

	if final := msg.lastUpdate(); !strings.Contains(strings.ToLower(final), "couldn't start") {
		t.Errorf("expected error surfaced in update, got %q", final)
	}
}

func TestStripMention(t *testing.T) {
	cases := map[string]string{
		"<@U123> hello there": "hello there",
		"no mention here":     "no mention here",
		"<@U9> ":              "",
	}
	for in, want := range cases {
		if got := stripMention(in); got != want {
			t.Errorf("stripMention(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestThreadRoot(t *testing.T) {
	if got := threadRoot("T1", "ts9"); got != "T1" {
		t.Errorf("threadRoot with thread = %q", got)
	}
	if got := threadRoot("", "ts9"); got != "ts9" {
		t.Errorf("threadRoot without thread = %q", got)
	}
}
