package handler

import "testing"

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		url         string
		owner, repo string
		ok          bool
	}{
		{"https://github.com/octocat/hello.git", "octocat", "hello", true},
		{"https://github.com/octocat/hello", "octocat", "hello", true},
		{"git@github.com:octocat/hello.git", "octocat", "hello", true},
		{"https://gitlab.com/octocat/hello.git", "", "", false},
		{"not a url", "", "", false},
	}
	for _, c := range cases {
		owner, repo, ok := parseGitHubURL(c.url)
		if ok != c.ok || owner != c.owner || repo != c.repo {
			t.Errorf("parseGitHubURL(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.url, owner, repo, ok, c.owner, c.repo, c.ok)
		}
	}
}
