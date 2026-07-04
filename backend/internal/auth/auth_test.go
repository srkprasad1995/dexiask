package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenCipher_RoundTrip(t *testing.T) {
	// 32 hex chars == 16 bytes == AES-128.
	c, err := NewTokenCipher("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	const plain = "gho_secrettoken"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plain || enc == "" {
		t.Fatalf("ciphertext looks wrong: %q", enc)
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round trip = %q, want %q", got, plain)
	}
}

func TestTokenCipher_BadKey(t *testing.T) {
	if _, err := NewTokenCipher("nothex"); err == nil {
		t.Fatal("expected error for non-hex key")
	}
	if _, err := NewTokenCipher("00112233"); err == nil {
		t.Fatal("expected error for wrong-length key")
	}
}

func TestCookieSigner_SetReadRoundTrip(t *testing.T) {
	s := NewCookieSigner("topsecret", false)
	rec := httptest.NewRecorder()
	s.Set(rec, "sess-123")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	got, err := s.Read(req)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "sess-123" {
		t.Fatalf("read = %q, want sess-123", got)
	}
}

func TestCookieSigner_RejectsTamper(t *testing.T) {
	s := NewCookieSigner("topsecret", false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "sess-123.badsig"})
	if _, err := s.Read(req); err == nil {
		t.Fatal("expected invalid signature error")
	}
	// A different secret must not verify a cookie signed by the first.
	other := NewCookieSigner("different", false)
	rec := httptest.NewRecorder()
	s.Set(rec, "sess-123")
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req2.AddCookie(c)
	}
	if _, err := other.Read(req2); err == nil {
		t.Fatal("expected cross-secret verification to fail")
	}
}

func TestGitHubClient_GetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"login":"octocat","name":"The Octocat","avatar_url":"http://a/x.png"}`))
	}))
	defer srv.Close()

	c := NewGitHubClientWithBase(srv.URL, time.Minute)
	u, err := c.GetUser(context.Background(), "tok")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.IDString() != "42" || u.Login != "octocat" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestGitHubClient_HasRepoAccess(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/repos/octocat/allowed":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewGitHubClientWithBase(srv.URL, time.Minute)
	ok, err := c.HasRepoAccess(context.Background(), "tok", "octocat", "allowed")
	if err != nil || !ok {
		t.Fatalf("expected access, got ok=%v err=%v", ok, err)
	}
	denied, err := c.HasRepoAccess(context.Background(), "tok", "octocat", "secret")
	if err != nil || denied {
		t.Fatalf("expected denied, got ok=%v err=%v", denied, err)
	}
	// A repeated call is served from cache (no extra HTTP hit).
	before := calls
	_, _ = c.HasRepoAccess(context.Background(), "tok", "octocat", "allowed")
	if calls != before {
		t.Fatalf("expected cache hit, but made %d extra calls", calls-before)
	}
}
