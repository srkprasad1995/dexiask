package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	mocks "github.com/dexiask/dexiask/test/mocks"
)

func newAttachmentSvc(t *testing.T) (*attachmentService, *mocks.MockAttachmentRepository, string) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockAttachmentRepository(ctrl)
	root := t.TempDir()
	svc := NewAttachmentService(root, repo, logger.NewNop()).(*attachmentService)
	return svc, repo, root
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"report.pdf":       "report.pdf",
		"../../etc/passwd": "passwd",
		"a/b/c.txt":        "c.txt",
		`a\b.txt`:          "a_b.txt", // backslashes are replaced with underscores
		"..":               "",
		".":                "",
		"":                 "",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJailPath(t *testing.T) {
	svc, _, root := newAttachmentSvc(t)

	// Valid path stays under root.
	good, err := svc.jailPath(".dexiask/conversations/c1/attachments/f.txt")
	if err != nil {
		t.Fatalf("jailPath valid: %v", err)
	}
	if !strings.HasPrefix(good, root) {
		t.Errorf("jailed path %q not under root %q", good, root)
	}

	// Traversal escaping the root must be rejected.
	if _, err := svc.jailPath("../../../etc/passwd"); err == nil {
		t.Error("jailPath allowed traversal escape")
	}
}

// TestStore_WritesUnderJail verifies Store writes bytes under the workspace root
// at the returned rel_path and never escapes the jail.
func TestStore_WritesUnderJail(t *testing.T) {
	svc, repo, root := newAttachmentSvc(t)

	repo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *model.StoreAttachmentInput) (*model.Attachment, error) {
			return &model.Attachment{
				ID:             in.ID,
				ConversationID: in.ConversationID,
				Filename:       in.Filename,
				MediaType:      in.MediaType,
				Size:           in.Size,
				RelPath:        in.RelPath,
			}, nil
		})

	att, err := svc.Store(context.Background(), StoreInput{
		ConversationID: "c1",
		Filename:       "../../evil.txt", // traversal in the name must be stripped
		MediaType:      "text/plain",
		Size:           5,
		Reader:         strings.NewReader("hello"),
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// rel_path must be namespaced under .dexiask/conversations/c1 and carry a safe basename.
	if !strings.HasPrefix(att.RelPath, filepath.Join(".dexiask", "conversations", "c1", "attachments")) {
		t.Fatalf("rel_path not jailed: %q", att.RelPath)
	}
	if strings.Contains(att.RelPath, "..") {
		t.Fatalf("rel_path contains traversal: %q", att.RelPath)
	}
	if !strings.HasSuffix(att.RelPath, "-evil.txt") {
		t.Fatalf("basename not sanitized: %q", att.RelPath)
	}

	// The bytes must exist on disk under the root.
	data, err := os.ReadFile(filepath.Join(root, att.RelPath))
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("stored bytes = %q", data)
	}
}

// TestReconcile_RequiresIDs guards the reconcile preconditions.
func TestReconcile_RequiresIDs(t *testing.T) {
	svc, _, _ := newAttachmentSvc(t)
	if err := svc.Reconcile(context.Background(), "bucket", "", "msg"); err == nil {
		t.Error("expected error when conversation_id is empty")
	}
	if err := svc.Reconcile(context.Background(), "bucket", "c1", ""); err == nil {
		t.Error("expected error when message_id is empty")
	}
}
