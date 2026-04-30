// Package command provides built-in command definitions for the remote control bot.
package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
)

// Constants for command usage messages
const (
	usageBind     = "Usage: /cd <project_path>"
	usageBash     = "Usage: /bash <command>"
	usageBashCD   = "Usage: /bash cd <path>"
	usageJoin     = "Usage: /join <group_id|@username|invite_link>"
	usagePairBind = "Usage: /bind <code>"
	usageVerbose  = "Usage: /verbose <on|off>"
)

// RegisterBuiltinCommands registers all built-in commands to the dispatcher.
func RegisterBuiltinCommands(dispatcher *Dispatcher) error {
	commands := []Command{
		NewHelpCommand(),
		NewBindCommand(),
		NewPairBindCommand(),
		NewClearCommand(),
		NewInterruptCommand(),
		NewProjectCommand(),
		NewStatusCommand(),
		NewBashCommand(),
		NewJoinCommand(),
		NewYoloCommand(),
		NewVerboseCommand(),
		NewQuietCommand(),
		NewMockCommand(),
	}

	for _, cmd := range commands {
		if err := dispatcher.Register(cmd); err != nil {
			return fmt.Errorf("failed to register command %s: %w", cmd.ID(), err)
		}
	}

	return nil
}

// Command implementations

// NewHelpCommand creates the help command
func NewHelpCommand() Command {
	return NewBuilder("cmd-help", "help", "Show available commands and help").
		WithAliases("h", "start").
		WithHandler(func(ctx *Context, h Handler) error {
			// Help is handled specially by the dispatcher
			// This handler is a fallback
			return h.SendText(ctx.ChatID, "Use /help to see available commands.")
		}).
		WithCategory("session").
		WithPriority(100).
		MustBuild()
}

// NewBindCommand creates the /cd (bind) command
func NewBindCommand() Command {
	return NewBuilder("cmd-bind", "cd", "Bind and cd into a project directory").
		WithHandler(func(ctx *Context, h Handler) error {
			args := ctx.Args
			projectPath, ok := joinedArgs(args)
			if !ok {
				return h.SendText(ctx.ChatID, usageBind)
			}

			// Expand and validate path
			expandedPath, err := ExpandPath(projectPath)
			if err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Invalid path: %v", err))
			}

			if err := ValidateProjectPath(expandedPath); err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Path validation failed: %v", err))
			}

			// Set the project path
			if err := h.SetProjectPath(ctx.ChatID, expandedPath); err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Failed to bind project: %v", err))
			}

			return h.SendText(ctx.ChatID, fmt.Sprintf("✅ Bound to project: %s", ShortenPath(expandedPath)))
		}).
		WithCategory("project").
		WithPriority(90).
		MustBuild()
}

// NewPairBindCommand creates the /bind (pairing) command
func NewPairBindCommand() Command {
	return NewBuilder("cmd-pair-bind", "bind", "Pair this chat with the bot using the operator's pairing code").
		WithHandler(func(ctx *Context, h Handler) error {
			if !ctx.IsDirect {
				return h.SendText(ctx.ChatID, "/bind only works in a direct message.")
			}
			code, ok := parsePairBindCode(ctx.Args)
			if !ok {
				return h.SendText(ctx.ChatID, usagePairBind)
			}
			botUUID := ""
			if ctx.Bot != nil {
				botUUID = ctx.Bot.UUID()
			}
			if err := h.VerifyAndPair(botUUID, ctx.ChatID, ctx.SenderID, string(ctx.Platform), code); err != nil {
				return h.SendText(ctx.ChatID, "❌ "+err.Error())
			}
			return h.SendText(ctx.ChatID, "✅ Paired. You can now send commands to this bot.")
		}).
		WithCategory("system").
		WithPriority(95).
		MustBuild()
}

