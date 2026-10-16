package github

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/db"
)

// MCPServerName is the name the write tools are mounted under, so the agent
// sees them as mcp__tingly-github__tb_push etc.
const MCPServerName = "tingly-github"

// mcpSession is what a bearer token stands for: the agent session and the
// directory it works in.
type mcpSession struct {
	id  string
	dir string
}

type ctxKey struct{}

// ---------- launch ----------

// Launch implements Launcher: the static hardening, plus — when the feature
// flag is on and GitHub is connected — a read-only token for the agent's gh
// and the write tools.
func (s *Service) Launch(ctx context.Context, projectPath, sessionID string) Launch {
	l := BaseLaunch()
	if !s.enabled() || !s.connected() {
		return l
	}
	l.AppendSystemPrompt = promptConnected

	if repo, err := RepoOf(ctx, projectPath); err == nil {
		if tok, err := s.ReadToken(ctx, repo); err != nil {
			logrus.WithError(err).WithField("repo", repo).Debug("github: no read token for session")
		} else if tok != "" {
			if dir, err := EmptyGHConfigDir(s.cfg.DataDir); err == nil {
				l.Env = append(l.Env, "GH_TOKEN="+tok, "GH_CONFIG_DIR="+dir)
			}
		}
	}

	if s.cfg.BaseURL != nil {
		if base := strings.TrimRight(s.cfg.BaseURL(), "/"); base != "" {
			l.MCPServers = map[string]interface{}{
				MCPServerName: map[string]interface{}{
					"type":    "http",
					"url":     base + "/github/mcp",
					"headers": map[string]string{"Authorization": "Bearer " + s.bearerFor(sessionID, projectPath)},
				},
			}
		}
	}
	return l
}

// bearerFor returns the session's stable bearer token, creating it once.
func (s *Service) bearerFor(sessionID, dir string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.bearers[sessionID]; ok {
		s.sessions[b].dir = dir
		return b
	}
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	b := "tbgh-" + hex.EncodeToString(raw)
	s.bearers[sessionID] = b
	s.sessions[b] = &mcpSession{id: sessionID, dir: dir}
	return b
}

func (s *Service) sessionOf(bearer string) (mcpSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[bearer]
	if !ok {
		return mcpSession{}, false
	}
	return *sess, true
}

// ---------- MCP endpoint ----------

// MCPHandler serves the write tools over streamable HTTP. Every request must
// carry a session bearer from Launch; it is how a tool call is tied to a
// session, a directory and a repository.
func (s *Service) MCPHandler() http.Handler {
	srv := server.NewMCPServer("tingly-github", "1.0.0", server.WithToolCapabilities(false))
	s.registerTools(srv)
	inner := server.NewStreamableHTTPServer(srv,
		server.WithStateLess(true),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			if sess, ok := s.sessionOf(bearerOf(r)); ok {
				return context.WithValue(ctx, ctxKey{}, sess)
			}
			return ctx
		}),
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.enabled() {
			http.NotFound(w, r)
			return
		}
		if _, ok := s.sessionOf(bearerOf(r)); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func bearerOf(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func (s *Service) registerTools(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("tb_push",
		mcp.WithDescription("Push the current HEAD of the working directory to this session's branch on GitHub (tb/<session>). Commit first. Only that one branch can be pushed; force pushes are refused."),
	), s.toolPush)

	srv.AddTool(mcp.NewTool("tb_pr_create",
		mcp.WithDescription("Open a pull request from this session's pushed branch. Call tb_push first."),
		mcp.WithString("title", mcp.Required(), mcp.Description("PR title")),
		mcp.WithString("body", mcp.Description("PR description (markdown)")),
		mcp.WithString("base", mcp.Description("Base branch; defaults to the repository's default branch")),
		mcp.WithBoolean("draft", mcp.Description("Open as a draft PR")),
	), s.toolPRCreate)

	srv.AddTool(mcp.NewTool("tb_pr_update",
		mcp.WithDescription("Change the title and/or description of the pull request this session opened."),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("body", mcp.Description("New description (markdown)")),
		mcp.WithNumber("number", mcp.Description("PR number; defaults to this session's PR")),
	), s.toolPRUpdate)

	srv.AddTool(mcp.NewTool("tb_pr_comment",
		mcp.WithDescription("Comment on the pull request this session opened, or reply to one of its review comments."),
		mcp.WithString("body", mcp.Required(), mcp.Description("Comment text (markdown)")),
		mcp.WithNumber("number", mcp.Description("PR number; defaults to this session's PR")),
		mcp.WithNumber("in_reply_to", mcp.Description("ID of a review comment to reply to")),
	), s.toolPRComment)
}

