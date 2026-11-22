package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGitHubRemote(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/acme/widgets.git":                           "acme/widgets",
		"https://github.com/acme/widgets":                               "acme/widgets",
		"https://github.com/acme/widgets/":                              "acme/widgets",
		"https://github.com/acme/widgets.git/":                          "acme/widgets",
		"http://github.com/acme/widgets":                                "acme/widgets",
		"https://GitHub.COM/Acme/Widgets.git":                           "Acme/Widgets",
		"https://www.github.com/acme/widgets":                           "acme/widgets",
		"https://x-access-token:ghs_secret@github.com/acme/widgets.git": "acme/widgets",
		"https://someone@github.com/acme/widgets.git":                   "acme/widgets",
		"git@github.com:acme/widgets.git":                               "acme/widgets",
		"git@github.com:acme/widgets":                                   "acme/widgets",
		"git@github.com:acme/widgets.git/":                              "acme/widgets",
		"git@github.com:/acme/widgets.git":                              "acme/widgets",
		"git@GITHUB.com:acme/widgets.git":                               "acme/widgets",
		"github.com:acme/widgets.git":                                   "acme/widgets",
		"ssh://git@github.com/acme/widgets.git":                         "acme/widgets",
		"ssh://git@github.com:22/acme/widgets.git":                      "acme/widgets",
		"ssh://git@ssh.github.com:443/acme/widgets.git":                 "acme/widgets",
		"SSH://git@github.com/acme/widgets":                             "acme/widgets",
		"git+ssh://git@github.com/acme/widgets.git":                     "acme/widgets",
		"git://github.com/acme/widgets.git":                             "acme/widgets",
		"https://github.com/acme/my.repo_name-2.git":                    "acme/my.repo_name-2",
		"  git@github.com:acme/widgets.git\n":                           "acme/widgets",
		// Not a repository on github.com.
		"https://gitlab.com/acme/widgets.git":       "",
		"git@gitlab.com:acme/widgets.git":           "",
		"https://github.example.com/acme/widgets":   "",
		"git@github.com-work:acme/widgets.git":      "", // an alias needs ssh -G: repoOfRemote
		"https://github.com/acme":                   "",
		"https://github.com/acme/widgets/extra.git": "",
		"https://github.com/acme/..":                "",
		"https://github.com/-acme/widgets":          "",
		"https://github.com/acme/wid%20gets":        "",
		"/some/local/path":                          "",
		"../widgets.git":                            "",
		"file:///srv/acme/widgets.git":              "",
		"ext::ssh -i key github.com acme/widgets":   "",
		"":                                  "",
		"ftp://github.com/acme/widgets.git": "",
		"https://github.com:443/acme/widgets.git?x=": "acme/widgets",
	} {
		r, err := parseGitHubRemote(remote)
		if want == "" {
			assert.Error(t, err, remote)
			continue
		}
		require.NoError(t, err, remote)
		assert.Equal(t, want, r.Slug(), remote)
		assert.Equal(t, "https://github.com/"+want, r.URL(), remote)
	}
}

func TestARemoteThatIsNotGitHubSaysWhereItIs(t *testing.T) {
	_, err := parseGitHubRemote("https://gitlab.com/acme/widgets.git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin is on gitlab.com, not github.com")
	assert.Contains(t, err.Error(), "GitHub Enterprise is not supported yet")

	_, err = parseGitHubRemote("https://x-access-token:ghs_secret@gitlab.com/acme/widgets.git")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "ghs_secret", "the credentials of a URL are never shown")

	_, err = parseGitHubRemote("/some/local/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a GitHub URL")
}

// sshConfig points sshHostName at a config file of the test's own (the
// user's ~/.ssh/config is not read).
func sshConfig(t *testing.T, config string) {
	t.Helper()
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh is not installed")
	}
	file := filepath.Join(t.TempDir(), "ssh_config")
	require.NoError(t, os.WriteFile(file, []byte(config), 0o600))
	old := sshConfigArgs
	sshConfigArgs = []string{"-F", file}
	t.Cleanup(func() { sshConfigArgs = old })
}

