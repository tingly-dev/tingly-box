package live

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"
)

// Options say where the binaries are and where the run lives.
type Options struct {
	// TB is the tingly-box binary that serves the model (`tingly-box vmodel`).
	TB string
	// Projects is the tingly-project binary to run the server with.
	Projects string
	// WorkDir holds the run (repository, data, homes, logs, capture); empty
	// makes a new one under the system temp directory.
	WorkDir string
	// Keep leaves the work directory after a passing run (a failing run is
	// always kept).
	Keep bool
	// Out receives progress; nil discards it.
	Out io.Writer
}

// Result is the outcome of one journey.
type Result struct {
	Journey  string
	WorkDir  string
	Failures []string
	Script   *ScriptReport
}

func (r *Result) OK() bool { return len(r.Failures) == 0 }

func (r *Result) fail(format string, args ...any) {
	r.Failures = append(r.Failures, fmt.Sprintf(format, args...))
}

// Run plays a journey against the real server and CLI.
func Run(ctx context.Context, j *Journey, o Options) (res *Result, err error) {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	timeout, err := j.timeout()
	if err != nil {
		return nil, err
	}
	dir := o.WorkDir
	if dir == "" {
		if dir, err = os.MkdirTemp("", "tingly-project-harness-"+j.Name+"-"); err != nil {
			return nil, err
		}
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return nil, err
	}
	res = &Result{Journey: j.Name, WorkDir: dir}
	defer func() {
		if err == nil && res.OK() && !o.Keep {
			_ = os.RemoveAll(dir)
		}
	}()
	for _, w := range dirtyAncestors(dir) {
		fmt.Fprintf(out, "warning: %s exists above the work directory; Claude Code will put it into every request (use --work-dir under a clean path such as /dev/shm)\n", w)
	}

	w, err := newWorld(dir, j)
	if err != nil {
		return nil, err
	}
	stopModel, modelURL, err := w.startModel(ctx, o.TB, j.ModelID())
	if err != nil {
		return nil, err
	}
	defer stopModel()
	stopProjects, projectsURL, err := w.startProjects(ctx, o.Projects)
	if err != nil {
		return nil, err
	}
	defer stopProjects()
	fmt.Fprintf(out, "journey %s: model %s, projects %s, work dir %s\n", j.Name, modelURL, projectsURL, dir)

	m := &model{api: newAPI(modelURL), id: j.ModelID()}
	p := &projects{api: newAPI(projectsURL)}
	if err := p.setup(ctx, j, w.repo, modelURL); err != nil {
		return nil, err
	}

	for i, a := range j.Steps {
		stepCtx, cancel := context.WithTimeout(ctx, timeout)
		note, err := act(stepCtx, p, m, a, timeout)
		cancel()
		if err != nil {
			fmt.Fprintf(out, "  ✗ %d %s: %v\n", i+1, a, err)
			res.fail("step %d %s: %v", i+1, a, err)
			break
		}
		if note != "" {
			note = " (" + note + ")"
		}
		fmt.Fprintf(out, "  ✓ %d %s%s\n", i+1, a, note)
	}
	if res.OK() {
		settleCtx, cancel := context.WithTimeout(ctx, timeout)
		if err := p.settle(settleCtx); err != nil {
			res.fail("%v", err)
		}
		cancel()
		for _, f := range expectations(ctx, p, j.Expect) {
			res.fail("expect: %s", f)
		}
	}
	report, err := m.finish(ctx)
	if err != nil {
		res.fail("finish script: %v", err)
	} else {
		res.Script = report
		for _, f := range report.Failures {
			where := strings.Trim(f.Track+"."+f.Step, ".")
			if f.Seq > 0 {
				where += fmt.Sprintf(" (request %d)", f.Seq)
			}
			res.fail("model %s %s: %s", f.Kind, where, f.Detail)
		}
		for _, w := range report.Warnings {
			fmt.Fprintf(out, "  warn model %s %s: %s\n", w.Kind, w.Track, w.Detail)
		}
	}
	return res, nil
}