// NewClearCommand creates the /clear command
func NewClearCommand() Command {
	return NewBuilder("cmd-clear", "clear", "Clear context and start new session").
		WithHandler(func(ctx *Context, h Handler) error {
			agentType, err := h.GetCurrentAgent(ctx.ChatID)
			if err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Failed to get current agent: %v", err))
			}

			if err := h.ClearSession(ctx.ChatID, agentType); err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Failed to clear session: %v", err))
			}

			return h.SendText(ctx.ChatID, "✅ Session cleared. Send a message to start a new session.")
		}).
		WithCategory("session").
		WithPriority(80).
		MustBuild()
}

// NewInterruptCommand creates the /interrupt (/stop) command
func NewInterruptCommand() Command {
	return NewBuilder("cmd-interrupt", "interrupt", "Interrupt / Stop current running task").
		WithAliases("s", "i", "stop").
		WithHandler(func(ctx *Context, h Handler) error {
			if h.StopExecution(ctx.ChatID) {
				return h.SendText(ctx.ChatID, "🛑 Task stopped.")
			}
			return h.SendText(ctx.ChatID, "No running task to stop.")
		}).
		WithCategory("session").
		WithPriority(70).
		MustBuild()
}

// NewProjectCommand creates the /project command
func NewProjectCommand() Command {
	return NewBuilder("cmd-project", "project", "Show and switch between projects").
		WithHandler(func(ctx *Context, h Handler) error {
			currentPath, _ := h.GetProjectPath(ctx.ChatID)

			var buf strings.Builder
			if currentPath != "" {
				buf.WriteString(fmt.Sprintf("Current Project:\n📁 %s\n\n", currentPath))
			} else {
				buf.WriteString("No project bound to this chat.\n\n")
			}

			// Get all projects for user (direct messages only)
			if ctx.IsDirect {
				projectPaths, err := h.ListProjectPaths(ctx.SenderID, string(ctx.Platform))
				if err == nil && len(projectPaths) > 0 {
					buf.WriteString("Your Projects:\n")
					// Build inline keyboard with projects
					var rows [][]imbot.InlineKeyboardButton
					for _, path := range projectPaths {
						marker := ""
						if path == currentPath {
							marker = " ✓"
						}
						btn := imbot.InlineKeyboardButton{
							Text:         fmt.Sprintf("📁 %s%s", filepath.Base(path), marker),
							CallbackData: imbot.FormatCallbackData("project", "switch", path),
						}
						rows = append(rows, []imbot.InlineKeyboardButton{btn})
					}
					// Add "Bind New" button
					rows = append(rows, []imbot.InlineKeyboardButton{{
						Text:         "📁 Bind New Project",
						CallbackData: imbot.FormatCallbackData("action", "bind"),
					}})

					keyboard := imbot.InlineKeyboardMarkup{InlineKeyboard: rows}
					tgKeyboard := imbot.BuildTelegramActionKeyboard(keyboard)

					_, err = ctx.Bot.SendMessage(context.Background(), ctx.ChatID, &imbot.SendMessageOptions{
						Text:      buf.String(),
						ParseMode: imbot.ParseModeMarkdown,
						Metadata:  buildTrackedReplyMetadata(tgKeyboard),
					})
					if err != nil {
						logrus.WithError(err).Error("Failed to send project list")
					}
					return nil
				}
			}

			buf.WriteString("Use /cd <path> to bind a project.")
			return h.SendText(ctx.ChatID, buf.String())
		}).
		WithCategory("project").
		WithPriority(60).
		MustBuild()
}