func TestAnSSHAliasForGitHubIsLookedUp(t *testing.T) {
	sshConfig(t, "Host github.com-work\n  HostName github.com\n  User git\n  IdentityFile ~/.ssh/work\nHost gl\n  HostName gitlab.com\n")
	ctx := context.Background()

	for _, remote := range []string{"git@github.com-work:acme/widgets.git", "ssh://git@github.com-work/acme/widgets.git", "github.com-work:acme/widgets"} {
		r, err := repoOfRemote(ctx, remote)
		require.NoError(t, err, remote)
		assert.Equal(t, "acme/widgets", r.Slug(), remote)
	}

	_, err := repoOfRemote(ctx, "git@gl:acme/widgets.git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin is on gl, an SSH alias for gitlab.com, not github.com")

	_, err = repoOfRemote(ctx, "git@unknown-host:acme/widgets.git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin is on unknown-host, not github.com")

	// An https host is never an SSH alias.
	_, err = repoOfRemote(ctx, "https://github.com-work/acme/widgets.git")
	assert.Error(t, err)
}

func TestAnAliasThatCannotBeReadSaysSo(t *testing.T) {
	sshConfig(t, "this is not ssh config\n")
	_, err := repoOfRemote(context.Background(), "git@github.com-work:acme/widgets.git")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "whether it is an SSH alias for github.com could not be read (`ssh -G github.com-work`")

	// A host that would be an option to ssh is never passed to it.
	_, err = repoOfRemote(context.Background(), "ssh://-oProxyCommand=x/acme/widgets.git")
	assert.Error(t, err)
}

func TestOriginIsReadAsGitExpandsIt(t *testing.T) {
	repo := newRepo(t)
	run(t, repo, "git", "remote", "add", "origin", "gh:acme/widgets")
	run(t, repo, "git", "config", "url.git@github.com:.insteadOf", "gh:")
	r, err := originRepo(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, "acme/widgets", r.Slug())

	run(t, repo, "git", "remote", "remove", "origin")
	_, err = originRepo(context.Background(), repo)
	assert.ErrorIs(t, err, errNoOrigin)
}

// gitWith runs git in dir with env and returns its output.
func gitWith(t *testing.T, dir string, env []string, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

// userGitConfig gives the test a home of its own whose ~/.gitconfig is
// config: the user's global git config, as git finds it.
func userGitConfig(t *testing.T, config string) {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(config), 0o644))
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if old, ok := os.LookupEnv("GIT_CONFIG_GLOBAL"); ok {
		require.NoError(t, os.Unsetenv("GIT_CONFIG_GLOBAL"))
		t.Cleanup(func() { os.Setenv("GIT_CONFIG_GLOBAL", old) })
	}
}

// markerScript writes an executable that leaves a marker (and prints what is
// in tokenEnv) when it runs, and fails.
func markerScript(t *testing.T, dir, name string) (script, marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), name+"-ran")
	script = filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"$"+tokenEnv+"\" > "+marker+"\nexit 1\n"), 0o755))
	return script, marker
}

func assertNotRun(t *testing.T, marker, what string) {
	t.Helper()
	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "%s ran", what)
}

func TestTheToolsGitSignsInWithTheTokenAndStoresNothing(t *testing.T) {
	repo := newRepo(t)
	marker := filepath.Join(t.TempDir(), "user-helper-ran")
	helper := "!f() { echo \"$1\" >> " + marker + "; }; f"
	// The user's own helpers (gh's, a keychain) would store the token.
	userGitConfig(t, "[credential]\n\thelper = "+helper+"\n[credential \"https://github.com\"]\n\thelper = "+helper+"\n")
	run(t, repo, "git", "config", "credential.helper", helper)
	gr := githubRemote{repo: GitHubRepo{Owner: "acme", Name: "widgets"}, cred: GitHubCredential{Token: fakeToken}}
	env := gr.env(nil)

	out := gitWith(t, repo, env, "protocol=https\nhost=github.com\npath=acme/widgets.git\n\n", "credential", "fill")
	assert.Contains(t, out, "username=x-access-token\n")
	assert.Contains(t, out, "password="+fakeToken+"\n")
	gitWith(t, repo, env, "protocol=https\nhost=github.com\nusername=x-access-token\npassword="+fakeToken+"\n\n", "credential", "approve")
	assertNotRun(t, marker, "a credential helper of the user's")

	// Another host is never offered the token.
	cmd := exec.Command("git", "-C", repo, "credential", "fill")
	cmd.Env, cmd.Stdin = env, strings.NewReader("protocol=https\nhost=example.com\n\n")
	out2, _ := cmd.CombinedOutput()
	assert.NotContains(t, string(out2), fakeToken)

	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_CONFIG_VALUE_") || strings.HasPrefix(kv, "GIT_CONFIG_KEY_") {
			assert.NotContains(t, kv, fakeToken, "the token is not in git's config, only in its own variable")
		}
	}
}

