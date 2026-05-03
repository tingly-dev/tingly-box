// Package claudecode implements the Claude Code hook scenario plugin.
// It is the first concrete consumer of the remote middle layer
// (channel + interaction + scenario + binding) and replaces the
// previous internal/hookbridge wiring.
//
// The plugin classifies incoming hook events as either push (Stop /
// PostToolUse / informational Notification) or interactive (PreToolUse /
// AskUserQuestion / permission Notification). Push events fire-and-
// forget through a Channel; interactive events spawn a goroutine that
// drives channel.Prompt and resolves the matching interaction.Registry
// entry, after which the long-poll wait endpoint returns the decision
// to the hook script.
package claudecode

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
)

// Name is the registered scenario identifier (matches the URL path
// segment of /tingly/claude_code/notify and the TINGLY_SCENARIO env
// variable in the hook script).
const Name = "claude_code"

// HookInput mirrors the JSON Claude Code hooks send via stdin. We keep
// it here (rather than depending on the notify module) so the plugin is
// self-contained.
type HookInput struct {
	SessionID            string `json:"session_id"`
	TranscriptPath       string `json:"transcript_path"`
	Cwd                  string `json:"cwd"`
	PermissionMode       string `json:"permission_mode"`
	HookEventName        string `json:"hook_event_name"`
	StopHookActive       bool   `json:"stop_hook_active"`
	LastAssistantMessage string `json:"last_assistant_message"`
	ToolName             string `json:"tool_name"`
	ToolInput            string `json:"tool_input"`
	ToolOutput           string `json:"tool_output"`
	NotificationMessage  string `json:"notification_message"`
}

// PermissionPolicy mirrors the binding.Options shape stored on the
// scenario binding. Only the fields the plugin reads are listed.
type PermissionPolicy struct {
	OnTimeout          string `json:"on_timeout,omitempty"`
	OnDisconnect       string `json:"on_disconnect,omitempty"`
	TotalBudgetSeconds int    `json:"total_budget_seconds,omitempty"`
}

// Plugin is the scenario.Scenario implementation. Construct via New so
// the result registry is wired up.
type Plugin struct {
	results *interaction.Registry[interaction.Result]
}

// New constructs the plugin with a result registry. The same registry
// must be provided to the wait HTTP endpoint so long-poll readers can
// pick up resolutions.
func New(results *interaction.Registry[interaction.Result]) *Plugin {
	return &Plugin{results: results}
}

// Name is the registered scenario identifier.
func (p *Plugin) Name() string { return Name }

// Trigger handles one Claude Code hook event. For push-only events it
// fires the notification asynchronously and returns an empty Outcome.
// For interactive events it spawns a goroutine driving the channel
// prompt and returns Outcome with the InteractionID + ExpiresAt so the
// notify HTTP source can issue a 202 + wait URL.
func (p *Plugin) Trigger(ctx context.Context, ev scenario.Event, rt scenario.Runtime) (scenario.Outcome, error) {
	input := parseHookInput(ev)

	ch, target, ok, err := rt.Resolve(ctx, ev)
	if err != nil {
		return scenario.Outcome{}, err
	}
	if !ok {
		// No binding (or bot not running). Source falls through.
		return scenario.Outcome{}, nil
	}

	policy := readPolicy(scenario.BindingOptions(ev))

	if !isInteractive(input) {
		text := buildPushText(input)
		go func() {
			pCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = rt.Notify(pCtx, ch, target, interaction.Notification{
				Title: "Claude Code · " + shortenPath(input.Cwd, 2),
				Body:  text,
			})
		}()
		rt.Audit("claude_code.push", map[string]any{
			"event":    input.HookEventName,
			"channel":  ch.ID(),
			"platform": ch.Platform(),
		})
		return scenario.Outcome{Handled: true}, nil
	}

	id := hookRequestID(input)
	budget := budgetDuration(policy.TotalBudgetSeconds)

	if !p.results.Begin(id) {
		// Already inflight or recently answered — return the existing
		// id so the script reuses the wait URL.
		return scenario.Outcome{
			Handled:       true,
			InteractionID: id,
			ExpiresAt:     time.Now().Add(budget),
		}, nil
	}

	ix := buildInteraction(id, input, budget)

	go p.run(ch, target, ix, input, policy, budget, rt)

	rt.Audit("claude_code.interactive.start", map[string]any{
		"interaction_id": id,
		"event":          input.HookEventName,
		"tool_name":      input.ToolName,
		"channel":        ch.ID(),
		"platform":       ch.Platform(),
	})

	return scenario.Outcome{
		Handled:       true,
		InteractionID: id,
		ExpiresAt:     time.Now().Add(budget),
	}, nil
}

