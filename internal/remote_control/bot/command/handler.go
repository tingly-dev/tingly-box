// Package command provides a clean command handling system for the bot.
// It separates command definition from execution and provides a simplified
// interface for command handlers to interact with the bot.
package command

import (
	"github.com/tingly-dev/tingly-box/imbot"
)

// Handler defines the interface that command handlers need.
// This is a simplified version of BotHandlerAdapter, focused only on
// the essential methods that commands actually need.
type Handler interface {
	// SendText sends a text message to a chat
	SendText(chatID, text string) error

	// GetProjectPath gets the current project path for a chat
	GetProjectPath(chatID string) (string, error)

	// SetProjectPath sets the project path for a chat
	SetProjectPath(chatID, path string) error

	// GetProjectPathForGroup gets project path with group fallback
	GetProjectPathForGroup(chatID, platform string) (string, bool)

	// StopExecution stops a running execution, returns true if one was running
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

	// VerifyAndPair verifies a one-time pairing code and pairs the chat
	VerifyAndPair(botUUID, chatID, senderID, platform, code string) error

	// ClearSession clears a session
	ClearSession(chatID, agentType string) error

	// FindOrCreateSession finds an existing session or creates a new one
	FindOrCreateSession(chatID, agentType, projectPath string) (*SessionInfo, error)

	// UpdatePermissionMode updates the permission mode for a session
	UpdatePermissionMode(sessionID, mode string) error

	// GetSession gets session info
	GetSession(chatID, agentType, projectPath string) (*SessionInfo, error)
}

// SessionInfo holds session information for commands
type SessionInfo struct {
	ID             string
	Status         string
	Project        string
	Request        string
	Error          string
	PermissionMode string
	LastActivity   interface{} // Can be time.Time or similar
}

// Context provides command execution context.
// It encapsulates all the information a command needs to execute.
type Context struct {
	// Chat identification
	ChatID    string
	SenderID  string
	BotUUID   string
	Platform  imbot.Platform
	MessageID string

	// Message content
	Text   string
	Args   []string

	// Flags
	IsDirect   bool
	IsPlatform func(imbot.Platform) bool

	// Bot reference for sending messages
	Bot imbot.Bot
}

// IsPlatformFunc returns a function that checks if the context matches a specific platform
func (c *Context) IsPlatformFunc(platform imbot.Platform) bool {
	return c.Platform == platform
}

// Command represents a bot command
type Command interface {
	// ID returns the unique identifier for this command
	ID() string

	// Name returns the command name (e.g., "help", "cd")
	Name() string

	// Description returns a brief description of what the command does
	Description() string

	// Category returns the command category for grouping in help text
	Category() string

	// Aliases returns alternative names for this command
	Aliases() []string

	// Handler executes the command with the given context and arguments
	Handler(ctx *Context, handler Handler) error

	// Hidden returns whether this command should be hidden from help text
	Hidden() bool

	// Priority returns the command priority for ordering in help text
	Priority() int
}

// Builder provides a fluent interface for building commands
type Builder struct {
	command *commandImpl
}

// NewBuilder creates a new command builder
func NewBuilder(id, name, description string) *Builder {
	return &Builder{
		command: &commandImpl{
			id:          id,
			name:        name,
			description: description,
			category:    "general",
			aliases:     []string{},
			priority:    0,
			hidden:      false,
		},
	}
}

// WithCategory sets the command category
func (b *Builder) WithCategory(category string) *Builder {
	b.command.category = category
	return b
}

// WithAliases sets the command aliases
func (b *Builder) WithAliases(aliases ...string) *Builder {
	b.command.aliases = aliases
	return b
}

// WithHandler sets the command handler function
func (b *Builder) WithHandler(handler func(*Context, Handler) error) *Builder {
	b.command.handler = handler
	return b
}

// WithPriority sets the command priority
func (b *Builder) WithPriority(priority int) *Builder {
	b.command.priority = priority
	return b
}

// Hidden marks the command as hidden from help text
func (b *Builder) Hidden() *Builder {
	b.command.hidden = true
	return b
}

// Build builds the command
func (b *Builder) Build() Command {
	return b.command
}

// MustBuild builds the command and panics if there's an error
func (b *Builder) MustBuild() Command {
	if b.command.handler == nil {
		panic("command handler is required")
	}
	return b.command
}

// commandImpl is the default implementation of Command
type commandImpl struct {
	id          string
	name        string
	description string
	category    string
	aliases     []string
	handler     func(*Context, Handler) error
	priority    int
	hidden      bool
}

func (c *commandImpl) ID() string              { return c.id }
func (c *commandImpl) Name() string            { return c.name }
func (c *commandImpl) Description() string     { return c.description }
func (c *commandImpl) Category() string        { return c.category }
func (c *commandImpl) Aliases() []string       { return c.aliases }
func (c *commandImpl) Priority() int           { return c.priority }
func (c *commandImpl) Hidden() bool            { return c.hidden }
func (c *commandImpl) Handler(ctx *Context, h Handler) error {
	if c.handler == nil {
		return ErrNoHandler
	}
	return c.handler(ctx, h)
}

// Errors
var (
	ErrNoHandler     = &CommandError{Message: "command has no handler"}
	ErrNotFound      = &CommandError{Message: "command not found"}
	ErrInvalidArgs   = &CommandError{Message: "invalid arguments"}
	ErrNotAuthorized = &CommandError{Message: "not authorized"}
)

// CommandError represents a command-specific error
type CommandError struct {
	Message string
}

func (e *CommandError) Error() string {
	return e.Message
}