func TestTheToolsGitReadsNoneOfTheUsersGitSetup(t *testing.T) {
	repo := newRepo(t)
	hooks := t.TempDir()
	_, globalHook := markerScript(t, hooks, "pre-push")
	userSSH, globalSSH := markerScript(t, t.TempDir(), "ssh")
	// A user who prefers SSH for GitHub, with global hooks and a proxy.
	userGitConfig(t, "[url \"git@github.com:\"]\n\tinsteadOf = https://github.com/\n\tpushInsteadOf = https://github.com/\n"+
		"[core]\n\thooksPath = "+hooks+"\n\tsshCommand = "+userSSH+"\n[http]\n\tproxy = http://127.0.0.1:1\n")
	// And the checkout's own: a hook, an SSH command, a rewrite elsewhere.
	_, localHook := markerScript(t, filepath.Join(repo, ".git", "hooks"), "pre-push")
	repoSSH, localSSH := markerScript(t, t.TempDir(), "ssh")
	run(t, repo, "git", "config", "core.sshCommand", repoSSH)
	run(t, repo, "git", "config", "url.https://evil.example/acme/.insteadOf", "https://github.com/acme/")
	gr := githubRemote{repo: GitHubRepo{Owner: "acme", Name: "widgets"}, cred: GitHubCredential{Token: fakeToken}, url: "https://github.com/acme/widgets.git"}
	// The setup is live for the user's own git.
	assert.Equal(t, "https://evil.example/acme/widgets.git", run(t, repo, "git", "ls-remote", "--get-url", gr.url))
	assert.Equal(t, "git@github.com:acme/widgets.git", run(t, t.TempDir(), "git", "ls-remote", "--get-url", gr.url))

	sha := run(t, repo, "git", "rev-parse", "HEAD")
	out, err := gr.run(context.Background(), repo, 30*time.Second, "push", gr.url, sha+":refs/heads/x")
	require.Error(t, err)
	// The user's proxy (carried over) is where https went, and it is not there.
	assert.Contains(t, out, "127.0.0.1", "the push went to https://github.com through the user's proxy")
	assert.NotContains(t, out, "evil.example")
	assertNotRun(t, globalHook, "the user's global pre-push hook")
	assertNotRun(t, localHook, "the checkout's pre-push hook")
	assertNotRun(t, globalSSH, "the user's SSH command")
	assertNotRun(t, localSSH, "the checkout's SSH command")
	assert.Equal(t, sha, run(t, repo, "git", "rev-parse", "HEAD"), "the checkout is as it was")
}

func TestScrubRemovesTheTokenWhateverItsShape(t *testing.T) {
	gr := githubRemote{cred: GitHubCredential{Token: "plain-token-value"}}
	assert.Equal(t, "remote: leak [redacted]", gr.scrub("remote: leak plain-token-value"))
}