func act(ctx context.Context, p *projects, m *model, a Action, timeout time.Duration) (string, error) {
	switch {
	case a.Say != "":
		_, err := p.do(ctx, http.MethodPost, "/api/v1/threads/"+p.project.MainThreadID+"/messages", map[string]string{"content": a.Say}, nil)
		return "", err
	case a.Message != nil:
		t, err := p.thread(ctx, a.Message.Thread)
		if err != nil {
			return "", err
		}
		_, err = p.do(ctx, http.MethodPost, "/api/v1/threads/"+t.ID+"/messages", map[string]string{"content": a.Message.Content, "mode": a.Message.Mode}, nil)
		return "", err
	case a.Approve != nil:
		return p.answer(ctx, a.Approve.Thread, true, a.Approve.Scope)
	case a.Deny != nil:
		return p.answer(ctx, a.Deny.Thread, false, a.Deny.Scope)
	case a.Wait != nil:
		return "", p.waitStatus(ctx, a.Wait.Thread, a.Wait.Status)
	case a.Served != "":
		return "", m.served(ctx, a.Served, timeout)
	}
	return "", m.signal(ctx, a.Signal)
}

func expectations(ctx context.Context, p *projects, e Expect) []string {
	var fails []string
	ths, err := p.threads(ctx)
	if err != nil {
		return []string{err.Error()}
	}
	if e.Threads != nil && len(ths) != *e.Threads {
		fails = append(fails, fmt.Sprintf("%d threads, want %d", len(ths), *e.Threads))
	}
	for title, want := range e.Thread {
		var got *thread
		for i := range ths {
			if ths[i].Title == title {
				got = &ths[i]
			}
		}
		if got == nil {
			fails = append(fails, "no thread "+quote(title))
			continue
		}
		if want.Status != "" && got.Status != want.Status {
			fails = append(fails, fmt.Sprintf("%s is %s, want %s", quote(title), got.Status, want.Status))
		}
		if want.CommitsAhead != nil && got.CommitsAhead != *want.CommitsAhead {
			fails = append(fails, fmt.Sprintf("%s is %d commits ahead, want %d", quote(title), got.CommitsAhead, *want.CommitsAhead))
		}
		if want.BranchPrefix != "" && !strings.HasPrefix(got.Branch, want.BranchPrefix) {
			fails = append(fails, fmt.Sprintf("%s is on branch %q, want prefix %q", quote(title), got.Branch, want.BranchPrefix))
		}
	}
	if len(e.ConversationContains) > 0 {
		evs, err := p.events(ctx, p.project.MainThreadID)
		if err != nil {
			return append(fails, err.Error())
		}
		var said []string
		for _, ev := range evs {
			if ev.Kind == "assistant" {
				said = append(said, ev.Content)
			}
		}
		all := strings.Join(said, "\n")
		for _, want := range e.ConversationContains {
			if !strings.Contains(all, want) {
				fails = append(fails, fmt.Sprintf("the coordinator never said %q", want))
			}
		}
	}
	return fails
}

// setup points Projects at the model and creates the journey's project.
func (p *projects) setup(ctx context.Context, j *Journey, repo, modelURL string) error {
	if err := poll(ctx, "the projects server", func() (bool, string, error) {
		_, err := p.do(ctx, http.MethodGet, "/api/v1/health", nil, nil)
		return err == nil, fmt.Sprint(err), nil
	}); err != nil {
		return err
	}
	if _, err := p.do(ctx, http.MethodPut, "/api/v1/settings", map[string]string{
		"api_style": "anthropic", "base_url": modelURL, "api_key": "sk-ant-harness-fake", "default_model": "harness-model",
	}, nil); err != nil {
		return err
	}
	if _, err := p.do(ctx, http.MethodPost, "/api/v1/projects", map[string]any{"name": j.Project.Name, "goal": j.Project.Goal, "repos": []string{repo}}, &p.project); err != nil {
		return err
	}
	if j.Project.Instructions != "" {
		if _, err := p.do(ctx, http.MethodPatch, "/api/v1/projects/"+p.project.ID, map[string]string{"instructions": j.Project.Instructions}, nil); err != nil {
			return err
		}
	}
	return nil
}

// world is the run's directory: homes, repository, data, logs.
type world struct {
	dir, home, repo, data string
}

