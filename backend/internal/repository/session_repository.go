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

//go:generate go run go.uber.org/mock/mockgen -source=session_repository.go -destination=../../test/mocks/session_repository_mock.go -package=mocks

// SessionRepository defines the interface for login-session data access.
type SessionRepository interface {
	Create(ctx context.Context, userID string, ttl time.Duration) (*model.Session, error)
	GetByID(ctx context.Context, id string) (*model.Session, error)
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
}

type sessionRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
	newID     func() string
	now       func() time.Time
}

// NewSessionRepository creates a new SessionRepository.
func NewSessionRepository(txManager *transaction.TxManager, log *logger.Logger) SessionRepository {
	return &sessionRepository{
		txManager: txManager,
		logger:    log,
		newID:     func() string { return uuid.New().String() },
		now:       time.Now,
	}
}

func (r *sessionRepository) Create(ctx context.Context, userID string, ttl time.Duration) (*model.Session, error) {
	if userID == "" {
		return nil, pkgerrors.InvalidArgument("user_id is required")
	}
	sess := &model.Session{
		ID:        r.newID(),
		UserID:    userID,
		ExpiresAt: r.now().Add(ttl),
	}
	if result := r.txManager.GetDB(ctx).Create(sess); result.Error != nil {
		r.logger.Error("failed to create session", zap.Error(result.Error))
		return nil, pkgerrors.Internal("failed to create session", result.Error)
	}
	return sess, nil
}

func (r *sessionRepository) GetByID(ctx context.Context, id string) (*model.Session, error) {
	if id == "" {
		return nil, pkgerrors.InvalidArgument("id is required")
	}
	var sess model.Session
	result := r.txManager.GetDB(ctx).First(&sess, "id = ?", id)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, pkgerrors.NotFoundf("session %s not found", id)
	}
	if result.Error != nil {
		r.logger.Error("failed to get session", zap.Error(result.Error))
		return nil, pkgerrors.Internal("failed to get session", result.Error)
	}
	return &sess, nil
}

func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return pkgerrors.InvalidArgument("id is required")
	}
	if result := r.txManager.GetDB(ctx).Delete(&model.Session{}, "id = ?", id); result.Error != nil {
		r.logger.Error("failed to delete session", zap.Error(result.Error))
		return pkgerrors.Internal("failed to delete session", result.Error)
	}
	return nil
}

func (r *sessionRepository) DeleteExpired(ctx context.Context) error {
	if result := r.txManager.GetDB(ctx).Where("expires_at < ?", r.now()).Delete(&model.Session{}); result.Error != nil {
		r.logger.Error("failed to delete expired sessions", zap.Error(result.Error))
		return pkgerrors.Internal("failed to delete expired sessions", result.Error)
	}
	return nil
}