// ---------- tools ----------

func sessionFrom(ctx context.Context) (mcpSession, error) {
	sess, ok := ctx.Value(ctxKey{}).(mcpSession)
	if !ok {
		return mcpSession{}, errors.New("unknown session")
	}
	return sess, nil
}

// branchOf is the one ref a session may push.
func branchOf(sessionID string) string {
	short := sessionID
	if len(short) > 8 {
		short = short[:8]
	}
	return "tb/" + short
}

func toolErr(err error) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(err.Error()), nil
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, _ := json.MarshalIndent(v, "", "  ")
	return mcp.NewToolResultText(string(b)), nil
}

// target resolves the session and its repository, and the write token.
func (s *Service) target(ctx context.Context) (sess mcpSession, repo, token string, err error) {
	if sess, err = sessionFrom(ctx); err != nil {
		return
	}
	if repo, err = RepoOf(ctx, sess.dir); err != nil {
		return
	}
	token, err = s.WriteToken(ctx, repo)
	return
}

func (s *Service) toolPush(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, repo, token, err := s.target(ctx)
	if err != nil {
		return toolErr(err)
	}
	branch := branchOf(sess.id)
	if runtime.GOOS == "windows" {
		return toolErr(errors.New("tb_push is not supported on Windows yet"))
	}
	askpass, cleanup, err := writeAskpass()
	if err != nil {
		return toolErr(err)
	}
	defer cleanup()

	remote := "https://github.com/" + repo + ".git"
	if s.cfg.RemoteURL != nil {
		remote = s.cfg.RemoteURL(repo)
	}
	// --no-verify keeps the user's pre-push hooks (and the token in this
	// process's environment) apart; no leading "+" in the refspec and no
	// --force keep it non-destructive.
	cmd := exec.CommandContext(ctx, "git", "-C", sess.dir, "-c", "credential.helper=",
		"push", "--no-verify", remote, "HEAD:refs/heads/"+branch)
	cmd.Env = append(os.Environ(),
		"GIT_ASKPASS="+askpass, "TB_GIT_TOKEN="+token, "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	out, runErr := cmd.CombinedOutput()
	text := strings.ReplaceAll(string(out), token, "***")
	if runErr != nil {
		return toolErr(fmt.Errorf("git push failed: %s\nNext: make sure HEAD is committed and the branch %s has not been rewritten", strings.TrimSpace(text), branch))
	}
	sha, _ := exec.CommandContext(ctx, "git", "-C", sess.dir, "rev-parse", "HEAD").Output()
	prev, _ := s.cfg.Store.SessionPR(sess.id, repo)
	rec := db.GitHubSessionPRRecord{SessionID: sess.id, Repo: repo, Branch: branch}
	if prev != nil {
		rec.PRNumber = prev.PRNumber
	}
	if err := s.cfg.Store.SaveSessionPR(rec); err != nil {
		logrus.WithError(err).Warn("github: record session branch")
	}
	logrus.WithFields(logrus.Fields{"session": sess.id, "repo": repo, "branch": branch}).Info("github: tb_push")
	return toolJSON(map[string]any{
		"repository": repo, "branch": branch, "commit": strings.TrimSpace(string(sha)),
		"next": "open a pull request with tb_pr_create",
	})
}

// writeAskpass writes the one-off GIT_ASKPASS helper that answers git's
// credential prompts from $TB_GIT_TOKEN, so the token never lands in a git
// config or on a command line.
func writeAskpass() (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "tb-askpass-")
	if err != nil {
		return "", nil, err
	}
	path = filepath.Join(dir, "askpass.sh")
	script := "#!/bin/sh\ncase \"$1\" in\n  Username*) echo x-access-token ;;\n  *) printf '%s\\n' \"$TB_GIT_TOKEN\" ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func (s *Service) toolPRCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, repo, token, err := s.target(ctx)
	if err != nil {
		return toolErr(err)
	}
	rec, _ := s.cfg.Store.SessionPR(sess.id, repo)
	if rec == nil || rec.Branch == "" {
		return toolErr(errors.New("nothing pushed yet; call tb_push first"))
	}
	title := strings.TrimSpace(req.GetString("title", ""))
	if title == "" {
		return toolErr(errors.New("title is required"))
	}
	base := req.GetString("base", "")
	if base == "" {
		var r struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := s.api.do(ctx, http.MethodGet, "/repos/"+repo, "Bearer "+token, nil, &r); err != nil {
			return toolErr(err)
		}
		base = r.DefaultBranch
	}
	var pr prInfo
	err = s.api.do(ctx, http.MethodPost, "/repos/"+repo+"/pulls", "Bearer "+token, map[string]any{
		"title": title, "head": rec.Branch, "base": base,
		"body": req.GetString("body", ""), "draft": req.GetBool("draft", false),
	}, &pr)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusUnprocessableEntity {
		// Most likely this branch already has an open PR: hand it back.
		owner, _, _ := strings.Cut(repo, "/")
		var existing []prInfo
		q := "/repos/" + repo + "/pulls?state=open&head=" + owner + ":" + rec.Branch
		if e2 := s.api.do(ctx, http.MethodGet, q, "Bearer "+token, nil, &existing); e2 == nil && len(existing) > 0 {
			pr, err = existing[0], nil
		}
	}
	if err != nil {
		return toolErr(fmt.Errorf("%w\nNext: check that the branch %s exists on GitHub (tb_push) and differs from %s", err, rec.Branch, base))
	}
	rec.PRNumber = pr.Number
	if err := s.cfg.Store.SaveSessionPR(*rec); err != nil {
		logrus.WithError(err).Warn("github: record session PR")
	}
	return toolJSON(map[string]any{"number": pr.Number, "url": pr.HTMLURL, "head": rec.Branch, "base": base})
}

