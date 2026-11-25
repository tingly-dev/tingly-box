package process

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSandboxWrapBuildsAConfinedCommand(t *testing.T) {
	old := bwrapPath
	bwrapPath = func() (string, error) { return "/usr/bin/bwrap", nil }
	defer func() { bwrapPath = old }()

	s := &Sandbox{
		Home:    "/home/u",
		KeepEnv: []string{"PATH", "ANTHROPIC_*"},
		Mounts: []Mount{
			{Path: "/repo/.git/config", ReadOnly: true},
			{Path: "/repo"},
			{Path: "/home/u/.claude/skills", ReadOnly: true, Optional: true},
		},
	}
	got, err := s.wrap(LaunchSpec{Command: []string{"claude", "-p"}, WorkDir: "/repo",
		Env: []string{"PATH=/usr/bin", "HOME=/home/u", "ANTHROPIC_BASE_URL=http://gw", "GH_TOKEN=secret", "AWS_SECRET_ACCESS_KEY=x"}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := strings.Join(got.Command, " ")
	for _, want := range []string{
		"/usr/bin/bwrap ", "--unshare-pid", "--disable-userns", "--cap-drop ALL", "--new-session", "--die-with-parent",
		"--ro-bind /usr /usr", "--tmpfs /home/u",
		"--bind /repo /repo", "--ro-bind /repo/.git/config /repo/.git/config", "--ro-bind-try /home/u/.claude/skills /home/u/.claude/skills",
		"--chdir /repo", "-- claude -p",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command lacks %q:\n%s", want, cmd)
		}
	}
	if strings.Contains(cmd, "--unshare-net") {
		t.Error("the network is not isolated yet")
	}
	if i, j := strings.Index(cmd, "--bind /repo /repo"), strings.Index(cmd, "--ro-bind /repo/.git/config"); i > j {
		t.Error("a read-only file must be mounted after the directory it covers")
	}
	if strings.Index(cmd, "--tmpfs /home/u") > strings.Index(cmd, "/home/u/.claude/skills") {
		t.Error("mounts under HOME must come after its tmpfs")
	}
	slices.Sort(got.Env)
	if want := []string{"ANTHROPIC_BASE_URL=http://gw", "HOME=/home/u", "PATH=/usr/bin"}; !slices.Equal(got.Env, want) {
		t.Errorf("env = %v, want %v", got.Env, want)
	}
	if got.WorkDir != "" || got.Sandbox != nil {
		t.Errorf("the wrapped spec runs bwrap from here and is not wrapped again: %+v", got)
	}
}

func TestSandboxRefusesToStartWithoutBwrap(t *testing.T) {
	old := bwrapPath
	bwrapPath = func() (string, error) { return "", exec.ErrNotFound }
	defer func() { bwrapPath = old }()
	_, err := NewOSExecFactory().Start(context.Background(), LaunchSpec{Command: []string{"true"}, Sandbox: &Sandbox{Home: "/tmp/h"}})
	if !errors.Is(err, ErrNoSandbox) {
		t.Fatalf("err = %v, want ErrNoSandbox: never run unconfined", err)
	}
}

// The rest run real bubblewrap; they are skipped where it cannot run.
func requireBwrap(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	if err := exec.Command("bwrap", "--ro-bind", "/", "/", "--unshare-user", "--disable-userns", "--unshare-pid", "true").Run(); err != nil {
		t.Skipf("bwrap cannot create namespaces here: %v", err)
	}
}

func runSandboxed(t *testing.T, s *Sandbox, script string, env []string) (string, error) {
	t.Helper()
	h, err := NewOSExecFactory().Start(context.Background(), LaunchSpec{Command: []string{"sh", "-c", script}, Env: env, Sandbox: s, Stderr: io.Discard})
	if err != nil {
		return "", err
	}
	_ = h.Stdin().Close()
	out, _ := io.ReadAll(h.Stdout())
	return string(out), h.Wait()
}

func TestSandboxSeesOnlyWhatItIsGiven(t *testing.T) {
	requireBwrap(t)
	root := t.TempDir()
	work, secret, home := filepath.Join(root, "work"), filepath.Join(root, "secret"), filepath.Join(root, "home")
	for _, d := range []string{work, secret, home, filepath.Join(work, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(secret, "token"), []byte("s3cret"), 0o600)
	_ = os.WriteFile(filepath.Join(work, ".git", "config"), []byte("[core]\n"), 0o644)
	s := &Sandbox{Home: home, KeepEnv: []string{"PATH"}, Mounts: []Mount{
		{Path: work},
		{Path: filepath.Join(work, ".git", "config"), ReadOnly: true},
		{Path: filepath.Join(root, "missing"), Optional: true},
	}}
	out, err := runSandboxed(t, s, `
cat `+secret+`/token 2>/dev/null && echo LEAK-secret
[ -e `+root+`/secret ] && echo LEAK-dir
echo ok > `+work+`/written && echo wrote
echo x >> `+work+`/.git/config 2>/dev/null && echo LEAK-config
[ -z "$GH_TOKEN" ] || echo LEAK-env
echo "home=$HOME"
ls /proc | grep -c '^[0-9]'
`, []string{"PATH=/usr/bin:/bin", "GH_TOKEN=x"})
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "LEAK") {
		t.Fatalf("the sandbox reached what it was not given:\n%s", out)
	}
	if !strings.Contains(out, "wrote") || !strings.Contains(out, "home="+home) {
		t.Fatalf("the workspace must be writable and HOME set:\n%s", out)
	}
	if b, _ := os.ReadFile(filepath.Join(work, "written")); string(b) != "ok\n" {
		t.Fatalf("a write in the workspace lands on the host, got %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(work, ".git", "config")); string(b) != "[core]\n" {
		t.Fatalf("the read-only config changed: %q", b)
	}
}

func TestKillingASandboxEndsEverythingInIt(t *testing.T) {
	requireBwrap(t)
	root := t.TempDir()
	tick := filepath.Join(root, "tick")
	s := &Sandbox{Home: filepath.Join(root, "home"), KeepEnv: []string{"PATH"}, Mounts: []Mount{{Path: root}}}
	_ = os.MkdirAll(s.Home, 0o755)
	h, err := NewOSExecFactory().Start(context.Background(), LaunchSpec{
		Command: []string{"sh", "-c", `(while true; do date +%s%N >> ` + tick + `; sleep 0.05; done) & sleep 300`},
		Env:     []string{"PATH=/usr/bin:/bin"}, Sandbox: s, Stderr: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if err := h.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = h.Wait()
	time.Sleep(200 * time.Millisecond)
	before, _ := os.ReadFile(tick)
	time.Sleep(300 * time.Millisecond)
	after, _ := os.ReadFile(tick)
	if len(before) == 0 {
		t.Fatal("the background process never ran")
	}
	if len(after) != len(before) {
		t.Fatal("a background process outlived the sandbox")
	}
}
