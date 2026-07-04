package repository

import (
	"context"
	"errors"
	"time"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/pkg/transaction"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:generate go run go.uber.org/mock/mockgen -source=slack_thread_repository.go -destination=../../test/mocks/slack_thread_repository_mock.go -package=mocks

// SlackThreadRepository maps Slack threads to Dexiask conversations.
type SlackThreadRepository interface {
	// Get returns the conversation ID mapped to threadKey, or ("", nil) if none.
	Get(ctx context.Context, threadKey string) (string, error)
	// Put stores (or upserts) the threadKey → conversationID mapping.
	Put(ctx context.Context, threadKey, conversationID string) error
}

type slackThreadRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
}

// NewSlackThreadRepository creates a new SlackThreadRepository.
func NewSlackThreadRepository(txManager *transaction.TxManager, log *logger.Logger) SlackThreadRepository {
	return &slackThreadRepository{txManager: txManager, logger: log}
}

func (r *slackThreadRepository) Get(ctx context.Context, threadKey string) (string, error) {
	if threadKey == "" {
		return "", pkgerrors.InvalidArgument("thread_key is required")
	}
	var t model.SlackThread
	result := r.txManager.GetDB(ctx).First(&t, "thread_key = ?", threadKey)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if result.Error != nil {
		return "", pkgerrors.Internal("failed to get slack thread", result.Error)
	}
	return t.ConversationID, nil
}

func (r *slackThreadRepository) Put(ctx context.Context, threadKey, conversationID string) error {
	if threadKey == "" || conversationID == "" {
		return pkgerrors.InvalidArgument("thread_key and conversation_id are required")
	}
	t := &model.SlackThread{
		ThreadKey:      threadKey,
		ConversationID: conversationID,
		CreatedAt:      time.Now(),
	}
	// Upsert: first mapping wins; ignore conflicts on re-put.
	result := r.txManager.GetDB(ctx).
		Clauses(clauseDoNothing()).
		Create(t)
	if result.Error != nil {
		r.logger.Error("failed to put slack thread", zap.Error(result.Error), zap.String("thread_key", threadKey))
		return pkgerrors.Internal("failed to put slack thread", result.Error)
	}
	return nil
}
