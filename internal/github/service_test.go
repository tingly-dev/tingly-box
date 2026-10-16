package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/tingly-dev/tingly-box/internal/db"
)

// fakeGitHub answers the handful of endpoints the Service uses.
type fakeGitHub struct {
	t          *testing.T
	pem        string
	tokenCalls atomic.Int32
	lastToken  map[string]any
	prBodies   []map[string]any
	srv        *httptest.Server
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeGitHub{t: t, pem: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /app-manifests/{code}/conversions", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("code") != "good-code" {
			http.Error(w, `{"message":"Not Found"}`, 404)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": 42, "slug": "tb-test", "client_id": "Iv1.abc", "client_secret": "sec", "webhook_secret": "wh",
			"pem": f.pem, "owner": map[string]any{"login": "octo", "type": "User"},
		})
	})
	mux.HandleFunc("GET /app/installations", func(w http.ResponseWriter, r *http.Request) {
		f.requireBearer(r)
		json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "repository_selection": "selected", "account": map[string]any{"login": "octo", "type": "User"}}})
	})
	mux.HandleFunc("GET /repos/octo/hello/installation", func(w http.ResponseWriter, r *http.Request) {
		f.requireBearer(r)
		json.NewEncoder(w).Encode(map[string]any{"id": 7})
	})
	mux.HandleFunc("GET /repos/octo/nope/installation", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, 404)
	})
	mux.HandleFunc("POST /app/installations/7/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		f.requireBearer(r)
		n := f.tokenCalls.Add(1)
		json.NewDecoder(r.Body).Decode(&f.lastToken)
		json.NewEncoder(w).Encode(map[string]any{"token": "ghs_" + string(rune('a'+n)), "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	})
	mux.HandleFunc("GET /repos/octo/hello", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"default_branch": "main"})
	})
	mux.HandleFunc("POST /repos/octo/hello/pulls", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		f.prBodies = append(f.prBodies, b)
		json.NewEncoder(w).Encode(map[string]any{"number": 9, "html_url": "https://github.com/octo/hello/pull/9"})
	})
	mux.HandleFunc("PATCH /repos/octo/hello/pulls/9", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"number": 9, "html_url": "https://github.com/octo/hello/pull/9"})
	})
	mux.HandleFunc("POST /repos/octo/hello/issues/9/comments", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"html_url": "https://github.com/octo/hello/pull/9#c1"})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) requireBearer(r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
		f.t.Errorf("%s %s: expected an App JWT, got %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
	}
}

func newTestService(t *testing.T, f *fakeGitHub, enabled bool) *Service {
	t.Helper()
	sm, err := db.NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	return New(Config{
		Store:   sm.GitHub(),
		DataDir: t.TempDir(),
		BaseURL: func() string { return "http://127.0.0.1:12580" },
		Enabled: func() bool { return enabled },
		APIBase: f.srv.URL,
		WebBase: "https://github.example",
	})
}

// connect runs the manifest and install steps against the fake.
func connect(t *testing.T, s *Service) {
	t.Helper()
	plan, err := s.StartConnect("user", "", "http://localhost:12580")
	if err != nil {
		t.Fatal(err)
	}
	installURL, err := s.FinishManifest(context.Background(), "good-code", plan.State)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(installURL)
	if u.Path != "/apps/tb-test/installations/new" || u.Query().Get("state") == "" {
		t.Fatalf("install url = %s", installURL)
	}
	back, err := s.FinishInstall(context.Background(), 7, u.Query().Get("state"))
	if err != nil || back != "http://localhost:12580" {
		t.Fatalf("FinishInstall = %q, %v", back, err)
	}
}

func TestStartConnect_ManifestAndTargets(t *testing.T) {
	s := newTestService(t, newFakeGitHub(t), true)

	plan, err := s.StartConnect("user", "", "http://localhost:12580/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plan.ActionURL, "https://github.example/settings/apps/new?state=") {
		t.Errorf("user action = %s", plan.ActionURL)
	}
	var m Manifest
	if err := json.Unmarshal([]byte(plan.Manifest), &m); err != nil {
		t.Fatal(err)
	}
	if m.RedirectURL != "http://localhost:12580/github/manifest/callback" || m.SetupURL != "http://localhost:12580/github/install/callback" {
		t.Errorf("callbacks = %s, %s", m.RedirectURL, m.SetupURL)
	}
	if m.Public || m.HookAttributes.Active {
		t.Errorf("app must be private with webhooks off: %+v", m)
	}
	if m.DefaultPermissions["contents"] != "write" || m.DefaultPermissions["administration"] != "" || m.DefaultPermissions["workflows"] != "" {
		t.Errorf("permissions = %v", m.DefaultPermissions)
	}

	org, err := s.StartConnect("org", "my-org", "http://localhost:12580")
	if err != nil || !strings.HasPrefix(org.ActionURL, "https://github.example/organizations/my-org/settings/apps/new?state=") {
		t.Errorf("org action = %v, %v", org.ActionURL, err)
	}
	if _, err := s.StartConnect("org", "bad/org", "http://localhost:12580"); err == nil {
		t.Error("org name with a slash must be rejected")
	}
	if _, err := s.StartConnect("user", "", "javascript:alert(1)"); err == nil {
		t.Error("non-http base_url must be rejected")
	}
}

