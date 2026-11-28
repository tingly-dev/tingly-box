package project

import (
	"path/filepath"
	"regexp"
	"strings"
)

// What a thread's gh command line comes to at the approval prompt (see
// .design/github-tools.md, "gh in a thread"). In auto mode every gh command
// asks (grantAutoGit), so it reaches the prompt instead of the classifier, and
// is answered here before it becomes a card:
//
//   - ghSteer: a read or write a Projects tool does (view, create_pr, merge,
//     comment, close, create_issue). Refused with the tool to use: the tool
//     reads more, records what it does and asks the user by the project's
//     settings, so the user never gets a card for a command the tool covers.
//   - ghRead: another read (a run's log, a list, a search, a GET of gh api).
//     Runs without a card in auto mode, where the classifier would have let it
//     through anyway. A line with another command in it (other than a filter
//     such as head or jq) is a card instead, since allowing the line would
//     allow that command too.
//   - ghAsk: anything else, writes above all (a review, a release, gh api with
//     a method or fields, an edit with labels): the user decides on a card.
type ghVerdict int

const (
	ghNone ghVerdict = iota
	ghRead
	ghAsk
	ghSteer
)

// ghSteered are the gh commands a Projects tool does, with what to say.
var ghSteered = map[string]string{
	"pr view":       viewSteer,
	"issue view":    viewSteer,
	"pr checks":     viewSteer,
	"pr create":     "Open the PR with the create_pr tool, not gh: it pushes the branch, records the PR for the thread and asks the user when the project wants it.",
	"pr merge":      "Merge with the merge tool, not gh, and only when the user asked for the merge: it asks the user on a card when the project wants it.",
	"pr comment":    "Comment with the comment tool, not gh.",
	"issue comment": "Comment with the comment tool, not gh.",
	"pr close":      "Close it with the close tool, not gh: it asks the user on a card.",
	"issue close":   "Close it with the close tool, not gh: it asks the user on a card.",
	"issue create":  "Open the issue with the create_issue tool, not gh: it records the issue for the thread.",
}

const viewSteer = "Read it with the view tool, not gh: it brings the description, the last comments and, for a PR, its reviews, unresolved review threads, commits, changed files and failing checks with their links (number; repo owner/name for another repository)."

// ghReads are the gh commands that only read and that no tool does.
var ghReads = map[string]bool{
	"pr list": true, "pr diff": true, "pr status": true,
	"issue list": true, "issue status": true,
	"run list": true, "run view": true, "run watch": true,
	"workflow list": true, "workflow view": true,
	"release list": true, "release view": true,
	"repo view": true, "repo list": true,
	"label list": true, "gist list": true, "gist view": true, "cache list": true,
	"ruleset list": true, "ruleset view": true, "auth status": true,
}

// ghReadGroups read whatever their subcommand: gh search …, gh status.
var ghReadGroups = map[string]bool{"search": true, "status": true, "version": true, "help": true, "--version": true}

var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// ghFilters are the commands a gh read may be piped through and still run
// without a card: they only read what they are given.
var ghFilters = map[string]bool{"head": true, "tail": true, "grep": true, "jq": true, "wc": true, "cut": true, "sort": true, "uniq": true}

// classifyGh is the verdict on a Bash command line, and for ghSteer what to
// tell the model. The strongest verdict of its gh commands wins.
func classifyGh(line string) (ghVerdict, string) {
	verdict, why, others := ghNone, "", 0
	for _, part := range commandSeparators.Split(line, -1) {
		fields := strings.Fields(part)
		for len(fields) > 0 && envAssignment.MatchString(fields[0]) {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		if filepath.Base(fields[0]) != "gh" {
			if !ghFilters[fields[0]] {
				others++
			}
			continue
		}
		v, w := classifyGhArgs(fields[1:])
		if v > verdict {
			verdict, why = v, w
		}
	}
	if verdict == ghRead && others > 0 {
		return ghAsk, ""
	}
	return verdict, why
}

func classifyGhArgs(args []string) (ghVerdict, string) {
	var words []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") && len(words) > 0 {
			break // flags start: the command is named
		}
		words = append(words, a)
		if len(words) == 2 {
			break
		}
	}
	if len(words) == 0 {
		return ghAsk, ""
	}
	if words[0] == "api" {
		if ghAPIWrites(args[1:]) {
			return ghAsk, ""
		}
		return ghRead, ""
	}
	if ghReadGroups[words[0]] {
		return ghRead, ""
	}
	if len(words) < 2 {
		return ghAsk, ""
	}
	cmd := words[0] + " " + words[1]
	if why, ok := ghSteered[cmd]; ok {
		return ghSteer, why
	}
	if ghReads[cmd] {
		return ghRead, ""
	}
	return ghAsk, ""
}

// ghAPIWrites is whether gh api sends anything but a GET: a method that is not
// GET, or, with none named, fields or an input (gh then posts them).
func ghAPIWrites(args []string) bool {
	method, sends := "", false
	for i, a := range args {
		switch {
		case a == "-X" || a == "--method":
			if i+1 < len(args) {
				method = args[i+1]
			}
		case strings.HasPrefix(a, "--method="):
			method = strings.TrimPrefix(a, "--method=")
		case strings.HasPrefix(a, "-X") && len(a) > 2:
			method = a[2:]
		case a == "-f" || a == "-F" || a == "--field" || a == "--raw-field" || a == "--input" ||
			strings.HasPrefix(a, "--field=") || strings.HasPrefix(a, "--raw-field=") || strings.HasPrefix(a, "--input=") ||
			(strings.HasPrefix(a, "-f") || strings.HasPrefix(a, "-F")) && len(a) > 2:
			sends = true
		}
	}
	if method != "" {
		return !strings.EqualFold(strings.Trim(method, `"'`), "GET")
	}
	return sends
}
