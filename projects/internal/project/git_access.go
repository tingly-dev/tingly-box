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

// GitHubAccess values of db.Project.ThreadGitHubAccess.
const (
	GitHubAsk   = "ask"
	GitHubAllow = "allow"
)

// githubToolRule is the permission rule that names one of the Projects tools.
func githubToolRule(tool string) string { return "mcp__tingly-projects__" + tool }

// grantGitHubTools sets up the permission rules of a thread's GitHub tools
// (see .design/github-tools.md). The tools are answered like the `gh` command
// each replaces; the project setting only decides whether the answer is
// remembered:
//
//   - view only reads an issue or PR of the thread's own repository and is
//     always allowed;
//   - the tools that write to GitHub are allowed without a card when the
//     project's access is "allow", in every permission mode;
//   - otherwise they ask in auto mode, whose classifier would deny an unlisted
//     tool with no card (the same reason as for `gh pr create`); the other
//     modes already prompt for a tool nobody allowed, and bypassPermissions
//     keeps meaning "run without asking".
//
// Nothing here touches the Bash rules: `gh pr create`, `gh pr merge` and
// `gh issue create` stay on the ask list of grantAutoGit.
func grantGitHubTools(doc map[string]any, p *db.Project, mode string) {
	perms := permissionsOf(doc)
	addRules(perms, "allow", githubToolRule(toolView))
	var rules []string
	for _, tool := range githubWriteTools {
		rules = append(rules, githubToolRule(tool))
	}
	switch {
	case p.ThreadGitHubAccess == GitHubAllow:
		addRules(perms, "allow", rules...)
	case mode == permissionModeAuto:
		addRules(perms, "ask", rules...)
	}
}
