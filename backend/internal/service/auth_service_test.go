package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
	mocks "github.com/dexiask/dexiask/test/mocks"
)

// githubStub serves a fixed /user response for a valid token.
func githubStub(t *testing.T, login string, id int64) *auth.GitHubClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":` + itoa(id) + `,"login":"` + login + `","name":"X"}`))
	}))
	t.Cleanup(srv.Close)
	return auth.NewGitHubClientWithBase(srv.URL, 0)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func newAuthSvc(t *testing.T) (service.AuthService, *mocks.MockUserRepository, *mocks.MockSessionRepository, *mocks.MockInviteRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	ur := mocks.NewMockUserRepository(ctrl)
	sr := mocks.NewMockSessionRepository(ctrl)
	ir := mocks.NewMockInviteRepository(ctrl)
	cipher, err := auth.NewTokenCipher("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewAuthService(nil, githubStub(t, "octocat", 42), cipher, ur, sr, ir, logger.NewNop())
	return svc, ur, sr, ir
}

func TestTokenLogin_FirstUserBecomesAdmin(t *testing.T) {
	svc, ur, sr, _ := newAuthSvc(t)
	ur.EXPECT().GetByID(gomock.Any(), "42").Return(nil, pkgerrors.NotFoundf("nope"))
	ur.EXPECT().Count(gomock.Any()).Return(int64(0), nil)
	ur.EXPECT().Upsert(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.UpsertUserInput) (*model.User, error) {
			if in.Role != model.RoleAdmin {
				t.Fatalf("first user role = %q, want admin", in.Role)
			}
			return &model.User{ID: in.ID, Login: in.Login, Role: in.Role}, nil
		})
	sr.EXPECT().Create(gomock.Any(), "42", gomock.Any()).Return(&model.Session{ID: "s"}, nil)

	_, user, err := svc.TokenLogin(context.Background(), "ghp_x")
	if err != nil || user.Role != model.RoleAdmin {
		t.Fatalf("user=%+v err=%v", user, err)
	}
}

func TestTokenLogin_InvitedBecomesMember(t *testing.T) {
	svc, ur, sr, ir := newAuthSvc(t)
	ur.EXPECT().GetByID(gomock.Any(), "42").Return(nil, pkgerrors.NotFoundf("nope"))
	ur.EXPECT().Count(gomock.Any()).Return(int64(1), nil)
	ir.EXPECT().GetByLogin(gomock.Any(), "octocat").Return(&model.Invite{Login: "octocat"}, nil)
	ir.EXPECT().Delete(gomock.Any(), "octocat").Return(nil)
	ur.EXPECT().Upsert(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.UpsertUserInput) (*model.User, error) {
			if in.Role != model.RoleMember {
				t.Fatalf("invited user role = %q, want member", in.Role)
			}
			return &model.User{ID: in.ID, Login: in.Login, Role: in.Role}, nil
		})
	sr.EXPECT().Create(gomock.Any(), "42", gomock.Any()).Return(&model.Session{ID: "s"}, nil)

	if _, _, err := svc.TokenLogin(context.Background(), "ghp_x"); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestTokenLogin_UninvitedRefused(t *testing.T) {
	svc, ur, _, ir := newAuthSvc(t)
	ur.EXPECT().GetByID(gomock.Any(), "42").Return(nil, pkgerrors.NotFoundf("nope"))
	ur.EXPECT().Count(gomock.Any()).Return(int64(1), nil)
	ir.EXPECT().GetByLogin(gomock.Any(), "octocat").Return(nil, pkgerrors.NotFoundf("no invite"))

	_, _, err := svc.TokenLogin(context.Background(), "ghp_x")
	if err == nil || pkgerrors.HTTPStatus(err) != http.StatusForbidden {
		t.Fatalf("expected 403 forbidden, got %v (status %d)", err, pkgerrors.HTTPStatus(err))
	}
}

func TestTokenLogin_ExistingUserKeepsRole(t *testing.T) {
	svc, ur, sr, _ := newAuthSvc(t)
	ur.EXPECT().GetByID(gomock.Any(), "42").Return(&model.User{ID: "42", Login: "octocat", Role: model.RoleMember}, nil)
	ur.EXPECT().Upsert(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *model.UpsertUserInput) (*model.User, error) {
			if in.Role != model.RoleMember {
				t.Fatalf("existing user role = %q, want member (kept)", in.Role)
			}
			return &model.User{ID: in.ID, Role: in.Role}, nil
		})
	sr.EXPECT().Create(gomock.Any(), "42", gomock.Any()).Return(&model.Session{ID: "s"}, nil)

	if _, _, err := svc.TokenLogin(context.Background(), "ghp_x"); err != nil {
		t.Fatalf("err=%v", err)
	}
}
