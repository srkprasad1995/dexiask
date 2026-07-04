package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/model"
	pkgerrors "github.com/dexiask/dexiask/internal/pkg/errors"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/repository"
)

//go:generate go run go.uber.org/mock/mockgen -source=auth_service.go -destination=../../test/svcmocks/auth_service_mock.go -package=svcmocks

// AuthService orchestrates the GitHub OAuth login flow: authorize-URL building,
// code exchange, user upsert (with encrypted token), and session lifecycle.
type AuthService interface {
	// LoginURL returns the GitHub authorize URL for the given anti-CSRF state.
	LoginURL(state string) string
	// Complete exchanges the OAuth code, upserts the user, and creates a session.
	// It returns the new session id and the resolved user.
	Complete(ctx context.Context, code string) (sessionID string, user *model.User, err error)
	// Logout deletes the session.
	Logout(ctx context.Context, sessionID string) error
	// Me returns the user for an authenticated principal.
	Me(ctx context.Context, userID string) (*model.User, error)
}

type authService struct {
	oauth       *auth.OAuth
	github      *auth.GitHubClient
	cipher      *auth.TokenCipher
	userRepo    repository.UserRepository
	sessionRepo repository.SessionRepository
	logger      *logger.Logger
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	oauth *auth.OAuth,
	github *auth.GitHubClient,
	cipher *auth.TokenCipher,
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	log *logger.Logger,
) AuthService {
	return &authService{
		oauth:       oauth,
		github:      github,
		cipher:      cipher,
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		logger:      log,
	}
}

func (s *authService) LoginURL(state string) string {
	return s.oauth.AuthCodeURL(state)
}

func (s *authService) Complete(ctx context.Context, code string) (string, *model.User, error) {
	token, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		s.logger.Error("oauth exchange failed", zap.Error(err))
		return "", nil, pkgerrors.Unavailable("github oauth exchange failed", err)
	}
	ghUser, err := s.github.GetUser(ctx, token)
	if err != nil {
		s.logger.Error("github get user failed", zap.Error(err))
		return "", nil, pkgerrors.Unavailable("github user lookup failed", err)
	}
	encToken, err := s.cipher.Encrypt(token)
	if err != nil {
		return "", nil, pkgerrors.Internal("failed to encrypt token", err)
	}
	user, err := s.userRepo.Upsert(ctx, &model.UpsertUserInput{
		ID:             ghUser.IDString(),
		Login:          ghUser.Login,
		Name:           ghUser.Name,
		AvatarURL:      ghUser.AvatarURL,
		EncryptedToken: encToken,
	})
	if err != nil {
		return "", nil, err
	}
	sess, err := s.sessionRepo.Create(ctx, user.ID, auth.SessionTTL)
	if err != nil {
		return "", nil, err
	}
	s.logger.Info("user logged in", zap.String("user_id", user.ID), zap.String("login", user.Login))
	return sess.ID, user, nil
}

func (s *authService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.sessionRepo.Delete(ctx, sessionID)
}

func (s *authService) Me(ctx context.Context, userID string) (*model.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}
