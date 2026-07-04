package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	mocks "github.com/dexiask/dexiask/test/mocks"
	"go.uber.org/mock/gomock"
)

// probe records the principal seen by the wrapped handler.
func probe(seen *auth.Principal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := auth.UserFromContext(r.Context()); ok {
			*seen = p
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddleware_DevFallback_InjectsDevPrincipal(t *testing.T) {
	a := auth.NewAuthenticator(false, "dexiask", nil, nil, nil, nil, logger.NewNop())
	var seen auth.Principal
	rec := httptest.NewRecorder()
	a.Middleware(probe(&seen)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if seen.UserID != "dexiask" {
		t.Fatalf("dev principal = %q, want dexiask", seen.UserID)
	}
}

func TestMiddleware_RequireAuth_NoCookie_401(t *testing.T) {
	ctrl := gomock.NewController(t)
	signer := auth.NewCookieSigner("s3cret", false)
	a := auth.NewAuthenticator(true, "dexiask", signer, nil,
		mocks.NewMockSessionRepository(ctrl), mocks.NewMockUserRepository(ctrl), logger.NewNop())

	rec := httptest.NewRecorder()
	a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be reached without a session")
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_RequireAuth_ValidSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	signer := auth.NewCookieSigner("s3cret", false)
	cipher, err := auth.NewTokenCipher("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	encTok, _ := cipher.Encrypt("gho_tok")

	sessRepo := mocks.NewMockSessionRepository(ctrl)
	userRepo := mocks.NewMockUserRepository(ctrl)
	sessRepo.EXPECT().GetByID(gomock.Any(), "sess-1").
		Return(&model.Session{ID: "sess-1", UserID: "42", ExpiresAt: time.Now().Add(time.Hour)}, nil)
	userRepo.EXPECT().GetByID(gomock.Any(), "42").
		Return(&model.User{ID: "42", Login: "octocat", EncryptedToken: encTok}, nil)

	a := auth.NewAuthenticator(true, "dexiask", signer, cipher, sessRepo, userRepo, logger.NewNop())

	// Build a request carrying a validly-signed session cookie.
	setRec := httptest.NewRecorder()
	signer.Set(setRec, "sess-1")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range setRec.Result().Cookies() {
		req.AddCookie(c)
	}

	var seen auth.Principal
	rec := httptest.NewRecorder()
	a.Middleware(probe(&seen)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if seen.UserID != "42" || seen.GitHubToken != "gho_tok" {
		t.Fatalf("principal = %+v, want id=42 token=gho_tok (decrypted)", seen)
	}
}
