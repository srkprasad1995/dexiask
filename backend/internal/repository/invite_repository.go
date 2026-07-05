package repository

import (
	"context"
	"errors"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/pkg/transaction"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:generate go run go.uber.org/mock/mockgen -source=invite_repository.go -destination=../../test/mocks/invite_repository_mock.go -package=mocks

// InviteRepository defines the interface for workspace-invite data access.
type InviteRepository interface {
	Create(ctx context.Context, login, createdBy string) (*model.Invite, error)
	GetByLogin(ctx context.Context, login string) (*model.Invite, error)
	Delete(ctx context.Context, login string) error
	List(ctx context.Context) ([]*model.Invite, error)
}

type inviteRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
}

// NewInviteRepository creates a new InviteRepository.
func NewInviteRepository(txManager *transaction.TxManager, log *logger.Logger) InviteRepository {
	return &inviteRepository{txManager: txManager, logger: log}
}

func (r *inviteRepository) Create(ctx context.Context, login, createdBy string) (*model.Invite, error) {
	login = model.NormalizeLogin(login)
	if login == "" {
		return nil, pkgerrors.InvalidArgument("login is required")
	}
	inv := &model.Invite{Login: login, CreatedBy: createdBy}
	// Idempotent: re-inviting an existing login is a no-op refresh.
	res := r.txManager.GetDB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(inv)
	if res.Error != nil {
		r.logger.Error("failed to create invite", zap.Error(res.Error))
		return nil, pkgerrors.Internal("failed to create invite", res.Error)
	}
	return inv, nil
}

func (r *inviteRepository) GetByLogin(ctx context.Context, login string) (*model.Invite, error) {
	login = model.NormalizeLogin(login)
	var inv model.Invite
	result := r.txManager.GetDB(ctx).First(&inv, "login = ?", login)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, pkgerrors.NotFoundf("invite for %q not found", login)
	}
	if result.Error != nil {
		return nil, pkgerrors.Internal("failed to get invite", result.Error)
	}
	return &inv, nil
}

func (r *inviteRepository) Delete(ctx context.Context, login string) error {
	login = model.NormalizeLogin(login)
	if result := r.txManager.GetDB(ctx).Delete(&model.Invite{}, "login = ?", login); result.Error != nil {
		return pkgerrors.Internal("failed to delete invite", result.Error)
	}
	return nil
}

func (r *inviteRepository) List(ctx context.Context) ([]*model.Invite, error) {
	var invites []*model.Invite
	if err := r.txManager.GetDB(ctx).Order("created_at ASC").Find(&invites).Error; err != nil {
		return nil, pkgerrors.Internal("failed to list invites", err)
	}
	return invites, nil
}
