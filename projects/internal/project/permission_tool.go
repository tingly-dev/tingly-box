package project

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// toolRequestPermission is how a thread asks the user for a permission on
// purpose, instead of hoping a native prompt reaches them: the call raises a
// card (web and IM) and returns with the answer. See
// .design/thread-permissions.md §3.
const toolRequestPermission = "request_permission"

// grantMaxMinutes caps how long a grant for the rest of the thread may be set
// to last.
const grantMaxMinutes = 7 * 24 * 60

func (f *sessionToolsMCP) permissionTools() []Tool {
	return []Tool{{Name: toolRequestPermission, ThreadOnly: true, Strict: true,
		Description: "Ask the user for a permission Claude Code does not give you, on a card in the web and their chat; the call returns with their answer. Say the concrete action and why. command is an exact command line your settings or auto mode block (it was denied, or you know it will be): a yes lets your own Bash tool run exactly that command, so run it next, as written, in the same turn; a call that was blocked goes through when you run it again. rule is a Claude Code permission rule with a pattern for commands you will run many times, such as Bash(./bin/tool *) or WebFetch(domain:example.com). Neither lifts an OS sandbox or a deny rule. With neither, the yes is only recorded, for you to cite. Do not ask here for the GitHub tools: call them, they ask the user on their own card when the project wants it and run on a yes. After a no, do not ask again for the same thing.",
		Properties: map[string]any{
			"action":  strProp("What you want to do, concretely (for example \"run task codegen to regenerate the client\")."),
			"reason":  strProp("Why you need it."),
			"command": strProp("Optional. The exact command line you will run with your Bash tool (for example \"task codegen\" or \"HOME=$TMPDIR/home ./bin/tingly-project swagger\"). No * in it."),
			"rule":    strProp("Optional. A Claude Code permission rule with a pattern, such as Bash(npm run build *) or WebFetch(domain:example.com)."),
			"scope":   map[string]any{"type": "string", "enum": []string{db.GrantOnce, db.GrantThread}, "description": "once (default): this turn; a command is used up by the call that runs it. thread: until the thread is closed or minutes pass."},
			"minutes": map[string]any{"type": "integer", "description": "Optional. How long a thread-scoped grant lasts."},
		},
		Required: []string{"action", "reason"},
		Handle: threadTool(f, func(ctx context.Context, t *db.Thread, in PermissionRequest) (string, error) {
			if f.service.Threads == nil {
				return "", errors.New("there is no one to ask here")
			}
			return f.service.Threads.RequestPermission(ctx, t, in)
		})}}
}

// PermissionRequest is what a thread asks for with request_permission.
type PermissionRequest struct {
	Action, Reason, Command, Rule, Scope string
	Minutes                              int
}

