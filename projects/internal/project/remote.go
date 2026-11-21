package project

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A thread's repository on GitHub is known two ways, kept apart (see
// .design/github-tools.md, "Which remote"): its identity, owner/name read from
// origin in whatever form origin is written, and the way git reaches it for
// the tools, always https://github.com/owner/name.git with a token the tools
// hold. Origin itself is never rewritten.

// errNoOrigin is a checkout without an origin remote.
var errNoOrigin = errors.New("this thread's repository has no origin remote")

// remoteParts is a git remote URL taken apart: its host and path, and whether
// git reaches it over SSH (so the host may be an alias in ~/.ssh/config).
type remoteParts struct {
	host, path string
	ssh        bool
}

// splitRemote takes a remote URL apart the way git reads it: a URL with a
// scheme git speaks to GitHub with, or the scp-like [user@]host:path. A local
// path, file://, or a transport helper (ext::, …) is not a GitHub remote.
func splitRemote(remote string) (remoteParts, bool) {
	remote = strings.TrimSpace(remote)
	if scheme, _, ok := strings.Cut(remote, "://"); ok {
		u, err := url.Parse(remote)
		if err != nil {
			return remoteParts{}, false
		}
		switch strings.ToLower(scheme) {
		case "https", "http", "git":
			return remoteParts{host: u.Hostname(), path: u.Path}, true
		case "ssh", "git+ssh", "ssh+git":
			return remoteParts{host: u.Hostname(), path: u.Path, ssh: true}, true
		}
		return remoteParts{}, false
	}
	// scp-like: a colon before any slash (git's own rule), and not a helper's "::".
	colon := strings.Index(remote, ":")
	if colon <= 0 || strings.Contains(remote[:colon], "/") || strings.HasPrefix(remote[colon:], "::") {
		return remoteParts{}, false
	}
	host := remote[:colon]
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	return remoteParts{host: host, path: remote[colon+1:], ssh: true}, true
}

// githubHosts are the names github.com answers to: ssh.github.com is its SSH
// over port 443.
var githubHosts = []string{"github.com", "www.github.com", "ssh.github.com"}

func isGitHubHost(host string) bool {
	for _, h := range githubHosts {
		if strings.EqualFold(host, h) {
			return true
		}
	}
	return false
}

var (
	ownerShape = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)
	nameShape  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// repoOfPath reads owner/name from a remote URL's path: /owner/name.git,
// owner/name, with or without a trailing slash. The names are GitHub's own
// shape, as they become an API path and a push URL.
func repoOfPath(path string) (GitHubRepo, bool) {
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	owner, name, ok := strings.Cut(path, "/")
	if !ok || !ownerShape.MatchString(owner) || !nameShape.MatchString(name) || name == "." || name == ".." {
		return GitHubRepo{}, false
	}
	return GitHubRepo{Owner: owner, Name: name}, true
}

// parseGitHubRemote reads owner and name from a github.com remote URL in any
// of its spellings (https, ssh://, scp-like, git://). Credentials in the URL
// are dropped, never kept or shown. An SSH alias is not looked up here: see
// repoOfRemote.
func parseGitHubRemote(remote string) (GitHubRepo, error) {
	p, ok := splitRemote(remote)
	if !ok {
		return GitHubRepo{}, errNotGitHubURL
	}
	return repoOn(p, p.host)
}

// repoOn is owner/name of a remote whose host is really host (an SSH alias
// already looked up).
func repoOn(p remoteParts, host string) (GitHubRepo, error) {
	if !isGitHubHost(host) {
		return GitHubRepo{}, notGitHubHost(host)
	}
	repo, ok := repoOfPath(p.path)
	if !ok {
		return GitHubRepo{}, errors.New("origin is on github.com but does not name a repository as owner/name")
	}
	return repo, nil
}

var errNotGitHubURL = errors.New("origin is not a GitHub URL (such as https://github.com/owner/name or git@github.com:owner/name.git); the issue and PR tools only work with github.com repositories")

func notGitHubHost(host string) error {
	return fmt.Errorf("origin is on %s, not github.com; the issue and PR tools only work with github.com repositories (GitHub Enterprise is not supported yet)", printableHost(host))
}

// printableHost is a host as an error may show it: nothing else of the URL,
// and nothing that is not a host name.
func printableHost(host string) string {
	if !hostShape.MatchString(host) {
		return "a host that is not a name"
	}
	return host
}

var hostShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// repoOfRemote is owner/name of the repository a remote URL points at. An SSH
// host that is not github.com may be an alias (Host github.com-work in
// ~/.ssh/config): `ssh -G` says what it stands for, without connecting.
func repoOfRemote(ctx context.Context, remote string) (GitHubRepo, error) {
	p, ok := splitRemote(remote)
	if !ok {
		return GitHubRepo{}, errNotGitHubURL
	}
	if isGitHubHost(p.host) || !p.ssh {
		return repoOn(p, p.host)
	}
	real, err := sshHostName(ctx, p.host)
	if err != nil {
		return GitHubRepo{}, fmt.Errorf("origin is on %s, not github.com, and whether it is an SSH alias for github.com could not be read (`ssh -G %s`: %v); the issue and PR tools only work with github.com repositories", printableHost(p.host), printableHost(p.host), err)
	}
	if !isGitHubHost(real) {
		if strings.EqualFold(real, p.host) {
			return GitHubRepo{}, notGitHubHost(p.host)
		}
		return GitHubRepo{}, fmt.Errorf("origin is on %s, an SSH alias for %s, not github.com; the issue and PR tools only work with github.com repositories (GitHub Enterprise is not supported yet)", printableHost(p.host), printableHost(real))
	}
	return repoOn(p, real)
}

// sshConfigArgs are put before -G; tests point ssh at their own config file.
var sshConfigArgs []string

// sshHostName is the host an SSH host name stands for in the user's SSH
// configuration (its HostName), read with `ssh -G`, which connects nowhere.
func sshHostName(ctx context.Context, host string) (string, error) {
	if !hostShape.MatchString(host) {
		return "", errors.New("not a host name")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := append(append([]string{}, sshConfigArgs...), "-G", "--", host)
	out, err := exec.CommandContext(ctx, "ssh", args...).Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("ssh is not installed")
		}
		return "", errors.New(strings.TrimSpace(err.Error()))
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), " "); ok && strings.EqualFold(k, "hostname") {
			return strings.TrimSpace(v), nil
		}
	}
	return "", errors.New("no hostname in its answer")
}

// originRepo is the repository a checkout's origin points at: origin as
// configured, else as git expands it (url.<base>.insteadOf, so a shorthand
// such as gh:owner/name works too).
func originRepo(ctx context.Context, dir string) (GitHubRepo, error) {
	remote, err := git(ctx, dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return GitHubRepo{}, errNoOrigin
	}
	repo, err := repoOfRemote(ctx, remote)
	if err == nil {
		return repo, nil
	}
	if expanded, xerr := git(ctx, dir, "remote", "get-url", "origin"); xerr == nil && expanded != remote {
		if repo, xerr := repoOfRemote(ctx, expanded); xerr == nil {
			return repo, nil
		}
	}
	return GitHubRepo{}, err
}

// URL is the repository's page, https://github.com/owner/name.
func (r GitHubRepo) URL() string { return "https://github.com/" + r.Owner + "/" + r.Name }

// githubRemote is how the tools' git reaches a repository on GitHub: over
// https with the tools' own credential, whatever origin says, and from a
// scratch repository (see run), so nothing of the user's git setup is in the
// way: no SSH key a thread cannot unlock (a passphrase, an agent that asks, a
// hardware key that wants a touch), no credential helper, no insteadOf rule,
// no hook.
type githubRemote struct {
	repo GitHubRepo
	cred GitHubCredential
	// url is where git reaches the repository: https://github.com/owner/name.git
	// unless the GitHub backend says otherwise (gitURLer).
	url string
}

// gitURLer is a GitHub backend that is not github.com itself (a test's local
// repository; later an Enterprise host): it says where git reaches a
// repository.
type gitURLer interface {
	GitURL(r GitHubRepo) string
}

// scrub is git's output as an error may carry it: no token, of any shape.
func (g githubRemote) scrub(s string) string {
	if g.cred.Token != "" {
		s = strings.ReplaceAll(s, g.cred.Token, "[redacted]")
	}
	return redact(s)
}

// tokenEnv and userEnv are the variables the credential helper reads. The
// token is in that one git's environment only: never in an argument (other
// users see those), a config file or a log.
const (
	tokenEnv = "TINGLY_GITHUB_TOKEN"
	userEnv  = "TINGLY_GITHUB_USER"
)

// credentialHelper answers git's "get" with the credential and ignores "store"
// and "erase", so the token is never written anywhere. It is configured for
// https://github.com only, so no other host is ever offered it.
const credentialHelper = `!f() { test "$1" = get || exit 0; echo "username=$` + userEnv + `"; echo "password=$` + tokenEnv + `"; }; f`

