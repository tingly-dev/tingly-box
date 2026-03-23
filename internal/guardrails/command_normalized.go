package guardrails

import "strings"

// NormalizedCommand is a tool-agnostic semantic view used for matching.
type NormalizedCommand struct {
	Kind       string                 `json:"kind,omitempty" yaml:"kind,omitempty"`
	Raw        string                 `json:"raw,omitempty" yaml:"raw,omitempty"`
	Terms      []string               `json:"terms,omitempty" yaml:"terms,omitempty"`
	Resources  []string               `json:"resources,omitempty" yaml:"resources,omitempty"`
	Actions    []string               `json:"actions,omitempty" yaml:"actions,omitempty"`
	Structured map[string]interface{} `json:"structured,omitempty" yaml:"structured,omitempty"`
}

// MatchText returns a stable semantic representation used by policy matching.
func (n *NormalizedCommand) MatchText() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	if n.Kind != "" {
		b.WriteString(" normalized.kind: ")
		b.WriteString(n.Kind)
	}
	if n.Raw != "" {
		b.WriteString(" normalized.raw: ")
		b.WriteString(n.Raw)
	}
	if len(n.Actions) > 0 {
		b.WriteString(" normalized.actions: ")
		b.WriteString(strings.Join(n.Actions, " "))
	}
	if len(n.Resources) > 0 {
		b.WriteString(" normalized.resources: ")
		b.WriteString(strings.Join(n.Resources, " "))
	}
	if len(n.Terms) > 0 {
		b.WriteString(" normalized.terms: ")
		b.WriteString(strings.Join(n.Terms, " "))
	}
	return b.String()
}

func normalizeShellCommand(shell *ShellCommand) *NormalizedCommand {
	if shell == nil {
		return nil
	}
	n := &NormalizedCommand{
		Kind:       "shell",
		Raw:        shell.Raw,
		Structured: map[string]interface{}{},
	}

	terms := make([]string, 0)
	resources := make([]string, 0)
	actions := make([]string, 0)
	programs := make([]string, 0, len(shell.Commands))

	for _, cmd := range shell.Commands {
		if cmd.Program == "" {
			continue
		}
		programs = append(programs, cmd.Program)
		terms = appendUniqueString(terms, cmd.Program)
		// Any shell command invocation is an execution event. More specific
		// actions like read/delete/network are added on top so command execution
		// policies can still match commands such as `rm`.
		actions = appendUniqueString(actions, ActionExecute)
		actions = appendUniqueString(actions, normalizeShellAction(cmd.Program))

		for _, arg := range cmd.Args {
			terms = appendUniqueString(terms, arg)
			if isResourceLikeToken(arg) {
				resources = appendUniqueString(resources, arg)
			}
		}
	}

	for _, redirect := range shell.Redirects {
		if redirect.Op != "" {
			terms = appendUniqueString(terms, redirect.Op)
		}
		if redirect.Target != "" {
			terms = appendUniqueString(terms, redirect.Target)
			if isResourceLikeToken(redirect.Target) {
				resources = appendUniqueString(resources, redirect.Target)
			}
			// Shell redirection writes command output into a target, so model it
			// as a write action instead of exposing a separate redirect action.
			actions = appendUniqueString(actions, ActionWrite)
		}
	}

	for _, op := range shell.Operators {
		terms = appendUniqueString(terms, op)
	}

	n.Terms = terms
	n.Resources = resources
	n.Actions = actions
	n.Structured["programs"] = programs
	n.Structured["operators"] = append([]string(nil), shell.Operators...)
	n.Structured["redirects"] = append([]ShellRedirect(nil), shell.Redirects...)
	return n
}

func normalizeShellAction(program string) string {
	switch program {
	case "cat", "less", "more", "head", "tail", "grep", "rg", "find", "ls", "stat":
		return ActionRead
	case "cp", "mv", "tee", "touch", "mkdir", "chmod", "chown", "sed", "awk":
		return ActionWrite
	case "rm", "rmdir", "shred":
		return ActionDelete
	case "curl", "wget", "scp", "rsync":
		return ActionNetwork
	case "bash", "sh", "zsh", "python", "node", "ruby", "perl":
		return ActionExecute
	default:
		return ActionExecute
	}
}

func isResourceLikeToken(token string) bool {
	if token == "" {
		return false
	}
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "~/") || strings.HasPrefix(token, "./") || strings.HasPrefix(token, "../") {
		return true
	}
	return strings.Contains(token, "/") || strings.Contains(token, ".")
}

func appendUniqueString(items []string, value string) []string {
	if value == "" {
		return items
	}
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
