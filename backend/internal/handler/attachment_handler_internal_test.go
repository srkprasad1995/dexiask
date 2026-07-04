package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/pkg/logger"
	svcmocks "github.com/dexiask/dexiask/test/svcmocks"
)

// TestUpload_RejectsOversizeBody verifies the request body is hard-capped: a
// body larger than maxBody is refused with 413 and the service is never called.
// maxBody is shrunk so the test sends a few hundred bytes instead of 50 MB.
func TestUpload_RejectsOversizeBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockAttachmentService(ctrl) // no calls expected → any call fails the test
	h := NewAttachmentHandler(svc, logger.NewNop())
	h.maxBody = 64

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "big.bin")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(bytes.Repeat([]byte("a"), 256)); err != nil {
		t.Fatalf("write body: %v", err)
	}
	_ = mw.WriteField("conversationId", "c1")
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/attachments", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	h.Upload(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}
