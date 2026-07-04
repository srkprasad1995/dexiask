package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/handler"
	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	svcmocks "github.com/dexiask/dexiask/test/svcmocks"
)

func TestAuthHandler_Me_DevFallback(t *testing.T) {
	// In dev mode svc/signer are nil; /me echoes the dev principal.
	h := handler.NewAuthHandler(nil, nil, "http://web", false, false, "dexiask", logger.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil).
		WithContext(auth.WithUser(context.Background(), auth.Principal{UserID: "dexiask", Login: "dexiask"}))
	rec := httptest.NewRecorder()
	h.Me(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !contains(rec.Body.String(), `"id":"dexiask"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestAuthHandler_Me_Authenticated(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockAuthService(ctrl)
	svc.EXPECT().Me(gomock.Any(), "42").Return(&model.User{ID: "42", Login: "octocat", Name: "The Octocat"}, nil)

	h := handler.NewAuthHandler(svc, auth.NewCookieSigner("s", false), "http://web", true, false, "dexiask", logger.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil).
		WithContext(auth.WithUser(context.Background(), auth.Principal{UserID: "42", Login: "octocat"}))
	rec := httptest.NewRecorder()
	h.Me(rec, req)
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"login":"octocat"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_Login_RedirectsToGitHub(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockAuthService(ctrl)
	svc.EXPECT().LoginURL(gomock.Any()).Return("https://github.com/login/oauth/authorize?state=x")

	h := handler.NewAuthHandler(svc, auth.NewCookieSigner("s", false), "http://web", true, false, "dexiask", logger.NewNop())
	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/login", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !contains(loc, "github.com/login/oauth/authorize") {
		t.Fatalf("redirect = %q", loc)
	}
	// The anti-CSRF state cookie must be set.
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dexiask_oauth_state" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected oauth state cookie to be set")
	}
}

func TestAuthHandler_Callback_StateMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := svcmocks.NewMockAuthService(ctrl) // Complete must NOT be called
	h := handler.NewAuthHandler(svc, auth.NewCookieSigner("s", false), "http://web", true, false, "dexiask", logger.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/callback?state=evil&code=abc", nil)
	req.AddCookie(&http.Cookie{Name: "dexiask_oauth_state", Value: "good"})
	rec := httptest.NewRecorder()
	h.Callback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 on state mismatch", rec.Code)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
