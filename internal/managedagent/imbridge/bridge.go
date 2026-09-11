// Package imbridge hooks managed agent sessions into the remote notify
// pipeline: session milestones become one-way notifications and approval
// / ask requests become interactive prompts on whichever chat a Notify
// route with source "tasks" points at. It is deliberately thin — it uses
// the same scenario.Runtime the Claude Code hook plugin uses, so routing,
// authorization and rendering are the bot layer's, not ours
// (.design/managed-agent.md §7).
package imbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
)

type (
	channelT = channel.Channel
	targetT  = channel.Target
)

// ScenarioName is the route source users bind: POST /api/v1/bots/:bot/routes
// with {"source": "tasks"}.
const ScenarioName = "tasks"

// Event names a route's event_filter can select on.
const (
	EventStarted    = "started"
	EventNeedsInput = "needs_input"
	EventFinished   = "finished"
	EventFailed     = "failed"
)

// promptBudget bounds how long an IM prompt waits for an answer. The
// session itself keeps waiting; the prompt just stops occupying the chat.
const promptBudget = 2 * time.Hour

// Bridge implements scenario.Scenario (so it is visible to the registry and
// reachable from /tingly/tasks/notify) and observes the session event bus.
type Bridge struct {
	svc *managedagent.Service
	rt  scenario.Runtime
	log *logrus.Entry
	now func() time.Time
}

var _ scenario.Scenario = (*Bridge)(nil)

// New builds a bridge over the Service and the bot runtime.
func New(svc *managedagent.Service, rt scenario.Runtime) *Bridge {
	return &Bridge{svc: svc, rt: rt, log: logrus.WithField("component", "managedagent.imbridge"), now: time.Now}
}

// Name implements scenario.Scenario.
func (b *Bridge) Name() string { return ScenarioName }

// Trigger implements scenario.Scenario for HTTP-sourced events: a plain
// notification with a "text" payload, so scripts can post into the same
// route. In-process session events arrive through OnEvent instead.
func (b *Bridge) Trigger(ctx context.Context, ev scenario.Event, rt scenario.Runtime) (scenario.Outcome, error) {
	text, _ := ev.Payload["text"].(string)
	if strings.TrimSpace(text) == "" {
		return scenario.Outcome{}, nil
	}
	ch, target, ok, err := rt.Resolve(ctx, ev)
	if err != nil || !ok {
		return scenario.Outcome{}, err
	}
	_ = rt.Notify(ctx, ch, target, interaction.Notification{Title: "Tasks", Body: text})
	return scenario.Outcome{Handled: true}, nil
}

// OnEvent is the EventBus subscriber. It returns immediately; delivery
// runs on its own goroutine.
func (b *Bridge) OnEvent(e managedagent.Event) {
	switch e.Kind {
	case managedagent.EventUserMessage:
		if e.Seq == 1 {
			go b.notify(e, EventStarted)
		}
	case managedagent.EventApprovalRequest, managedagent.EventAskRequest:
		go b.prompt(e)
	case managedagent.EventStatus:
		// A bare "idle" is a completed turn. "idle: interrupted" (user stop)
		// and "idle: interrupted by restart" are not something to announce.
		switch {
		case e.Text == string(managedagent.SessionIdle):
			go b.notify(e, EventFinished)
		case strings.HasPrefix(e.Text, string(managedagent.SessionFailed)):
			go b.notify(e, EventFailed)
		}
	}
}

func (b *Bridge) resolve(ctx context.Context, sessionID, event string) (scenario.Event, bool) {
	ev := scenario.Event{
		Source:   "inproc",
		Scenario: ScenarioName,
		Payload:  map[string]any{"event": event, "session_id": sessionID},
	}
	ch, target, ok, err := b.rt.Resolve(ctx, ev)
	if err != nil {
		b.log.WithError(err).Warn("route resolution failed")
		return ev, false
	}
	if !ok {
		return ev, false
	}
	if ev.Meta == nil {
		ev.Meta = map[string]any{}
	}
	ev.Meta["__channel"] = ch
	ev.Meta["__target"] = target
	return ev, true
}

func (b *Bridge) notify(e managedagent.Event, event string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ev, ok := b.resolve(ctx, e.SessionID, event)
	if !ok {
		return
	}
	sess, err := b.svc.GetSession(ctx, e.SessionID)
	if err != nil {
		return
	}
	ws, _ := b.svc.GetWorkspace(ctx, sess.WorkspaceID)
	title, body := render(event, sess, ws)
	if event == EventFinished {
		// Like the @cc bridge: the agent's last words, with the turn's
		// activity folded into one line under them.
		if events, err := b.svc.ListEvents(ctx, sess.ID, 0, 0); err == nil {
			if summary := turnSummary(events); summary != "" {
				body = summary + "\n" + body
			}
		}
	}
	ch := ev.Meta["__channel"].(channelT)
	target := ev.Meta["__target"].(targetT)
	if err := b.rt.Notify(ctx, ch, target, interaction.Notification{Title: title, Body: body,
		Meta: map[string]any{"session_id": sess.ID, "event": event}}); err != nil {
		b.log.WithError(err).WithField("session", sess.ID).Warn("notify failed")
		return
	}
	b.rt.Audit("tasks.notify", map[string]any{"session_id": sess.ID, "event": event, "channel": ch.ID()})
}

