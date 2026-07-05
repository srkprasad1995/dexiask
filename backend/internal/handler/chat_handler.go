package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dexiask/dexiask/internal/agent"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
	"go.uber.org/zap"
)

// ChatHandler serves the chat SSE endpoints.
//
//	POST /v1/chat/stream  — start a new turn / conversation
//	GET  /v1/chat/stream  — resume an in-progress run (Last-Event-ID cursor)
//	POST /v1/chat/stop    — explicitly stop an in-progress run
//
// Generation is decoupled from the HTTP connection: the engine runs on a
// background goroutine (run-scoped context) that never gets cancelled by client
// disconnect. HTTP handlers subscribe to the Run's event buffer and unsubscribe
// on disconnect, leaving the run running.
type ChatHandler struct {
	chatSvc service.ChatService
	logger  *logger.Logger
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(chatSvc service.ChatService, log *logger.Logger) *ChatHandler {
	return &ChatHandler{chatSvc: chatSvc, logger: log}
}

// chatRequest is the JSON body for POST /v1/chat/stream.
type chatRequest struct {
	ConversationID string             `json:"conversationId"`
	Messages       []agent.Message    `json:"messages"`
	Attachments    []agent.Attachment `json:"attachments,omitempty"`
	UploadBucket   string             `json:"uploadBucket,omitempty"`
}

// ServeHTTP dispatches GET / POST to the right handler.
func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleStart(w, r)
	case http.MethodGet:
		h.handleResume(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ServeStop handles POST /v1/chat/stop.
func (h *ChatHandler) ServeStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	convID := r.URL.Query().Get("conversationId")
	if convID == "" {
		http.Error(w, "conversationId query param required", http.StatusBadRequest)
		return
	}
	if !h.chatSvc.Stop(convID) {
		http.Error(w, "no active run for conversation", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleStart handles POST /v1/chat/stream.
func (h *ChatHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	hasContent := false
	for _, m := range req.Messages {
		if m.Content != "" || len(m.Attachments) > 0 {
			hasContent = true
			break
		}
	}
	if !hasContent && (len(req.Attachments) > 0 || req.UploadBucket != "") {
		hasContent = true
	}
	if !hasContent {
		http.Error(w, "no non-empty messages", http.StatusBadRequest)
		return
	}

	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	result, err := h.chatSvc.Start(r.Context(), service.ChatRequest{
		ConversationID: req.ConversationID,
		UserID:         p.UserID,
		GitHubToken:    p.GitHubToken,
		IsAdmin:        p.IsAdmin(),
		Messages:       req.Messages,
		Attachments:    req.Attachments,
		UploadBucket:   req.UploadBucket,
	})
	if err != nil {
		h.logger.Error("failed to start chat", zap.Error(err))
		writeSSEError(w, err.Error())
		return
	}

	h.logger.Info("chat run started",
		zap.String("conversation_id", result.ConversationID),
		zap.Bool("is_new", result.IsNew))

	sseHeaders(w)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	convFrame, _ := json.Marshal(map[string]string{"type": "conversation", "id": result.ConversationID})
	fmt.Fprintf(w, "id: 0\ndata: %s\n\n", convFrame)
	flusher.Flush()

	streamRun(w, r, flusher, result.Run, 0, h.logger)
}

// handleResume handles GET /v1/chat/stream?conversationId=…
func (h *ChatHandler) handleResume(w http.ResponseWriter, r *http.Request) {
	convID := r.URL.Query().Get("conversationId")
	if convID == "" {
		http.Error(w, "conversationId query param required", http.StatusBadRequest)
		return
	}

	run, ok, err := h.chatSvc.Resume(r.Context(), convID)
	if err != nil {
		h.logger.Error("resume failed", zap.Error(err), zap.String("conversation_id", convID))
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	fromIndex := 0
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		if n, err := strconv.Atoi(lastID); err == nil && n >= 0 {
			fromIndex = n + 1
		}
	}

	sseHeaders(w)
	flusher, ok2 := w.(http.Flusher)
	if !ok2 {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	h.logger.Info("client resumed run",
		zap.String("conversation_id", convID), zap.Int("from_index", fromIndex))

	streamRun(w, r, flusher, run, fromIndex, h.logger)
}

// streamRun subscribes to the run and writes SSE frames until the run is done
// or r.Context() is cancelled (client disconnect). The run goroutine is NOT
// affected — this is a pure subscriber.
func streamRun(w http.ResponseWriter, r *http.Request, flusher http.Flusher, run *agent.Run, fromIndex int, log *logger.Logger) {
	sub := run.Subscribe(r.Context(), fromIndex)
	idx := fromIndex
	for ev := range sub {
		b, err := json.Marshal(ev)
		if err != nil {
			log.Warn("failed to marshal event", zap.Error(err))
			continue
		}
		fmt.Fprintf(w, "id: %d\ndata: %s\n\n", idx, b)
		flusher.Flush()
		idx++
	}
	if r.Context().Err() == nil {
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

// sseHeaders sets the required SSE response headers.
func sseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// writeSSEError writes an error event + [DONE], for failures before the stream
// is open (no flusher yet).
func writeSSEError(w http.ResponseWriter, msg string) {
	sseHeaders(w)
	ev := agent.Event{Type: "error", Message: msg}
	if b, err := json.Marshal(ev); err == nil {
		fmt.Fprintf(w, "data: %s\n\n", b)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}
