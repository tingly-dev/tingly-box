// Package notify is the HTTP front end for scenario plugin events. It
// is intentionally thin: it parses the request, dispatches to the
// registered scenario plugin (via internal/remote/scenario), and maps
// the plugin's Outcome into HTTP responses (200 push / 202 + wait URL /
// 404 unknown).
//
// All business logic — what an event "means", how to render a prompt,
// how to encode a decision — lives in the plugin (see
// internal/remote/scenario/builtin/claudecode for the first one).
//
// When no scenario plugin is registered for the URL parameter (or the
// plugin chose not to handle the event) the handler falls back to a
// desktop notification through pkg/notify so stock setups without IM
// bindings still surface hook activity.
package notify

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/pkg/notify"
	systemnotify "github.com/tingly-dev/tingly-box/pkg/notify/provider/system"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
)

// Handler routes /tingly/:scenario/{notify,wait/:id} to the appropriate
// scenario plugin and the shared interaction registry.
type Handler struct {
	scenarios *scenario.Registry
	results   *interaction.Registry[interaction.Result]
	runtime   scenario.Runtime
	notifier  notify.Notifier
}

// NewHandler creates a handler with no scenario routing — every event
// falls back to a desktop notification. Used by stock setups before a
// scenario registry is wired in.
func NewHandler() *Handler {
	mux := notify.NewMultiplexer()
	mux.AddProvider(systemnotify.New(systemnotify.Config{AppName: "Tingly Box"}))
	return &Handler{notifier: mux}
}

// NewHandlerWithRouting wires the handler to a scenario registry, the
// shared interaction.Registry (used by Wait), and a runtime that
// exposes channels + bindings to plugins.
func NewHandlerWithRouting(scenarios *scenario.Registry, results *interaction.Registry[interaction.Result], runtime scenario.Runtime) *Handler {
	h := NewHandler()
	h.scenarios = scenarios
	h.results = results
	h.runtime = runtime
	return h
}

// Notify handles POST /tingly/:scenario/notify.
//
//	200: scenario plugin handled the event as a push (no reply expected).
//	202: scenario plugin started an interactive flow; client polls wait_url.
//	200 + desktop fallback: no plugin (or plugin declined to handle).
func (h *Handler) Notify(c *gin.Context) {
	scenarioName := c.Param("scenario")

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if h.scenarios != nil {
		plugin, ok := h.scenarios.Get(scenarioName)
		if ok && plugin != nil && h.runtime != nil {
			outcome, err := plugin.Trigger(c.Request.Context(), scenario.Event{
				Source:   "http",
				Scenario: scenarioName,
				Payload:  payload,
			}, h.runtime)
			if err != nil {
				logrus.WithError(err).WithField("scenario", scenarioName).Warn("scenario trigger failed")
				h.fallbackPush(payload)
				c.JSON(http.StatusOK, gin.H{"ok": true, "kind": "push"})
				return
			}
			if outcome.IsInteractive() {
				c.JSON(http.StatusAccepted, gin.H{
					"kind":       "interactive",
					"request_id": outcome.InteractionID,
					"wait_url":   "/tingly/" + scenarioName + "/wait/" + outcome.InteractionID,
					"expires_at": outcome.ExpiresAt.UTC().Format(time.RFC3339),
				})
				return
			}
			if outcome.Handled {
				c.JSON(http.StatusOK, gin.H{"ok": true, "kind": "push"})
				return
			}
		}
	}

	// No plugin handled it — desktop notify so stock setups still see
	// something.
	h.fallbackPush(payload)
	c.JSON(http.StatusOK, gin.H{"ok": true, "kind": "push"})
}

// Wait handles GET /tingly/:scenario/wait/:request_id?timeout=45s.
// Maps interaction.Result.Status to HTTP shape:
//
//	200 answered  — final decision available
//	200 cancelled — user cancelled
//	410 timeout   — fallback decision (policy on_timeout)
//	504 pending   — long-poll timed out without an answer; client retries
//	404 expired   — id is unknown / evicted
func (h *Handler) Wait(c *gin.Context) {
	if h.results == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	requestID := c.Param("request_id")
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing request_id"})
		return
	}

	timeout := parseTimeout(c.Query("timeout"))

	ch, ok := h.results.Await(requestID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"status": "expired"})
		return
	}

	select {
	case res := <-ch:
		respondResult(c, res)
	case <-time.After(timeout):
		c.JSON(http.StatusGatewayTimeout, gin.H{"status": "pending"})
	case <-c.Request.Context().Done():
		c.JSON(http.StatusGatewayTimeout, gin.H{"status": "pending"})
	}
}

func respondResult(c *gin.Context, res interaction.Result) {
	switch res.Status {
	case interaction.StatusAnswered:
		c.JSON(http.StatusOK, gin.H{"status": "answered", "decision": res.Decision})
	case interaction.StatusCancelled:
		c.JSON(http.StatusOK, gin.H{"status": "cancelled", "decision": res.Decision})
	case interaction.StatusTimeout:
		c.JSON(http.StatusGone, gin.H{"status": "timeout", "decision": res.Decision, "reason": res.Reason})
	default:
		c.JSON(http.StatusGone, gin.H{"status": string(res.Status), "decision": res.Decision, "reason": res.Reason})
	}
}

func parseTimeout(raw string) time.Duration {
	const (
		defaultTimeout = 45 * time.Second
		maxTimeout     = 50 * time.Second
	)
	if raw == "" {
		return defaultTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultTimeout
	}
	if d > maxTimeout {
		return maxTimeout
	}
	if d < time.Second {
		return time.Second
	}
	return d
}

// fallbackPush sends a desktop notification using a coarse summary of
// the event payload. This is the legacy behavior for stock setups
// without a scenario plugin / IM binding.
func (h *Handler) fallbackPush(payload map[string]any) {
	if h.notifier == nil {
		return
	}
	title, message := summarize(payload)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = h.notifier.Send(ctx, &notify.Notification{
			Title:   title,
			Message: message,
			Level:   notify.LevelInfo,
		})
	}()
}

// summarize is a generic title/body extractor for desktop fallback. It
// has no Claude-specific knowledge — that lives in the plugin.
func summarize(payload map[string]any) (string, string) {
	title := "Tingly Box"
	body := ""
	if event, ok := payload["hook_event_name"].(string); ok {
		body = event
	}
	if msg, ok := payload["last_assistant_message"].(string); ok && msg != "" {
		body = msg
	}
	if msg, ok := payload["notification_message"].(string); ok && msg != "" {
		body = msg
	}
	if cwd, ok := payload["cwd"].(string); ok && cwd != "" {
		title = title + " · " + cwd
	}
	return title, body
}