// NewStatusCommand creates the /status command
func NewStatusCommand() Command {
	return NewBuilder("cmd-status", "status", "Show current session status").
		WithHandler(func(ctx *Context, h Handler) error {
			agentType, _ := h.GetCurrentAgent(ctx.ChatID)

			// Smart Guide is stateless
			if agentType == AgentNameTinglyBox {
				projectPath := resolveProjectPath(h, ctx.ChatID, string(ctx.Platform))
				var parts []string
				parts = append(parts, "Agent: Smart Guide (@tb)")
				parts = append(parts, "Status: Stateless (no session)")
				if projectPath != "" {
					parts = append(parts, fmt.Sprintf("Project: %s", projectPath))
				}
				return h.SendText(ctx.ChatID, strings.Join(parts, "\n"))
			}

			// For other agents (claude, mock), find the session
			projectPath := resolveProjectPath(h, ctx.ChatID, string(ctx.Platform))
			if projectPath == "" {
				return h.SendText(ctx.ChatID, "No project bound. Use /cd <project_path> first.")
			}

			sess, err := h.GetSession(ctx.ChatID, agentType, projectPath)
			if err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("No session found for agent %s in project %s", agentType, projectPath))
			}

			var parts []string
			parts = append(parts, fmt.Sprintf("Agent: %s", GetAgentDisplayName(agentType)))
			parts = append(parts, fmt.Sprintf("Session: %s", sess.ID))
			parts = append(parts, fmt.Sprintf("Status: %s", sess.Status))

			// Show running duration if running
			if sess.Status == "running" {
				if t, ok := sess.LastActivity.(time.Time); ok && !t.IsZero() {
					runningFor := time.Since(t).Round(time.Second)
					parts = append(parts, fmt.Sprintf("Running for: %s", runningFor))
				}
			}

			// Show current request if any
			if sess.Request != "" {
				reqPreview := sess.Request
				if len(reqPreview) > 100 {
					reqPreview = reqPreview[:100] + "..."
				}
				parts = append(parts, fmt.Sprintf("Current task: %s", reqPreview))
			}

			// Show project path
			if sess.Project != "" {
				parts = append(parts, fmt.Sprintf("Project: %s", sess.Project))
			}

			// Show error if failed
			if sess.Status == "failed" && sess.Error != "" {
				errPreview := sess.Error
				if len(errPreview) > 100 {
					errPreview = errPreview[:100] + "..."
				}
				parts = append(parts, fmt.Sprintf("Error: %s", errPreview))
			}

			return h.SendText(ctx.ChatID, strings.Join(parts, "\n"))
		}).
		WithCategory("project").
		WithPriority(50).
		MustBuild()
}

// NewBashCommand creates the /bash command
func NewBashCommand() Command {
	return NewBuilder("cmd-bash", "bash", "Execute bash commands (cd, ls, pwd)").
		WithHandler(func(ctx *Context, h Handler) error {
			args := ctx.Args
			subcommandArg, ok := firstArg(args)
			if !ok {
				return h.SendText(ctx.ChatID, usageBash)
			}

			subcommand := strings.ToLower(subcommandArg)
			allowlist := h.GetBashAllowlist()

			if _, ok := allowlist[subcommand]; !ok {
				return h.SendText(ctx.ChatID, "Command not allowed.")
			}

			projectPath, _ := h.GetProjectPath(ctx.ChatID)
			bashCwd, _ := h.GetBashCwd(ctx.ChatID)
			baseDir := bashCwd
			if baseDir == "" {
				baseDir = projectPath
			}

			switch subcommand {
			case "pwd":
				if baseDir == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return h.SendText(ctx.ChatID, "Unable to resolve working directory.")
					}
					return h.SendText(ctx.ChatID, cwd)
				}
				return h.SendText(ctx.ChatID, baseDir)

			case "cd":
				nextPath, ok := joinedArgs(args[1:])
				if !ok {
					return h.SendText(ctx.ChatID, usageBashCD)
				}
				cdBase := baseDir
				if cdBase == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return h.SendText(ctx.ChatID, "Unable to resolve working directory.")
					}
					cdBase = cwd
				}
				if !filepath.IsAbs(nextPath) {
					nextPath = filepath.Join(cdBase, nextPath)
				}
				if stat, err := os.Stat(nextPath); err != nil || !stat.IsDir() {
					return h.SendText(ctx.ChatID, "Directory not found.")
				}
				absPath, err := filepath.Abs(nextPath)
				if err == nil {
					nextPath = absPath
				}
				if err := h.SetBashCwd(ctx.ChatID, nextPath); err != nil {
					logrus.WithError(err).Warn("Failed to update bash cwd")
				}
				return h.SendText(ctx.ChatID, fmt.Sprintf("Bash working directory set to %s", nextPath))

			case "ls":
				if baseDir == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return h.SendText(ctx.ChatID, "Unable to resolve working directory.")
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
					return h.SendText(ctx.ChatID, fmt.Sprintf("Command failed: %v", err))
				}
				return h.SendText(ctx.ChatID, strings.TrimSpace(string(output)))

			default:
				return h.SendText(ctx.ChatID, "Command not allowed.")
			}
		}).
		WithCategory("system").
		WithPriority(40).
		MustBuild()
}