// run is the background goroutine that drives the channel prompt and
// resolves the interaction registry entry with the final decision.
func (p *Plugin) run(ch channel.Channel, target channel.Target, ix interaction.Interaction, input HookInput, policy PermissionPolicy, budget time.Duration, rt scenario.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	reply, err := rt.Ask(ctx, ch, target, ix)
	if err != nil {
		decision := fallbackDecision(input, normalizePolicy(policy.OnTimeout), err.Error())
		p.results.Resolve(ix.ID, interaction.Result{
			Status:   resultStatusFromError(err),
			Decision: decision,
			Reason:   err.Error(),
		})
		rt.Audit("claude_code.interactive.error", map[string]any{
			"interaction_id": ix.ID,
			"err":            err.Error(),
		})
		return
	}

	decision := encodeDecision(input, reply)
	status := interaction.StatusAnswered
	if reply.Status == interaction.StatusCancelled {
		status = interaction.StatusCancelled
	}
	p.results.Resolve(ix.ID, interaction.Result{
		Status:   status,
		Decision: decision,
	})
	rt.Audit("claude_code.interactive.done", map[string]any{
		"interaction_id": ix.ID,
		"status":         string(status),
	})
}

// parseHookInput extracts the HookInput from the event payload.
func parseHookInput(ev scenario.Event) HookInput {
	in := HookInput{}
	if ev.Payload == nil {
		return in
	}
	if v, ok := ev.Payload["session_id"].(string); ok {
		in.SessionID = v
	}
	if v, ok := ev.Payload["transcript_path"].(string); ok {
		in.TranscriptPath = v
	}
	if v, ok := ev.Payload["cwd"].(string); ok {
		in.Cwd = v
	}
	if v, ok := ev.Payload["permission_mode"].(string); ok {
		in.PermissionMode = v
	}
	if v, ok := ev.Payload["hook_event_name"].(string); ok {
		in.HookEventName = v
	}
	if v, ok := ev.Payload["stop_hook_active"].(bool); ok {
		in.StopHookActive = v
	}
	if v, ok := ev.Payload["last_assistant_message"].(string); ok {
		in.LastAssistantMessage = v
	}
	if v, ok := ev.Payload["tool_name"].(string); ok {
		in.ToolName = v
	}
	if v, ok := ev.Payload["tool_input"].(string); ok {
		in.ToolInput = v
	}
	if v, ok := ev.Payload["tool_output"].(string); ok {
		in.ToolOutput = v
	}
	if v, ok := ev.Payload["notification_message"].(string); ok {
		in.NotificationMessage = v
	}
	return in
}

// readPolicy decodes the binding's free-form options into the typed
// PermissionPolicy.
func readPolicy(opts map[string]any) PermissionPolicy {
	pp := PermissionPolicy{}
	if opts == nil {
		return pp
	}
	if v, ok := opts["permission_policy"].(map[string]any); ok {
		if s, ok := v["on_timeout"].(string); ok {
			pp.OnTimeout = s
		}
		if s, ok := v["on_disconnect"].(string); ok {
			pp.OnDisconnect = s
		}
		if n, ok := v["total_budget_seconds"].(float64); ok {
			pp.TotalBudgetSeconds = int(n)
		}
		if n, ok := v["total_budget_seconds"].(int); ok {
			pp.TotalBudgetSeconds = n
		}
	}
	return pp
}

// isInteractive reports whether a hook event needs a user response fed
// back to Claude.
func isInteractive(input HookInput) bool {
	switch input.HookEventName {
	case "PreToolUse":
		return true
	case "Notification":
		lower := strings.ToLower(input.NotificationMessage)
		return strings.Contains(lower, "permission") || strings.Contains(lower, "approve")
	}
	return false
}

