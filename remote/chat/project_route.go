package chat

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/agent"
	bot2 "github.com/tingly-dev/tingly-box/remote/bot"
	"github.com/tingly-dev/tingly-box/remote/commands"
)

// ProjectRouter is how a chat talks to Claude Code: through Projects, where
// every chat has a folder or a session (.design/remote-into-projects.md). It
// is the agent port (agent.Chat) plus the host's switch; the host
// implements it (see internal/server/module/projects). A BotHandler without
// one, or with Projects switched off, gives chats access and notifications
// only.
type ProjectRouter interface {
	agent.Chat
	// Enabled reports whether chats are handled by Projects at all (its
	// switch). When it is off a chat gets no Claude Code, only pairing and
	// notifications.
	Enabled() bool
}

// ProjectAnswerPayload names the callbacks of a Projects prompt's buttons.
const ProjectAnswerPayload = agent.AnswerPayload

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
	ref := h.folderRef(hCtx)
	if folder, ok := h.getProjectPath(hCtx); ok {
		err := h.projectRouter.ToFolder(ref, folder, true)
		if err == nil {
			return
		}
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("The chat's folder cannot be used; trying the default")
	}
	if err := h.projectRouter.ToFolder(ref, h.defaultProjectPath(), true); err != nil {
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
	if err := h.projectRouter.ToFolder(h.folderRef(hCtx), folder, false); err != nil {
		h.SendText(hCtx, fmt.Sprintf("Could not move this chat to %s: %v", folder, err))
		return false
	}
	return true
}

func (h *BotHandler) chatRef(hCtx HandlerContext) agent.ChatRef {
	return agent.ChatRef{
		Platform: string(hCtx.Platform),
		BotUUID:  hCtx.BotUUID,
		ChatID:   hCtx.ChatID,
	}
}

// folderRef is the chat moving to a folder, with the bot's profile.
func (h *BotHandler) folderRef(hCtx HandlerContext) agent.ChatRef {
	ref := h.chatRef(hCtx)
	ref.Profile = botProfile(h.currentSetting().DefaultAgent)
	return ref
}

// botProfile is the Claude Code profile a bot's default agent names:
// "claude_code:<id>" is profile <id>; "" and "claude_code" are the default.
func botProfile(defaultAgent string) string {
	id, _ := strings.CutPrefix(strings.TrimSpace(defaultAgent), "claude_code:")
	if id == "claude_code" {
		return ""
	}
	return id
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
func (h *BotHandler) deliverToProject(hCtx HandlerContext, text string, files []agent.File) {
	ref := h.chatRef(hCtx)
	if h.answerAllowed != nil {
		ref.AuthorizeAnswer = func() bool {
			return h.answerAllowed(hCtx.Platform, hCtx.BotUUID, hCtx.ChatID, hCtx.SenderID, hCtx.Message.IsGroupMessage())
		}
	}
	handled, err := h.projectRouter.Post(ref, text, files)
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
	files := make([]agent.File, 0, len(mc.Media))
	for _, m := range mc.Media {
		if !h.fileStore.IsAllowedType(m.MimeType) {
			kind := m.MimeType
			if kind == "" {
				kind = m.Filename
			}
			h.SendText(hCtx, fmt.Sprintf("File type not supported: %s", kind))
			return
		}
		stored, err := h.fileStore.DownloadFile(h.ctx, dir, m.URL, m.MimeType, m.Filename)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Failed to download file: %v", err))
			return
		}
		files = append(files, agent.File{Name: stored.Filename, Path: stored.Path})
	}
	h.reactReceived(hCtx)
	h.deliverToProject(hCtx, strings.TrimSpace(mc.Caption), files)
}

// projectDirectory lists projects or switches to the coordinator shown in the
// last list. Snapshotting the IDs prevents concurrent list reorder from
// selecting a different project than the user saw.
func (h *BotHandler) projectDirectory(hCtx HandlerContext, arg string) {
	if !h.projectsOn() {
		h.SendText(hCtx, "Project selection is not available.")
		return
	}
	ref := h.chatRef(hCtx)
	arg = strings.TrimSpace(arg)
	if arg == "" {
		choices, err := h.projectRouter.Projects()
		if err != nil {
			h.SendText(hCtx, "Could not list projects: "+err.Error())
			return
		}
		place, err := h.projectRouter.Place(ref)
		if err != nil {
			h.SendText(hCtx, "Could not list projects: "+err.Error())
			return
		}
		current := place.ProjectID
		h.projectListsMu.Lock()
		if h.projectLists == nil {
			h.projectLists = map[string][]agent.ProjectChoice{}
		}
		h.projectLists[hCtx.ChatID] = append([]agent.ProjectChoice(nil), choices...)
		h.projectListsMu.Unlock()
		if len(choices) == 0 {
			h.SendText(hCtx, "No projects are available.")
			return
		}
		var out strings.Builder
		out.WriteString("Projects:\n")
		for i, p := range choices {
			marker := ""
			if p.ID == current {
				marker = " ✓"
			}
			fmt.Fprintf(&out, "  %d. %s%s\n", i+1, p.Name, marker)
		}
		out.WriteString("\n/project <number> switches to its coordinator.")
		h.SendText(hCtx, out.String())
		return
	}
	n, valid := bot2.ParseListNumber(arg)
	h.projectListsMu.Lock()
	list := append([]agent.ProjectChoice(nil), h.projectLists[hCtx.ChatID]...)
	h.projectListsMu.Unlock()
	if !valid || n > len(list) {
		h.SendText(hCtx, "Use /project to list projects, then /project <number> to switch.")
		return
	}
	chosen := list[n-1]
	if err := h.projectRouter.ToProject(ref, chosen.ID); err != nil {
		h.SendText(hCtx, "Could not switch project: "+err.Error())
		return
	}
	h.SendText(hCtx, "This chat now talks to the coordinator of "+chosen.Name+".")
}

// projectCommand reads a chat command that Projects runs (see
// agent.Commands); the text after it is the argument. ok is false
// for anything else.
func projectCommand(text string, group bool) (name, arg string, ok bool) {
	cmd, ok := imbot.ParseSlashCommand(text, group)
	if !ok {
		return "", "", false
	}
	name = commands.Canonical(cmd.Name)
	if !isSessionCommand(name) {
		return "", "", false
	}
	return name, cmd.Args, true
}

// runProjectCommand runs a session command in Projects and tells the chat
// the result.
func (h *BotHandler) runProjectCommand(hCtx HandlerContext, name, arg string) {
	if !h.projectsOn() {
		h.SendText(hCtx, chatControlOff)
		return
	}
	handled, reply, err := h.projectRouter.Command(h.chatRef(hCtx), name, arg)
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
	reply, err := h.projectRouter.Answer(h.chatRef(hCtx), threadID, requestID, choice)
	if err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to answer a project's request")
		h.SendText(hCtx, "Could not answer: "+err.Error())
		return
	}
	if reply != "" {
		h.SendText(hCtx, reply)
	}
}
