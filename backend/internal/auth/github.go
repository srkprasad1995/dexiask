package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultGitHubAPIBase is the public GitHub REST API root.
const defaultGitHubAPIBase = "https://api.github.com"

// GitHubUser is the subset of the GitHub /user response we persist.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// IDString returns the numeric id as a string (our user primary key).
func (u GitHubUser) IDString() string { return strconv.FormatInt(u.ID, 10) }

// GitHubClient calls the GitHub REST API on behalf of a user's OAuth token.
type GitHubClient struct {
	http      *http.Client
	base      string
	accessTTL time.Duration

	mu    sync.Mutex
	cache map[string]repoAccessEntry // key: token+owner/repo
}

type repoAccessEntry struct {
	allow   bool
	expires time.Time
}

// NewGitHubClient builds a GitHubClient with a repo-access cache TTL (~5 min) to
// blunt rate limits.
func NewGitHubClient(accessTTL time.Duration) *GitHubClient {
	return NewGitHubClientWithBase(defaultGitHubAPIBase, accessTTL)
}

// NewGitHubClientWithBase is NewGitHubClient with a custom API base (GitHub
// Enterprise, or a test server).
func NewGitHubClientWithBase(base string, accessTTL time.Duration) *GitHubClient {
	return &GitHubClient{
		http:      &http.Client{Timeout: 10 * time.Second},
		base:      strings.TrimRight(base, "/"),
		accessTTL: accessTTL,
		cache:     make(map[string]repoAccessEntry),
	}
}

// GetUser fetches the authenticated user for the given OAuth token.
func (c *GitHubClient) GetUser(ctx context.Context, token string) (*GitHubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/user", nil)
	if err != nil {
		return nil, err
	}
	c.authHeaders(req, token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github /user returned %d", resp.StatusCode)
	}
	var u GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("github /user returned no id")
	}
	return &u, nil
}

// HasRepoAccess reports whether the token can see owner/repo (GET /repos → 200).
// Results are cached per (token, owner/repo) for accessTTL.
func (c *GitHubClient) HasRepoAccess(ctx context.Context, token, owner, repo string) (bool, error) {
	key := token + "\x00" + owner + "/" + repo
	if v, ok := c.cachedAccess(key); ok {
		return v, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/repos/%s/%s", c.base, owner, repo), nil)
	if err != nil {
		return false, err
	}
	c.authHeaders(req, token)
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		c.storeAccess(key, true)
		return true, nil
	case http.StatusNotFound, http.StatusForbidden:
		c.storeAccess(key, false)
		return false, nil
	default:
		return false, fmt.Errorf("github /repos returned %d", resp.StatusCode)
	}
}

func (c *GitHubClient) cachedAccess(key string) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expires) {
		return false, false
	}
	return e.allow, true
}

func (c *GitHubClient) storeAccess(key string, allow bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = repoAccessEntry{allow: allow, expires: time.Now().Add(c.accessTTL)}
}

func (c *GitHubClient) authHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}
