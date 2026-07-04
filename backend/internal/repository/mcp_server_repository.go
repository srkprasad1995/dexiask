package repository

import (
	"context"
	"errors"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/pkg/transaction"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:generate go run go.uber.org/mock/mockgen -source=mcp_server_repository.go -destination=../../test/mocks/mcp_server_repository_mock.go -package=mocks

// MCPServerRepository defines the interface for MCP server data access.
type MCPServerRepository interface {
	Create(ctx context.Context, input *model.CreateMCPServerInput) (*model.MCPServer, error)
	GetByID(ctx context.Context, id string) (*model.MCPServer, error)
	List(ctx context.Context, filter *model.ListMCPServersFilter) ([]*model.MCPServer, error)
	Update(ctx context.Context, input *model.UpdateMCPServerInput) (*model.MCPServer, error)
	Delete(ctx context.Context, id string) error
}

type mcpServerRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
}

// NewMCPServerRepository creates a new MCPServerRepository.
func NewMCPServerRepository(txManager *transaction.TxManager, log *logger.Logger) MCPServerRepository {
	return &mcpServerRepository{txManager: txManager, logger: log}
}

func (r *mcpServerRepository) Create(ctx context.Context, input *model.CreateMCPServerInput) (*model.MCPServer, error) {
	if err := input.Validate(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	srv := &model.MCPServer{
		ID:          uuid.New().String(),
		WorkspaceID: input.WorkspaceID,
		Name:        input.Name,
		Type:        input.Type,
		URL:         input.URL,
		Headers:     input.Headers,
		Enabled:     input.Enabled,
	}
	if result := r.txManager.GetDB(ctx).Create(srv); result.Error != nil {
		r.logger.Error("failed to create mcp server", zap.Error(result.Error))
		return nil, pkgerrors.Internal("failed to create mcp server", result.Error)
	}
	r.logger.Info("mcp server created", zap.String("id", srv.ID))
	return srv, nil
}

func (r *mcpServerRepository) GetByID(ctx context.Context, id string) (*model.MCPServer, error) {
	if id == "" {
		return nil, pkgerrors.InvalidArgument("id is required")
	}
	var srv model.MCPServer
	result := r.txManager.GetDB(ctx).First(&srv, "id = ?", id)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, pkgerrors.NotFoundf("mcp server %s not found", id)
	}
	if result.Error != nil {
		r.logger.Error("failed to get mcp server", zap.Error(result.Error), zap.String("id", id))
		return nil, pkgerrors.Internal("failed to get mcp server", result.Error)
	}
	return &srv, nil
}

func (r *mcpServerRepository) List(ctx context.Context, filter *model.ListMCPServersFilter) ([]*model.MCPServer, error) {
	if err := filter.Validate(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	db := r.txManager.GetDB(ctx).Where("workspace_id = ?", filter.WorkspaceID)
	if filter.EnabledOnly {
		db = db.Where("enabled = ?", true)
	}
	var servers []*model.MCPServer
	if result := db.Order("created_at ASC, id ASC").Find(&servers); result.Error != nil {
		r.logger.Error("failed to list mcp servers", zap.Error(result.Error))
		return nil, pkgerrors.Internal("failed to list mcp servers", result.Error)
	}
	return servers, nil
}

func (r *mcpServerRepository) Update(ctx context.Context, input *model.UpdateMCPServerInput) (*model.MCPServer, error) {
	if err := input.Validate(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	updates := map[string]interface{}{}
	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.Type != nil {
		updates["type"] = *input.Type
	}
	if input.URL != nil {
		updates["url"] = *input.URL
	}
	if input.Headers != nil {
		updates["headers"] = *input.Headers
	}
	if input.Enabled != nil {
		updates["enabled"] = *input.Enabled
	}

	db := r.txManager.GetDB(ctx)
	if len(updates) > 0 {
		result := db.Model(&model.MCPServer{}).Where("id = ?", input.ID).Updates(updates)
		if result.Error != nil {
			r.logger.Error("failed to update mcp server", zap.Error(result.Error), zap.String("id", input.ID))
			return nil, pkgerrors.Internal("failed to update mcp server", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil, pkgerrors.NotFoundf("mcp server %s not found", input.ID)
		}
	}
	return r.GetByID(ctx, input.ID)
}

func (r *mcpServerRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return pkgerrors.InvalidArgument("id is required")
	}
	result := r.txManager.GetDB(ctx).Delete(&model.MCPServer{}, "id = ?", id)
	if result.Error != nil {
		r.logger.Error("failed to delete mcp server", zap.Error(result.Error), zap.String("id", id))
		return pkgerrors.Internal("failed to delete mcp server", result.Error)
	}
	if result.RowsAffected == 0 {
		return pkgerrors.NotFoundf("mcp server %s not found", id)
	}
	return nil
}
