package repository

import (
	"context"
	"path"
	"time"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/pkg/transaction"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:generate go run go.uber.org/mock/mockgen -source=attachment_repository.go -destination=../../test/mocks/attachment_repository_mock.go -package=mocks

// AttachmentRepository defines the interface for attachment data access.
type AttachmentRepository interface {
	Create(ctx context.Context, input *model.StoreAttachmentInput) (*model.Attachment, error)
	GetByID(ctx context.Context, id string) (*model.Attachment, error)
	// ListByConversation returns all attachments bound to a conversation message.
	ListByConversation(ctx context.Context, conversationID string) ([]*model.Attachment, error)
	// Reconcile binds pending uploads to a real conversation + message, rewriting
	// rel_path to the conversation directory. Returns the updated attachment IDs.
	Reconcile(ctx context.Context, uploadBucket, conversationID, messageID, newRelPathPrefix string) ([]string, error)
}

type attachmentRepository struct {
	txManager *transaction.TxManager
	logger    *logger.Logger
}

// NewAttachmentRepository creates a new AttachmentRepository.
func NewAttachmentRepository(txManager *transaction.TxManager, log *logger.Logger) AttachmentRepository {
	return &attachmentRepository{txManager: txManager, logger: log}
}

func (r *attachmentRepository) Create(ctx context.Context, input *model.StoreAttachmentInput) (*model.Attachment, error) {
	if err := input.Validate(); err != nil {
		return nil, pkgerrors.InvalidArgument(err.Error())
	}
	att := &model.Attachment{
		ID:             input.ID,
		ConversationID: input.ConversationID,
		UploadBucket:   input.UploadBucket,
		Filename:       input.Filename,
		MediaType:      input.MediaType,
		Size:           input.Size,
		RelPath:        input.RelPath,
		CreatedAt:      time.Now(),
	}
	if err := r.txManager.GetDB(ctx).Create(att).Error; err != nil {
		r.logger.Error("failed to create attachment", zap.Error(err), zap.String("id", input.ID))
		return nil, pkgerrors.Internal("failed to create attachment", err)
	}
	return att, nil
}

func (r *attachmentRepository) GetByID(ctx context.Context, id string) (*model.Attachment, error) {
	if id == "" {
		return nil, pkgerrors.InvalidArgument("id is required")
	}
	var att model.Attachment
	result := r.txManager.GetDB(ctx).Where("id = ?", id).First(&att)
	if result.Error != nil {
		return nil, pkgerrors.NotFoundf("attachment %s not found", id)
	}
	return &att, nil
}

func (r *attachmentRepository) ListByConversation(ctx context.Context, conversationID string) ([]*model.Attachment, error) {
	if conversationID == "" {
		return nil, pkgerrors.InvalidArgument("conversation_id is required")
	}
	var atts []*model.Attachment
	result := r.txManager.GetDB(ctx).
		Where("conversation_id = ? AND message_id IS NOT NULL", conversationID).
		Order("created_at ASC").
		Find(&atts)
	if result.Error != nil {
		r.logger.Error("failed to list attachments", zap.Error(result.Error), zap.String("conversation_id", conversationID))
		return nil, pkgerrors.Internal("failed to list attachments", result.Error)
	}
	return atts, nil
}

func (r *attachmentRepository) Reconcile(ctx context.Context, uploadBucket, conversationID, messageID, newRelPathPrefix string) ([]string, error) {
	if uploadBucket == "" && conversationID == "" {
		return nil, pkgerrors.InvalidArgument("upload_bucket or conversation_id required")
	}
	db := r.txManager.GetDB(ctx)

	var atts []*model.Attachment
	var query *gorm.DB
	if uploadBucket != "" {
		query = db.Where("message_id IS NULL AND (upload_bucket = ? OR conversation_id = ?)", uploadBucket, conversationID)
	} else {
		query = db.Where("message_id IS NULL AND conversation_id = ?", conversationID)
	}
	if err := query.Find(&atts).Error; err != nil {
		return nil, pkgerrors.Internal("failed to find attachments for reconcile", err)
	}
	if len(atts) == 0 {
		return []string{}, nil
	}

	ids := make([]string, 0, len(atts))
	for _, att := range atts {
		ids = append(ids, att.ID)
		updates := map[string]interface{}{
			"conversation_id": conversationID,
			"message_id":      messageID,
			"upload_bucket":   "",
		}
		if newRelPathPrefix != "" {
			updates["rel_path"] = path.Join(newRelPathPrefix, path.Base(att.RelPath))
		}
		if err := db.Model(&model.Attachment{}).Where("id = ?", att.ID).Updates(updates).Error; err != nil {
			r.logger.Error("failed to reconcile attachment", zap.Error(err), zap.String("id", att.ID))
			return nil, pkgerrors.Internal("failed to reconcile attachments", err)
		}
	}
	return ids, nil
}
