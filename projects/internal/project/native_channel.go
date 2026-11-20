package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

// Claude Code lets sessions on one machine message each other (ListAgents and
// SendMessage over a per-session socket). A Projects session may not use that:
// the registry is the whole config dir, so a thread would reach other
// projects and the user's own terminal sessions, and the turn it starts in the
// receiver is invisible to Projects and carries no trust label. Threads reach
// their coordinator with notify_coordinator instead. See
// .design/thread-coordinator-channel.md.
const (
	toolSendMessage = "SendMessage"
	toolListAgents  = "ListAgents"
)

// subagentTarget is what SendMessage's `to` looks like when it continues one of
// the session's own subagents: the agentId the Agent tool printed. A session
// name never matches (it is the working directory's name or --name).
var subagentTarget = regexp.MustCompile(`^a[0-9a-f]{16}$`)

// grantNativeChannel closes the native channel in a session's settings:
//   - crossSessionInbound refuse: nothing can start a hidden turn here;
//   - ListAgents denied: no discovery of other sessions;
//   - SendMessage asks: the approval prompt (sendMessageAllowed) answers, so
//     the model can still continue its own subagents, which the Agent tool
//     tells it to do with this tool.
func grantNativeChannel(doc map[string]any) {
	doc["crossSessionInbound"] = "refuse"
	perms := permissionsOf(doc)
	addRules(perms, "deny", toolListAgents)
	addRules(perms, "ask", toolSendMessage)
}

// sendMessageAllowed answers a SendMessage that reached the approval prompt:
// only a message to a subagent of the session. The reason names the tool to
// use instead.
func sendMessageAllowed(input map[string]any) (bool, string) {
	to, _ := input["to"].(string)
	if subagentTarget.MatchString(to) {
		return true, ""
	}
	return false, "SendMessage may only continue one of your own subagents (its agentId, from the Agent tool's result). To reach the coordinator use notify_coordinator; other sessions cannot be messaged from a project."
}

// coordinatorSettings writes the settings file of a project's coordinator
// session: the profile's settings, if the project has one, plus the closed
// native channel. The coordinator has no other settings file.
func (l Layout) coordinatorSettings(projectID, profile string) (string, error) {
	doc := map[string]any{}
	if profile != "" {
		b, err := os.ReadFile(profile)
		if err != nil {
			return profile, err
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return profile, err
		}
	}
	grantNativeChannel(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		return profile, err
	}
	path := filepath.Join(l.Dir(projectID), "coordinator.settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return profile, err
	}
	return path, writeFileIfChanged(path, out, 0o644)
}
