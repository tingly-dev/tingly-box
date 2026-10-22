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
	// Enabled reports whether chats are handled by Projects at all (its
	// switch). When it is off a chat gets no Claude Code, only pairing and
	// notifications.
	Enabled() bool
	// ToFolder moves the chat to a folder: its next message starts a new
	// session there. With ifNew a chat that already has a place is left alone.
	ToFolder(ctx context.Context, in ProjectInbound, folder string, ifNew bool) error
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

// chatControlOff is what a chat hears while Projects is switched off.
const chatControlOff = "Chatting with Claude Code from here is off. Turn on Projects in Tingly Box to use it."

// adoptChat runs before anything else a chat sends when the host has
// Projects. With Projects off it answers that chat control is off and takes
// the message (pairing and /join still work). With it on, a chat that has no
// place in Projects yet gets one, its folder (/cd) or the bot's default
// folder, so everything after this treats it as a bound chat. It reports
// whether the message was taken.
func (h *BotHandler) adoptChat(hCtx HandlerContext) bool {
	if h.projectRouter == nil {
		return false
	}
	if !h.projectRouter.Enabled() {
		if text := hCtx.Text(); isBindCommand(text) || isJoinCommand(text) {
			return false
		}
		h.SendText(hCtx, chatControlOff)
		return true
	}
	in := h.projectInbound(hCtx, "")
	folder, ok := h.getProjectPath(hCtx)
	if ok {
		if err := h.projectRouter.ToFolder(h.ctx, in, folder, true); err == nil {
			return false
		} else {
			logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("The chat's folder cannot be used; trying the default")
		}
	}
	if err := h.projectRouter.ToFolder(h.ctx, in, h.defaultProjectPath(), true); err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to give the chat a folder in Projects")
	}
	return false
}

// moveChatToFolder moves a chat that changed folder (/cd, /dir) there in
// Projects, so its next message starts a session in it.
func (h *BotHandler) moveChatToFolder(hCtx HandlerContext, folder string) {
	if h.projectRouter == nil || !h.projectRouter.Enabled() {
		return
	}
	if err := h.projectRouter.ToFolder(h.ctx, h.projectInbound(hCtx, ""), folder, false); err != nil {
		h.SendText(hCtx, fmt.Sprintf("Could not move this chat to %s: %v", folder, err))
	}
}

func (h *BotHandler) projectInbound(hCtx HandlerContext, text string) ProjectInbound {
	return ProjectInbound{
		Platform: string(hCtx.Platform),
		BotUUID:  hCtx.BotUUID,
		ChatID:   hCtx.ChatID,
		SenderID: hCtx.SenderID,
		Text:     text,
	}
}

// stripAgentPrefix drops the old agent words ("@cc fix it", "/tb hi"): every
// chat now talks to Claude Code in its folder.
func stripAgentPrefix(text string) string {
	t := strings.TrimSpace(text)
	for _, p := range []string{"@cc", "/cc", "@tb", "/tb"} {
		if t == p {
			return ""
		}
		if rest, ok := strings.CutPrefix(t, p+" "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return text
}

// routeToProject sends a chat's text to its bound project. It reports whether
// the message was taken; only a chat with a binding is ever taken, so every
// other chat keeps the @cc / @tb routing untouched.
func (h *BotHandler) routeToProject(hCtx HandlerContext, text string) bool {
	if h.projectRouter == nil {
		return false
	}
	if text = stripAgentPrefix(text); text == "" {
		if !h.projectRouter.Bound(string(hCtx.Platform), hCtx.ChatID) {
			return false
		}
		h.SendText(hCtx, "This chat talks to Claude Code; just send your message.")
		return true
	}
	return h.deliverToProject(hCtx, text, nil)
}

// deliverToProject hands a message to the chat's project and says why when
// it could not be delivered; it reports whether the chat is bound.
func (h *BotHandler) deliverToProject(hCtx HandlerContext, text string, files []ProjectFile) bool {
	in := h.projectInbound(hCtx, text)
	in.Files = files
	handled, err := h.projectRouter.RouteInbound(h.ctx, in)
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

// projectCommand reads a chat command that Projects runs: /new and /stop
// (with an optional argument), /clear, /session [N], /status, /verbose
// [on|off] and /yolo. ok is false for anything else.
func projectCommand(text string) (name, arg string, ok bool) {
	head, rest, _ := strings.Cut(strings.TrimSpace(text), " ")
	rest = strings.TrimSpace(rest)
	switch head = strings.ToLower(head); head {
	case "/new", "/stop", "/session", "/verbose":
		return strings.TrimPrefix(head, "/"), rest, true
	case "/clear", "/status", "/yolo":
		if rest == "" {
			return strings.TrimPrefix(head, "/"), "", true
		}
	}
	return "", "", false
}

// hostCommands are the commands a chat keeps outside Projects: which folder
// (/cd, /dir) and access (/bind, /join). /help and /start are answered with
// the Projects help.
var hostCommands = map[string]bool{"/cd": true, "/dir": true, "/bind": true, "/join": true, "/help": true, "/start": true}

// projectsOn reports whether chats are handled by Projects.
func (h *BotHandler) projectsOn() bool {
	return h.projectRouter != nil && h.projectRouter.Enabled()
}

// unknownCommand answers a slash command Projects mode does not have (the old
// /bash, /resume, /project, ...). It reports whether it did.
func (h *BotHandler) unknownCommand(hCtx HandlerContext, text string) bool {
	if !h.projectsOn() || !strings.HasPrefix(text, "/") || stripAgentPrefix(text) != text || h.directoryBrowser.IsWaitingInput(hCtx.ChatID) {
		return false
	}
	head, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	if hostCommands[strings.ToLower(head)] {
		return false
	}
	h.SendText(hCtx, "Unknown command "+head+". /help lists what works here.")
	return true
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