func TestFinishManifest_StateIsOneTimeAndChecked(t *testing.T) {
	s := newTestService(t, newFakeGitHub(t), true)
	if _, err := s.FinishManifest(context.Background(), "good-code", "forged"); err == nil {
		t.Fatal("forged state accepted")
	}
	plan, _ := s.StartConnect("user", "", "http://localhost:12580")
	if _, err := s.FinishManifest(context.Background(), "good-code", plan.State); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishManifest(context.Background(), "good-code", plan.State); err == nil {
		t.Fatal("state replayed")
	}
}

func TestConnect_StoresAppAndInstallation(t *testing.T) {
	s := newTestService(t, newFakeGitHub(t), true)
	connect(t, s)

	st, err := s.Status(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Connected || st.AppSlug != "tb-test" || len(st.Installations) != 1 || st.Installations[0].Account != "octo" {
		t.Fatalf("status = %+v", st)
	}
	if err := s.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Status(context.Background(), false); st.Connected {
		t.Fatal("still connected after Disconnect")
	}
}

func TestTokens_ScopedAndCached(t *testing.T) {
	f := newFakeGitHub(t)
	s := newTestService(t, f, true)
	connect(t, s)
	ctx := context.Background()

	r1, err := s.ReadToken(ctx, "octo/hello")
	if err != nil {
		t.Fatal(err)
	}
	perms := f.lastToken["permissions"].(map[string]any)
	if perms["contents"] != "read" || perms["pull_requests"] != "read" {
		t.Errorf("read token permissions = %v", perms)
	}
	if repos := f.lastToken["repositories"].([]any); len(repos) != 1 || repos[0] != "hello" {
		t.Errorf("read token not limited to the repo: %v", f.lastToken["repositories"])
	}
	if r2, _ := s.ReadToken(ctx, "octo/hello"); r2 != r1 {
		t.Error("read token was not cached")
	}

	w, err := s.WriteToken(ctx, "octo/hello")
	if err != nil {
		t.Fatal(err)
	}
	if w == r1 || f.lastToken["permissions"].(map[string]any)["contents"] != "write" {
		t.Errorf("write token should be a separate write-scoped mint: %v %v", w, f.lastToken)
	}

	_, err = s.WriteToken(ctx, "octo/nope")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("uncovered repo error = %v", err)
	}
}

func TestPAT_FallbackWritesButAgentGetsNoToken(t *testing.T) {
	s := newTestService(t, newFakeGitHub(t), true)
	if err := s.SetPAT("ghp_abc"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if tok, err := s.ReadToken(ctx, "octo/hello"); tok != "" || err != nil {
		t.Errorf("read token in PAT mode = %q, %v", tok, err)
	}
	if tok, err := s.WriteToken(ctx, "octo/hello"); tok != "ghp_abc" || err != nil {
		t.Errorf("write token in PAT mode = %q, %v", tok, err)
	}
}

// repoDir makes a git repo whose origin is github.com/octo/hello.
func repoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("remote", "add", "origin", "git@github.com:octo/hello.git")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "init")
	return dir
}

func TestLaunch_FlagOffOrDisconnectedIsStaticHardeningOnly(t *testing.T) {
	f := newFakeGitHub(t)
	dir := repoDir(t)

	off := newTestService(t, f, false)
	connect(t, off)
	l := off.Launch(context.Background(), dir, "sess1")
	if len(l.MCPServers) != 0 || containsPrefix(l.Env, "GH_TOKEN=") || l.AppendSystemPrompt != promptReadOnly {
		t.Errorf("flag off: %+v", l)
	}

	on := newTestService(t, f, true)
	l = on.Launch(context.Background(), dir, "sess1")
	if len(l.MCPServers) != 0 || containsPrefix(l.Env, "GH_TOKEN=") {
		t.Errorf("not connected: %+v", l)
	}
}

