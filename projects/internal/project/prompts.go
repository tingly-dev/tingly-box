package project

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// MaxInstructions is the instructions length limit, as in the official
// Projects.
const MaxInstructions = 16000

func (l Layout) memoryIndexText(projectID string) string {
	idx, err := l.ReadMemory(projectID, MemoryIndex)
	if err != nil {
		return ""
	}
	idx = strings.TrimSpace(idx)
	if idx == strings.TrimSpace(memoryIndexHeader) {
		return ""
	}
	return idx
}

// threadSystemPrompt is appended to Claude Code's own system prompt for
// every turn of a thread.
func threadSystemPrompt(l Layout, p *db.Project, t *db.Thread) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are a thread in the project %q, run by a coordinator that hands you one task and reads your final report.\n", p.Name)
	if g := strings.TrimSpace(p.Goal); g != "" {
		fmt.Fprintf(&b, "Project goal: %s\n", g)
	}
	fmt.Fprintf(&b, "Your task: %s\n\n", t.Title)

	b.WriteString("<rules>\n")
	b.WriteString("- Do only the task you were given. If you notice other work worth doing, mention it in your report instead of doing it.\n")
	if t.Branch != "" {
		fmt.Fprintf(&b, "- You are on branch %s (from %s) in your own worktree (%s). Commit your work to this branch when it is done and verified. By default, do not push, merge, or modify other branches. User authorization permits these actions within its stated scope.\n", t.Branch, t.BaseBranch, t.Cwd)
	} else {
		fmt.Fprintf(&b, "- This thread has no repository. Write deliverables (documents, data, scripts) under %s so they appear in the project Library.\n", l.OutputDir(p.ID, slugify(t.Title)))
	}
	b.WriteString("- Execute a push, merge, deployment, or publication when the user has authorized it, including authorization relayed by the coordinator in your task or a follow-up, or recorded in project instructions or memory. Keep to the authorized action and target; a general request to work autonomously alone does not authorize publication. Do not ask again for authorization already given. If authorization is missing, finish the implementation and verification you can do, then request only the missing authorization.\n")
	b.WriteString("- Verify your work before calling it done (run the relevant tests, builds or checks) and say how you verified it.\n")
	b.WriteString("- If something you need is missing (access, a file, a secret, a decision), say exactly what is missing and stop. Do not substitute, mock or guess.\n")
	b.WriteString("- If you need the user's decision or input to continue, make the last line of your final message: NEEDS_INPUT: <your question>\n")
	b.WriteString("- End every turn with a short report: what you did, what is on the branch (or which files you produced), how you verified it, and anything the user should know.\n")
	fmt.Fprintf(&b, "- Project memory lives in %s. Read MEMORY.md there when relevant. To remember a project-level fact, decision or pitfall for future threads, write a short .md file there and add a one-line link to it in MEMORY.md.\n", l.MemoryDir(p.ID))
	fmt.Fprintf(&b, "- Files the user added to the project are in %s. Memory and uploads are the same for every thread of this project, and they live outside the repository so you can write to them from a worktree.\n", l.LibraryDir(p.ID))
	b.WriteString("</rules>\n")

	if len(p.Repos) > 1 {
		b.WriteString("\nOther repositories in this project (readable):\n")
		for _, r := range p.Repos {
			if r.Path != t.RepoPath {
				fmt.Fprintf(&b, "- %s\n", r.Path)
			}
		}
	}
	if ins := strings.TrimSpace(p.Instructions); ins != "" {
		fmt.Fprintf(&b, "\n<project_instructions>\n%s\n</project_instructions>\n", ins)
	}
	if mem := l.memoryIndexText(p.ID); mem != "" {
		fmt.Fprintf(&b, "\n<project_memory_index>\n%s\n</project_memory_index>\n", mem)
	}
	return b.String()
}