type prInfo struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
}

// ownPR resolves the PR number a write tool may touch: the session's own.
func (s *Service) ownPR(req mcp.CallToolRequest, sess mcpSession, repo string) (int, error) {
	rec, _ := s.cfg.Store.SessionPR(sess.id, repo)
	if rec == nil || rec.PRNumber == 0 {
		return 0, errors.New("this session has not opened a pull request; use tb_pr_create first")
	}
	n := req.GetInt("number", rec.PRNumber)
	if n != rec.PRNumber {
		return 0, fmt.Errorf("#%d was not opened by this session; only #%d can be changed", n, rec.PRNumber)
	}
	return n, nil
}

func (s *Service) toolPRUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, repo, token, err := s.target(ctx)
	if err != nil {
		return toolErr(err)
	}
	n, err := s.ownPR(req, sess, repo)
	if err != nil {
		return toolErr(err)
	}
	patch := map[string]any{}
	if v := req.GetString("title", ""); v != "" {
		patch["title"] = v
	}
	if v := req.GetString("body", ""); v != "" {
		patch["body"] = v
	}
	if len(patch) == 0 {
		return toolErr(errors.New("give a title and/or body to change"))
	}
	var pr prInfo
	if err := s.api.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/pulls/%d", repo, n), "Bearer "+token, patch, &pr); err != nil {
		return toolErr(err)
	}
	return toolJSON(map[string]any{"number": pr.Number, "url": pr.HTMLURL})
}

func (s *Service) toolPRComment(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, repo, token, err := s.target(ctx)
	if err != nil {
		return toolErr(err)
	}
	n, err := s.ownPR(req, sess, repo)
	if err != nil {
		return toolErr(err)
	}
	body := strings.TrimSpace(req.GetString("body", ""))
	if body == "" {
		return toolErr(errors.New("body is required"))
	}
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, n)
	if replyTo := req.GetInt("in_reply_to", 0); replyTo != 0 {
		path = fmt.Sprintf("/repos/%s/pulls/%d/comments/%d/replies", repo, n, replyTo)
	}
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	if err := s.api.do(ctx, http.MethodPost, path, "Bearer "+token, map[string]string{"body": body}, &out); err != nil {
		return toolErr(err)
	}
	return toolJSON(map[string]any{"url": out.HTMLURL})
}