func TestLaunch_ConnectedInjectsReadTokenAndTools(t *testing.T) {
	f := newFakeGitHub(t)
	s := newTestService(t, f, true)
	connect(t, s)
	dir := repoDir(t)

	l := s.Launch(context.Background(), dir, "sess1")
	if !containsPrefix(l.Env, "GH_TOKEN=ghs_") || !containsPrefix(l.Env, "GH_CONFIG_DIR=") || !containsPrefix(l.Env, "GH_PROMPT_DISABLED=1") {
		t.Errorf("env = %v", l.Env)
	}
	srv, _ := l.MCPServers[MCPServerName].(map[string]interface{})
	if srv == nil || srv["url"] != "http://127.0.0.1:12580/github/mcp" {
		t.Fatalf("mcp servers = %v", l.MCPServers)
	}
	if l.AppendSystemPrompt != promptConnected {
		t.Errorf("prompt = %q", l.AppendSystemPrompt)
	}
	// The bearer is stable for a session, so relaunching does not churn it.
	l2 := s.Launch(context.Background(), dir, "sess1")
	if l2.MCPServers[MCPServerName].(map[string]interface{})["headers"].(map[string]string)["Authorization"] !=
		srv["headers"].(map[string]string)["Authorization"] {
		t.Error("bearer changed between launches of one session")
	}
	// An unknown repo yields no token but keeps the tools (they explain the error).
	other := t.TempDir()
	exec.Command("git", "-C", other, "init", "-q").Run()
	exec.Command("git", "-C", other, "remote", "add", "origin", "https://github.com/octo/nope.git").Run()
	l3 := s.Launch(context.Background(), other, "sess2")
	if containsPrefix(l3.Env, "GH_TOKEN=") || len(l3.MCPServers) == 0 {
		t.Errorf("uncovered repo: %+v", l3)
	}
}

func TestMCPHandler_RequiresSessionBearerAndFlag(t *testing.T) {
	f := newFakeGitHub(t)
	s := newTestService(t, f, true)
	connect(t, s)
	h := s.MCPHandler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/github/mcp", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no bearer: %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/github/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer tbgh-forged")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("forged bearer: %d", rec.Code)
	}

	off := newTestService(t, f, false)
	rec = httptest.NewRecorder()
	off.MCPHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/github/mcp", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("flag off: %d", rec.Code)
	}
}

func TestTools_PushPRUpdateComment(t *testing.T) {
	f := newFakeGitHub(t)
	s := newTestService(t, f, true)
	connect(t, s)
	dir := repoDir(t)

	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	s.cfg.RemoteURL = func(string) string { return bare }

	sessionID := "0123456789abcdef"
	ctx := context.WithValue(context.Background(), ctxKey{}, mcpSession{id: sessionID, dir: dir})
	call := func(args map[string]any) mcp.CallToolRequest {
		var r mcp.CallToolRequest
		r.Params.Arguments = args
		return r
	}
	text := func(res *mcp.CallToolResult) string { return res.Content[0].(mcp.TextContent).Text }

	// A PR before any push is refused with the next step.
	if res, _ := s.toolPRCreate(ctx, call(map[string]any{"title": "t"})); !res.IsError || !strings.Contains(text(res), "tb_push") {
		t.Fatalf("pr before push: %v", text(res))
	}

	res, _ := s.toolPush(ctx, call(nil))
	if res.IsError {
		t.Fatalf("push: %s", text(res))
	}
	branch := "tb/01234567"
	if out, err := exec.Command("git", "-C", bare, "rev-parse", "--verify", "refs/heads/"+branch).CombinedOutput(); err != nil {
		t.Fatalf("branch %s missing on the remote: %v %s", branch, err, out)
	}
	if strings.Contains(text(res), "ghs_") {
		t.Error("token leaked into tool output")
	}

	res, _ = s.toolPRCreate(ctx, call(map[string]any{"title": "Add x", "body": "why", "draft": true}))
	if res.IsError {
		t.Fatalf("pr create: %s", text(res))
	}
	got := f.prBodies[0]
	if got["head"] != branch || got["base"] != "main" || got["draft"] != true {
		t.Errorf("PR request = %v", got)
	}

	if res, _ = s.toolPRUpdate(ctx, call(map[string]any{"title": "New"})); res.IsError {
		t.Errorf("update: %s", text(res))
	}
	if res, _ = s.toolPRUpdate(ctx, call(map[string]any{"title": "New", "number": 3})); !res.IsError || !strings.Contains(text(res), "not opened by this session") {
		t.Errorf("update of a foreign PR: %v", text(res))
	}
	if res, _ = s.toolPRComment(ctx, call(map[string]any{"body": "hi"})); res.IsError {
		t.Errorf("comment: %s", text(res))
	}
	if res, _ = s.toolPRComment(ctx, call(map[string]any{"body": ""})); !res.IsError {
		t.Error("empty comment accepted")
	}
}

func TestParseRemote(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r.git":        "o/r",
		"https://github.com/o/r":            "o/r",
		"git@github.com:o/r.git":            "o/r",
		"ssh://git@github.com/o/r.git":      "o/r",
		"https://x:tok@github.com/o/r.git/": "o/r",
		"https://gitlab.com/o/r.git":        "",
		"https://github.com/o":              "",
		"/some/local/path":                  "",
		"git@github.com:o/r/extra.git":      "",
	} {
		o, r, ok := ParseRemote(in)
		got := ""
		if ok {
			got = o + "/" + r
		}
		if got != want {
			t.Errorf("ParseRemote(%q) = %q, want %q", in, got, want)
		}
	}
}

func containsPrefix(ss []string, p string) bool {
	for _, s := range ss {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
