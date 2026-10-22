package remoteagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
)

// RegisterBuiltinCommands registers the chat commands
// (.design/remote-into-projects.md, "命令精简"): which folder (/cd, /dir),
// the session commands Projects runs, /help, and access (/bind, /join, which
// /help does not list). The registry is also what a platform shows as its
// command menu.
func RegisterBuiltinCommands(registry *imbot.CommandRegistry, botHandler BotHandlerAdapter) error {
	commands := []imbot.Command{
		newHelpCommand(botHandler),
		newCdCommand(botHandler),
		newDirCommand(botHandler),
		newPairBindCommand(botHandler),
		newJoinCommand(botHandler),
	}
	for _, c := range sessionCommands {
		commands = append(commands, newSessionCommand(botHandler, c))
	}
	for _, cmd := range commands {
		if err := registry.Register(cmd); err != nil {
			return fmt.Errorf("failed to register command %s: %w", cmd.ID, err)
		}
	}
	return nil
}

// BotHandlerAdapter provides methods needed by command handlers.
// This allows commands to interact with the bot without direct coupling.
type BotHandlerAdapter interface {
	// SendText sends a text message to a chat
	SendText(chatID, text string) error

	// GetProjectPath gets the folder of a chat
	GetProjectPath(chatID string) (string, error)

	// SetProjectPath moves a chat to a folder, and says so in the chat
	SetProjectPath(chatID, path string) error

	// ListChatProjectPaths lists the folders a chat used, most recent first.
	ListChatProjectPaths(chatID string) ([]string, error)

	// IsWhitelisted checks if a group is whitelisted
	IsWhitelisted(groupID string) bool

	// AddToWhitelist adds a group to whitelist
	AddToWhitelist(groupID, platform, userID string) error

	// VerifyAndPair verifies a one-time pairing code and, on success, records
	// the chat as paired with the bot. Implementations should also emit the
	// matching audit events (success / failure).
	VerifyAndPair(botUUID, chatID, senderID, platform, code string) error

	// RunSessionCommand runs one of sessionCommands in Projects and tells the
	// chat the result.
	RunSessionCommand(ctx *imbot.HandlerContext, name, arg string)
}

const (
	usageCd       = "Usage: /cd <path|number>"
	usageJoin     = "Usage: /join <group_id|@username|invite_link>"
	usagePairBind = "Usage: /bind <code>"
)

func sendCommandText(adapter BotHandlerAdapter, ctx *imbot.HandlerContext, text string) error {
	return adapter.SendText(ctx.ChatID, text)
}

func sendCommandTextf(adapter BotHandlerAdapter, ctx *imbot.HandlerContext, format string, args ...interface{}) error {
	return sendCommandText(adapter, ctx, fmt.Sprintf(format, args...))
}

func joinedArgs(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	value := strings.TrimSpace(strings.Join(args, " "))
	return value, value != ""
}

func parsePairBindCode(args []string) (string, bool) {
	if len(args) < 1 {
		return "", false
	}
	code := strings.TrimSpace(args[0])
	if code == "" {
		return "", false
	}
	return code, true
}

// sessionCommand is a command Projects runs (see projectCommand).
type sessionCommand struct {
	name, description string
	priority          int
}

// sessionCommands are the commands Projects runs, in /help order.
var sessionCommands = []sessionCommand{
	{"session", "List the sessions here, or switch to #N", 80},
	{"new", "Start a new session (with this message, or the next one)", 79},
	{"stop", "Stop the running session (or #N)", 78},
	{"clear", "Stop the session; the next message starts a new one", 77},
	{"status", "Where this chat is and how its session runs", 76},
	{"verbose", "Also see each step a session takes (on|off)", 75},
	{"yolo", "Let the session use every tool without asking (toggle)", 74},
}

func newSessionCommand(adapter BotHandlerAdapter, c sessionCommand) imbot.Command {
	return imbot.NewCommand("cmd-"+c.name, c.name, c.description).
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			arg, _ := joinedArgs(args)
			adapter.RunSessionCommand(ctx, c.name, arg)
			return nil
		}).
		WithCategory("session").
		WithPriority(c.priority).
		MustBuild()
}

func newHelpCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-help", "help", "Show what this chat can do").
		WithAliases("start").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			return sendCommandText(adapter, ctx, helpText(ctx.SenderID, ctx.ChatID))
		}).
		WithCategory("system").
		WithPriority(100).
		MustBuild()
}

func newCdCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-cd", "cd", "Work in another folder").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			input, ok := joinedArgs(args)
			if !ok {
				return sendCommandText(adapter, ctx, usageCd)
			}
			return changeDir(adapter, ctx, input)
		}).
		WithCategory("project").
		WithPriority(90).
		MustBuild()
}