// prompt turns an approval / ask request into a chat prompt and feeds the
// answer back through the Service. An answer that arrives after the web
// (or another chat) already answered is dropped: Respond reports the
// request as no longer pending.
func (b *Bridge) prompt(e managedagent.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), promptBudget)
	defer cancel()
	ev, ok := b.resolve(ctx, e.SessionID, EventNeedsInput)
	if !ok {
		return
	}
	sess, err := b.svc.GetSession(ctx, e.SessionID)
	if err != nil {
		return
	}
	ch := ev.Meta["__channel"].(channelT)
	target := ev.Meta["__target"].(targetT)

	ix := buildInteraction(e, sess)
	reply, err := b.rt.Ask(ctx, ch, target, ix)
	if err != nil {
		// Timeout or cancel: the session keeps waiting; the web can answer.
		b.rt.Audit("tasks.prompt.unanswered", map[string]any{"session_id": sess.ID, "request_id": e.RequestID, "err": err.Error()})
		return
	}
	resp := managedagent.Response{RequestID: e.RequestID}
	if e.Kind == managedagent.EventAskRequest {
		resp.Approved = true
		resp.Answer = strings.TrimSpace(reply.FreeText)
		if resp.Answer == "" {
			resp.Answer = reply.Selected
		}
	} else {
		resp.Approved = reply.Status == interaction.StatusAnswered && reply.Selected == "allow"
	}
	if err := b.svc.Respond(ctx, sess.ID, resp); err != nil {
		if errors.Is(err, managedagent.ErrNotFound) || errors.Is(err, managedagent.ErrConflict) {
			return // answered elsewhere first
		}
		b.log.WithError(err).WithField("session", sess.ID).Warn("respond failed")
		return
	}
	b.rt.Audit("tasks.prompt.answered", map[string]any{"session_id": sess.ID, "request_id": e.RequestID, "channel": ch.ID()})
}

func buildInteraction(e managedagent.Event, sess *managedagent.Session) interaction.Interaction {
	var payload map[string]any
	_ = json.Unmarshal(e.Payload, &payload)
	if e.Kind == managedagent.EventAskRequest {
		body := e.Text
		if body == "" {
			body, _ = payload["message"].(string)
		}
		return interaction.Interaction{
			ID:      "task-" + sess.ID + "-" + e.RequestID,
			Kind:    interaction.KindAsk,
			Title:   "Task · " + sess.Title,
			Body:    body,
			Timeout: promptBudget,
			Meta:    map[string]any{"session_id": sess.ID, "request_id": e.RequestID},
		}
	}
	tool := e.Text
	if tool == "" {
		tool, _ = payload["tool"].(string)
	}
	detail := briefInput(payload["input"])
	body := "The agent wants to run " + tool
	if detail != "" {
		body += ":\n" + detail
	}
	return interaction.Interaction{
		ID:    "task-" + sess.ID + "-" + e.RequestID,
		Kind:  interaction.KindConfirm,
		Title: "Task · " + sess.Title,
		Body:  body,
		Options: []interaction.Option{
			{Value: "allow", Label: "Allow", Style: "primary"},
			{Value: "deny", Label: "Deny", Style: "danger"},
		},
		Timeout: promptBudget,
		Meta:    map[string]any{"session_id": sess.ID, "request_id": e.RequestID},
	}
}

func render(event string, sess *managedagent.Session, ws *managedagent.Workspace) (title, body string) {
	title = "Task · " + sess.Title
	branch := sess.Artifact.Branch
	if ws != nil && ws.Branch != "" {
		branch = ws.Branch
	}
	switch event {
	case EventStarted:
		body = "Started.\n" + sess.Prompt
	case EventFinished:
		body = fmt.Sprintf("Finished its turn — %d changed file(s) on %s.", sess.Artifact.Changed, branch)
		if sess.Artifact.Pushed {
			body += "\nBranch pushed."
		}
	case EventFailed:
		body = "Failed"
		if sess.Error != "" {
			body += ": " + sess.Error
		}
	default:
		body = event
	}
	return title, body
}

func briefInput(input any) string {
	m, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	for _, k := range []string{"command", "file_path", "path", "pattern", "url", "description"} {
		if v, ok := m[k].(string); ok && v != "" {
			if len(v) > 300 {
				v = v[:297] + "…"
			}
			return v
		}
	}
	raw, _ := json.Marshal(m)
	if len(raw) > 300 {
		return string(raw[:297]) + "…"
	}
	return string(raw)
}

// turnSummary renders the last turn as IM shows it: the final assistant
// message (trimmed) and, folded under it, how many tool calls it took.
// The full transcript stays in the web UI.
func turnSummary(events []managedagent.Event) string {
	start := 0
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == managedagent.EventUserMessage {
			start = i
			break
		}
	}
	var last string
	tools := 0
	for _, e := range events[start:] {
		switch e.Kind {
		case managedagent.EventAssistantMessage:
			last = e.Text
		case managedagent.EventToolUse:
			tools++
		}
	}
	const max = 1200
	if len(last) > max {
		last = last[:max] + "…"
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(last))
	if tools > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "(%d tool call(s) this turn)", tools)
	}
	return b.String()
}
