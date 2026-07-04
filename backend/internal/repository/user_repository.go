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

//go:generate go run go.uber.org/mock/mockgen -source=user_repository.go -destination=../../test/mocks/user_repository_mock.go -package=mocks

// UserRepository defines the interface for GitHub user data access.
type UserRepository interface {
	Upsert(ctx context.Context, input *model.UpsertUserInput) (*model.User, error)
	GetByID(ctx context.Context, id string) (*model.User, error)
}

type userRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(txManager *transaction.TxManager, log *logger.Logger) UserRepository {
	return &userRepository{txManager: txManager, logger: log}
}

func (r *userRepository) Upsert(ctx context.Context, input *model.UpsertUserInput) (*model.User, error) {
	if err := input.Validate(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	user := &model.User{
		ID:             input.ID,
		Login:          input.Login,
		Name:           input.Name,
		AvatarURL:      input.AvatarURL,
		EncryptedToken: input.EncryptedToken,
	}
	// On conflict (returning user) refresh the mutable fields, including the
	// freshly-issued encrypted OAuth token.
	res := r.txManager.GetDB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"login", "name", "avatar_url", "encrypted_token", "updated_at",
		}),
	}).Create(user)
	if res.Error != nil {
		r.logger.Error("failed to upsert user", zap.Error(res.Error), zap.String("id", input.ID))
		return nil, pkgerrors.Internal("failed to upsert user", res.Error)
	}
	return user, nil
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	if id == "" {
		return nil, pkgerrors.InvalidArgument("id is required")
	}
	var user model.User
	result := r.txManager.GetDB(ctx).First(&user, "id = ?", id)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, pkgerrors.NotFoundf("user %s not found", id)
	}
	if result.Error != nil {
		r.logger.Error("failed to get user", zap.Error(result.Error), zap.String("id", id))
		return nil, pkgerrors.Internal("failed to get user", result.Error)
	}
	return &user, nil
}
