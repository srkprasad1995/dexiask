package service

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/repository"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

//go:generate go run go.uber.org/mock/mockgen -source=attachment_service.go -destination=../../test/svcmocks/attachment_service_mock.go -package=svcmocks

const maxAttachmentSize = 50 * 1024 * 1024 // 50 MB

// workspaceSubdir is the single fixed workspace root under the mount (mirrors
// agent.WorkspacePath). All attachment rel_paths are namespaced under it.
const workspaceSubdir = ".dexiask"

// AttachmentService manages file attachment storage and retrieval.
type AttachmentService interface {
	// Store writes a file under the workspace filesystem (path-jailed) and
	// inserts an attachment record. conversationID may be empty on first-turn
	// uploads; uploadBucket must be set then.
	Store(ctx context.Context, in StoreInput) (*model.Attachment, error)
	// Reconcile moves pending-bucket files to the real conversation directory and
	// stamps conversation_id + message_id on all affected rows.
	Reconcile(ctx context.Context, uploadBucket, conversationID, messageID string) error
	// Open returns an io.ReadCloser for the attachment file plus its metadata.
	Open(ctx context.Context, id string) (io.ReadCloser, *model.Attachment, error)
	// ListByConversation returns all attachments linked to a conversation.
	ListByConversation(ctx context.Context, conversationID string) ([]*model.Attachment, error)
}

// StoreInput is the input for AttachmentService.Store.
type StoreInput struct {
	ConversationID string
	UploadBucket   string
	Filename       string
	MediaType      string
	Size           int64
	Reader         io.Reader
}

type attachmentService struct {
	workspaceRoot  string
	attachmentRepo repository.AttachmentRepository
	logger         *logger.Logger
	maxSize        int64
}

// NewAttachmentService creates a new AttachmentService. workspaceRoot is the
// host path mounted at /workspace (DEXIASK_WORKSPACE_MOUNT).
func NewAttachmentService(workspaceRoot string, repo repository.AttachmentRepository, log *logger.Logger) AttachmentService {
	return &attachmentService{
		workspaceRoot:  workspaceRoot,
		attachmentRepo: repo,
		logger:         log,
		maxSize:        maxAttachmentSize,
	}
}

func (s *attachmentService) Store(ctx context.Context, in StoreInput) (*model.Attachment, error) {
	if in.ConversationID == "" && in.UploadBucket == "" {
		return nil, pkgerrors.InvalidArgument("conversation_id or upload_bucket is required")
	}
	if in.Size > s.maxSize {
		return nil, pkgerrors.InvalidArgumentf("file exceeds max size of %d bytes", s.maxSize)
	}

	safe := sanitizeFilename(in.Filename)
	if safe == "" {
		return nil, pkgerrors.InvalidArgument("invalid filename")
	}

	mediaType := in.MediaType
	if mediaType == "" || mediaType == "application/octet-stream" {
		if t := mime.TypeByExtension(filepath.Ext(safe)); t != "" {
			mediaType = t
		} else {
			mediaType = "application/octet-stream"
		}
	}
	if idx := strings.Index(mediaType, ";"); idx >= 0 {
		mediaType = strings.TrimSpace(mediaType[:idx])
	}

	fileID := uuid.New().String()

	var relDir string
	if in.ConversationID != "" {
		relDir = filepath.Join(workspaceSubdir, "conversations", in.ConversationID, "attachments")
	} else {
		relDir = filepath.Join(workspaceSubdir, "conversations", "pending-"+in.UploadBucket, "attachments")
	}
	relPath := filepath.Join(relDir, fileID+"-"+safe)
	absPath, err := s.jailPath(relPath)
	if err != nil {
		return nil, pkgerrors.InvalidArgument("invalid attachment path")
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, pkgerrors.Internal("failed to create attachment directory", err)
	}

	f, err := os.Create(absPath)
	if err != nil {
		return nil, pkgerrors.Internal("failed to create attachment file", err)
	}
	defer f.Close()

	// Enforce the cap on the actual byte stream, not just the client-declared
	// size: read at most maxSize+1 bytes so an oversized (or size-spoofed)
	// upload is rejected rather than written to disk unbounded.
	written, err := io.Copy(f, io.LimitReader(in.Reader, s.maxSize+1))
	if err != nil {
		os.Remove(absPath)
		return nil, pkgerrors.Internal("failed to write attachment", err)
	}
	if written > s.maxSize {
		os.Remove(absPath)
		return nil, pkgerrors.InvalidArgumentf("file exceeds max size of %d bytes", s.maxSize)
	}
	if in.Size > 0 && written != in.Size {
		os.Remove(absPath)
		return nil, pkgerrors.InvalidArgumentf("attachment size mismatch: expected %d, got %d", in.Size, written)
	}

	att, err := s.attachmentRepo.Create(ctx, &model.StoreAttachmentInput{
		ID:             fileID,
		ConversationID: in.ConversationID,
		UploadBucket:   in.UploadBucket,
		Filename:       safe,
		MediaType:      mediaType,
		Size:           written,
		RelPath:        relPath,
	})
	if err != nil {
		os.Remove(absPath)
		return nil, err
	}

	s.logger.Info("attachment stored", zap.String("id", fileID), zap.String("rel_path", relPath))
	return att, nil
}

