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

// AuthService orchestrates login (GitHub OAuth or personal-access token), the
// role model (first user is admin; others must be invited), sessions, and the
// admin invite/user surface.
type AuthService interface {
	// LoginURL returns the GitHub authorize URL for the given anti-CSRF state.
	LoginURL(state string) string
	// Complete exchanges the OAuth code, resolves the user (+ role), and creates a
	// session. Returns the new session id and the resolved user.
	Complete(ctx context.Context, code string) (sessionID string, user *model.User, err error)
	// TokenLogin validates a GitHub token (resolving the user via the GitHub API),
	// resolves the role, stores the token encrypted, and creates a session.
	TokenLogin(ctx context.Context, token string) (sessionID string, user *model.User, err error)
	// Logout deletes the session.
	Logout(ctx context.Context, sessionID string) error
	// Me returns the user for an authenticated principal.
	Me(ctx context.Context, userID string) (*model.User, error)

	// --- admin: invites + users ---
	CreateInvite(ctx context.Context, login, createdBy string) (*model.Invite, error)
	ListInvites(ctx context.Context) ([]*model.Invite, error)
	DeleteInvite(ctx context.Context, login string) error
	ListUsers(ctx context.Context) ([]*model.User, error)
}

type authService struct {
	oauth       *auth.OAuth
	github      *auth.GitHubClient
	cipher      *auth.TokenCipher
	userRepo    repository.UserRepository
	sessionRepo repository.SessionRepository
	inviteRepo  repository.InviteRepository
	logger      *logger.Logger
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	oauth *auth.OAuth,
	github *auth.GitHubClient,
	cipher *auth.TokenCipher,
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	inviteRepo repository.InviteRepository,
	log *logger.Logger,
) AuthService {
	return &authService{
		oauth:       oauth,
		github:      github,
		cipher:      cipher,
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		inviteRepo:  inviteRepo,
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
	return s.loginWithToken(ctx, token, false)
}

func (s *authService) TokenLogin(ctx context.Context, token string) (string, *model.User, error) {
	return s.loginWithToken(ctx, token, true)
}

// loginWithToken resolves the user + role for a GitHub token and creates a
// session. tokenLogin distinguishes the two entry points only for the error
// message on an invalid token.
func (s *authService) loginWithToken(ctx context.Context, token string, tokenLogin bool) (string, *model.User, error) {
	ghUser, err := s.github.GetUser(ctx, token)
	if err != nil {
		s.logger.Warn("login: github user lookup failed", zap.Error(err))
		if tokenLogin {
			return "", nil, pkgerrors.InvalidArgument("invalid GitHub token — check the token and its scopes")
		}
		return "", nil, pkgerrors.Unavailable("github user lookup failed", err)
	}

	role, err := s.resolveRole(ctx, ghUser)
	if err != nil {
		return "", nil, err
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
		Role:           role,
		EncryptedToken: encToken,
	})
	if err != nil {
		return "", nil, err
	}
	sess, err := s.sessionRepo.Create(ctx, user.ID, auth.SessionTTL)
	if err != nil {
		return "", nil, err
	}
	s.logger.Info("user logged in", zap.String("user_id", user.ID),
		zap.String("login", user.Login), zap.String("role", user.Role))
	return sess.ID, user, nil
}

// resolveRole returns the role for a signing-in GitHub user: existing users keep
// their role; the first user ever bootstraps as admin; anyone else must hold a
// pending invite (which is consumed) or login is refused.
func (s *authService) resolveRole(ctx context.Context, ghUser *auth.GitHubUser) (string, error) {
	existing, err := s.userRepo.GetByID(ctx, ghUser.IDString())
	if err == nil {
		if existing.Role == "" {
			return model.RoleMember, nil
		}
		return existing.Role, nil
	}
	if !pkgerrors.IsNotFound(err) {
		return "", err
	}
	count, err := s.userRepo.Count(ctx)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return model.RoleAdmin, nil
	}
	if _, ierr := s.inviteRepo.GetByLogin(ctx, ghUser.Login); ierr == nil {
		_ = s.inviteRepo.Delete(ctx, ghUser.Login) // consume the invite
		return model.RoleMember, nil
	}
	return "", pkgerrors.PermissionDenied(
		"your GitHub account has not been invited — ask a workspace admin to invite '" + ghUser.Login + "'")
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

func (s *authService) CreateInvite(ctx context.Context, login, createdBy string) (*model.Invite, error) {
	return s.inviteRepo.Create(ctx, login, createdBy)
}

func (s *authService) ListInvites(ctx context.Context) ([]*model.Invite, error) {
	return s.inviteRepo.List(ctx)
}

func (s *authService) DeleteInvite(ctx context.Context, login string) error {
	return s.inviteRepo.Delete(ctx, login)
}

func (s *authService) ListUsers(ctx context.Context) ([]*model.User, error) {
	return s.userRepo.List(ctx)
}
