package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
	"go.uber.org/zap"
)

// AttachmentHandler serves the file attachment endpoints.
//
//	POST /v1/attachments        — upload a file
//	GET  /v1/attachments/{id}   — serve a file by ID
type AttachmentHandler struct {
	svc    service.AttachmentService
	logger *logger.Logger
}

// NewAttachmentHandler creates a new AttachmentHandler.
func NewAttachmentHandler(svc service.AttachmentService, log *logger.Logger) *AttachmentHandler {
	return &AttachmentHandler{svc: svc, logger: log}
}

const maxUploadBytes = 50 * 1024 * 1024 // 50 MB

// Upload handles POST /v1/attachments.
// Expects multipart/form-data with fields: file (required), conversationId, uploadBucket.
func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "request too large or not multipart", http.StatusBadRequest)
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer f.Close()

	conversationID := r.FormValue("conversationId")
	uploadBucket := r.FormValue("uploadBucket")
	if conversationID == "" && uploadBucket == "" {
		http.Error(w, "conversationId or uploadBucket is required", http.StatusBadRequest)
		return
	}

	att, err := h.svc.Store(r.Context(), service.StoreInput{
		ConversationID: conversationID,
		UploadBucket:   uploadBucket,
		Filename:       header.Filename,
		MediaType:      header.Header.Get("Content-Type"),
		Size:           header.Size,
		Reader:         f,
	})
	if err != nil {
		h.logger.Error("failed to store attachment", zap.Error(err))
		http.Error(w, "failed to store attachment", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":        att.ID,
		"url":       "/v1/attachments/" + att.ID,
		"mediaType": att.MediaType,
		"filename":  att.Filename,
		"size":      att.Size,
	})
}

// Serve handles GET /v1/attachments/{id}.
func (h *AttachmentHandler) Serve(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/attachments/"), "/")
	if id == "" {
		http.Error(w, "attachment id is required", http.StatusBadRequest)
		return
	}
	rc, att, err := h.svc.Open(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to open attachment", zap.Error(err), zap.String("id", id))
		http.Error(w, "attachment not found", http.StatusNotFound)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", att.MediaType)
	w.Header().Set("Content-Disposition", `inline; filename="`+att.Filename+`"`)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if _, err := io.Copy(w, rc); err != nil {
		h.logger.Error("failed to stream attachment", zap.Error(err), zap.String("id", id))
	}
}