func (s *attachmentService) Reconcile(ctx context.Context, uploadBucket, conversationID, messageID string) error {
	if conversationID == "" {
		return pkgerrors.InvalidArgument("conversation_id is required for reconcile")
	}
	if messageID == "" {
		return pkgerrors.InvalidArgument("message_id is required for reconcile")
	}

	if uploadBucket != "" {
		pendingDir, err := s.jailPath(filepath.Join(workspaceSubdir, "conversations", "pending-"+uploadBucket, "attachments"))
		if err == nil {
			if info, statErr := os.Stat(pendingDir); statErr == nil && info.IsDir() {
				convDir, err := s.jailPath(filepath.Join(workspaceSubdir, "conversations", conversationID, "attachments"))
				if err != nil {
					return pkgerrors.Internal("invalid conversation attachment path", err)
				}
				if err := os.MkdirAll(convDir, 0o755); err != nil {
					return pkgerrors.Internal("failed to create conversation attachment dir", err)
				}
				entries, err := os.ReadDir(pendingDir)
				if err != nil {
					return pkgerrors.Internal("failed to read pending attachment dir", err)
				}
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					src := filepath.Join(pendingDir, e.Name())
					dst := filepath.Join(convDir, e.Name())
					if err := os.Rename(src, dst); err != nil {
						s.logger.Error("failed to move attachment file",
							zap.String("src", src), zap.String("dst", dst), zap.Error(err))
					}
				}
				os.Remove(pendingDir)
				os.Remove(filepath.Dir(pendingDir))
			}
		}
	}

	newPrefix := filepath.Join(workspaceSubdir, "conversations", conversationID, "attachments")
	_, err := s.attachmentRepo.Reconcile(ctx, uploadBucket, conversationID, messageID, newPrefix)
	return err
}

func (s *attachmentService) Open(ctx context.Context, id string) (io.ReadCloser, *model.Attachment, error) {
	att, err := s.attachmentRepo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	absPath, err := s.jailPath(att.RelPath)
	if err != nil {
		return nil, nil, pkgerrors.Internal("invalid attachment path", err)
	}
	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, pkgerrors.NotFoundf("attachment file not found: %s", id)
		}
		return nil, nil, pkgerrors.Internal("failed to open attachment", err)
	}
	return f, att, nil
}

func (s *attachmentService) ListByConversation(ctx context.Context, conversationID string) ([]*model.Attachment, error) {
	return s.attachmentRepo.ListByConversation(ctx, conversationID)
}

// jailPath resolves relPath under workspaceRoot and asserts it stays inside.
func (s *attachmentService) jailPath(relPath string) (string, error) {
	root, err := filepath.Abs(s.workspaceRoot)
	if err != nil {
		return "", err
	}
	candidate := filepath.Clean(filepath.Join(root, relPath))
	if !strings.HasPrefix(candidate, root+string(os.PathSeparator)) && candidate != root {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return candidate, nil
}

// sanitizeFilename returns a safe basename, stripping directory components and
// path-traversal sequences.
func sanitizeFilename(name string) string {
	base := filepath.Base(name)
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, "\\", "_")
	if base == "." || base == ".." || base == "" {
		return ""
	}
	return base
}
