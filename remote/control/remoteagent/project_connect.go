package remoteagent

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
)

// /connect binds a chat to a Projects project or folder from the chat itself,
// as binding it on the web does; /help in a bound chat lists what Projects
// does there. A chat that never connects keeps @cc / @tb. See
// .design/projects-im.md, "M5 的落地".

// ProjectTarget is a project or folder a chat may be connected to.
type ProjectTarget struct {
	ID   string
	Name string
	// Folder is set for a folder; Path is its folder.
	Folder bool
	Path   string
}

// ProjectConnectPayload names the callbacks of the /connect buttons (payload:
// name, project id).
const ProjectConnectPayload = "pjc"

// projectsOn reports whether Projects is on.
func (h *BotHandler) projectsOn() bool {
	return h.projectRouter != nil && h.projectRouter.Enabled()
}

// routeProjectsChat handles the commands Projects owns while it is on:
// /connect in any chat, and /help in a bound chat. It reports whether the
// message was taken.
func (h *BotHandler) routeProjectsChat(hCtx HandlerContext, text string) bool {
	if !h.projectsOn() {
		return false
	}
	head, rest, _ := strings.Cut(strings.TrimSpace(text), " ")
	switch strings.ToLower(head) {
	case "/connect":
		if arg := strings.TrimSpace(rest); arg != "" {
			h.connectByNumber(hCtx, arg)
		} else {
			h.sendConnectPicker(hCtx)
		}
		return true
	case "/help", "/h", "/start":
		if !h.chatBoundToProject(hCtx) {
			return false
		}
		h.SendText(hCtx, projectsHelp(hCtx))
		return true
	}
	return false
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

// sendConnectPicker offers the projects and folders a chat may talk to: one
// button each where the platform has buttons, else a numbered list for
// /connect N.
func (h *BotHandler) sendConnectPicker(hCtx HandlerContext) {
	targets, err := h.projectRouter.Targets(h.ctx)
	if err != nil {
		h.SendText(hCtx, "Could not list projects: "+err.Error())
		return
	}
	if len(targets) == 0 {
		h.SendText(hCtx, "There is nothing to connect to yet: create a project, or start a session in a folder, in Projects on the web.")
		return
	}
	caps := imbot.GetPlatformCapabilities(string(hCtx.Platform))
	if caps == nil || !caps.SupportsInteraction() {
		lines := []string{"Reply /connect N to pick what this chat talks to:"}
		for i, t := range targets {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, targetLabel(t)))
		}
		h.SendText(hCtx, strings.Join(lines, "\n"))
		return
	}
	kb := imbot.NewKeyboardBuilder()
	for _, t := range targets {
		kb.AddRow(imbot.ActionButton(targetLabel(t), ProjectConnectPayload, t.ID))
	}
	if _, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:    "Pick what this chat talks to:",
		Actions: kb.BuildActions(),
	}); err != nil {
		logrus.WithError(err).WithField("chatID", hCtx.ChatID).Warn("Failed to send the /connect picker")
	}
}

// targetLabel names a project or folder in the picker.
func targetLabel(t ProjectTarget) string {
	if t.Folder {
		return "Folder: " + ShortenPath(t.Path)
	}
	return "Project: " + t.Name
}

// connectByNumber connects the chat to the Nth entry of the picker.
func (h *BotHandler) connectByNumber(hCtx HandlerContext, arg string) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		h.SendText(hCtx, "Send /connect to see the list, then /connect N to pick one.")
		return
	}
	targets, err := h.projectRouter.Targets(h.ctx)
	if err != nil {
		h.SendText(hCtx, "Could not list projects: "+err.Error())
		return
	}
	if n > len(targets) {
		h.SendText(hCtx, fmt.Sprintf("The list has %d entries. Send /connect to see it.", len(targets)))
		return
	}
	h.connectProject(hCtx, targets[n-1].ID)
}

// connectProject connects the chat to a project or folder it picked.
func (h *BotHandler) connectProject(hCtx HandlerContext, projectID string) {
	if !h.projectsOn() || projectID == "" {
		return
	}
	reply, err := h.projectRouter.Connect(h.ctx, h.projectInbound(hCtx, ""), projectID)
	if err != nil {
		h.SendText(hCtx, "Could not connect this chat: "+err.Error())
		return
	}
	h.SendText(hCtx, reply)
}

// projectsHelp is /help in a chat bound to Projects.
func projectsHelp(hCtx HandlerContext) string {
	return strings.Join([]string{
		"This chat talks to Projects. Send a message to talk to it.",
		"@N <message>: talk to thread #N once",
		"/threads: list the project's threads",
		"/focus N: stay with thread #N; /back: return to the one above",
		"/new [message]: start a new session (in a folder)",
		"/stop [#N]: stop the running turn",
		"/clear: stop, and start a new session with the next message (in a folder)",
		"/connect: connect this chat to another project or folder",
		"",
		"Your ID: " + hCtx.SenderID,
		"Chat ID: " + hCtx.ChatID,
	}, "\n")
}
