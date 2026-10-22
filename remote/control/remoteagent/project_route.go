package remoteagent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
)

// ProjectRouter is how a chat talks to Claude Code: through Projects, where
// every chat has a folder or a session (.design/remote-into-projects.md). The
// host implements it (see internal/server/module/projects); a BotHandler
// without one, or with Projects switched off, gives chats access and
// notifications only.
type ProjectRouter interface {
	// RouteInbound hands a chat's text to its session. handled is false when
	// the chat has no place in Projects. When handled is true Projects owns the message:
	// its reply comes back through the host's outbound path, and err, if set,
	// is why the message could not be delivered (for example a paused
	// project) and is shown to the chat.
	RouteInbound(ctx context.Context, in ProjectInbound) (handled bool, err error)
	// RouteCommand runs a session command (see projectCommand); in.Text is
	// the text after the command. handled is false when the chat has no
	// place in Projects. reply is
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

// SetProjectRouter plugs in the router; nil turns chat control off.
func (h *BotHandler) SetProjectRouter(r ProjectRouter) { h.projectRouter = r }

// chatControlOff is what a chat hears while Projects is switched off.
const chatControlOff = "Chatting with Claude Code from here is off. Turn on Projects in Tingly Box to use it."

// noPlace is what a chat hears when it has no place in Projects, because
// neither its folder nor the bot's default could be used.
const noPlace = "This chat has no folder to work in yet. /cd <path> picks one."

// projectsOn reports whether chats are handled by Projects.
func (h *BotHandler) projectsOn() bool {
	return h.projectRouter != nil && h.projectRouter.Enabled()
}

// adoptChat gives a chat that has no place in Projects yet one: its folder
// (/cd) or the bot's default folder.
func (h *BotHandler) adoptChat(hCtx HandlerContext) {
	in := h.projectInbound(hCtx, "")
	if folder, ok := h.getProjectPath(hCtx); ok {
		err := h.projectRouter.ToFolder(h.ctx, in, folder, true)
		if err == nil {
			return
		}
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("The chat's folder cannot be used; trying the default")
	}
	if err := h.projectRouter.ToFolder(h.ctx, in, h.defaultProjectPath(), true); err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to give the chat a folder in Projects")
	}
}

// moveChatToFolder moves a chat that changed folder (/cd, /dir) there in
// Projects, so its next message starts a session in it. It reports whether
// it did, having said why not.
func (h *BotHandler) moveChatToFolder(hCtx HandlerContext, folder string) bool {
	if !h.projectsOn() {
		h.SendText(hCtx, chatControlOff)
		return false
	}
	if err := h.projectRouter.ToFolder(h.ctx, h.projectInbound(hCtx, ""), folder, false); err != nil {
		h.SendText(hCtx, fmt.Sprintf("Could not move this chat to %s: %v", folder, err))
		return false
	}
	return true
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
// chat talks to Claude Code in its folder.
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

// routeToProject sends a chat's text to its session.
func (h *BotHandler) routeToProject(hCtx HandlerContext, text string) {
	if text = stripAgentPrefix(text); text == "" {
		h.SendText(hCtx, "This chat talks to Claude Code; just send your message.")
		return
	}
	h.deliverToProject(hCtx, text, nil)
}

// deliverToProject hands a message to the chat's session and says why when
// it could not be delivered.
func (h *BotHandler) deliverToProject(hCtx HandlerContext, text string, files []ProjectFile) {
	in := h.projectInbound(hCtx, text)
	in.Files = files
	handled, err := h.projectRouter.RouteInbound(h.ctx, in)
	switch {
	case !handled:
		h.SendText(hCtx, noPlace)
	case err != nil:
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to deliver chat message to its project")
		h.SendText(hCtx, "Could not send this to the project: "+err.Error())
	}
}

// routeMediaToProject sends a chat's files, with their caption, to its
// session: each is downloaded (FileStore's types and sizes), handed over,
// then removed. Nothing is sent when one cannot be taken.
func (h *BotHandler) routeMediaToProject(hCtx HandlerContext, mc *imbot.MediaContent) {
	if mc == nil || len(mc.Media) == 0 {
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

// runProjectCommand runs a session command in Projects and tells the chat
// the result.
func (h *BotHandler) runProjectCommand(hCtx HandlerContext, name, arg string) {
	if !h.projectsOn() {
		h.SendText(hCtx, chatControlOff)
		return
	}
	handled, reply, err := h.projectRouter.RouteCommand(h.ctx, name, h.projectInbound(hCtx, arg))
	switch {
	case !handled:
		h.SendText(hCtx, noPlace)
	case err != nil:
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to run a chat command in its project")
		h.SendText(hCtx, "Could not do that: "+err.Error())
	case reply != "":
		h.SendText(hCtx, reply)
	}
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
