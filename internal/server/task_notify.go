package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/pkg/notify"
	systemnotify "github.com/tingly-dev/tingly-box/pkg/notify/provider/system"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	remotescenario "github.com/tingly-dev/tingly-box/remote/scenario"

	coretask "github.com/tingly-dev/tingly-box/internal/task"
	"github.com/tingly-dev/tingly-box/internal/task/agenttask"
)

// taskNotificationScenario is the binding key users attach a chat to for
// task lifecycle pushes (same mechanism as the claudecode hook scenario).
const taskNotificationScenario = "task"

// taskTransitionObserver pushes settled task transitions to the user. An
// unattended task that pauses (needs_input / handoff_required) must reach a
// human — that is the half of the unattended loop the page alone cannot
// provide (design: .design/task-board.md §10-3). Delivery prefers the bound
// IM channel; without a binding it falls back to a desktop notification.
func (s *Server) taskTransitionObserver() coretask.TransitionObserver {
	desktop := notify.NewMultiplexer()
	desktop.AddProvider(systemnotify.New(systemnotify.Config{AppName: "Tingly Box"}))

	return func(t coretask.Task, status coretask.TaskStatus, result json.RawMessage, errMsg string) {
		title, body := taskNotificationContent(t, status, result, errMsg)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Resolve lazily: the scenario runtime is wired during route setup,
		// which may happen after the task runtime starts.
		if runtime := s.scenarioRuntime; runtime != nil {
			ev := remotescenario.Event{
				Source:   "task",
				Scenario: taskNotificationScenario,
				Payload:  map[string]any{"event": string(status), "task_id": t.ID},
			}
			ch, target, ok, err := runtime.Resolve(ctx, ev)
			if err != nil {
				logrus.WithError(err).Warn("task notify: binding resolve failed")
			} else if ok {
				msg := interaction.Notification{
					Title: title,
					Body:  body,
					Meta:  map[string]any{"task_id": t.ID, "status": string(status)},
				}
				if err := runtime.Notify(ctx, ch, target, msg); err != nil {
					logrus.WithError(err).Warn("task notify: channel delivery failed")
				}
				return
			}
		}
		if _, err := desktop.Send(ctx, &notify.Notification{Title: title, Message: body, Category: "task"}); err != nil {
			logrus.WithError(err).Debug("task notify: desktop notification failed")
		}
	}
}

// taskNotificationContent renders a transition into a short, actionable
// message: what settled, why, and the artifact for the next action
// (question to answer, or the native takeover command for handoffs).
func taskNotificationContent(t coretask.Task, status coretask.TaskStatus, result json.RawMessage, errMsg string) (string, string) {
	subject := taskSubject(t)
	var lines []string

	var parsed struct {
		Summary  string `json:"summary"`
		Question string `json:"question"`
	}
	if len(result) > 0 {
		_ = json.Unmarshal(result, &parsed)
	}

	var title string
	switch status {
	case coretask.StatusSucceeded:
		title = fmt.Sprintf("Task done: %s", subject)
	case coretask.StatusFailed:
		title = fmt.Sprintf("Task failed: %s", subject)
		if errMsg != "" {
			lines = append(lines, clipForNotification(errMsg, 300))
		}
	case coretask.StatusNeedsInput:
		title = fmt.Sprintf("Task waiting for you: %s", subject)
	case coretask.StatusHandoff:
		title = fmt.Sprintf("Task needs takeover: %s", subject)
	default:
		title = fmt.Sprintf("Task %s: %s", status, subject)
	}

	if parsed.Summary != "" {
		lines = append(lines, clipForNotification(parsed.Summary, 400))
	}
	if parsed.Question != "" {
		lines = append(lines, "Q: "+clipForNotification(parsed.Question, 300))
	}
	if status == coretask.StatusHandoff {
		if cmd := taskResumeCommand(t); cmd != "" {
			lines = append(lines, "Take over: "+cmd)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, string(status))
	}
	return title, strings.Join(lines, "\n")
}

// taskSubject derives a human-readable subject from the task payload.
func taskSubject(t coretask.Task) string {
	switch t.Type {
	case agenttask.TaskType:
		var payload agenttask.Payload
		if json.Unmarshal(t.Payload, &payload) == nil {
			if title := strings.TrimSpace(payload.Title); title != "" {
				return clipForNotification(title, 80)
			}
			if goal := strings.TrimSpace(payload.Goal); goal != "" {
				return clipForNotification(goal, 80)
			}
		}
	default:
		var payload struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(t.Payload, &payload) == nil && strings.TrimSpace(payload.Command) != "" {
			return clipForNotification(payload.Command, 80)
		}
	}
	return t.ID
}

// taskResumeCommand mirrors the module handler's native takeover command so
// a handoff notification carries the artifact, not just the news.
func taskResumeCommand(t coretask.Task) string {
	if t.Type != agenttask.TaskType {
		return ""
	}
	var payload agenttask.Payload
	if json.Unmarshal(t.Payload, &payload) != nil {
		return ""
	}
	if payload.WorkspacePath == "" || payload.SessionID == "" {
		return ""
	}
	switch payload.Agent {
	case agenttask.AgentClaude:
		return fmt.Sprintf("cd %s && claude --resume %s", payload.WorkspacePath, payload.SessionID)
	case agenttask.AgentCodex:
		return fmt.Sprintf("cd %s && codex resume %s", payload.WorkspacePath, payload.SessionID)
	}
	return ""
}

func clipForNotification(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
