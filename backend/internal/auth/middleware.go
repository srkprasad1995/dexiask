package auth

import (
	"net/http"
	"time"

	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/repository"
	"go.uber.org/zap"
)

// Authenticator resolves the request principal from the session cookie and
// injects it into the request context. In dev-fallback mode (RequireAuth false)
// it injects a single fixed principal so the app works with no login.
type Authenticator struct {
	require     bool
	devUserID   string
	signer      *CookieSigner
	cipher      *TokenCipher
	sessionRepo repository.SessionRepository
	userRepo    repository.UserRepository
	logger      *logger.Logger
}

// NewAuthenticator builds the middleware. When require is false, signer/cipher/
// repos are unused and may be nil.
func NewAuthenticator(
	require bool,
	devUserID string,
	signer *CookieSigner,
	cipher *TokenCipher,
	sessionRepo repository.SessionRepository,
	userRepo repository.UserRepository,
	log *logger.Logger,
) *Authenticator {
	return &Authenticator{
		require:     require,
		devUserID:   devUserID,
		signer:      signer,
		cipher:      cipher,
		sessionRepo: sessionRepo,
		userRepo:    userRepo,
		logger:      log,
	}
}

// Middleware wraps next, requiring a valid session in auth mode and injecting the
// dev principal otherwise.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.require {
			// Dev-fallback: the single local user is an admin so it can do everything.
			r = r.WithContext(WithUser(r.Context(), Principal{UserID: a.devUserID, Login: a.devUserID, Role: "admin"}))
			next.ServeHTTP(w, r)
			return
		}
		p, ok := a.resolve(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), p)))
	})
}

// resolve validates the cookie and loads the session + user.
func (a *Authenticator) resolve(r *http.Request) (Principal, bool) {
	sessionID, err := a.signer.Read(r)
	if err != nil {
		return Principal{}, false
	}
	sess, err := a.sessionRepo.GetByID(r.Context(), sessionID)
	if err != nil {
		return Principal{}, false
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = a.sessionRepo.Delete(r.Context(), sessionID)
		return Principal{}, false
	}
	user, err := a.userRepo.GetByID(r.Context(), sess.UserID)
	if err != nil {
		return Principal{}, false
	}
	token, err := a.cipher.Decrypt(user.EncryptedToken)
	if err != nil {
		a.logger.Error("failed to decrypt user token", zap.Error(err), zap.String("user_id", user.ID))
		return Principal{}, false
	}
	return Principal{UserID: user.ID, Login: user.Login, Role: user.Role, GitHubToken: token}, true
}