// grantRuleShape is a permission rule with a pattern: Tool(pattern).
var grantRuleShape = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\((.+)\)$`)

// githubCommands are what the GitHub tools do with the project's settings: no
// grant runs them past those tools.
var githubCommands = []string{"gh", "git push"}

// grantRefusedCommands are the other commands a Bash rule may not start with:
// what runs any other command (shells, wrappers and interpreters), which would
// make the rule a whole-Bash rule. Claude Code's auto mode also drops wildcard
// rules for interpreters (probed for python3 -c * and node -e *), so such a
// rule would be granted and still blocked; an exact command is not dropped.
var grantRefusedCommands = []string{"bash", "sh", "zsh", "fish", "env", "xargs", "sudo", "doas", "eval", "exec", "nohup", "timeout", "nice", "command", "builtin", "su", "ssh",
	"python", "python3", "node", "deno", "bun", "ruby", "perl", "php", "npx", "bunx"}

// startsWith is whether the command line cmd starts with the command c.
func startsWith(cmd, c string) bool {
	return cmd == c || strings.HasPrefix(cmd, c+" ") || strings.HasPrefix(cmd, c+":")
}

// checkGrantRule refuses a rule the user could not weigh on a card or that
// would reach past the tools' own gate: one without a pattern, one whose
// pattern is everything (also after a "domain:" style prefix), a Bash rule for
// the GitHub CLI, git push, a shell or an interpreter, and any rule for MCP
// tools.
func checkGrantRule(rule string) error {
	m := grantRuleShape.FindStringSubmatch(rule)
	if m == nil {
		return fmt.Errorf("rule %q is not a permission rule with a pattern, such as Bash(npm run build *): a whole tool is not granted this way", rule)
	}
	if strings.HasPrefix(m[1], "mcp__") {
		return errors.New("MCP tools are not granted by a rule: the Projects tools ask on their own cards, so call them")
	}
	p := strings.TrimSpace(m[2])
	value := p
	if _, v, ok := strings.Cut(p, ":"); ok && m[1] != "Bash" {
		value = v
	}
	if strings.Trim(p, "*: ") == "" || strings.Trim(value, "*: ") == "" {
		return fmt.Errorf("rule %q matches everything: name the command (Bash(npm run build *)), or ask with command for one command line", rule)
	}
	if m[1] == "Bash" {
		cmd := strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSuffix(p, "*"), ":")), " ")
		for _, c := range githubCommands {
			if startsWith(cmd, c) {
				return fmt.Errorf("rule %q is for %s, which the GitHub tools do with the project's settings: call them, they ask the user when needed", rule, c)
			}
		}
		for _, c := range grantRefusedCommands {
			if startsWith(cmd, c) {
				return fmt.Errorf("rule %q starts with %s, which runs any command: ask with command for the exact command line instead", rule, c)
			}
		}
	}
	return nil
}

// commandSeparators split a command line into the simple commands Claude Code
// checks one by one.
var commandSeparators = regexp.MustCompile(`&&|\|\||[;|\n]`)

// commandRules are the permission rules that let the thread's Bash tool run
// exactly cmd: the whole line, and each simple command in it, since Claude
// Code checks a pipeline part by part (probed: an exact rule for "a | b" alone
// is not enough). A part is only ever allowed as written.
func commandRules(cmd string) []string {
	rules := []string{"Bash(" + cmd + ")"}
	for _, part := range commandSeparators.Split(cmd, -1) {
		part = strings.TrimSpace(part)
		if r := "Bash(" + part + ")"; part != "" && !slices.Contains(rules, r) {
			rules = append(rules, r)
		}
	}
	return rules
}

// checkGrantCommand refuses a command line a rule cannot allow exactly (a * is
// a wildcard in a rule), or that is for the GitHub tools.
func checkGrantCommand(cmd string) error {
	if strings.Contains(cmd, "*") {
		return errors.New("a * in a command is a wildcard in Claude Code's rules, so the grant would cover more than the card says: spell the files out, or ask with rule for a pattern")
	}
	for _, part := range commandSeparators.Split(cmd, -1) {
		part = strings.Join(strings.Fields(part), " ")
		for _, c := range githubCommands {
			if startsWith(part, c) {
				return fmt.Errorf("%s is done by the GitHub tools with the project's settings: call them, they ask the user when needed", c)
			}
		}
	}
	return nil
}

// RequestPermission puts in to the user on a card and records the answer: the
// card and a note in the thread with the user's answer, and on a yes the grant
// (threads.grants). A command or rule grant is written into the settings the
// thread runs on and sent to its running process at once, so the thread's own
// next call runs: execution stays in the thread's Claude Code. The card is
// always the user's: nothing answers it for them.
func (m *Threads) RequestPermission(ctx context.Context, t *db.Thread, in PermissionRequest) (string, error) {
	in.Action, in.Reason, in.Command, in.Rule = strings.TrimSpace(in.Action), strings.TrimSpace(in.Reason), strings.TrimSpace(in.Command), strings.TrimSpace(in.Rule)
	if in.Action == "" || in.Reason == "" {
		return "", errors.New("action and reason are required")
	}
	if in.Command != "" && in.Rule != "" {
		return "", errors.New("ask for a command or a rule, not both")
	}
	if in.Scope == "" {
		in.Scope = db.GrantOnce
	}
	if in.Scope != db.GrantOnce && in.Scope != db.GrantThread {
		return "", fmt.Errorf("unknown scope %q", in.Scope)
	}
	if in.Minutes < 0 || in.Minutes > grantMaxMinutes {
		return "", fmt.Errorf("minutes must be between 0 and %d", grantMaxMinutes)
	}
	switch {
	case in.Command != "":
		if err := checkGrantCommand(in.Command); err != nil {
			return "", err
		}
	case in.Rule != "":
		if err := checkGrantRule(in.Rule); err != nil {
			return "", err
		}
	}
	if (in.Command != "" || in.Rule != "") && m.isFolder(t.ProjectID) {
		return "", errors.New("commands and rules are granted to project threads only: a folder session's settings come from its profile")
	}
	id := uuid.NewString()
	input := map[string]any{"action": in.Action, "scope": in.Scope}
	switch {
	case in.Command != "":
		input["command"] = in.Command
	case in.Rule != "":
		input["rule"] = in.Rule
	}
	if in.Minutes > 0 {
		input["minutes"] = in.Minutes
	}
	ans, err := m.prompter(t.ID).wait(ctx, db.EvApproval, githubToolRule(toolRequestPermission), "The thread asks for permission: "+clip(in.Action, 200), requestPayload{
		RequestID: id, Tool: githubToolRule(toolRequestPermission), Input: input, Reason: in.Reason, State: "pending",
	})
	if err != nil {
		return "", errors.New("the user was not asked: " + err.Error())
	}
	ref := id[:8]
	fact := map[string]any{"user_answer": map[string]any{"request_id": id, "action": in.Action, "command": in.Command, "rule": in.Rule, "scope": in.Scope, "allowed": ans.Allow, "answer": ans.Answer, "answered_via": ans.Via}}
	if !ans.Allow {
		why := strings.TrimSpace(ans.Answer)
		note := "The user denied: " + in.Action
		if why != "" {
			note += " (" + why + ")"
		}
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: note, Payload: jsonString(fact)})
		if why != "" {
			return fmt.Sprintf("denied (request %s): %s. Do not run it, and do not ask again for this.", ref, why), nil
		}
		return fmt.Sprintf("denied (request %s). Do not run it, and do not ask again for this.", ref), nil
	}
	// "Allow for this thread" on the card widens this one request to the thread.
	scope := in.Scope
	if ans.Scope == "thread" {
		scope = db.GrantThread
	}
	now := time.Now()
	g := db.ThreadGrant{ID: id, Action: in.Action, Command: in.Command, Rule: in.Rule, Scope: scope, GrantedAt: now}
	until := ""
	if in.Minutes > 0 && scope == db.GrantThread {
		exp := now.Add(time.Duration(in.Minutes) * time.Minute)
		g.ExpiresAt, until = &exp, ", until "+exp.Format("15:04")
	}
	// The thread may have been closed while the card waited: a grant then
	// would outlive the close that cleared the others.
	if cur, err := m.store.GetThread(t.ID); err != nil {
		return "", err
	} else if cur.ResolvedAt != nil || cur.ArchivedAt != nil {
		return "", errors.New("the thread was closed while the user answered; nothing was granted")
	}
	if _, err := m.store.UpdateThreadGrants(t.ID, func(gs []db.ThreadGrant) []db.ThreadGrant { return append(gs, g) }); err != nil {
		return "", err
	}
	fact["user_answer"].(map[string]any)["scope"] = scope
	m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: fmt.Sprintf("The user granted: %s (%s%s; request %s)", in.Action, scopeWords(scope), until, ref), Payload: jsonString(fact)})
	head := fmt.Sprintf("granted (request %s, %s%s)", ref, scopeWords(scope), until)
	cite := fmt.Sprintf(" Cite request %s in your report.", ref)
	switch {
	case in.Command != "":
		if why, live := m.applyGrants(ctx, t); !live {
			return head + ": the command is allowed from your next turn (" + why + "). Say so in your report and end the turn instead of running it now." + cite, nil
		}
		return head + ": run it now with your Bash tool, exactly as written: " + in.Command + "\nClaude Code's own permission check allows it, with no prompt and no auto-mode classifier; if it was blocked before, run it again and it goes through. A deny rule of the user's profile and an OS sandbox still apply." + cite, nil
	case in.Rule != "":
		if why, live := m.applyGrants(ctx, t); !live {
			return head + ": " + in.Rule + " holds from your next turn (" + why + ")." + cite, nil
		}
		return head + ": " + in.Rule + " holds from your next tool call: run the commands now. A deny rule of the user's profile, an ask rule of the thread's (such as force pushes) and an OS sandbox still win." + cite, nil
	}
	return head + ": recorded; nothing is enforced by it." + cite, nil
}

// scopeWords says how long a grant of scope lasts.
func scopeWords(scope string) string {
	if scope == db.GrantThread {
		return "for this thread"
	}
	return "for this turn"
}

// applyGrants writes thread t's granted commands and rules into the settings
// its turns run on and sends them to its running process at once (agentboot's
// apply_flag_settings: no restart, held from the process's next tool call;
// probed with a real Claude Code in auto mode). live is false, with why, when
// they hold only from the next turn.
func (m *Threads) applyGrants(ctx context.Context, t *db.Thread) (why string, live bool) {
	fresh, err := m.store.GetThread(t.ID)
	if err != nil {
		return "it could not be written to the thread's settings: " + err.Error(), false
	}
	p, err := m.store.GetProject(t.ProjectID)
	if err != nil {
		return "it could not be written to the thread's settings: " + err.Error(), false
	}
	path, err := m.turnSettingsFile(ctx, p, fresh)
	if err != nil {
		return "it could not be written to the thread's settings: " + err.Error(), false
	}
	if m.engine != nil {
		if err := m.engine.RefreshSettings(ctx, m.sessionOf(t.ID), path); err != nil {
			logrus.WithError(err).WithField("thread_id", t.ID).Warn("projects: a grant could not reach the running process")
			return "the running process did not take it now", false
		}
	}
	return "", true
}

// turnSettingsFile writes the settings file thread t's turns run on, with its
// grants as they are now, and returns its path: the project's settings file,
// or for a thread under a profile the one built from the profile's settings
// (workerSettings, as each turn does).
func (m *Threads) turnSettingsFile(ctx context.Context, p *db.Project, t *db.Thread) (string, error) {
	if l, ok := m.env.(Launcher); ok && workerProfile(t) != "" {
		_, host, err := l.LaunchEnv(ctx, workerProfile(t))
		if err != nil {
			return "", err
		}
		return m.workerSettings(p, t, host)
	}
	if err := m.writeSettings(p, t); err != nil {
		return "", err
	}
	return m.layout.SettingsFile(p.ID, t.ID), nil
}

// grantRules adds the commands and rules the user granted thread t, still
// live at now, to its settings document.
func grantRules(doc map[string]any, t *db.Thread, now time.Time) {
	var rules []string
	for _, g := range t.Grants {
		if !g.Live(now) {
			continue
		}
		switch {
		case g.Command != "":
			rules = append(rules, commandRules(g.Command)...)
		case g.Rule != "":
			rules = append(rules, g.Rule)
		}
	}
	if len(rules) > 0 {
		addRules(permissionsOf(doc), "allow", rules...)
	}
}

// grantedCommand is the live command grant of thread t that a Bash call with
// input runs, if any: the call's command is the granted line as written.
func grantedCommand(t *db.Thread, input map[string]any, now time.Time) (db.ThreadGrant, bool) {
	cmd, _ := input["command"].(string)
	if cmd = strings.TrimSpace(cmd); cmd == "" {
		return db.ThreadGrant{}, false
	}
	for _, g := range t.Grants {
		if g.Command == cmd && g.Live(now) {
			return g, true
		}
	}
	return db.ThreadGrant{}, false
}

// noteGrantedRun records in the thread that a Bash call ran a granted command,
// and how it ended, and uses up a grant for this turn: the coordinator reads
// the grant, the call and its outcome together.
func (m *Threads) noteGrantedRun(t *db.Thread, g db.ThreadGrant, toolUseID string, failed bool) {
	if g.Scope == db.GrantOnce {
		if _, err := m.store.UpdateThreadGrants(t.ID, func(gs []db.ThreadGrant) []db.ThreadGrant {
			return slices.DeleteFunc(gs, func(x db.ThreadGrant) bool { return x.ID == g.ID })
		}); err != nil {
			logrus.WithError(err).WithField("thread_id", t.ID).Warn("projects: cannot use up a grant")
		}
	}
	outcome := "it succeeded"
	if failed {
		outcome = "it failed"
	}
	fact := map[string]any{"granted_run": map[string]any{"request_id": g.ID, "command": g.Command, "tool_use_id": toolUseID, "failed": failed}}
	m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: fmt.Sprintf("Ran with the user's grant (request %s): %s; %s.", shortID(g.ID), g.Command, outcome), Payload: jsonString(fact)})
}

// endTurnGrants drops thread t's grants for this turn when a turn ends, and
// notes a granted command the turn did not run. The running process loses
// their rules when the next turn starts (its settings are sent again then).
func (m *Threads) endTurnGrants(t *db.Thread) {
	var unused []db.ThreadGrant
	if _, err := m.store.UpdateThreadGrants(t.ID, func(gs []db.ThreadGrant) []db.ThreadGrant {
		return slices.DeleteFunc(gs, func(g db.ThreadGrant) bool {
			if g.Scope != db.GrantOnce {
				return false
			}
			if g.Command != "" {
				unused = append(unused, g)
			}
			return true
		})
	}); err != nil {
		logrus.WithError(err).WithField("thread_id", t.ID).Warn("projects: cannot end a turn's grants")
		return
	}
	for _, g := range unused {
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: fmt.Sprintf("The grant for %s (request %s) ended with the turn; the command was not run.", g.Command, shortID(g.ID))})
	}
}

// shortID is the part of a request id the thread's notes quote.
func shortID(id string) string { return id[:min(8, len(id))] }
