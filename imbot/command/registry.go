// Package command provides a simple, generic command management system for bots.
package command

import (
	"fmt"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// CommandHandler is the function signature for command handlers.
// It receives the handler context and command arguments, returning an error if failed.
type CommandHandler func(ctx *HandlerContext, args []string) error

// Command represents a command definition.
type Command struct {
	// ID is the unique identifier for this command
	ID string

	// Name is the primary command name (without slash)
	Name string

	// Aliases are alternative names for this command
	Aliases []string

	// Description is the user-facing description
	Description string

	// Category groups related commands (e.g., "session", "project", "system")
	Category string

	// Handler is the function to execute when this command is invoked
	Handler CommandHandler

	// Hidden hides this command from menus (but command still works)
	Hidden bool

	// Priority determines display order (higher = first)
	Priority int
}

// Match checks if a name matches this command (name or alias).
// The name should be provided without the slash prefix.
func (c *Command) Match(name string) bool {
	if c.Name == name {
		return true
	}
	for _, alias := range c.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}

// IsHidden returns true if the command should be hidden from menus.
func (c *Command) IsHidden() bool {
	return c.Hidden
}

// AllNames returns all names that can match this command (name + aliases).
func (c *Command) AllNames() []string {
	names := make([]string, 0, len(c.Aliases)+1)
	names = append(names, c.Name)
	names = append(names, c.Aliases...)
	return names
}

// Validate checks if the command is valid.
func (c *Command) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("command ID cannot be empty")
	}
	if c.Name == "" {
		return fmt.Errorf("command name cannot be empty")
	}
	if c.Handler == nil {
		return fmt.Errorf("handler required for command %s", c.ID)
	}
	return nil
}

// CommandRegistry manages command registration and lookup.
type CommandRegistry struct {
	mu       sync.RWMutex
	commands map[string]*Command // by ID
	byName   map[string]*Command // by name/alias
}

// NewRegistry creates a new command registry.
func NewRegistry() *CommandRegistry {
	return &CommandRegistry{
		commands: make(map[string]*Command),
		byName:   make(map[string]*Command),
	}
}

// Register adds a command to the registry.
// The command is validated and indexed by ID, name, and aliases.
func (r *CommandRegistry) Register(cmd Command) error {
	if err := cmd.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Store by ID
	r.commands[cmd.ID] = &cmd

	// Index by name (primary)
	if _, exists := r.byName[cmd.Name]; exists {
		return fmt.Errorf("command name '%s' already registered", cmd.Name)
	}
	r.byName[cmd.Name] = &cmd

	// Index by aliases
	for _, alias := range cmd.Aliases {
		if _, exists := r.byName[alias]; exists {
			return fmt.Errorf("command alias '%s' already registered", alias)
		}
		r.byName[alias] = &cmd
	}

	return nil
}

// Get returns a command by name or alias.
func (r *CommandRegistry) Get(name string) (*Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.byName[name]
	return cmd, ok
}

// GetByID returns a command by ID.
func (r *CommandRegistry) GetByID(id string) (*Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.commands[id]
	return cmd, ok
}

// Match finds a command handler by name.
// Returns the handler and true if found, nil and false otherwise.
func (r *CommandRegistry) Match(name string) (CommandHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cmd, ok := r.byName[name]
	if !ok {
		return nil, false
	}
	return cmd.Handler, true
}

// All returns all registered commands sorted by priority.
func (r *CommandRegistry) All() []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Command, 0)
	for _, cmd := range r.commands {
		result = append(result, cmd)
	}

	sortByPriority(result)
	return result
}

// ForCategory returns commands in a specific category.
func (r *CommandRegistry) ForCategory(category string) []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Command, 0)
	for _, cmd := range r.commands {
		if cmd.Category == category && !cmd.Hidden {
			result = append(result, cmd)
		}
	}

	sortByPriority(result)
	return result
}

// ForPlatform returns commands visible for a specific platform.
// Currently returns non-hidden commands (platform filtering can be added later).
func (r *CommandRegistry) ForPlatform(platform core.Platform) []*Command {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Command, 0)
	for _, cmd := range r.commands {
		if !cmd.Hidden {
			result = append(result, cmd)
		}
	}

	sortByPriority(result)
	return result
}

// Count returns the number of registered commands.
func (r *CommandRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.commands)
}

// sortByPriority sorts commands by priority (higher first).
func sortByPriority(cmds []*Command) {
	for i := 0; i < len(cmds)-1; i++ {
		for j := i + 1; j < len(cmds); j++ {
			if cmds[i].Priority < cmds[j].Priority {
				cmds[i], cmds[j] = cmds[j], cmds[i]
			}
		}
	}
}

// BuildHelpText generates help text for a chat type.
func (r *CommandRegistry) BuildHelpText(isDirectMessage bool) string {
	commands := r.All()

	var text strings.Builder
	if isDirectMessage {
		text.WriteString("Bot Commands:\n")
	} else {
		text.WriteString("Group Chat Commands:\n")
	}

	// Group by category
	categories := make(map[string][]*Command)
	for _, cmd := range commands {
		if cmd.Hidden {
			continue
		}
		cat := cmd.Category
		if cat == "" {
			cat = "other"
		}
		categories[cat] = append(categories[cat], cmd)
	}

	// Define category order
	order := []string{"session", "project", "system", "advanced", "other"}

	for _, cat := range order {
		cmds, ok := categories[cat]
		if !ok || len(cmds) == 0 {
			continue
		}

		// Add category header if not first
		if cat != "session" {
			text.WriteString("\n")
		}

		for _, cmd := range cmds {
			aliases := ""
			if len(cmd.Aliases) > 0 {
				aliasStr := ""
				for i, a := range cmd.Aliases {
					if i > 0 {
						aliasStr += ", "
					}
					aliasStr += "/" + a
				}
				aliases = " (" + aliasStr + ")"
			}
			text.WriteString(fmt.Sprintf("/%s%s - %s\n", cmd.Name, aliases, cmd.Description))
		}
	}

	return text.String()
}