// carriedConfig are the user's settings the tools' git keeps for github.com:
// how this machine reaches the internet (a proxy, a company's CA), nothing
// that changes where a push goes or who it is.
var carriedConfig = []string{"http.proxy", "http.sslCAInfo", "http.sslCAPath"}

// run runs git with args against the repository from a scratch bare
// repository that borrows dir's objects (objects/info/alternates), so none of
// the checkout's config or hooks are read, and with the user's global and
// system config off: what a push needs is passed in the environment. The
// scratch repository is removed afterwards; git's output comes back scrubbed.
func (g githubRemote) run(ctx context.Context, dir string, timeout time.Duration, args ...string) (string, error) {
	common, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	carried := carriedSettings(ctx)
	scratch, err := os.MkdirTemp("", "tingly-git-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(scratch)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	env := g.env(carried)
	runIn := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir, cmd.Env = scratch, env
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err := cmd.Run()
		return g.scrub(strings.TrimSpace(out.String())), err
	}
	if out, err := runIn("init", "--quiet", "--bare", "--template=", scratch); err != nil {
		return out, fmt.Errorf("a scratch repository for git could not be made: %s", out)
	}
	if err := os.MkdirAll(filepath.Join(scratch, "objects", "info"), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(scratch, "objects", "info", "alternates"), []byte(filepath.Join(common, "objects")+"\n"), 0o644); err != nil {
		return "", err
	}
	// A shallow clone's boundary goes with its objects.
	if b, err := os.ReadFile(filepath.Join(common, "shallow")); err == nil {
		if err := os.WriteFile(filepath.Join(scratch, "shallow"), b, 0o644); err != nil {
			return "", err
		}
	}
	return runIn(append([]string{"--git-dir=" + scratch}, args...)...)
}

// carriedSettings reads carriedConfig for github.com from the user's global
// config, else the system's: never the checkout's own, which a thread writes.
func carriedSettings(ctx context.Context) [][2]string {
	var out [][2]string
	for _, key := range carriedConfig {
		for _, scope := range []string{"--global", "--system"} {
			cmd := exec.CommandContext(ctx, "git", "config", scope, "--get-urlmatch", key, "https://github.com/")
			cmd.Dir = os.TempDir()
			if v, err := cmd.Output(); err == nil && strings.TrimSpace(string(v)) != "" {
				out = append(out, [2]string{key, strings.TrimSpace(string(v))})
				break
			}
		}
	}
	return out
}

// env is the environment of the tools' git: the process's own without any
// GIT_ variable (one could point git at another repository or config), the
// user's global and system config off, no prompt of any kind, the credential
// helper above and the carried settings in GIT_CONFIG_*, and the credential.
func (g githubRemote) env(carried [][2]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	user := g.cred.Username
	if user == "" {
		user = "x-access-token"
	}
	env = append(env, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "LC_ALL=C", "LANGUAGE=C")
	pairs := append([][2]string{
		{"credential.helper", ""},
		{"credential.https://github.com.helper", credentialHelper},
	}, carried...)
	env = withConfig(env, pairs)
	return append(env, tokenEnv+"="+g.cred.Token, userEnv+"="+user)
}

// withConfig adds config entries to env through GIT_CONFIG_COUNT, after any
// the environment already carries.
func withConfig(env []string, pairs [][2]string) []string {
	n := 0
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "GIT_CONFIG_COUNT="); ok {
			n, _ = strconv.Atoi(v)
		}
	}
	for i, p := range pairs {
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", n+i, p[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", n+i, p[1]))
	}
	return append(env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", n+len(pairs)))
}

// nonInteractiveEnv is the environment for a git that still goes through
// origin as the user set it up (the diff's best-effort fetch of a base), where
// nobody can answer: no terminal prompt, and SSH in batch mode (it fails at
// once on a key that needs a passphrase or a confirmation, instead of
// waiting) unless the user set their own SSH command, which is left alone.
func nonInteractiveEnv(ctx context.Context, dir string) []string {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "SSH_ASKPASS_REQUIRE=never", "LC_ALL=C", "LANGUAGE=C")
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" {
		return env
	}
	if cmd, err := git(ctx, dir, "config", "--get", "core.sshCommand"); err == nil && strings.TrimSpace(cmd) != "" {
		return env
	}
	return append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=15")
}
