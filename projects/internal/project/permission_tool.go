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
		Description: "Ask the user for a permission you need and do not have, on a card in the web and their chat; the call returns with their answer. Say the concrete action and why. group is a GitHub group of the project's settings (publish, rewrite, others, close, merge): a yes lets those GitHub tool calls run without their card. rule is a Claude Code permission rule for a command your settings block, such as Bash(task codegen) or Bash(./bin/tool *): a yes adds it to this thread's settings from your next tool call (scope thread only; it cannot lift an OS sandbox or a deny rule). With neither, the yes is only recorded, for you to cite. Pushing to a default or protected branch and force-pushing another thread's branch always ask on their own card. After a no, do not ask again for the same thing.",
		Properties: map[string]any{
			"action":  strProp("What you want to do, concretely (for example \"force-push tp/a with lease\", \"run task codegen with an isolated HOME\")."),
			"reason":  strProp("Why you need it."),
			"group":   map[string]any{"type": "string", "enum": GitHubGroups, "description": "Optional. The GitHub group the action belongs to."},
			"rule":    strProp("Optional. A Claude Code permission rule with a pattern, such as Bash(task codegen) or Bash(npm run build *)."),
			"scope":   map[string]any{"type": "string", "enum": []string{db.GrantOnce, db.GrantThread}, "description": "once (default): the next call it covers; thread: until the thread is closed or minutes pass. A rule needs thread."},
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
	Action, Reason, Group, Rule, Scope string
	Minutes                            int
}

