package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/dexiask/dexiask/internal/auth"
	"github.com/dexiask/dexiask/internal/model"
	"github.com/dexiask/dexiask/internal/pkg/logger"
	"github.com/dexiask/dexiask/internal/service"
)

const oauthStateCookie = "dexiask_oauth_state"

// AuthHandler serves the GitHub OAuth login endpoints.
//
//	GET  /v1/auth/login    — redirect to GitHub authorize
//	GET  /v1/auth/callback — exchange code, set session cookie, redirect to web
//	POST /v1/auth/logout   — clear session
//	GET  /v1/auth/me       — current user (public fields)
type AuthHandler struct {
	svc          service.AuthService
	signer       *auth.CookieSigner
	webBaseURL   string
	require      bool
	oauthEnabled bool
	devUserID    string
	secure       bool
	logger       *logger.Logger
}

// meResponse is the public shape of GET /v1/auth/me.
type meResponse struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
}

// NewAuthHandler creates a new AuthHandler. In dev-fallback mode (require false)
// svc/signer may be nil: /me returns the dev user and /login/callback are inert.
func NewAuthHandler(svc service.AuthService, signer *auth.CookieSigner, webBaseURL string, require, oauthEnabled, secure bool, devUserID string, log *logger.Logger) *AuthHandler {
	return &AuthHandler{
		svc:          svc,
		signer:       signer,
		webBaseURL:   webBaseURL,
		require:      require,
		oauthEnabled: oauthEnabled,
		devUserID:    devUserID,
		secure:       secure,
		logger:       log,
	}
}

// Config handles GET /v1/auth/config — an unauthenticated capability probe so the
// login page knows which methods to offer.
func (h *AuthHandler) Config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"authEnabled": h.require,
		"oauth":       h.oauthEnabled,
		"tokenLogin":  h.require, // token-login is available whenever auth is on
	})
}

// TokenLogin handles POST /v1/auth/token-login — sign in with a GitHub PAT.
func (h *AuthHandler) TokenLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.require {
		writeError(w, http.StatusBadRequest, "auth is not enabled on this deployment")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	sessionID, user, err := h.svc.TokenLogin(r.Context(), token)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	h.signer.Set(w, sessionID)
	h.logger.Info("token login complete", zap.String("user_id", user.ID))
	writeJSON(w, http.StatusOK, meResponse{
		ID:        user.ID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
		Role:      user.Role,
	})
}

// Login handles GET /v1/auth/login (GitHub OAuth authorize redirect).
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.oauthEnabled {
		// No OAuth app configured (token-login only, or dev-fallback) — bounce back.
		http.Redirect(w, r, h.webBaseURL, http.StatusFound)
		return
	}
	state, err := randomState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start login")
		return
	}
	// Path "/" so the browser sends it back on the callback even when the
	// callback is served under a different path prefix by the web BFF.
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((10 * time.Minute).Seconds()),
	})
	http.Redirect(w, r, h.svc.LoginURL(state), http.StatusFound)
}

// Callback handles GET /v1/auth/callback.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if !h.oauthEnabled {
		http.Redirect(w, r, h.webBaseURL, http.StatusFound)
		return
	}
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value == "" || r.URL.Query().Get("state") != stateCookie.Value {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}
	// Clear the state cookie.
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Path: "/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code")
		return
	}
	sessionID, user, err := h.svc.Complete(r.Context(), code)
	if err != nil {
		h.logger.Error("oauth callback failed", zap.Error(err))
		writeServiceError(w, err)
		return
	}
	h.signer.Set(w, sessionID)
	h.logger.Info("login complete", zap.String("user_id", user.ID))
	http.Redirect(w, r, h.webBaseURL, http.StatusFound)
}

// Logout handles POST /v1/auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.require && h.signer != nil {
		if id, err := h.signer.Read(r); err == nil {
			_ = h.svc.Logout(r.Context(), id)
		}
		h.signer.Clear(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /v1/auth/me. It reads the principal the auth middleware injected.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !h.require {
		// Dev-fallback user is an admin.
		writeJSON(w, http.StatusOK, meResponse{ID: p.UserID, Login: p.Login, Name: "Local Dev", Role: "admin"})
		return
	}
	user, err := h.svc.Me(r.Context(), p.UserID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meResponse{
		ID:        user.ID,
		Login:     user.Login,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
		Role:      user.Role,
	})
}

// --- admin: invites + users -------------------------------------------------

// Invites handles GET (list) and POST (create) on /v1/invites (admin only).
func (h *AuthHandler) Invites(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		invites, err := h.svc.ListInvites(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if invites == nil {
			invites = []*model.Invite{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"invites": invites})
	case http.MethodPost:
		var body struct {
			Login string `json:"login"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		inv, err := h.svc.CreateInvite(r.Context(), body.Login, p.UserID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, inv)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// DeleteInvite handles DELETE /v1/invites/{login} (admin only).
func (h *AuthHandler) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	login := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/invites/"), "/")
	if login == "" {
		writeError(w, http.StatusBadRequest, "login is required")
		return
	}
	if err := h.svc.DeleteInvite(r.Context(), login); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Users handles GET /v1/users (admin only) — the workspace member roster.
func (h *AuthHandler) Users(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAdmin(w, r); !ok {
		return
	}
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if users == nil {
		users = []*model.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
