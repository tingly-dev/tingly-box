package project

import (
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

const permissionModeAuto = "auto"

// grantAutoGit sets up the commands a worker needs to hand its work over for
// auto mode, whose classifier otherwise denies them with no way for the user
// to approve (the denial never reaches the approval prompt):
//
//   - pushing the thread's own branch is allowed, so it never asks;
//   - the commands that publish or land work on the user's behalf (a pull
//     request, an issue, a merge), and pushes the thread has no business
//     making (to the base branch, forced, deleting), always ask, so the user
//     can tap Allow in the chat or the web.
//
// Rules are checked deny, ask, then allow, and run before the classifier. A
// blanket `git push *` ask would therefore also catch the allowed own-branch
// push, which is why pushes are asked by shape and not as a whole; any other
// push is left to the classifier.
func grantAutoGit(doc map[string]any, p *db.Project, t *db.Thread) {
	if threadPermissionMode(p, t) != permissionModeAuto {
		return
	}
	perms := permissionsOf(doc)
	if ruleSafe(t.Branch) {
		var allow []string
		for _, flags := range []string{"", "-u ", "--set-upstream "} {
			allow = append(allow, "Bash(git push "+flags+"origin "+t.Branch+")")
		}
		addRules(perms, "allow", allow...)
	}
	ask := []string{
		"Bash(gh pr create)", "Bash(gh pr create *)",
		"Bash(gh pr merge)", "Bash(gh pr merge *)",
		"Bash(gh issue create)", "Bash(gh issue create *)",
		"Bash(git push --force *)", "Bash(git push -f *)", "Bash(git push --force-with-lease *)",
		"Bash(git push * --force*)", "Bash(git push * -f)", "Bash(git push --delete *)", "Bash(git push * --delete *)",
	}
	for _, base := range []string{t.BaseBranch, "main", "master"} {
		if ruleSafe(base) {
			ask = append(ask, "Bash(git push origin "+base+")", "Bash(git push origin "+base+" *)")
		}
	}
	addRules(perms, "ask", ask...)
}

// ruleSafe reports whether name can sit inside a permission rule's
// parentheses as written.
func ruleSafe(name string) bool {
	return name != "" && !strings.ContainsAny(name, "()*? \t\n")
}

// GitHubAccess values of db.Project.ThreadGitHubAccess. Empty is allow.
const (
	GitHubAsk   = "ask"
	GitHubAllow = "allow"
)

// builtinToolsRule is the permission rule that names every tool of the
// Projects MCP server, the ones added later included.
const builtinToolsRule = "mcp__tingly-projects"

// githubToolRule is the permission rule that names one of the Projects tools.
func githubToolRule(tool string) string { return builtinToolsRule + "__" + tool }

// grantBuiltinTools sets up the permission rules of the Projects tools a thread
// is offered (see .design/github-tools.md). They are Tingly Box's own: a person
// who gave the thread work expects them to run, so they are allowed without a
// card in every permission mode. The scope stops at this server: Claude's own
// tools keep following the permission mode, and the tools cannot merge, close
// or force-push.
//
// A project can opt out of the tools that publish on GitHub with access "ask";
// ask rules win over the allow rule (rules are checked deny, ask, then allow).
// The other tools of the server stay allowed.
//
// Nothing here touches the Bash rules: `gh pr create`, `gh pr merge` and
// `gh issue create` stay on the ask list of grantAutoGit.
func grantBuiltinTools(doc map[string]any, p *db.Project) {
	perms := permissionsOf(doc)
	addRules(perms, "allow", builtinToolsRule)
	if p.ThreadGitHubAccess == GitHubAsk {
		var rules []string
		for _, tool := range githubWriteTools {
			rules = append(rules, githubToolRule(tool))
		}
		addRules(perms, "ask", rules...)
	}
}