// changeDir moves the chat to a folder: input is a path (relative to the
// current one) or the number of a folder /dir listed. SetProjectPath says
// where the chat is now.
func changeDir(adapter BotHandlerAdapter, ctx *imbot.HandlerContext, input string) error {
	if idx, ok := parsePositiveInt(input); ok {
		paths, err := adapter.ListChatProjectPaths(ctx.ChatID)
		if err != nil {
			return sendCommandTextf(adapter, ctx, "Failed to list folders: %v", err)
		}
		if idx < 1 || idx > len(paths) {
			return sendCommandTextf(adapter, ctx, "No folder %d (there are %d). /dir lists them.", idx, len(paths))
		}
		return adapter.SetProjectPath(ctx.ChatID, paths[idx-1])
	}

	currentPath, _ := adapter.GetProjectPath(ctx.ChatID)
	expandedPath, err := ExpandPathFrom(input, currentPath)
	if err != nil {
		return sendCommandTextf(adapter, ctx, "Invalid path: %v", err)
	}
	if err := ValidateProjectPath(expandedPath); err != nil {
		return sendCommandTextf(adapter, ctx, "Path validation failed: %v", err)
	}
	return adapter.SetProjectPath(ctx.ChatID, expandedPath)
}

// parsePositiveInt parses a string into a positive integer (>= 1). Returns
// (0, false) on any non-numeric input or non-positive values.
func parsePositiveInt(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		if n > 1<<20 {
			return 0, false
		}
	}
	if n < 1 {
		return 0, false
	}
	return n, true
}

func newDirCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-dir", "dir", "List the folders this chat used, or switch to one").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			if input, ok := joinedArgs(args); ok {
				return changeDir(adapter, ctx, input)
			}
			currentPath, _ := adapter.GetProjectPath(ctx.ChatID)
			paths, _ := adapter.ListChatProjectPaths(ctx.ChatID)
			text := buildDirText(currentPath, paths)

			caps := imbot.GetPlatformCapabilities(string(ctx.Platform))
			if ctx.IsDirectMessage && caps != nil && caps.SupportsInteraction() && len(paths) > 0 {
				keyboard := buildDirKeyboard(currentPath, paths)
				_, err := ctx.Bot.SendMessage(context.Background(), ctx.ChatID, &imbot.SendMessageOptions{
					Text:    text,
					Actions: keyboard.ToActionSet(),
				})
				if err != nil {
					logrus.WithError(err).Error("Failed to send folder list")
				}
				return nil
			}
			return sendCommandText(adapter, ctx, text)
		}).
		WithCategory("project").
		WithPriority(85).
		MustBuild()
}

func newJoinCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-join", "join", "Add group to whitelist (Telegram only)").
		WithPlatforms(imbot.PlatformTelegram).
		Hidden().
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			if !ctx.IsDirectMessage {
				return sendCommandText(adapter, ctx, "/join can only be used in general chat.")
			}

			if !ctx.IsPlatform(imbot.PlatformTelegram) {
				return sendCommandText(adapter, ctx, "Join command is only supported for Telegram bot.")
			}

			input, ok := joinedArgs(args)
			if !ok {
				return sendCommandText(adapter, ctx, usageJoin)
			}

			// Resolve the chat ID via Telegram bot
			tgBot, ok := imbot.AsTelegramBot(ctx.Bot)
			if !ok {
				return sendCommandText(adapter, ctx, "Join command is only supported for Telegram bot.")
			}

			groupID, err := tgBot.ResolveChatID(input)
			if err != nil {
				logrus.WithError(err).Error("Failed to resolve chat ID")
				return sendCommandTextf(adapter, ctx, "Failed to resolve chat ID: %v\n\nNote: Bot must already be a member of the group to add it to whitelist.", err)
			}

			// Check if already whitelisted
			if adapter.IsWhitelisted(groupID) {
				return sendCommandTextf(adapter, ctx, "Group %s is already in whitelist.", groupID)
			}

			// Add group to whitelist
			if err := adapter.AddToWhitelist(groupID, string(ctx.Platform), ctx.SenderID); err != nil {
				logrus.WithError(err).Error("Failed to add group to whitelist")
				return sendCommandTextf(adapter, ctx, "Failed to add group to whitelist: %v", err)
			}

			logrus.Infof("Group %s added to whitelist by %s", groupID, ctx.SenderID)
			return sendCommandTextf(adapter, ctx, "Successfully added group to whitelist.\nGroup ID: %s", groupID)
		}).
		WithCategory("system").
		WithPriority(30).
		MustBuild()
}

// newPairBindCommand registers the `/bind <code>` command used by an end-user
// to pair their direct chat with the bot. It is the only command an unpaired
// chat is allowed to invoke when RequirePairing is on.
func newPairBindCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-pair-bind", "bind",
		"Pair this chat with the bot using the operator's pairing code").
		Hidden().
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			if !ctx.IsDirectMessage {
				return sendCommandText(adapter, ctx, "/bind only works in a direct message.")
			}
			code, ok := parsePairBindCode(args)
			if !ok {
				return sendCommandText(adapter, ctx, usagePairBind)
			}
			botUUID := ""
			if ctx.Bot != nil {
				botUUID = ctx.Bot.UUID()
			}
			if err := adapter.VerifyAndPair(botUUID, ctx.ChatID, ctx.SenderID,
				string(ctx.Platform), code); err != nil {
				return sendCommandText(adapter, ctx, "❌ "+err.Error())
			}
			return sendCommandText(adapter, ctx,
				"✅ Paired. You can now send commands to this bot.")
		}).
		WithCategory("system").
		WithPriority(95).
		MustBuild()
}
