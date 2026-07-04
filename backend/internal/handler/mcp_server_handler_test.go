package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/handler"
	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	mocks "github.com/dexiask/dexiask/test/mocks"
)

const testUserID = "u1"

// withPrincipal returns req carrying an authenticated principal, as the auth
// middleware would inject.
func withPrincipal(req *http.Request) *http.Request {
	return req.WithContext(auth.WithUser(req.Context(), auth.Principal{UserID: testUserID, Login: "octocat"}))
}

func newMCPHandler(t *testing.T) (*handler.MCPServerHandler, *mocks.MockMCPServerRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := mocks.NewMockMCPServerRepository(ctrl)
	return handler.NewMCPServerHandler(repo, logger.NewNop()), repo
}

func TestMCPServerHandler_List(t *testing.T) {
	h, repo := newMCPHandler(t)
	repo.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, f *model.ListMCPServersFilter) ([]*model.MCPServer, error) {
			if f.UserID != testUserID {
				t.Fatalf("list must be scoped to user, got %q", f.UserID)
			}
			return []*model.MCPServer{{ID: "m1", Name: "github", Type: "http", URL: "http://gh/mcp", Enabled: true}}, nil
		})

	req := withPrincipal(httptest.NewRequest(http.MethodGet, "/v1/mcp-servers", nil))
	rec := httptest.NewRecorder()
	h.ServeCollection(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		MCPServers []*model.MCPServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.MCPServers) != 1 || body.MCPServers[0].Name != "github" {
		t.Fatalf("unexpected list body: %+v", body.MCPServers)
	}
}

func TestMCPServerHandler_List_Unauthenticated(t *testing.T) {
	h, _ := newMCPHandler(t)
	// No principal on the context.
	req := httptest.NewRequest(http.MethodGet, "/v1/mcp-servers", nil)
	rec := httptest.NewRecorder()
	h.ServeCollection(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without principal, got %d", rec.Code)
	}
}

func TestMCPServerHandler_Create_HappyPath(t *testing.T) {
	h, repo := newMCPHandler(t)
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.CreateMCPServerInput) (*model.MCPServer, error) {
			if in.WorkspaceID == "" {
				t.Fatalf("workspace id must be stamped, got empty")
			}
			if in.UserID != testUserID {
				t.Fatalf("user id must be stamped from principal, got %q", in.UserID)
			}
			return &model.MCPServer{ID: "m1", Name: in.Name, Type: in.Type, URL: in.URL, Enabled: in.Enabled}, nil
		})

	req := withPrincipal(httptest.NewRequest(http.MethodPost, "/v1/mcp-servers",
		strings.NewReader(`{"name":"github","type":"http","url":"http://gh/mcp"}`)))
	rec := httptest.NewRecorder()
	h.ServeCollection(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestMCPServerHandler_Create_BadType(t *testing.T) {
	h, repo := newMCPHandler(t)
	// Repository returns an invalid-argument error for a bad type.
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, pkgerrors.InvalidArgument("type must be one of: http, sse"))

	req := withPrincipal(httptest.NewRequest(http.MethodPost, "/v1/mcp-servers",
		strings.NewReader(`{"name":"github","type":"ftp","url":"http://gh/mcp"}`)))
	rec := httptest.NewRecorder()
	h.ServeCollection(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad type, got %d", rec.Code)
	}
}

func TestMCPServerHandler_Update_EnabledToggle(t *testing.T) {
	h, repo := newMCPHandler(t)
	// Ownership check loads the server first.
	repo.EXPECT().GetByID(gomock.Any(), "m1").Return(&model.MCPServer{ID: "m1", UserID: testUserID}, nil)
	repo.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.UpdateMCPServerInput) (*model.MCPServer, error) {
			if in.ID != "m1" || in.Enabled == nil || *in.Enabled {
				t.Fatalf("unexpected update input: %+v", in)
			}
			return &model.MCPServer{ID: "m1", Name: "github", Type: "http", URL: "http://gh/mcp", Enabled: false}, nil
		})

	req := withPrincipal(httptest.NewRequest(http.MethodPut, "/v1/mcp-servers/m1", strings.NewReader(`{"enabled":false}`)))
	rec := httptest.NewRecorder()
	h.ServeItem(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestMCPServerHandler_Update_NotOwner(t *testing.T) {
	h, repo := newMCPHandler(t)
	// Server belongs to a different user — must 404, never call Update.
	repo.EXPECT().GetByID(gomock.Any(), "m1").Return(&model.MCPServer{ID: "m1", UserID: "someone-else"}, nil)

	req := withPrincipal(httptest.NewRequest(http.MethodPut, "/v1/mcp-servers/m1", strings.NewReader(`{"enabled":false}`)))
	rec := httptest.NewRecorder()
	h.ServeItem(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owner, got %d", rec.Code)
	}
}

func TestMCPServerHandler_Delete(t *testing.T) {
	h, repo := newMCPHandler(t)
	repo.EXPECT().GetByID(gomock.Any(), "m1").Return(&model.MCPServer{ID: "m1", UserID: testUserID}, nil)
	repo.EXPECT().Delete(gomock.Any(), "m1").Return(nil)

	req := withPrincipal(httptest.NewRequest(http.MethodDelete, "/v1/mcp-servers/m1", nil))
	rec := httptest.NewRecorder()
	h.ServeItem(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestMCPServerHandler_Item_MissingID(t *testing.T) {
	h, _ := newMCPHandler(t)
	req := withPrincipal(httptest.NewRequest(http.MethodDelete, "/v1/mcp-servers/", nil))
	rec := httptest.NewRecorder()
	h.ServeItem(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing id, got %d", rec.Code)
	}
}
