package remoteagent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
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
	// RouteAnswer answers a Projects thread's approval or question with the
	// button a chat pressed (payload ProjectAnswerPayload, thread id, request
	// id, choice). reply, when set, is what to tell the chat; the prompt
	// itself is restated by the project.
	RouteAnswer(ctx context.Context, in ProjectInbound, threadID, requestID, choice string) (reply string, err error)
}

// ProjectAnswerPayload names the callbacks of a Projects prompt's buttons.
const ProjectAnswerPayload = "pj"

// ProjectInbound is one message from a chat.
type ProjectInbound struct {
	Platform string
	BotUUID  string
	ChatID   string
	SenderID string
	Text     string
	// Files are what the chat sent with the message, saved on local disk;
	// they are removed once RouteInbound returns.
	Files []ProjectFile
}

// ProjectFile is a file a chat sent, saved at Path under its own Name.
type ProjectFile struct {
	Name string
	Path string
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
	return h.deliverToProject(hCtx, text, nil)
}

// deliverToProject hands a message to the chat's project and says why when
// it could not be delivered; it reports whether the chat is bound.
func (h *BotHandler) deliverToProject(hCtx HandlerContext, text string, files []ProjectFile) bool {
	handled, err := h.projectRouter.RouteInbound(h.ctx, ProjectInbound{
		Platform: string(hCtx.Platform),
		BotUUID:  hCtx.BotUUID,
		ChatID:   hCtx.ChatID,
		SenderID: hCtx.SenderID,
		Text:     text,
		Files:    files,
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

// routeMediaToProject sends a bound chat's files, with their caption, to its
// project: each is downloaded as the @cc path would (same types and sizes),
// handed over, then removed. Nothing is sent when one cannot be taken.
func (h *BotHandler) routeMediaToProject(hCtx HandlerContext, mc *imbot.MediaContent) {
	if len(mc.Media) == 0 {
		h.SendText(hCtx, "This message has no file in it.")
		return
	}
	if strings.HasPrefix(mc.Media[0].URL, "tgfile://") {
		if token := h.botSetting.Auth["token"]; token != "" {
			h.fileStore.SetTelegramToken(token)
		}
	}
	dir, err := os.MkdirTemp("", "tb-chat-files-")
	if err != nil {
		h.SendText(hCtx, "Could not save the files: "+err.Error())
		return
	}
	defer os.RemoveAll(dir)
	files := make([]ProjectFile, 0, len(mc.Media))
	for _, m := range mc.Media {
		if !h.fileStore.IsAllowedType(m.MimeType) {
			h.SendText(hCtx, fmt.Sprintf("File type not supported: %s", m.MimeType))
			return
		}
		stored, err := h.fileStore.DownloadFile(h.ctx, dir, m.URL, m.MimeType)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Failed to download file: %v", err))
			return
		}
		name := m.Filename
		if name == "" {
			name = stored.Filename
		}
		files = append(files, ProjectFile{Name: name, Path: stored.Path})
	}
	h.reactReceived(hCtx)
	h.deliverToProject(hCtx, strings.TrimSpace(mc.Caption), files)
}

// chatBoundToProject reports whether the chat talks to a project.
func (h *BotHandler) chatBoundToProject(hCtx HandlerContext) bool {
	return h.projectRouter != nil && h.projectRouter.Bound(string(hCtx.Platform), hCtx.ChatID)
}

// projectCommand reads a chat command a bound chat sends to its project: the
// stop words the old routing knew (/stop, stop, /interrupt, /clear; /stop
// may name a thread, "/stop #2"), /new with an optional first message, and
// the thread addressing commands /focus N, /back and /threads. ok is false for
// anything else.
func projectCommand(text string) (name, arg string, ok bool) {
	trimmed := strings.TrimSpace(text)
	head, rest, _ := strings.Cut(trimmed, " ")
	rest = strings.TrimSpace(rest)
	switch strings.ToLower(head) {
	case "/stop":
		return "stop", rest, true
	case "stop", "/interrupt":
		if rest == "" {
			return "stop", "", true
		}
	case "/clear", "/back", "/threads":
		if rest == "" {
			return strings.TrimPrefix(strings.ToLower(head), "/"), "", true
		}
	case "/quiet", "/noverbose":
		if rest == "" {
			return "verbose", "off", true
		}
	case "/focus", "/new", "/verbose":
		return strings.TrimPrefix(strings.ToLower(head), "/"), rest, true
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

// routeProjectAnswer hands a press on a Projects prompt to the project.
func (h *BotHandler) routeProjectAnswer(hCtx HandlerContext, threadID, requestID, choice string) {
	if h.projectRouter == nil || threadID == "" || requestID == "" || choice == "" {
		return
	}
	reply, err := h.projectRouter.RouteAnswer(h.ctx, ProjectInbound{
		Platform: string(hCtx.Platform),
		BotUUID:  hCtx.BotUUID,
		ChatID:   hCtx.ChatID,
		SenderID: hCtx.SenderID,
	}, threadID, requestID, choice)
	if err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to answer a project's request")
		h.SendText(hCtx, "Could not answer: "+err.Error())
		return
	}
	if reply != "" {
		h.SendText(hCtx, reply)
	}
}
