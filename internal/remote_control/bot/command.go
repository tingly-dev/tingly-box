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
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
)

// RegisterBuiltinCommands registers all built-in commands to the registry.
func RegisterBuiltinCommands(registry *imbot.CommandRegistry, botHandler BotHandlerAdapter) error {
	commands := []imbot.Command{
		newHelpCommand(botHandler),
		newBindCommand(botHandler),
		newPairBindCommand(botHandler),
		newClearCommand(botHandler),
		newInterruptCommand(botHandler),
		newProjectCommand(botHandler),
		newStatusCommand(botHandler),
		newBashCommand(botHandler),
		newJoinCommand(botHandler),
		newYoloCommand(botHandler),
		newVerboseCommand(botHandler),
		newQuietCommand(botHandler),
		newMockCommand(botHandler),
		newResumeCommand(botHandler),
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

	// GetProjectPath gets the current project path for a chat
	GetProjectPath(chatID string) (string, error)

	// SetProjectPath sets the project path for a chat
	SetProjectPath(chatID, path string) error

	// GetProjectPathForGroup gets project path with group fallback
	GetProjectPathForGroup(chatID, platform string) (string, bool)

	// GetSession gets session info
	GetSession(chatID, agentType, projectPath string) (*SessionInfo, error)

	// FindOrCreateSession finds an existing session or creates a new one
	FindOrCreateSession(chatID, agentType, projectPath string) (*SessionInfo, error)

	// UpdatePermissionMode updates the permission mode for a session
	UpdatePermissionMode(sessionID, mode string) error

	// ClearSession clears a session
	ClearSession(chatID, agentType string) error

	// StopExecution cancels a running execution, returns true if one was running
	StopExecution(chatID string) bool

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

	// GetDefaultProjectPath returns the default project path
	GetDefaultProjectPath() string

	// GetBashAllowlist returns the configured bash allowlist
	GetBashAllowlist() map[string]struct{}

	// ListProjectPaths lists all project paths for a user
	ListProjectPaths(ownerID, platform string) ([]string, error)

	// ListChatProjectPaths lists the MRU per-chat project-path history.
	ListChatProjectPaths(chatID string) ([]string, error)

	// VerifyAndPair verifies a one-time pairing code and, on success, records
	// the chat as paired with the bot. Implementations should also emit the
	// matching audit events (success / failure).
	VerifyAndPair(botUUID, chatID, senderID, platform, code string) error

	// BuildReplyFooter returns a compact footer (agent + project path) that
	// command replies append for context continuity. Returns an empty string
	// when neither agent nor project path is resolvable for the chat.
	BuildReplyFooter(chatID, platform string) string

	// ListResumableSessions lists the most recent Claude sessions on disk for
	// the given project, newest first, capped to limit.
	ListResumableSessions(projectPath string, limit int) ([]ResumableSession, error)

	// PrepareResume binds the given Claude session_id as the next session for
	// (chatID, agentType, projectPath). The next user message will be sent with
	// --resume <sessionID>.
	PrepareResume(chatID, agentType, projectPath, sessionID string) error

	// RememberResumeListing stores the session IDs presented to the user so
	// /resume <n> can resolve back to them. Order is the same as the displayed
	// list (1-indexed externally).
	RememberResumeListing(chatID string, sessionIDs []string)

	// RecallResumeListing returns the most recently displayed session IDs, in
	// display order. Returns nil if no listing was remembered.
	RecallResumeListing(chatID string) []string
}

// ResumableSession is the per-row info returned by ListResumableSessions.
// Channel-neutral so command code can render either compact text or buttons.
type ResumableSession struct {
	SessionID    string
	ProjectPath  string
	StartTime    time.Time
	EndTime      time.Time
	NumTurns     int
	Status       string
	FirstMessage string
}

// SessionInfo holds session information.
type SessionInfo struct {
	ID             string
	Status         string
	Project        string
	Request        string
	Error          string
	PermissionMode string
	LastActivity   time.Time
}

const (
	usageBind     = "Usage: /cd <project_path|number>"
	usageBash     = "Usage: /bash <command>"
	usageBashCD   = "Usage: /bash cd <path>"
	usageJoin     = "Usage: /join <group_id|@username|invite_link>"
	usagePairBind = "Usage: /bind <code>"
	usageVerbose  = "Usage: /verbose <on|off>"
	usageResume   = "Usage: /resume [number]   (no number = list recent sessions)"

	// resumeListLimit caps how many recent sessions /resume shows. Keep small
	// enough that the list fits on a phone screen.
	resumeListLimit = 10
)

func sendCommandText(adapter BotHandlerAdapter, ctx *imbot.HandlerContext, text string) error {
	footer := adapter.BuildReplyFooter(ctx.ChatID, string(ctx.Platform))
	return adapter.SendText(ctx.ChatID, text+footer)
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

// Command implementations

func newHelpCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-help", "help", "Show available commands and help").
		WithAliases("h", "start").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			// Help will be built by the registry's BuildHelpText, then formatted with meta
			return nil // Handled specially in handleSlashCommands
		}).
		WithCategory("session").
		WithPriority(100).
		MustBuild()
}

func newBindCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-bind", "cd", "Bind and cd into a project directory").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			input, ok := joinedArgs(args)
			if !ok {
				return sendCommandText(adapter, ctx, usageBind)
			}

			// Numeric index → resolve from this chat's project history.
			if idx, ok := parsePositiveInt(input); ok {
				paths, err := adapter.ListChatProjectPaths(ctx.ChatID)
				if err != nil {
					return sendCommandTextf(adapter, ctx, "Failed to list projects: %v", err)
				}
				if idx < 1 || idx > len(paths) {
					return sendCommandTextf(adapter, ctx, "Invalid project number: %d (have %d). Use /project to see the list.", idx, len(paths))
				}
				selected := paths[idx-1]
				if err := adapter.SetProjectPath(ctx.ChatID, selected); err != nil {
					return sendCommandTextf(adapter, ctx, "Failed to bind project: %v", err)
				}
				return sendCommandTextf(adapter, ctx, "✅ Switched to project: %s", ShortenPath(selected))
			}

			// Path argument: expand relative to the chat's currently-bound project.
			currentPath, _ := adapter.GetProjectPath(ctx.ChatID)
			expandedPath, err := ExpandPathFrom(input, currentPath)
			if err != nil {
				return sendCommandTextf(adapter, ctx, "Invalid path: %v", err)
			}

			if err := ValidateProjectPath(expandedPath); err != nil {
				return sendCommandTextf(adapter, ctx, "Path validation failed: %v", err)
			}

			if err := adapter.SetProjectPath(ctx.ChatID, expandedPath); err != nil {
				return sendCommandTextf(adapter, ctx, "Failed to bind project: %v", err)
			}

			return sendCommandTextf(adapter, ctx, "✅ Bound to project: %s", ShortenPath(expandedPath))
		}).
		WithCategory("project").
		WithPriority(90).
		MustBuild()
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

func newClearCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-clear", "clear", "Clear context and start new session").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			agentType, err := adapter.GetCurrentAgent(ctx.ChatID)
			if err != nil {
				return sendCommandTextf(adapter, ctx, "Failed to get current agent: %v", err)
			}

			if err := adapter.ClearSession(ctx.ChatID, agentType); err != nil {
				return sendCommandTextf(adapter, ctx, "Failed to clear session: %v", err)
			}

			return sendCommandText(adapter, ctx, "✅ Session cleared. Send a message to start a new session.")
		}).
		WithCategory("session").
		WithPriority(80).
		MustBuild()
}

func newInterruptCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-interrupt", "interrupt", "Interrupt / Stop current running task").
		WithAliases("s", "i", "stop").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			if adapter.StopExecution(ctx.ChatID) {
				return sendCommandText(adapter, ctx, "🛑 Task stopped.")
			}
			return sendCommandText(adapter, ctx, "No running task to stop.")
		}).
		WithCategory("session").
		WithPriority(70).
		MustBuild()
}

func newProjectCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-project", "project", "Show and switch between projects").
		WithAliases("p").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			currentPath := resolveProjectPath(adapter, ctx.ChatID, string(ctx.Platform))
			projectPaths, _ := adapter.ListChatProjectPaths(ctx.ChatID)

			text := buildProjectText(currentPath, projectPaths)

			caps := imbot.GetPlatformCapabilities(string(ctx.Platform))
			interactive := ctx.IsDirectMessage && caps != nil && caps.SupportsInteraction()

			if interactive && len(projectPaths) > 0 {
				keyboard := buildProjectKeyboard(currentPath, projectPaths)
				tgKeyboard := imbot.BuildTelegramActionKeyboard(keyboard)
				_, err := ctx.Bot.SendMessage(context.Background(), ctx.ChatID, &imbot.SendMessageOptions{
					Text:     text + adapter.BuildReplyFooter(ctx.ChatID, string(ctx.Platform)),
					Metadata: buildTrackedReplyMetadata(tgKeyboard),
				})
				if err != nil {
					logrus.WithError(err).Error("Failed to send project list")
				}
				return nil
			}

			return sendCommandText(adapter, ctx, text)
		}).
		WithCategory("project").
		WithPriority(60).
		MustBuild()
}

func newStatusCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-status", "status", "Show current session status").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			agentType, _ := adapter.GetCurrentAgent(ctx.ChatID)

			// Smart Guide is stateless
			if agentType == AgentNameTinglyBox {
				projectPath := resolveProjectPath(adapter, ctx.ChatID, string(ctx.Platform))
				var parts []string
				parts = append(parts, "Agent: Smart Guide (@tb)")
				parts = append(parts, "Status: Stateless (no session)")
				if projectPath != "" {
					parts = append(parts, fmt.Sprintf("Project: %s", projectPath))
				}
				return sendCommandText(adapter, ctx, strings.Join(parts, "\n"))
			}

			// For other agents (claude, mock), find the session
			projectPath := resolveProjectPath(adapter, ctx.ChatID, string(ctx.Platform))
			if projectPath == "" {
				return sendCommandText(adapter, ctx, "No project bound. Use /cd <project_path> first.")
			}

			sess, err := adapter.GetSession(ctx.ChatID, agentType, projectPath)
			if err != nil {
				return sendCommandTextf(adapter, ctx, "No session found for agent %s in project %s", agentType, projectPath)
			}

			var parts []string
			parts = append(parts, fmt.Sprintf("Agent: %s", GetAgentDisplayName(agentType)))
			parts = append(parts, fmt.Sprintf("Session: %s", sess.ID))
			parts = append(parts, fmt.Sprintf("Status: %s", sess.Status))

			// Show running duration if running
			if sess.Status == "running" && !sess.LastActivity.IsZero() {
				runningFor := time.Since(sess.LastActivity).Round(time.Second)
				parts = append(parts, fmt.Sprintf("Running for: %s", runningFor))
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

			return sendCommandText(adapter, ctx, strings.Join(parts, "\n"))
		}).
		WithCategory("project").
		WithPriority(50).
		MustBuild()
}

func newBashCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-bash", "bash", "Execute bash commands (cd, ls, pwd)").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			subcommandArg, ok := firstArg(args)
			if !ok {
				return sendCommandText(adapter, ctx, usageBash)
			}

			subcommand := strings.ToLower(subcommandArg)
			allowlist := adapter.GetBashAllowlist()

			if _, ok := allowlist[subcommand]; !ok {
				return sendCommandText(adapter, ctx, "Command not allowed.")
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
						return sendCommandText(adapter, ctx, "Unable to resolve working directory.")
					}
					return sendCommandText(adapter, ctx, cwd)
				}
				return sendCommandText(adapter, ctx, baseDir)

			case "cd":
				nextPath, ok := joinedArgs(args[1:])
				if !ok {
					return sendCommandText(adapter, ctx, usageBashCD)
				}
				cdBase := baseDir
				if cdBase == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return sendCommandText(adapter, ctx, "Unable to resolve working directory.")
					}
					cdBase = cwd
				}
				if !filepath.IsAbs(nextPath) {
					nextPath = filepath.Join(cdBase, nextPath)
				}
				if stat, err := os.Stat(nextPath); err != nil || !stat.IsDir() {
					return sendCommandText(adapter, ctx, "Directory not found.")
				}
				absPath, err := filepath.Abs(nextPath)
				if err == nil {
					nextPath = absPath
				}
				if err := adapter.SetBashCwd(ctx.ChatID, nextPath); err != nil {
					logrus.WithError(err).Warn("Failed to update bash cwd")
				}
				return sendCommandTextf(adapter, ctx, "Bash working directory set to %s", nextPath)

			case "ls":
				if baseDir == "" {
					cwd, err := os.Getwd()
					if err != nil {
						return sendCommandText(adapter, ctx, "Unable to resolve working directory.")
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
					return sendCommandTextf(adapter, ctx, "Command failed: %v", err)
				}
				return sendCommandText(adapter, ctx, strings.TrimSpace(string(output)))

			default:
				return sendCommandText(adapter, ctx, "Command not allowed.")
			}
		}).
		WithCategory("system").
		WithPriority(40).
		MustBuild()
}

func newJoinCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-join", "join", "Add group to whitelist (Telegram only)").
		WithPlatforms(imbot.PlatformTelegram).
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

func newYoloCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-yolo", "yolo", "Toggle auto-approve mode (Claude Code only)").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			agentType, _ := adapter.GetCurrentAgent(ctx.ChatID)
			if agentType != AgentNameClaude {
				return sendCommandText(adapter, ctx, "⚠️ Auto-approve mode is only available for Claude Code (@cc).\n\nSwitch to Claude Code first with: @cc")
			}

			projectPath := resolveProjectPath(adapter, ctx.ChatID, string(ctx.Platform))
			if projectPath == "" {
				return sendCommandText(adapter, ctx, "No project path found. Use /cd <project_path> to create a session first.")
			}

			// Find or create session
			sess, err := adapter.FindOrCreateSession(ctx.ChatID, AgentNameClaude, projectPath)
			if err != nil {
				return sendCommandTextf(adapter, ctx, "Failed to get session: %v", err)
			}

			// Toggle permission mode between bypassPermissions (yolo ON) and default (yolo OFF)
			newMode := string(claude.PermissionModeBypassPermissions)
			if sess.PermissionMode == string(claude.PermissionModeBypassPermissions) {
				newMode = string(claude.PermissionModeDefault)
			}

			if err := adapter.UpdatePermissionMode(sess.ID, newMode); err != nil {
				return sendCommandTextf(adapter, ctx, "Failed to update permission mode: %v", err)
			}

			if newMode == string(claude.PermissionModeBypassPermissions) {
				return sendCommandTextf(adapter, ctx, "🚀 **YOLO MODE ENABLED**\n\nAll permissions will be auto-approved for this session.\n⚠️ Use with caution!\n\nSession: %s\nProject: %s", sess.ID, projectPath)
			}
			return sendCommandTextf(adapter, ctx, "🔒 **YOLO MODE DISABLED**\n\nBack to normal approval mode.\nAll permission requests will require confirmation.\n\nSession: %s\nProject: %s", sess.ID, projectPath)
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
				return sendCommandText(adapter, ctx, fmt.Sprintf("📢 Verbose mode: %s\n\n%s", status, usageVerbose))
			}

			enabled, valid := parseToggleArg(args[0])
			if !valid {
				return sendCommandText(adapter, ctx, usageVerbose+"\n\nExample: /verbose on")
			}

			adapter.SetVerbose(ctx.ChatID, enabled)
			if enabled {
				return sendCommandText(adapter, ctx, "✅ Verbose mode enabled\n\nAll message details will be shown.")
			}
			return sendCommandText(adapter, ctx, "🔇 Quiet mode enabled\n\nOnly final results will be shown.")
		}).
		WithCategory("advanced").
		WithPriority(5).
		MustBuild()
}

