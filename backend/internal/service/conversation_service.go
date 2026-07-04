package service

import (
	"context"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/repository"
	"go.uber.org/zap"
)

//go:generate go run go.uber.org/mock/mockgen -source=conversation_service.go -destination=../../test/svcmocks/conversation_service_mock.go -package=svcmocks

// ConversationService provides read operations for conversation data. Writes
// (create, append) are performed by ChatService as part of the generation flow.
type ConversationService interface {
	ListConversations(ctx context.Context, filter *model.ListConversationsFilter) ([]*model.Conversation, string, error)
	GetMessages(ctx context.Context, workspaceID, userID, conversationID string) ([]*model.MessageWithAttachments, error)
}

type conversationService struct {
	convRepo       repository.ConversationRepository
	msgRepo        repository.MessageRepository
	attachmentRepo repository.AttachmentRepository
	logger         *logger.Logger
}

// NewConversationService creates a new ConversationService.
func NewConversationService(
	convRepo repository.ConversationRepository,
	msgRepo repository.MessageRepository,
	attachmentRepo repository.AttachmentRepository,
	log *logger.Logger,
) ConversationService {
	return &conversationService{
		convRepo:       convRepo,
		msgRepo:        msgRepo,
		attachmentRepo: attachmentRepo,
		logger:         log,
	}
}

func (s *conversationService) ListConversations(ctx context.Context, filter *model.ListConversationsFilter) ([]*model.Conversation, string, error) {
	return s.convRepo.List(ctx, filter)
}

func (s *conversationService) GetMessages(ctx context.Context, workspaceID, userID, conversationID string) ([]*model.MessageWithAttachments, error) {
	conv, err := s.convRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.WorkspaceID != workspaceID || conv.UserID != userID {
		return nil, pkgerrors.NotFoundf("conversation %s not found", conversationID)
	}

	msgs, err := s.msgRepo.ListByConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	atts, err := s.attachmentRepo.ListByConversation(ctx, conversationID)
	if err != nil {
		s.logger.Warn("failed to load attachments for conversation",
			zap.String("conversation_id", conversationID), zap.Error(err))
		atts = nil
	}
	attsByMsg := make(map[string][]*model.Attachment, len(atts))
	for _, a := range atts {
		if a.MessageID != nil {
			attsByMsg[*a.MessageID] = append(attsByMsg[*a.MessageID], a)
		}
	}

	result := make([]*model.MessageWithAttachments, 0, len(msgs))
	for _, m := range msgs {
		mwa := &model.MessageWithAttachments{Message: *m, Attachments: attsByMsg[m.ID]}
		if mwa.Attachments == nil {
			mwa.Attachments = []*model.Attachment{}
		}
		result = append(result, mwa)
	}
	return result, nil
}
