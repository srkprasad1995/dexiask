package repository

import (
	"context"
	"time"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/pkg/transaction"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

//go:generate go run go.uber.org/mock/mockgen -source=message_repository.go -destination=../../test/mocks/message_repository_mock.go -package=mocks

// MessageRepository defines the interface for message data access.
type MessageRepository interface {
	Append(ctx context.Context, input *model.AppendMessageInput) (*model.Message, error)
	UpdateMessage(ctx context.Context, input *model.UpdateMessageInput) error
	ListByConversation(ctx context.Context, conversationID string) ([]*model.Message, error)
}

type messageRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
}

// NewMessageRepository creates a new MessageRepository.
func NewMessageRepository(txManager *transaction.TxManager, log *logger.Logger) MessageRepository {
	return &messageRepository{txManager: txManager, logger: log}
}

// Append inserts a message with a monotonically-increasing seq per conversation.
// The seq is computed atomically inside a CTE so concurrent appends are handled
// safely by the UNIQUE(conversation_id, seq) constraint in the database.
func (r *messageRepository) Append(ctx context.Context, input *model.AppendMessageInput) (*model.Message, error) {
	if err := input.Validate(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	id := uuid.New().String()
	now := time.Now()

	var seq int
	err := r.txManager.GetDB(ctx).Raw(`
		WITH next_seq AS (
			SELECT COALESCE(MAX(seq), 0) + 1 AS seq
			FROM messages
			WHERE conversation_id = ?
		)
		INSERT INTO messages (id, conversation_id, role, content, seq, status, model, created_at)
		SELECT ?, ?, ?, ?, next_seq.seq, ?, ?, ?
		FROM next_seq
		RETURNING seq`,
		input.ConversationID,
		id, input.ConversationID, input.Role, input.Content,
		input.Status, input.Model, now,
	).Scan(&seq).Error
	if err != nil {
		r.logger.Error("failed to append message", zap.Error(err), zap.String("conversation_id", input.ConversationID))
		return nil, pkgerrors.Internal("failed to append message", err)
	}

	return &model.Message{
		ID:             id,
		ConversationID: input.ConversationID,
		Role:           input.Role,
		Content:        input.Content,
		Seq:            seq,
		Status:         input.Status,
		Model:          input.Model,
		CreatedAt:      now,
	}, nil
}

// UpdateMessage updates the content, status, model, and questions of a message.
func (r *messageRepository) UpdateMessage(ctx context.Context, input *model.UpdateMessageInput) error {
	if err := input.Validate(); err != nil {
		return pkgerrors.InvalidArgument(err.Error())
	}
	updates := map[string]interface{}{
		"content": input.Content,
		"status":  input.Status,
		"model":   input.Model,
	}
	if len(input.Questions) > 0 {
		updates["questions"] = string(input.Questions)
	} else {
		updates["questions"] = nil
	}

	result := r.txManager.GetDB(ctx).
		Model(&model.Message{}).
		Where("id = ?", input.ID).
		Updates(updates)
	if result.Error != nil {
		r.logger.Error("failed to update message", zap.Error(result.Error), zap.String("id", input.ID))
		return pkgerrors.Internal("failed to update message", result.Error)
	}
	if result.RowsAffected == 0 {
		return pkgerrors.NotFoundf("message %s not found", input.ID)
	}
	return nil
}

// ListByConversation returns all messages for a conversation ordered by seq ASC.
func (r *messageRepository) ListByConversation(ctx context.Context, conversationID string) ([]*model.Message, error) {
	if conversationID == "" {
		return nil, pkgerrors.InvalidArgument("conversation_id is required")
	}
	var msgs []*model.Message
	result := r.txManager.GetDB(ctx).
		Where("conversation_id = ?", conversationID).
		Order("seq ASC").
		Find(&msgs)
	if result.Error != nil {
		r.logger.Error("failed to list messages", zap.Error(result.Error), zap.String("conversation_id", conversationID))
		return nil, pkgerrors.Internal("failed to list messages", result.Error)
	}
	return msgs, nil
}
