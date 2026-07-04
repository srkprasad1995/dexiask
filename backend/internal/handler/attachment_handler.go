package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
	"go.uber.org/zap"
)

const (
	// maxUploadBytes is the attachment file-size cap.
	maxUploadBytes = 50 * 1024 * 1024 // 50 MB
	// maxRequestBytes caps the whole multipart request a little above the file
	// cap — headroom for multipart framing and the small form fields — so a
	// legitimate max-size file still fits while an oversized upload is refused
	// before it can spill large temporary files to disk during parsing.
	maxRequestBytes = maxUploadBytes + 1*1024*1024
)

// AttachmentHandler serves the file attachment endpoints.
//
//	POST /v1/attachments        — upload a file
//	GET  /v1/attachments/{id}   — serve a file by ID
type AttachmentHandler struct {
	svc     service.AttachmentService
	logger  *logger.Logger
	maxBody int64
}

// NewAttachmentHandler creates a new AttachmentHandler.
func NewAttachmentHandler(svc service.AttachmentService, log *logger.Logger) *AttachmentHandler {
	return &AttachmentHandler{svc: svc, logger: log, maxBody: maxRequestBytes}
}

// Upload handles POST /v1/attachments.
// Expects multipart/form-data with fields: file (required), conversationId, uploadBucket.
func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// Hard-cap the request body so an oversized upload is rejected up front
	// rather than buffered/spilled to disk while parsing the multipart form.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBody)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "upload exceeds max size", http.StatusRequestEntityTooLarge)
			return
		}
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