// NewJoinCommand creates the /join command (Telegram only)
func NewJoinCommand() Command {
	return NewBuilder("cmd-join", "join", "Add group to whitelist (Telegram only)").
		WithHandler(func(ctx *Context, h Handler) error {
			if !ctx.IsDirect {
				return h.SendText(ctx.ChatID, "/join can only be used in general chat.")
			}

			if !ctx.IsPlatformFunc(imbot.PlatformTelegram) {
				return h.SendText(ctx.ChatID, "Join command is only supported for Telegram bot.")
			}

			input, ok := joinedArgs(ctx.Args)
			if !ok {
				return h.SendText(ctx.ChatID, usageJoin)
			}

			// Resolve the chat ID via Telegram bot
			tgBot, ok := imbot.AsTelegramBot(ctx.Bot)
			if !ok {
				return h.SendText(ctx.ChatID, "Join command is only supported for Telegram bot.")
			}

			groupID, err := tgBot.ResolveChatID(input)
			if err != nil {
				logrus.WithError(err).Error("Failed to resolve chat ID")
				return h.SendText(ctx.ChatID, fmt.Sprintf("Failed to resolve chat ID: %v\n\nNote: Bot must already be a member of the group to add it to whitelist.", err))
			}

			// Check if already whitelisted
			if h.IsWhitelisted(groupID) {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Group %s is already in whitelist.", groupID))
			}

			// Add group to whitelist
			if err := h.AddToWhitelist(groupID, string(ctx.Platform), ctx.SenderID); err != nil {
				logrus.WithError(err).Error("Failed to add group to whitelist")
				return h.SendText(ctx.ChatID, fmt.Sprintf("Failed to add group to whitelist: %v", err))
			}

			logrus.Infof("Group %s added to whitelist by %s", groupID, ctx.SenderID)
			return h.SendText(ctx.ChatID, fmt.Sprintf("Successfully added group to whitelist.\nGroup ID: %s", groupID))
		}).
		WithCategory("system").
		WithPriority(30).
		MustBuild()
}

// NewYoloCommand creates the /yolo command
func NewYoloCommand() Command {
	return NewBuilder("cmd-yolo", "yolo", "Toggle auto-approve mode (Claude Code only)").
		WithHandler(func(ctx *Context, h Handler) error {
			agentType, _ := h.GetCurrentAgent(ctx.ChatID)
			if agentType != AgentNameClaude {
				return h.SendText(ctx.ChatID, "⚠️ Auto-approve mode is only available for Claude Code (@cc).\n\nSwitch to Claude Code first with: @cc")
			}

			projectPath := resolveProjectPath(h, ctx.ChatID, string(ctx.Platform))
			if projectPath == "" {
				return h.SendText(ctx.ChatID, "No project path found. Use /cd <project_path> to create a session first.")
			}

			// Find or create session
			sess, err := h.FindOrCreateSession(ctx.ChatID, AgentNameClaude, projectPath)
			if err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Failed to get session: %v", err))
			}

			// Toggle permission mode between bypassPermissions (yolo ON) and default (yolo OFF)
			newMode := string(claude.PermissionModeBypassPermissions)
			if sess.PermissionMode == string(claude.PermissionModeBypassPermissions) {
				newMode = string(claude.PermissionModeDefault)
			}

			if err := h.UpdatePermissionMode(sess.ID, newMode); err != nil {
				return h.SendText(ctx.ChatID, fmt.Sprintf("Failed to update permission mode: %v", err))
			}

			if newMode == string(claude.PermissionModeBypassPermissions) {
				return h.SendText(ctx.ChatID, fmt.Sprintf("🚀 **YOLO MODE ENABLED**\n\nAll permissions will be auto-approved for this session.\n⚠️ Use with caution!\n\nSession: %s\nProject: %s", sess.ID, projectPath))
			}
			return h.SendText(ctx.ChatID, fmt.Sprintf("🔒 **YOLO MODE DISABLED**\n\nBack to normal approval mode.\nAll permission requests will require confirmation.\n\nSession: %s\nProject: %s", sess.ID, projectPath))
		}).
		WithCategory("advanced").
		WithPriority(10).
		MustBuild()
}

