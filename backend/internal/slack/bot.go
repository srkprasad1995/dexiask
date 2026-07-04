// Package slack implements a Slack Socket Mode bot that is a second front-end
// onto ChatService — it does NOT change the Agent Job Protocol. Each Slack
// thread (channel + thread_ts) maps to one Dexiask conversation; the bot posts
// a placeholder message and edits it (chat.update) as text.delta events stream.
package slack

import (
	"context"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"go.uber.org/zap"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/repository"
	"github.com/dexiask/dexiask/internal/service"
)

// updateInterval throttles chat.update calls while streaming.
const updateInterval = 700 * time.Millisecond

// messenger is the minimal Slack surface the bot needs. Abstracted so the core
// message-handling logic is testable without a live Slack connection.
type messenger interface {
	// Post sends a new message in a thread and returns its timestamp.
	Post(ctx context.Context, channel, threadTS, text string) (ts string, err error)
	// Update edits a previously posted message.
	Update(ctx context.Context, channel, ts, text string) error
}

// Bot is the Slack Socket Mode bot.
type Bot struct {
	chatSvc    service.ChatService
	threadRepo repository.SlackThreadRepository
	logger     *logger.Logger

	appToken string
	botToken string
	enabled  bool

	// msg is the live Slack messenger, set in Run. Injectable for tests.
	msg messenger
}

// NewBot creates a Slack bot. It is inert (Run is a no-op) unless both
// SLACK_APP_TOKEN and SLACK_BOT_TOKEN are non-empty.
func NewBot(chatSvc service.ChatService, threadRepo repository.SlackThreadRepository, appToken, botToken string, log *logger.Logger) *Bot {
	return &Bot{
		chatSvc:    chatSvc,
		threadRepo: threadRepo,
		logger:     log,
		appToken:   appToken,
		botToken:   botToken,
		enabled:    appToken != "" && botToken != "",
	}
}

// Enabled reports whether both tokens are set.
func (b *Bot) Enabled() bool { return b.enabled }

// Run connects to Slack and processes events until ctx is cancelled. When the
// bot is disabled it logs and returns nil immediately. Run blocks, so callers
// launch it in a goroutine (see the Fx lifecycle hook).
func (b *Bot) Run(ctx context.Context) error {
	if !b.enabled {
		b.logger.Info("slack disabled (SLACK_APP_TOKEN / SLACK_BOT_TOKEN not both set)")
		return nil
	}

	api := slack.New(b.botToken, slack.OptionAppLevelToken(b.appToken))
	client := socketmode.New(api)
	b.msg = &slackMessenger{api: api}

	go b.loop(ctx, client)

	b.logger.Info("slack bot connecting (socket mode)")
	return client.RunContext(ctx)
}

// loop dispatches inbound Socket Mode events.
func (b *Bot) loop(ctx context.Context, client *socketmode.Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-client.Events:
			if !ok {
				return
			}
			if evt.Type != socketmode.EventTypeEventsAPI {
				continue
			}
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				continue
			}
			if evt.Request != nil {
				client.Ack(*evt.Request)
			}
			if eventsAPIEvent.Type != slackevents.CallbackEvent {
				continue
			}
			b.dispatch(ctx, eventsAPIEvent.InnerEvent)
		}
	}
}

// dispatch routes app_mention and DM message events to handleMessage.
func (b *Bot) dispatch(ctx context.Context, inner slackevents.EventsAPIInnerEvent) {
	switch ev := inner.Data.(type) {
	case *slackevents.AppMentionEvent:
		text := stripMention(ev.Text)
		go b.handleMessage(ctx, b.msg, ev.Channel, threadRoot(ev.ThreadTimeStamp, ev.TimeStamp), text)
	case *slackevents.MessageEvent:
		// Only handle human DMs: ignore bot echoes, edits, and non-IM channels.
		if ev.BotID != "" || ev.SubType != "" || ev.User == "" {
			return
		}
		if ev.ChannelType != "im" {
			return
		}
		go b.handleMessage(ctx, b.msg, ev.Channel, threadRoot(ev.ThreadTimeStamp, ev.TimeStamp), ev.Text)
	}
}

