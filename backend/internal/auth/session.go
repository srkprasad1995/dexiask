package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"
)

// SessionCookieName is the cookie holding the signed session id.
const SessionCookieName = "dexiask_session"

// SessionTTL is how long a login session (and its cookie) stays valid.
const SessionTTL = 30 * 24 * time.Hour

// CookieSigner signs and verifies the opaque session id carried in the cookie,
// so a tampered id is rejected before any DB lookup.
type CookieSigner struct {
	secret []byte
	secure bool
}

// NewCookieSigner builds a signer. secure marks the cookie Secure (set false for
// plain-HTTP local dev).
func NewCookieSigner(secret string, secure bool) *CookieSigner {
	return &CookieSigner{secret: []byte(secret), secure: secure}
}

func (s *CookieSigner) sign(value string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Set writes the signed session cookie.
func (s *CookieSigner) Set(w http.ResponseWriter, sessionID string) {
	value := sessionID + "." + s.sign(sessionID)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(SessionTTL),
		MaxAge:   int(SessionTTL.Seconds()),
	})
}

// Clear expires the session cookie (logout).
func (s *CookieSigner) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// Read extracts and verifies the session id from the request cookie.
func (s *CookieSigner) Read(r *http.Request) (string, error) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return "", err
	}
	id, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return "", errors.New("malformed session cookie")
	}
	expected := s.sign(id)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) != 1 {
		return "", errors.New("invalid session signature")
	}
	return id, nil
}