// NewVerboseCommand creates the /verbose command
func NewVerboseCommand() Command {
	return NewBuilder("cmd-verbose", "verbose", "Control message detail display").
		WithHandler(func(ctx *Context, h Handler) error {
			// No args: show current status
			if len(ctx.Args) == 0 {
				current := h.GetVerbose(ctx.ChatID)
				status := "off"
				if current {
					status = "on"
				}
				return h.SendText(ctx.ChatID, fmt.Sprintf("📢 Verbose mode: %s\n\n%s", status, usageVerbose))
			}

			enabled, valid := parseToggleArg(ctx.Args[0])
			if !valid {
				return h.SendText(ctx.ChatID, usageVerbose+"\n\nExample: /verbose on")
			}

			h.SetVerbose(ctx.ChatID, enabled)
			if enabled {
				return h.SendText(ctx.ChatID, "✅ Verbose mode enabled\n\nAll message details will be shown.")
			}
			return h.SendText(ctx.ChatID, "🔇 Quiet mode enabled\n\nOnly final results will be shown.")
		}).
		WithCategory("advanced").
		WithPriority(5).
		MustBuild()
}

// NewQuietCommand creates the /quiet command
func NewQuietCommand() Command {
	return NewBuilder("cmd-quiet", "quiet", "Disable verbose mode (alias for /verbose off)").
		WithAliases("noverbose").
		WithHandler(func(ctx *Context, h Handler) error {
			h.SetVerbose(ctx.ChatID, false)
			return h.SendText(ctx.ChatID, "🔇 Quiet mode enabled\n\nOnly final results will be shown. Use /verbose on to show all details.")
		}).
		WithCategory("advanced").
		WithPriority(4).
		MustBuild()
}

// NewMockCommand creates the /mock command
func NewMockCommand() Command {
	return NewBuilder("cmd-mock", "mock", "Test with mock agent").
		WithHandler(func(ctx *Context, h Handler) error {
			return h.SendText(ctx.ChatID, "Mock agent not implemented in new system yet.")
		}).
		Hidden().
		WithCategory("advanced").
		MustBuild()
}

// Helper functions

func buildTrackedReplyMetadata(tgKeyboard interface{}) map[string]interface{} {
	return map[string]interface{}{
		"reply_keyboard": tgKeyboard,
	}
}

func joinedArgs(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	value := strings.TrimSpace(strings.Join(args, " "))
	return value, value != ""
}

func firstArg(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	value := strings.TrimSpace(args[0])
	return value, value != ""
}

func parseToggleArg(arg string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on", "true", "1", "yes", "enable":
		return true, true
	case "off", "false", "0", "no", "disable":
		return false, true
	default:
		return false, false
	}
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

func resolveProjectPath(h Handler, chatID, platform string) string {
	projectPath, _ := h.GetProjectPath(chatID)
	if projectPath == "" {
		if path, found := h.GetProjectPathForGroup(chatID, platform); found {
			projectPath = path
		}
	}
	return projectPath
}

// Import constants from parent bot package (will be moved to command package later)
const (
	AgentNameTinglyBox = "tingly-box"
	AgentNameClaude    = "claude"
)

// getAgentDisplayName returns the short display name for an agent type
func GetAgentDisplayName(agentType string) string {
	switch agentType {
	case AgentNameTinglyBox:
		return "@tb"
	case AgentNameClaude, "claude-code":
		return "@cc"
	default:
		return agentType
	}
}