// handleMessage maps the Slack thread to a conversation, starts a ChatService
// turn, and streams the reply back into an edited Slack message.
func (b *Bot) handleMessage(ctx context.Context, msg messenger, channel, threadTS, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	threadKey := channel + ":" + threadTS

	convID, err := b.threadRepo.Get(ctx, threadKey)
	if err != nil {
		b.logger.Error("slack: failed to look up thread mapping", zap.Error(err), zap.String("thread_key", threadKey))
		// Continue with an empty convID — a new conversation will be created.
	}

	// Post a placeholder we will edit as the reply streams in.
	placeholderTS, err := msg.Post(ctx, channel, threadTS, "…")
	if err != nil {
		b.logger.Error("slack: failed to post placeholder", zap.Error(err), zap.String("channel", channel))
		return
	}

	result, err := b.chatSvc.Start(ctx, service.ChatRequest{
		ConversationID: convID,
		Messages:       []agent.Message{{Role: "user", Content: text}},
	})
	if err != nil {
		b.logger.Error("slack: chat start failed", zap.Error(err))
		_ = msg.Update(ctx, channel, placeholderTS, "Sorry — I couldn't start a response: "+err.Error())
		return
	}

	// Remember the mapping for a brand-new conversation so follow-ups reuse it.
	if convID == "" {
		if err := b.threadRepo.Put(ctx, threadKey, result.ConversationID); err != nil {
			b.logger.Error("slack: failed to persist thread mapping", zap.Error(err), zap.String("thread_key", threadKey))
		}
	}

	b.stream(ctx, msg, channel, placeholderTS, result.Run)
}

// stream subscribes to the run, accumulates the reply text, and edits the Slack
// message on a throttle plus a final update at the end.
func (b *Bot) stream(ctx context.Context, msg messenger, channel, ts string, run *agent.Run) {
	sub := run.Subscribe(ctx, 0)

	var sb strings.Builder
	var errMsg string
	lastUpdate := time.Now()
	lastRendered := ""

	flush := func(force bool) {
		text := render(sb.String(), errMsg)
		if text == "" || text == lastRendered {
			return
		}
		if !force && time.Since(lastUpdate) < updateInterval {
			return
		}
		if err := msg.Update(ctx, channel, ts, text); err != nil {
			b.logger.Warn("slack: chat.update failed", zap.Error(err))
			return
		}
		lastUpdate = time.Now()
		lastRendered = text
	}

	for ev := range sub {
		switch ev.Type {
		case "text.delta":
			sb.WriteString(ev.Text)
			flush(false)
		case "error":
			errMsg = ev.Message
		case "result":
			if ev.Status == "error" && errMsg == "" {
				errMsg = "the engine reported an error"
			}
		}
	}
	flush(true)
}

// render composes the final Slack message body from the accumulated reply and
// any error. An empty reply falls back to a short notice.
func render(reply, errMsg string) string {
	reply = strings.TrimSpace(reply)
	if errMsg != "" {
		if reply == "" {
			return "⚠️ " + errMsg
		}
		return reply + "\n\n⚠️ " + errMsg
	}
	if reply == "" {
		return "…"
	}
	return reply
}

// threadRoot returns the thread anchor: the containing thread when the message
// is already in one, otherwise the message's own timestamp (starting a thread).
func threadRoot(threadTS, ts string) string {
	if threadTS != "" {
		return threadTS
	}
	return ts
}

// stripMention removes a leading "<@BOTID>" mention token from app_mention text.
func stripMention(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "<@") {
		if idx := strings.Index(text, ">"); idx >= 0 {
			return strings.TrimSpace(text[idx+1:])
		}
	}
	return text
}

// slackMessenger is the live Slack implementation of messenger.
type slackMessenger struct {
	api *slack.Client
}

func (m *slackMessenger) Post(ctx context.Context, channel, threadTS, text string) (string, error) {
	_, ts, err := m.api.PostMessageContext(ctx, channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
	return ts, err
}

func (m *slackMessenger) Update(ctx context.Context, channel, ts, text string) error {
	_, _, _, err := m.api.UpdateMessageContext(ctx, channel, ts, slack.MsgOptionText(text, false))
	return err
}