// grantRuleShape is a permission rule with a pattern: Tool(pattern).
var grantRuleShape = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\((.+)\)$`)

// checkGrantRule refuses a rule the user could not weigh on a card or that
// would reach past the tools' own gate: one without a pattern, one whose
// pattern is everything, and any rule for the Projects tools.
func checkGrantRule(rule string) error {
	m := grantRuleShape.FindStringSubmatch(rule)
	if m == nil {
		return fmt.Errorf("rule %q is not a permission rule with a pattern, such as Bash(task codegen): a whole tool is not granted this way", rule)
	}
	if strings.HasPrefix(m[1], "mcp__") {
		return errors.New("MCP tools are not granted by a rule: the Projects tools ask on their own cards (pass group for GitHub)")
	}
	if p := strings.TrimSpace(m[2]); p == "" || strings.Trim(p, "*: ") == "" {
		return fmt.Errorf("rule %q matches everything: name the command (Bash(task codegen), Bash(npm run build *))", rule)
	}
	return nil
}

// RequestPermission puts in to the user on a card and records the answer: the
// card and a note in the thread with the user's answer, and on a yes the grant
// (threads.grants). A group grant is honoured by the GitHub tools' gate; a
// rule grant is written into the thread's settings and sent to its running
// process at once. The card is always the user's: nothing answers it for them.
func (m *Threads) RequestPermission(ctx context.Context, t *db.Thread, in PermissionRequest) (string, error) {
	in.Action, in.Reason, in.Rule = strings.TrimSpace(in.Action), strings.TrimSpace(in.Reason), strings.TrimSpace(in.Rule)
	if in.Action == "" || in.Reason == "" {
		return "", errors.New("action and reason are required")
	}
	if in.Group != "" && in.Rule != "" {
		return "", errors.New("ask for a group or a rule, not both")
	}
	if in.Group != "" && !slices.Contains(GitHubGroups, in.Group) {
		return "", fmt.Errorf("unknown group %q", in.Group)
	}
	if in.Scope == "" {
		in.Scope = db.GrantOnce
	}
	if in.Minutes < 0 || in.Minutes > grantMaxMinutes {
		return "", fmt.Errorf("minutes must be between 0 and %d", grantMaxMinutes)
	}
	if in.Rule != "" {
		if err := checkGrantRule(in.Rule); err != nil {
			return "", err
		}
		if in.Scope != db.GrantThread {
			return "", errors.New("a rule matches every command of its pattern, so it cannot be for one call: ask with scope thread, or let Claude Code ask for the one command")
		}
		if m.isFolder(t.ProjectID) {
			return "", errors.New("rules are granted to project threads only: a folder session's settings come from its profile")
		}
	}
	id := uuid.NewString()
	input := map[string]any{"action": in.Action, "scope": in.Scope}
	switch {
	case in.Group != "":
		input["group"] = in.Group
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
	fact := map[string]any{"user_answer": map[string]any{"request_id": id, "action": in.Action, "group": in.Group, "rule": in.Rule, "scope": in.Scope, "allowed": ans.Allow, "answer": ans.Answer, "answered_via": ans.Via}}
	if !ans.Allow {
		why := strings.TrimSpace(ans.Answer)
		note := "The user denied: " + in.Action
		if why != "" {
			note += " (" + why + ")"
		}
		m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: note, Payload: jsonString(fact)})
		if why != "" {
			return fmt.Sprintf("denied (request %s): %s. Do not ask again for this.", ref, why), nil
		}
		return fmt.Sprintf("denied (request %s). Do not ask again for this.", ref), nil
	}
	// "Allow for this thread" on the card widens this one request to the thread.
	scope := in.Scope
	if ans.Scope == "thread" {
		scope = db.GrantThread
	}
	now := time.Now()
	g := db.ThreadGrant{ID: id, Action: in.Action, Group: in.Group, Rule: in.Rule, Scope: scope, GrantedAt: now}
	until := ""
	if in.Minutes > 0 {
		exp := now.Add(time.Duration(in.Minutes) * time.Minute)
		g.ExpiresAt, until = &exp, ", until "+exp.Format("15:04")
	}
	if _, err := m.store.UpdateThreadGrants(t.ID, func(gs []db.ThreadGrant) []db.ThreadGrant { return append(gs, g) }); err != nil {
		return "", err
	}
	fact["user_answer"].(map[string]any)["scope"] = scope
	m.addEvent(t, &db.ThreadEvent{Kind: db.EvSystem, Content: fmt.Sprintf("The user granted: %s (%s%s; request %s)", in.Action, scope, until, ref), Payload: jsonString(fact)})
	head := fmt.Sprintf("granted (request %s, scope %s%s)", ref, scope, until)
	switch {
	case in.Group != "":
		if scope == db.GrantOnce {
			return head + fmt.Sprintf(": the next %s call runs without its card. Cite request %s in your report.", in.Group, ref), nil
		}
		return head + fmt.Sprintf(": %s calls run without their card while it lasts. Cite request %s in your report.", in.Group, ref), nil
	case in.Rule != "":
		return head + ": " + m.applyRuleGrant(ctx, t, in.Rule), nil
	}
	return head + fmt.Sprintf(": recorded; nothing is enforced by it. Cite request %s in your report.", ref), nil
}

// applyRuleGrant writes a granted rule into the thread's settings and sends it
// to the running process, and says when it holds.
func (m *Threads) applyRuleGrant(ctx context.Context, t *db.Thread, rule string) string {
	fresh, err := m.store.GetThread(t.ID)
	if err != nil {
		return "it could not be written to the thread's settings: " + err.Error()
	}
	p, err := m.store.GetProject(t.ProjectID)
	if err != nil {
		return "it could not be written to the thread's settings: " + err.Error()
	}
	if err := m.writeSettings(p, fresh); err != nil {
		return "it could not be written to the thread's settings: " + err.Error()
	}
	if m.engine != nil {
		if err := m.engine.RefreshSettings(ctx, m.sessionOf(t.ID), m.layout.SettingsFile(p.ID, t.ID)); err != nil {
			logrus.WithError(err).WithField("thread_id", t.ID).Warn("projects: a granted rule could not reach the running process")
			return rule + " is in this thread's settings and holds from your next turn (the running process did not take it now)."
		}
	}
	return rule + " is in this thread's settings and holds from your next tool call. A deny rule of the user's profile, an ask rule of the thread's (such as force pushes) and an OS sandbox still win."
}

// grantRules adds the rules the user granted thread t, still live at now, to
// its settings document.
func grantRules(doc map[string]any, t *db.Thread, now time.Time) {
	var rules []string
	for _, g := range t.Grants {
		if g.Rule != "" && g.Live(now) {
			rules = append(rules, g.Rule)
		}
	}
	if len(rules) > 0 {
		addRules(permissionsOf(doc), "allow", rules...)
	}
}