// newQuietCommand creates the /quiet command (alias for /verbose off)
// This is a convenient shorthand to disable verbose mode
func newQuietCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-quiet", "quiet", "Disable verbose mode (alias for /verbose off)").
		WithAliases("noverbose").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			adapter.SetVerbose(ctx.ChatID, false)
			return sendCommandText(adapter, ctx, "🔇 Quiet mode enabled\n\nOnly final results will be shown. Use /verbose on to show all details.")
		}).
		WithCategory("advanced").
		WithPriority(4).
		MustBuild()
}

func newMockCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-mock", "mock", "Test with mock agent").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			return sendCommandText(adapter, ctx, "Mock agent not implemented in new system yet.")
		}).
		Hidden().
		WithCategory("advanced").
		MustBuild()
}

// newPairBindCommand registers the `/bind <code>` command used by an end-user
// to pair their direct chat with the bot. It is the only command an unpaired
// chat is allowed to invoke when RequirePairing is on.
func newPairBindCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-pair-bind", "bind",
		"Pair this chat with the bot using the operator's pairing code").
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

// newResumeCommand registers `/resume`. With no args it lists the most recent
// Claude on-disk sessions for the chat's currently-bound project; `/resume <n>`
// arms the Nth session from that list so the user's next message resumes it.
func newResumeCommand(adapter BotHandlerAdapter) imbot.Command {
	return imbot.NewCommand("cmd-resume", "resume", "List or resume recent Claude sessions in the current project").
		WithAliases("r").
		WithHandler(func(ctx *imbot.HandlerContext, args []string) error {
			agentType, _ := adapter.GetCurrentAgent(ctx.ChatID)
			if agentType != AgentNameClaude {
				return sendCommandText(adapter, ctx,
					"⚠️ /resume only works with Claude Code (@cc). Switch with: @cc")
			}

			projectPath := resolveProjectPath(adapter, ctx.ChatID, string(ctx.Platform))
			if projectPath == "" {
				return sendCommandText(adapter, ctx,
					"No project bound. Use /cd <path> or /project to bind one first.")
			}

			// /resume <n>: pick from the previously displayed listing.
			if input, ok := firstArg(args); ok {
				idx, ok := parsePositiveInt(input)
				if !ok {
					return sendCommandText(adapter, ctx, usageResume)
				}
				ids := adapter.RecallResumeListing(ctx.ChatID)
				if len(ids) == 0 {
					return sendCommandText(adapter, ctx,
						"No recent /resume listing in this chat. Run /resume first to see options.")
				}
				if idx < 1 || idx > len(ids) {
					return sendCommandTextf(adapter, ctx,
						"Invalid number: %d (have %d). Run /resume to refresh the list.", idx, len(ids))
				}
				picked := ids[idx-1]
				if err := adapter.PrepareResume(ctx.ChatID, agentType, projectPath, picked); err != nil {
					return sendCommandTextf(adapter, ctx, "Failed to arm resume: %v", err)
				}
				return sendCommandTextf(adapter, ctx,
					"✅ Armed resume for session %s.\nSend your next message to continue, or /clear to abort.",
					shortSessionID(picked))
			}

			// /resume: list recent sessions for the bound project.
			sessions, err := adapter.ListResumableSessions(projectPath, resumeListLimit)
			if err != nil {
				return sendCommandTextf(adapter, ctx, "Failed to list sessions: %v", err)
			}
			if len(sessions) == 0 {
				return sendCommandText(adapter, ctx, fmt.Sprintf(
					"No prior sessions in %s.\nSwitch project with /p (list) + /cd <n>, or just send a message to start a new one.",
					ShortenPath(projectPath)))
			}

			ids := make([]string, 0, len(sessions))
			for _, s := range sessions {
				ids = append(ids, s.SessionID)
			}
			adapter.RememberResumeListing(ctx.ChatID, ids)

			text := buildResumeListText(projectPath, sessions)

			// DM on a platform with inline keyboards: also surface buttons so
			// users can tap a session instead of typing /resume <n>. Group
			// chats and platforms without interaction stay text-only.
			caps := imbot.GetPlatformCapabilities(string(ctx.Platform))
			if ctx.IsDirectMessage && caps != nil && caps.SupportsInteraction() {
				keyboard := buildResumeKeyboard(sessions)
				tgKeyboard := imbot.BuildTelegramActionKeyboard(keyboard)
				_, err := ctx.Bot.SendMessage(context.Background(), ctx.ChatID, &imbot.SendMessageOptions{
					Text:     text + adapter.BuildReplyFooter(ctx.ChatID, string(ctx.Platform)),
					Metadata: buildTrackedReplyMetadata(tgKeyboard),
				})
				if err != nil {
					logrus.WithError(err).Error("Failed to send resume list with keyboard")
					return sendCommandText(adapter, ctx, text)
				}
				return nil
			}

			return sendCommandText(adapter, ctx, text)
		}).
		WithCategory("session").
		WithPriority(85).
		MustBuild()
}

