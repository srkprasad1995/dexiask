package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/dexiask/dexiask/internal/config"
	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
	"go.uber.org/zap"
)

// ConversationHandler serves the conversation REST endpoints.
//
//	GET /v1/conversations                 — list conversations
//	GET /v1/conversations/{id}            — get message transcript
//	GET /v1/conversations/{id}/messages   — get message transcript
type ConversationHandler struct {
	svc    service.ConversationService
	logger *logger.Logger
}

// NewConversationHandler creates a new ConversationHandler.
func NewConversationHandler(svc service.ConversationService, log *logger.Logger) *ConversationHandler {
	return &ConversationHandler{svc: svc, logger: log}
}

// ListConversations handles GET /v1/conversations.
func (h *ConversationHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	pageSize := 20
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 {
			pageSize = n
		}
	}
	filter := &model.ListConversationsFilter{
		WorkspaceID: config.FixedWorkspaceID,
		UserID:      config.FixedUserID,
		PageSize:    pageSize,
		PageToken:   r.URL.Query().Get("page_token"),
	}
	convs, nextToken, err := h.svc.ListConversations(r.Context(), filter)
	if err != nil {
		h.logger.Error("ListConversations failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}
	if convs == nil {
		convs = []*model.Conversation{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"conversations":   convs,
		"next_page_token": nextToken,
	})
}

// GetConversation handles GET /v1/conversations/{id} and
// GET /v1/conversations/{id}/messages — both return the message transcript.
func (h *ConversationHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/v1/conversations/")
	p = strings.TrimSuffix(p, "/messages")
	convID := strings.Trim(p, "/")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "conversation id is required")
		return
	}

	msgs, err := h.svc.GetMessages(r.Context(), config.FixedWorkspaceID, config.FixedUserID, convID)
	if err != nil {
		h.logger.Error("GetMessages failed", zap.Error(err), zap.String("conversation_id", convID))
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if msgs == nil {
		msgs = []*model.MessageWithAttachments{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"messages": msgs})
}
