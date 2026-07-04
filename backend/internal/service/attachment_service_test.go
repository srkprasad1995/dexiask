package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	mocks "github.com/dexiask/dexiask/test/mocks"
)

// filler is an io.Reader that yields n bytes of 'a' without allocating them all,
// so a test can stream more than the size cap cheaply.
type filler struct{ remaining int64 }

func (r *filler) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = 'a'
	}
	r.remaining -= n
	return int(n), nil
}

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

// TestStore_RejectsOversizeStream verifies the size cap is enforced on the byte
// stream itself — not just the client-declared size — and that a rejected
// upload leaves no file behind. The cap is shrunk so the test streams a handful
// of bytes instead of the real 50 MB.
func TestStore_RejectsOversizeStream(t *testing.T) {
	svc, _, root := newAttachmentSvc(t)
	svc.maxSize = 10

	// Create must never be reached — the write is rejected before the DB insert
	// (the repo mock has no expectations, so any call fails the test).
	_, err := svc.Store(context.Background(), StoreInput{
		ConversationID: "c1",
		Filename:       "big.bin",
		MediaType:      "application/octet-stream",
		Size:           0,                       // client under-declares / omits the size
		Reader:         &filler{remaining: 100}, // 100 bytes > 10-byte cap
	})
	if err == nil {
		t.Fatal("expected Store to reject an over-cap stream")
	}

	// No partial file may remain under the workspace root.
	attDir := filepath.Join(root, ".dexiask", "conversations", "c1", "attachments")
	entries, _ := os.ReadDir(attDir)
	if len(entries) != 0 {
		t.Fatalf("expected no leftover files after rejection, found %d", len(entries))
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
