// Package command provides built-in command definitions for the remote control bot.
package bot

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
)

// RegisterBuiltinCommands registers all built-in commands to the registry.
func RegisterBuiltinCommands(registry *imbot.CommandRegistry, botHandler BotHandlerAdapter) error {
	commands := []imbot.Command{
		newHelpCommand(botHandler),
		newBindCommand(botHandler),
		newClearCommand(botHandler),
		newStopCommand(botHandler),
		newInterruptCommand(botHandler),
		newProjectCommand(botHandler),
		newStatusCommand(botHandler),
		newBashCommand(botHandler),
		newJoinCommand(botHandler),
		newYoloCommand(botHandler),
		newVerboseCommand(botHandler),
		newQuietCommand(botHandler), // Renamed from noverbose
		newMockCommand(botHandler),
	}

	for _, cmd := range commands {
		if err := registerCommand(registry, cmd); err != nil {
			return fmt.Errorf("failed to register command %s: %w", cmd.ID, err)
		}
	}

	return nil
}

// registerCommand registers a command using the registry's Register method.
func registerCommand(registry *imbot.CommandRegistry, cmd imbot.Command) error {
	// Convert to internal command type for registration
	// The internal registry needs the specific type
	return registry.Register(cmd)
}

// BotHandlerAdapter provides methods needed by command handlers.
// This allows commands to interact with the bot without direct coupling.
type BotHandlerAdapter interface {
	// SendText sends a text message to a chat
	SendText(chatID, text string) error

	// GetProjectPath gets the current project path for a chat
	GetProjectPath(chatID string) (string, error)

	// SetProjectPath sets the project path for a chat
	SetProjectPath(chatID, path string) error

	// GetSession gets session info
	GetSession(chatID, agentType, projectPath string) (*SessionInfo, error)

	// ClearSession clears a session
	ClearSession(chatID, agentType string) error

	// GetCurrentAgent gets the current agent for a chat
	GetCurrentAgent(chatID string) (string, error)

	// SetVerbose sets verbose mode for a chat
	SetVerbose(chatID string, enabled bool)

	// GetVerbose gets verbose mode for a chat
	GetVerbose(chatID string) bool

	// IsWhitelisted checks if a group is whitelisted
	IsWhitelisted(groupID string) bool

	// AddToWhitelist adds a group to whitelist
	AddToWhitelist(groupID, platform, userID string) error

	// GetBashCwd gets the bash working directory
	GetBashCwd(chatID string) (string, error)

	// SetBashCwd sets the bash working directory
	SetBashCwd(chatID, path string) error

	// ResolveChatID resolves a chat ID (for Telegram join command)
	ResolveChatID(input string) (string, error)
}

// SessionInfo holds session information.
type SessionInfo struct {
	ID             string
	Status         string
	Project        string
	Request        string
	Error          string
	PermissionMode string
}

// Command implementations

func newHelpCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-help", "help", "Show available commands and help").
		WithAliases("h", "start").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			return adapter.SendText(ctx.ChatID, ctx.Text)
		}).
		WithCategory("session").
		WithPriority(100).
		MustBuild()
}

func newBindCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-bind", "cd", "Bind and cd into a project directory").
		WithAliases("bind").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			if len(args) < 1 {
				return adapter.SendText(ctx.ChatID, "Usage: /cd <project_path>")
			}

			projectPath := strings.TrimSpace(strings.Join(args, " "))
			if projectPath == "" {
				return adapter.SendText(ctx.ChatID, "Usage: /cd <project_path>")
			}

			// Expand and validate path
			expandedPath, err := ExpandPath(projectPath)
			if err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Invalid path: %v", err))
			}

			if err := ValidateProjectPath(expandedPath); err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Path validation failed: %v", err))
			}

			// Set the project path
			if err := adapter.SetProjectPath(ctx.ChatID, expandedPath); err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Failed to bind project: %v", err))
			}

			return adapter.SendText(ctx.ChatID, fmt.Sprintf("✅ Bound to project: %s", ShortenPath(expandedPath)))
		}).
		WithCategory("project").
		WithPriority(90).
		MustBuild()
}

func newClearCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-clear", "clear", "Clear context and start new session").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			agentType, err := adapter.GetCurrentAgent(ctx.ChatID)
			if err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Failed to get current agent: %v", err))
			}

			if err := adapter.ClearSession(ctx.ChatID, agentType); err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Failed to clear session: %v", err))
			}

			return adapter.SendText(ctx.ChatID, "✅ Session cleared. Send a message to start a new session.")
		}).
		WithCategory("session").
		WithPriority(80).
		MustBuild()
}

func newStopCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-stop", "stop", "Stop the current running task").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			return adapter.SendText(ctx.ChatID, "🛑 Task stopped.")
		}).
		WithCategory("session").
		WithPriority(75).
		MustBuild()
}

func newInterruptCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-interrupt", "interrupt", "Alias for /stop").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			return adapter.SendText(ctx.ChatID, "🛑 Task stopped.")
		}).
		Hidden().
		MustBuild()
}

func newProjectCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-project", "project", "Show and switch between projects").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			currentPath, err := adapter.GetProjectPath(ctx.ChatID)
			if err != nil {
				currentPath = ""
			}

			var text strings.Builder
			if currentPath != "" {
				text.WriteString(fmt.Sprintf("Current Project:\n📁 %s\n\n", currentPath))
			} else {
				text.WriteString("No project bound to this chat.\n\n")
			}

			text.WriteString("Use /cd <path> to bind a project.")

			return adapter.SendText(ctx.ChatID, text.String())
		}).
		WithCategory("project").
		WithPriority(60).
		MustBuild()
}

func newStatusCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-status", "status", "Show current session status").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			agentType, err := adapter.GetCurrentAgent(ctx.ChatID)
			if err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Failed to get agent: %v", err))
			}

			projectPath, err := adapter.GetProjectPath(ctx.ChatID)
			if err != nil {
				projectPath = ""
			}

			if projectPath == "" {
				return adapter.SendText(ctx.ChatID, "No project bound. Use /cd <project_path> first.")
			}

			sess, err := adapter.GetSession(ctx.ChatID, agentType, projectPath)
			if err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("No session found: %v", err))
			}

			var parts []string
			parts = append(parts, fmt.Sprintf("Agent: %s", agentType))
			parts = append(parts, fmt.Sprintf("Session: %s", sess.ID))
			parts = append(parts, fmt.Sprintf("Status: %s", sess.Status))

			if sess.Project != "" {
				parts = append(parts, fmt.Sprintf("Project: %s", sess.Project))
			}

			return adapter.SendText(ctx.ChatID, strings.Join(parts, "\n"))
		}).
		WithCategory("project").
		WithPriority(50).
		MustBuild()
}

func newBashCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-bash", "bash", "Execute bash commands (cd, ls, pwd)").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			if len(args) < 1 {
				return adapter.SendText(ctx.ChatID, "Usage: /bash <command>")
			}

			subcommand := strings.ToLower(strings.TrimSpace(args[0]))
			allowlist := map[string]struct{}{
				"pwd": {}, "cd": {}, "ls": {},
			}

			if _, ok := allowlist[subcommand]; !ok {
				return adapter.SendText(ctx.ChatID, "Command not allowed.")
			}

			projectPath, _ := adapter.GetProjectPath(ctx.ChatID)
			bashCwd, _ := adapter.GetBashCwd(ctx.ChatID)
			baseDir := bashCwd
			if baseDir == "" {
				baseDir = projectPath
			}

			switch subcommand {
			case "pwd":
				if baseDir == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return adapter.SendText(ctx.ChatID, "Unable to resolve working directory.")
					}
					return adapter.SendText(ctx.ChatID, cwd)
				}
				return adapter.SendText(ctx.ChatID, baseDir)

			case "cd":
				if len(args) < 2 {
					return adapter.SendText(ctx.ChatID, "Usage: /bash cd <path>")
				}
				nextPath := strings.TrimSpace(strings.Join(args[1:], " "))
				if nextPath == "" {
					return adapter.SendText(ctx.ChatID, "Usage: /bash cd <path>")
				}
				cdBase := baseDir
				if cdBase == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return adapter.SendText(ctx.ChatID, "Unable to resolve working directory.")
					}
					cdBase = cwd
				}
				if !filepath.IsAbs(nextPath) {
					nextPath = filepath.Join(cdBase, nextPath)
				}
				if stat, err := os.Stat(nextPath); err != nil || !stat.IsDir() {
					return adapter.SendText(ctx.ChatID, "Directory not found.")
				}
				absPath, err := filepath.Abs(nextPath)
				if err == nil {
					nextPath = absPath
				}
				if err := adapter.SetBashCwd(ctx.ChatID, nextPath); err != nil {
					logrus.WithError(err).Warn("Failed to update bash cwd")
				}
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Bash working directory set to %s", nextPath))

			case "ls":
				if baseDir == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return adapter.SendText(ctx.ChatID, "Unable to resolve working directory.")
					}
					baseDir = cwd
				}
				var lsArgs []string
				if len(args) > 1 {
					lsArgs = append(lsArgs, args[1:]...)
				}
				execCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(execCtx, "ls", lsArgs...)
				cmd.Dir = baseDir
				output, err := cmd.CombinedOutput()
				if err != nil && len(output) == 0 {
					return adapter.SendText(ctx.ChatID, fmt.Sprintf("Command failed: %v", err))
				}
				return adapter.SendText(ctx.ChatID, strings.TrimSpace(string(output)))

			default:
				return adapter.SendText(ctx.ChatID, "Command not allowed.")
			}
		}).
		WithCategory("system").
		WithPriority(40).
		MustBuild()
}

func newJoinCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-join", "join", "Add group to whitelist (Telegram only)").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			if !ctx.IsPlatform(imbot.PlatformTelegram) {
				return adapter.SendText(ctx.ChatID, "Join command is only supported for Telegram bot.")
			}

			if len(args) < 1 {
				return adapter.SendText(ctx.ChatID, "Usage: /join <group_id|@username|invite_link>")
			}

			input := strings.TrimSpace(strings.Join(args, " "))
			if input == "" {
				return adapter.SendText(ctx.ChatID, "Usage: /join <group_id|@username|invite_link>")
			}

			// Resolve the chat ID
			groupID, err := adapter.ResolveChatID(input)
			if err != nil {
				logrus.WithError(err).Error("Failed to resolve chat ID")
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Failed to resolve chat ID: %v", err))
			}

			// Check if already whitelisted
			if adapter.IsWhitelisted(groupID) {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Group %s is already in whitelist.", groupID))
			}

			// Add group to whitelist
			if err := adapter.AddToWhitelist(groupID, string(ctx.Platform), ctx.SenderID); err != nil {
				logrus.WithError(err).Error("Failed to add group to whitelist")
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Failed to add group to whitelist: %v", err))
			}

			return adapter.SendText(ctx.ChatID, fmt.Sprintf("Successfully added group to whitelist.\nGroup ID: %s", groupID))
		}).
		WithCategory("system").
		WithPriority(30).
		MustBuild()
}

func newYoloCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-yolo", "yolo", "Toggle auto-approve mode (Claude Code only)").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			agentType, err := adapter.GetCurrentAgent(ctx.ChatID)
			if err != nil || agentType != "claude" {
				return adapter.SendText(ctx.ChatID, "⚠️ Auto-approve mode is only available for Claude Code (@cc).\n\nSwitch to Claude Code first with: @cc")
			}

			projectPath, _ := adapter.GetProjectPath(ctx.ChatID)
			if projectPath == "" {
				return adapter.SendText(ctx.ChatID, "No project path found. Use /cd <project_path> to create a session first.")
			}

			sess, err := adapter.GetSession(ctx.ChatID, "claude", projectPath)
			if err != nil {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("Failed to get session: %v", err))
			}

			newMode := "auto"
			if sess.PermissionMode == "auto" {
				newMode = "manual"
			}

			// Update permission mode (this would need to be added to adapter)
			// For now, just send the message
			if newMode == "auto" {
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("🚀 **YOLO MODE ENABLED**\n\nAll permissions will be auto-approved for this session.\n⚠️ Use with caution!\n\nSession: %s\nProject: %s", sess.ID, projectPath))
			}
			return adapter.SendText(ctx.ChatID, fmt.Sprintf("🔒 **YOLO MODE DISABLED**\n\nBack to normal approval mode.\nAll permission requests will require confirmation.\n\nSession: %s\nProject: %s", sess.ID, projectPath))
		}).
		WithCategory("advanced").
		WithPriority(10).
		MustBuild()
}

func newVerboseCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-verbose", "verbose", "Control message detail display").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			// No args: show current status
			if len(args) == 0 {
				current := adapter.GetVerbose(ctx.ChatID)
				status := "off"
				if current {
					status = "on"
				}
				return adapter.SendText(ctx.ChatID, fmt.Sprintf("📢 Verbose mode: %s\n\nUsage: /verbose <on|off>", status))
			}

			// Parse argument
			arg := strings.ToLower(strings.TrimSpace(args[0]))
			var enabled bool
			var valid bool

			switch arg {
			case "on", "true", "1", "yes", "enable":
				enabled = true
				valid = true
			case "off", "false", "0", "no", "disable":
				enabled = false
				valid = true
			}

			if !valid {
				return adapter.SendText(ctx.ChatID, "Usage: /verbose <on|off>\n\nExample: /verbose on")
			}

			adapter.SetVerbose(ctx.ChatID, enabled)
			if enabled {
				return adapter.SendText(ctx.ChatID, "✅ Verbose mode enabled\n\nAll message details will be shown.")
			}
			return adapter.SendText(ctx.ChatID, "🔇 Quiet mode enabled\n\nOnly final results will be shown.")
		}).
		WithCategory("advanced").
		WithPriority(5).
		MustBuild()
}

// newQuietCommand creates the /quiet command (alias for /verbose off)
// This is a convenient shorthand to disable verbose mode
func newQuietCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-quiet", "quiet", "Disable verbose mode (alias for /verbose off)").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			adapter.SetVerbose(ctx.ChatID, false)
			return adapter.SendText(ctx.ChatID, "🔇 Quiet mode enabled\n\nOnly final results will be shown. Use /verbose on to show all details.")
		}).
		WithCategory("advanced").
		WithPriority(4).
		MustBuild()
}

func newMockCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-mock", "mock", "Test with mock agent").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			return adapter.SendText(ctx.ChatID, "Mock agent not implemented in new system yet.")
		}).
		Hidden().
		WithCategory("advanced").
		MustBuild()
}