func newWorld(dir string, j *Journey) (*world, error) {
	w := &world{dir: dir, home: filepath.Join(dir, "home"), repo: filepath.Join(dir, "repo"), data: filepath.Join(dir, "data")}
	for _, d := range []string{filepath.Join(w.home, ".claude"), w.repo, w.data} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(w.home, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0o644); err != nil {
		return nil, err
	}
	files := j.Project.Files
	if len(files) == 0 {
		files = map[string]string{"README.md": j.Project.Name + "\n"}
	}
	for name, body := range files {
		path := filepath.Join(w.repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return nil, err
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = w.repo, w.env()
		if b, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("git %s: %v: %s", args[0], err, b)
		}
	}
	model, err := yaml.Marshal(j.Model)
	if err != nil {
		return nil, err
	}
	return w, os.WriteFile(filepath.Join(dir, "model.yaml"), model, 0o644)
}

// env is a fresh environment: nothing of the user's HOME, Claude Code config
// or credentials reaches the server or the CLIs it starts.
func (w *world) env() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + w.home,
		"CLAUDE_CONFIG_DIR=" + filepath.Join(w.home, ".claude"),
		"TINGLY_PROJECT_HOME=" + w.data,
		"LANG=C.UTF-8", "NO_PROXY=*", "no_proxy=*",
		"GIT_AUTHOR_NAME=harness", "GIT_AUTHOR_EMAIL=harness@example.com",
		"GIT_COMMITTER_NAME=harness", "GIT_COMMITTER_EMAIL=harness@example.com",
		"DISABLE_TELEMETRY=1", "DISABLE_AUTOUPDATER=1", "DISABLE_ERROR_REPORTING=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}
}

func (w *world) log(name string) (*os.File, error) {
	return os.OpenFile(filepath.Join(w.dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

// startModel runs `tingly-box vmodel` on the journey's script and returns its
// URL from the readiness line.
func (w *world) startModel(ctx context.Context, tb, id string) (func(), string, error) {
	logf, err := w.log("vmodel.log")
	if err != nil {
		return nil, "", err
	}
	cmd := exec.CommandContext(ctx, tb, "vmodel", "--config", filepath.Join(w.dir, "model.yaml"), "--fallback", id,
		"--capture", filepath.Join(w.dir, "capture"), "--json")
	cmd.Env, cmd.Stderr = w.env(), logf
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", err
	}
	if err := cmd.Start(); err != nil {
		logf.Close()
		return nil, "", fmt.Errorf("start %s vmodel: %w", tb, err)
	}
	stop := func() { terminate(cmd); logf.Close() }
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		ready <- line
		_, _ = io.Copy(logf, stdout)
	}()
	select {
	case line := <-ready:
		var r struct {
			URL     string   `json:"url"`
			Scripts []string `json:"scripts"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil || r.URL == "" {
			stop()
			return nil, "", fmt.Errorf("%s vmodel did not start (is it a tingly-box with kind: script? see %s): %q", tb, filepath.Join(w.dir, "vmodel.log"), strings.TrimSpace(line))
		}
		return stop, r.URL, nil
	case <-time.After(30 * time.Second):
		stop()
		return nil, "", errors.New("tingly-box vmodel did not report ready in 30s")
	}
}

// startProjects runs `tingly-project serve` on a free port.
func (w *world) startProjects(ctx context.Context, bin string) (func(), string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	logf, err := w.log("server.log")
	if err != nil {
		return nil, "", err
	}
	cmd := exec.CommandContext(ctx, bin, "serve", "--port", strconv.Itoa(port), "--data-dir", w.data)
	cmd.Env, cmd.Stdout, cmd.Stderr, cmd.Dir = w.env(), logf, logf, w.dir
	if err := cmd.Start(); err != nil {
		logf.Close()
		return nil, "", fmt.Errorf("start %s: %w", bin, err)
	}
	return func() { terminate(cmd); logf.Close() }, "http://127.0.0.1:" + strconv.Itoa(port), nil
}

// terminate asks a process to stop and kills it if it does not.
func terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

// dirtyAncestors are the CLAUDE.md files and .claude directories above dir:
// Claude Code walks up from its working directory and embeds them.
func dirtyAncestors(dir string) []string {
	var found []string
	for d := filepath.Dir(dir); ; d = filepath.Dir(d) {
		for _, name := range []string{"CLAUDE.md", ".claude"} {
			if _, err := os.Stat(filepath.Join(d, name)); err == nil {
				found = append(found, filepath.Join(d, name))
			}
		}
		if parent := filepath.Dir(d); parent == d {
			return found
		}
	}
}