func TestSSHNeverWaitsForAPerson(t *testing.T) {
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	repo := newRepo(t)
	env := nonInteractiveEnv(context.Background(), repo)
	assert.Contains(t, env, "GIT_TERMINAL_PROMPT=0")
	assert.Contains(t, env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=15")

	// The user's own SSH command is theirs: it is not replaced.
	run(t, repo, "git", "config", "core.sshCommand", "ssh -i ~/.ssh/work")
	env = nonInteractiveEnv(context.Background(), repo)
	for _, kv := range env {
		assert.NotContains(t, kv, "BatchMode")
	}
	assert.Contains(t, env, "SSH_ASKPASS_REQUIRE=never")
}

func TestWithConfigComesAfterTheEnvironmentsOwn(t *testing.T) {
	env := withConfig([]string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=a.b", "GIT_CONFIG_VALUE_0=c"}, [][2]string{{"x.y", "z"}})
	assert.Equal(t, []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=a.b", "GIT_CONFIG_VALUE_0=c", "GIT_CONFIG_KEY_1=x.y", "GIT_CONFIG_VALUE_1=z", "GIT_CONFIG_COUNT=2"}, env)
}

func TestCreatePRPushesAnSSHOriginWithoutItsSSHOrHooks(t *testing.T) {
	f := newLinksFixture(t)
	run(t, f.repo, "git", "remote", "set-url", "origin", "git@github.com:acme/widgets.git")
	ssh, sshRan := markerScript(t, t.TempDir(), "ssh")
	run(t, f.repo, "git", "config", "core.sshCommand", ssh)
	_, hookRan := markerScript(t, filepath.Join(f.repo, ".git", "hooks"), "pre-push")
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Add a"})
	require.False(t, isErr, out)
	assert.Contains(t, out, "opened PR")
	assertNotRun(t, sshRan, "origin's SSH")
	assertNotRun(t, hookRan, "the checkout's pre-push hook")

	head := run(t, f.repo, "git", "rev-parse", "tp/work")
	assert.Equal(t, head, run(t, f.remote, "git", "rev-parse", "refs/heads/tp/work"), "the branch reached the repository")
	assert.Equal(t, head, run(t, f.repo, "git", "rev-parse", "refs/remotes/origin/tp/work"), "origin's tracking branch moved as a push to origin would")
	assert.Equal(t, "git@github.com:acme/widgets.git", run(t, f.repo, "git", "config", "--get", "remote.origin.url"), "origin is never rewritten")
	assertNoTokenUnder(t, filepath.Join(f.repo, ".git"))
	assertNoTokenUnder(t, f.remote)
	assert.NotContains(t, out, fakeToken)
}

func TestCreatePRFromAWorktreeOfAShallowClone(t *testing.T) {
	f := newLinksFixture(t)
	// A shallow clone of what GitHub has, and a worktree of it.
	clone := filepath.Join(t.TempDir(), "clone")
	run(t, f.repo, "git", "clone", "-q", "--depth=1", "--branch", "main", "file://"+f.remote, clone)
	run(t, clone, "git", "remote", "set-url", "origin", "git@github.com:acme/widgets.git")
	wt := filepath.Join(t.TempDir(), "wt")
	run(t, clone, "git", "worktree", "add", "-q", "-b", "tp/shallow", wt)
	require.NoError(t, os.WriteFile(filepath.Join(wt, "b.txt"), []byte("b\n"), 0o644))
	run(t, wt, "git", "add", ".")
	run(t, wt, "git", "commit", "-qm", "b")
	f.worker.Cwd, f.worker.Branch = wt, "tp/shallow"
	require.NoError(t, f.store.SaveThread(f.worker))
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Add b"})
	require.False(t, isErr, out)
	assert.Equal(t, run(t, wt, "git", "rev-parse", "HEAD"), run(t, f.remote, "git", "rev-parse", "refs/heads/tp/shallow"))
	assert.Equal(t, run(t, wt, "git", "rev-parse", "HEAD"), run(t, wt, "git", "rev-parse", "refs/remotes/origin/tp/shallow"))
}

func TestCreatePRWithoutATokenPushesNothing(t *testing.T) {
	f := newLinksFixture(t)
	run(t, f.repo, "git", "remote", "set-url", "origin", "git@github.com:acme/widgets.git")
	f.gh.tokenFail = ErrGitHubUnavailable
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "Add a"})
	require.True(t, isErr, out)
	assert.Contains(t, out, "gh is missing or not signed in")
	assert.Empty(t, run(t, f.remote, "git", "branch", "--list", "tp/work"), "nothing was pushed")
	for _, c := range f.gh.called() {
		assert.NotContains(t, c, "create-pr")
	}
}

func TestAPushFailureNeverCarriesTheToken(t *testing.T) {
	f := newLinksFixture(t)
	// A hook that prints its environment: the token must not come back.
	hook := filepath.Join(f.remote, "hooks", "pre-receive")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho \"leak $"+tokenEnv+"\" >&2\nexit 1\n"), 0o755))
	cs, _ := f.client(t, f.worker.ID)

	out, isErr := f.call(t, cs, "create_pr", map[string]any{"title": "x"})
	require.True(t, isErr, out)
	assert.Contains(t, out, "pushing tp/work failed")
	assert.Contains(t, out, "leak [redacted]")
	assert.NotContains(t, out, fakeToken)
}

// assertNoTokenUnder fails when any file under dir holds the fake token.
func assertNoTokenUnder(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(b), fakeToken) {
			t.Errorf("%s holds the token", path)
		}
		return nil
	}))
}
