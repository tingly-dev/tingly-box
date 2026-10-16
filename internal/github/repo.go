package github

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

var scpRemote = regexp.MustCompile(`^[\w.-]+@([\w.-]+):(.+)$`)

// ParseRemote extracts owner and repo from a git remote URL on github.com
// (https, ssh:// and scp-style forms).
func ParseRemote(raw string) (owner, repo string, ok bool) {
	raw = strings.TrimSpace(raw)
	var host, path string
	if m := scpRemote.FindStringSubmatch(raw); m != nil && !strings.Contains(raw, "://") {
		host, path = m[1], m[2]
	} else if u, err := url.Parse(raw); err == nil && u.Host != "" {
		host, path = u.Hostname(), strings.TrimPrefix(u.Path, "/")
	} else {
		return "", "", false
	}
	if !strings.EqualFold(host, "github.com") && !strings.EqualFold(host, "www.github.com") {
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// RepoOf resolves dir's `origin` remote to "owner/repo".
func RepoOf(ctx context.Context, dir string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("%s has no git remote named origin", dir)
	}
	owner, repo, ok := ParseRemote(string(out))
	if !ok {
		return "", fmt.Errorf("origin of %s (%s) is not a github.com repository", dir, strings.TrimSpace(string(out)))
	}
	return owner + "/" + repo, nil
}
