package claude

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// askUserQuestionTool is the built-in Claude Code tool whose invocations must
// reach the stdio prompter instead of being self-answered by the CLI.
const askUserQuestionTool = "AskUserQuestion"

// ensureInteractiveAskRules returns the value to pass as --settings so that
// AskUserQuestion is routed over the stdio control channel.
//
// Claude Code ≥ 2.1 evaluates local permission rules before consulting
// --permission-prompt-tool stdio: only a tool call whose local outcome is
// "ask" produces a can_use_tool control request. Without a matching ask rule
// the CLI treats AskUserQuestion as directly executable and answers the
// question itself in non-interactive mode — the question never reaches the
// user. Injecting the ask rule at launch time restores routing without
// modifying any user-owned settings file: the rule exists only in the argv of
// the spawned process.
//
// base is the caller-selected settings source — a file path, an inline JSON
// document (Claude Code accepts both), or empty. The document is merged in
// memory and returned as inline JSON. The original file is never written to.
// If base already contains the rule it is returned unchanged, and on any
// read/parse failure it is returned unchanged so a malformed profile fails
// the same way it would have without injection.
func ensureInteractiveAskRules(base string) string {
	doc := make(map[string]any)

	if base != "" {
		var raw []byte
		if strings.HasPrefix(strings.TrimSpace(base), "{") {
			raw = []byte(base)
		} else {
			fileRaw, err := os.ReadFile(base)
			if err != nil {
				logrus.WithError(err).WithField("settings", base).
					Warn("claude: cannot read settings for ask-rule injection; passing through unchanged")
				return base
			}
			raw = fileRaw
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			logrus.WithError(err).WithField("settings", base).
				Warn("claude: cannot parse settings for ask-rule injection; passing through unchanged")
			return base
		}
	}

	perms, ok := doc["permissions"].(map[string]any)
	if !ok {
		if _, exists := doc["permissions"]; exists {
			logrus.WithField("settings", base).
				Warn("claude: settings 'permissions' has unexpected shape; passing through unchanged")
			return base
		}
		perms = make(map[string]any)
		doc["permissions"] = perms
	}

	askList, ok := perms["ask"].([]any)
	if !ok && perms["ask"] != nil {
		logrus.WithField("settings", base).
			Warn("claude: settings 'permissions.ask' has unexpected shape; passing through unchanged")
		return base
	}
	for _, entry := range askList {
		if s, ok := entry.(string); ok && s == askUserQuestionTool {
			return base // rule already present; no injection needed
		}
	}
	perms["ask"] = append(askList, askUserQuestionTool)

	merged, err := json.Marshal(doc)
	if err != nil {
		return base
	}
	return string(merged)
}
