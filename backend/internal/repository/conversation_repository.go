package repository

import (
	"context"
	"errors"
	"time"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/pkg/transaction"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:generate go run go.uber.org/mock/mockgen -source=conversation_repository.go -destination=../../test/mocks/conversation_repository_mock.go -package=mocks

// ConversationRepository defines the interface for conversation data access.
type ConversationRepository interface {
	Create(ctx context.Context, input *model.CreateConversationInput) (*model.Conversation, error)
	GetByID(ctx context.Context, id string) (*model.Conversation, error)
	List(ctx context.Context, filter *model.ListConversationsFilter) ([]*model.Conversation, string, error)
	Touch(ctx context.Context, id string) error
	// UpdateSessionID persists the Claude Agent SDK session identifier so the next
	// turn can resume the conversation natively.
	UpdateSessionID(ctx context.Context, id, sessionID string) error
}

type conversationRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
}

// NewConversationRepository creates a new ConversationRepository.
func NewConversationRepository(txManager *transaction.TxManager, log *logger.Logger) ConversationRepository {
	return &conversationRepository{txManager: txManager, logger: log}
}

func (r *conversationRepository) Create(ctx context.Context, input *model.CreateConversationInput) (*model.Conversation, error) {
	if err := input.Validate(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	title := input.Title
	if title == "" {
		title = "New conversation"
	}
	conv := &model.Conversation{
		ID:          uuid.New().String(),
		WorkspaceID: input.WorkspaceID,
		UserID:      input.UserID,
		Title:       title,
	}
	if result := r.txManager.GetDB(ctx).Create(conv); result.Error != nil {
		r.logger.Error("failed to create conversation", zap.Error(result.Error))
		return nil, pkgerrors.Internal("failed to create conversation", result.Error)
	}
	r.logger.Info("conversation created", zap.String("id", conv.ID))
	return conv, nil
}

func (r *conversationRepository) GetByID(ctx context.Context, id string) (*model.Conversation, error) {
	if id == "" {
		return nil, pkgerrors.InvalidArgument("id is required")
	}
	var conv model.Conversation
	result := r.txManager.GetDB(ctx).First(&conv, "id = ?", id)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, pkgerrors.NotFoundf("conversation %s not found", id)
	}
	if result.Error != nil {
		r.logger.Error("failed to get conversation", zap.Error(result.Error), zap.String("id", id))
		return nil, pkgerrors.Internal("failed to get conversation", result.Error)
	}
	return &conv, nil
}

func (r *conversationRepository) List(ctx context.Context, filter *model.ListConversationsFilter) ([]*model.Conversation, string, error) {
	if err := filter.Validate(); err != nil {
		return nil, "", pkgerrors.InvalidArgument(err.Error())
	}
	pageSize := filter.PageSize
	if pageSize == 0 {
		pageSize = 20
	}

	var convs []*model.Conversation
	db := r.txManager.GetDB(ctx).
		Where("workspace_id = ? AND user_id = ?", filter.WorkspaceID, filter.UserID)

	if filter.PageToken != "" {
		var cursor model.Conversation
		if err := r.txManager.GetDB(ctx).Select("updated_at").Where("id = ?", filter.PageToken).First(&cursor).Error; err == nil {
			db = db.Where("(updated_at, id) < (?, ?)", cursor.UpdatedAt, filter.PageToken)
		}
	}

	result := db.Order("updated_at DESC, id DESC").Limit(pageSize + 1).Find(&convs)
	if result.Error != nil {
		r.logger.Error("failed to list conversations", zap.Error(result.Error))
		return nil, "", pkgerrors.Internal("failed to list conversations", result.Error)
	}

	var nextPageToken string
	if len(convs) > pageSize {
		convs = convs[:pageSize]
		nextPageToken = convs[pageSize-1].ID
	}
	return convs, nextPageToken, nil
}

func (r *conversationRepository) Touch(ctx context.Context, id string) error {
	if id == "" {
		return pkgerrors.InvalidArgument("id is required")
	}
	result := r.txManager.GetDB(ctx).
		Model(&model.Conversation{}).
		Where("id = ?", id).
		Update("updated_at", time.Now())
	if result.Error != nil {
		r.logger.Error("failed to touch conversation", zap.Error(result.Error), zap.String("id", id))
		return pkgerrors.Internal("failed to touch conversation", result.Error)
	}
	return nil
}

func (r *conversationRepository) UpdateSessionID(ctx context.Context, id, sessionID string) error {
	if id == "" {
		return pkgerrors.InvalidArgument("id is required")
	}
	result := r.txManager.GetDB(ctx).
		Model(&model.Conversation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"session_id": sessionID, "updated_at": time.Now()})
	if result.Error != nil {
		r.logger.Error("failed to update conversation session_id", zap.Error(result.Error), zap.String("id", id))
		return pkgerrors.Internal("failed to update conversation session_id", result.Error)
	}
	return nil
}