// hookRequestID derives a stable id from the hook payload so Claude
// retries collapse onto a single inflight prompt.
func hookRequestID(input HookInput) string {
	parts := []string{
		input.SessionID,
		input.HookEventName,
		input.ToolName,
		input.ToolInput,
		input.NotificationMessage,
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

// buildInteraction shapes the channel-neutral Interaction. Meta carries
// Claude-specific hints (tool_name, parsed tool_input) so imchannel can
// build the right ask.Request.
func buildInteraction(id string, input HookInput, timeout time.Duration) interaction.Interaction {
	kind := interaction.KindConfirm
	if input.ToolName == "AskUserQuestion" {
		kind = interaction.KindChoose
	}
	return interaction.Interaction{
		ID:      id,
		Kind:    kind,
		Title:   buildPromptTitle(input),
		Body:    buildPromptBody(input),
		Timeout: timeout,
		Meta: map[string]any{
			"tool_name":  toolNameForHook(input),
			"tool_input": parseToolInput(input),
			"session_id": input.SessionID,
			"agent_type": string(agentboot.AgentTypeClaude),
			"reason":     fmt.Sprintf("Claude Code hook: %s", input.HookEventName),
		},
	}
}

func toolNameForHook(input HookInput) string {
	if input.ToolName != "" {
		return input.ToolName
	}
	return "ClaudeCode"
}

func parseToolInput(input HookInput) map[string]interface{} {
	out := map[string]interface{}{}
	if strings.TrimSpace(input.ToolInput) != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(input.ToolInput), &parsed); err == nil {
			for k, v := range parsed {
				out[k] = v
			}
		} else {
			out["_raw_input"] = input.ToolInput
		}
	}
	if input.LastAssistantMessage != "" {
		out["_last_assistant_message"] = input.LastAssistantMessage
	}
	if input.NotificationMessage != "" {
		out["_notification_message"] = input.NotificationMessage
	}
	return out
}

func buildPromptTitle(input HookInput) string {
	return "Claude Code · " + shortenPath(input.Cwd, 2)
}

func buildPromptBody(input HookInput) string {
	switch input.HookEventName {
	case "PreToolUse":
		if input.ToolName == "AskUserQuestion" {
			if input.LastAssistantMessage != "" {
				return input.LastAssistantMessage
			}
			return "Claude is asking a question"
		}
		if input.ToolName != "" {
			return fmt.Sprintf("Claude wants to run `%s`", input.ToolName)
		}
		return "Claude needs your approval"
	case "Notification":
		if input.NotificationMessage != "" {
			return input.NotificationMessage
		}
		return "Claude needs your attention"
	default:
		return input.HookEventName
	}
}

// buildPushText is the body of a notification for non-interactive events.
func buildPushText(input HookInput) string {
	switch input.HookEventName {
	case "Stop":
		if input.LastAssistantMessage != "" {
			return truncate(input.LastAssistantMessage, 240)
		}
		return "Task completed"
	case "PostToolUse":
		if input.ToolName != "" {
			return "Tool call finished: " + input.ToolName
		}
		return "Tool call finished"
	case "Notification":
		if input.NotificationMessage != "" {
			return input.NotificationMessage
		}
		return "Needs attention"
	default:
		return input.HookEventName
	}
}

// budgetDuration converts policy.TotalBudgetSeconds to a Duration with
// a 5-minute default.
func budgetDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

// normalizePolicy maps a configured policy string into Claude's
// permissionDecision values; unknown / empty defaults to "deny".
func normalizePolicy(policy string) string {
	switch policy {
	case "allow", "deny", "ask":
		return policy
	default:
		return "deny"
	}
}

// resultStatusFromError converts a prompt error into the status the
// long-poll endpoint reports.
func resultStatusFromError(err error) interaction.Status {
	if err == nil {
		return interaction.StatusAnswered
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "deadline") || strings.Contains(msg, "timeout") || strings.Contains(msg, "context cancel") {
		return interaction.StatusTimeout
	}
	return interaction.StatusError
}

// shortenPath keeps the last n segments of a path.
func shortenPath(p string, n int) string {
	p = strings.TrimRight(p, "/")
	segments := strings.Split(p, "/")
	if len(segments) <= n {
		return p
	}
	return strings.Join(segments[len(segments)-n:], "/")
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// Compile-time safety: confirm the plugin satisfies the scenario contract.
var _ scenario.Scenario = (*Plugin)(nil)