// buildResumeKeyboard renders a 2-buttons-per-row keyboard for /resume in
// DMs. Each button's label stays compact ("#1 · 2h · 14t") because the
// detailed previews already live in the message body; the button is just a
// tap-target keyed to the session's index. Callback data carries the actual
// session_id so the handler doesn't need to re-read the listing cache.
func buildResumeKeyboard(sessions []ResumableSession) imbot.InlineKeyboardMarkup {
	const cols = 2
	var rows [][]imbot.InlineKeyboardButton
	var current []imbot.InlineKeyboardButton
	for i, s := range sessions {
		label := fmt.Sprintf("#%d · %s · %s", i+1, formatRelativeTime(latestTime(s)), formatTurns(s.NumTurns))
		current = append(current, imbot.InlineKeyboardButton{
			Text:         label,
			CallbackData: imbot.FormatCallbackData("resume", "pick", s.SessionID),
		})
		if len(current) == cols {
			rows = append(rows, current)
			current = nil
		}
	}
	if len(current) > 0 {
		rows = append(rows, current)
	}
	rows = append(rows, []imbot.InlineKeyboardButton{{
		Text:         "Cancel",
		CallbackData: imbot.FormatCallbackData("resume", "cancel"),
	}})
	return imbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// buildResumeListText renders the compact one-line-per-session list shown by
// /resume. Lines are kept short so phone screens can fit ~10 entries without
// wrapping into noise.
func buildResumeListText(projectPath string, sessions []ResumableSession) string {
	var buf strings.Builder
	buf.WriteString("Recent sessions in ")
	buf.WriteString(ShortenPath(projectPath))
	buf.WriteString(":\n")
	for i, s := range sessions {
		fmt.Fprintf(&buf, "  %d. %s · %s · %s%s\n",
			i+1,
			formatRelativeTime(latestTime(s)),
			formatTurns(s.NumTurns),
			truncateRunes(strings.ReplaceAll(s.FirstMessage, "\n", " "), 50),
			statusGlyph(s.Status),
		)
	}
	buf.WriteString("\nUse /resume <number> to resume. Switch project with /p + /cd.")
	return buf.String()
}

func latestTime(s ResumableSession) time.Time {
	if !s.EndTime.IsZero() {
		return s.EndTime
	}
	return s.StartTime
}

func formatTurns(n int) string {
	if n <= 0 {
		return "—"
	}
	return fmt.Sprintf("%dt", n)
}

// formatRelativeTime renders a coarse "Nm/h/d ago" suitable for compact lists.
// We avoid pulling in a humanize dependency for one call site.
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

func statusGlyph(status string) string {
	switch strings.ToLower(status) {
	case "complete", "completed":
		return " ✓"
	case "error", "failed":
		return " ✗"
	case "active", "running":
		return " …"
	default:
		return ""
	}
}

// truncateRunes trims s to max runes (not bytes) and appends an ellipsis when
// it had to cut. Avoids slicing inside a multi-byte rune, which matters for
// Chinese first-message previews.
func truncateRunes(s string, max int) string {
	if max <= 1 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

func shortSessionID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
