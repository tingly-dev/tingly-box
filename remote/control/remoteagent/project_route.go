package remoteagent

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
)

// ProjectRouter is the optional seam through which a chat that was bound to a
// Projects project talks to that project's coordinator instead of @cc / @tb.
// The host implements it (see internal/server/module/projects); a BotHandler
// without one — the standalone CLI, the test harness — behaves exactly as it
// always did. See .design/projects-im.md.
type ProjectRouter interface {
	// Bound reports whether the chat is bound to a project.
	Bound(platform, chatID string) bool
	// RouteInbound hands a chat's text to the project it is bound to.
	// handled is false when the chat is not bound (the caller carries on with
	// the usual routing). When handled is true the project owns the message:
	// its reply comes back through the host's outbound path, and err, if set,
	// is why the message could not be delivered (for example a paused
	// project) and is shown to the chat.
	RouteInbound(ctx context.Context, in ProjectInbound) (handled bool, err error)
	// RouteCommand runs a chat command of a bound chat ("new", "stop",
	// "clear"); in.Text is the text after the command. handled is false when
	// the chat is not bound, and the caller carries on as before. reply is
	// what to tell the chat; err, if set, is why the command failed.
	RouteCommand(ctx context.Context, name string, in ProjectInbound) (handled bool, reply string, err error)
}

// ProjectInbound is one message from a chat.
type ProjectInbound struct {
	Platform string
	BotUUID  string
	ChatID   string
	SenderID string
	Text     string
}

// SetProjectRouter plugs in the router; nil turns it off.
func (h *BotHandler) SetProjectRouter(r ProjectRouter) { h.projectRouter = r }

// routeToProject sends a chat's text to its bound project. It reports whether
// the message was taken; only a chat with a binding is ever taken, so every
// other chat keeps the @cc / @tb routing untouched.
func (h *BotHandler) routeToProject(hCtx HandlerContext, text string) bool {
	if h.projectRouter == nil {
		return false
	}
	handled, err := h.projectRouter.RouteInbound(h.ctx, ProjectInbound{
		Platform: string(hCtx.Platform),
		BotUUID:  hCtx.BotUUID,
		ChatID:   hCtx.ChatID,
		SenderID: hCtx.SenderID,
		Text:     text,
	})
	if !handled {
		return false
	}
	if err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to deliver chat message to its project")
		h.SendText(hCtx, "Could not send this to the project: "+err.Error())
	}
	return true
}

// chatBoundToProject reports whether the chat talks to a project.
func (h *BotHandler) chatBoundToProject(hCtx HandlerContext) bool {
	return h.projectRouter != nil && h.projectRouter.Bound(string(hCtx.Platform), hCtx.ChatID)
}

// projectCommand reads a chat command a bound chat sends to its project: the
// stop words the old routing knew (/stop, stop, /interrupt, /clear) and /new
// with an optional first message. ok is false for anything else.
func projectCommand(text string) (name, arg string, ok bool) {
	trimmed := strings.TrimSpace(text)
	head, rest, _ := strings.Cut(trimmed, " ")
	switch strings.ToLower(head) {
	case "/stop", "stop", "/interrupt":
		if rest == "" {
			return "stop", "", true
		}
	case "/clear":
		if rest == "" {
			return "clear", "", true
		}
	case "/new":
		return "new", strings.TrimSpace(rest), true
	}
	return "", "", false
}

// routeProjectCommand runs a chat command in a bound chat. It reports whether
// the command was taken; an unbound chat's commands keep their old meaning.
func (h *BotHandler) routeProjectCommand(hCtx HandlerContext, text string) bool {
	if h.projectRouter == nil {
		return false
	}
	name, arg, ok := projectCommand(text)
	if !ok {
		return false
	}
	handled, reply, err := h.projectRouter.RouteCommand(h.ctx, name, ProjectInbound{
		Platform: string(hCtx.Platform),
		BotUUID:  hCtx.BotUUID,
		ChatID:   hCtx.ChatID,
		SenderID: hCtx.SenderID,
		Text:     arg,
	})
	if !handled {
		return false
	}
	if err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to run a chat command in its project")
		h.SendText(hCtx, "Could not do that in the project: "+err.Error())
		return true
	}
	if reply != "" {
		h.SendText(hCtx, reply)
	}
	return true
}
