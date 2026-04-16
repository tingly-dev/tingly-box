package core

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
		actions = appendUniqueString(actions, ActionExecute)
		actions = appendUniqueString(actions, normalizeShellAction(cmd.Program))
		if isInstallCommand(cmd) {
			actions = appendUniqueString(actions, ActionInstall)
		}

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

func isInstallCommand(cmd ShellSimpleCommand) bool {
	program := strings.ToLower(strings.TrimSpace(cmd.Program))
	args := lowerTrimmed(cmd.Args)
	if program == "" {
		return false
	}

	switch program {
	case "pip", "pip2", "pip3", "pipx":
		return firstArgIn(args, "install")
	case "uv":
		return hasArgSequence(args, "pip", "install") || hasArgSequence(args, "tool", "install")
	case "npm", "npm.cmd":
		return firstArgIn(args, "install", "i", "add")
	case "pnpm":
		return firstArgIn(args, "install", "i", "add")
	case "yarn":
		return firstArgIn(args, "install", "add") || hasArgSequence(args, "global", "add")
	case "cargo":
		return firstArgIn(args, "install", "add")
	case "go":
		return firstArgIn(args, "install", "get")
	case "dotnet":
		return hasArgSequence(args, "add", "package") || hasArgSequence(args, "tool", "install")
	case "nuget", "paket":
		return firstArgIn(args, "install", "add")
	case "gem":
		return firstArgIn(args, "install")
	case "bundle", "bundler":
		return firstArgIn(args, "install", "add")
	case "mvn", "mvnw":
		return firstArgIn(args, "dependency:get", "dependency:copy", "dependency:resolve")
	case "code", "code-insiders", "codium":
		return containsArg(args, "--install-extension")
	case "openvsx", "ovsx":
		return firstArgIn(args, "install")
	case "git":
		return firstArgIn(args, "clone")
	default:
		if strings.HasPrefix(program, "python") {
			return hasArgSequence(args, "-m", "pip", "install")
		}
		return false
	}
}

func lowerTrimmed(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func firstArgIn(args []string, values ...string) bool {
	if len(args) == 0 {
		return false
	}
	first := args[0]
	for _, value := range values {
		if first == value {
			return true
		}
	}
	return false
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func hasArgSequence(args []string, sequence ...string) bool {
	if len(args) < len(sequence) || len(sequence) == 0 {
		return false
	}
	for i := 0; i <= len(args)-len(sequence); i++ {
		match := true
		for j := range sequence {
			if args[i+j] != sequence[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
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
