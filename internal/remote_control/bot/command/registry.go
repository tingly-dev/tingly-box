package command

import (
	"strings"
)

// Registry manages command registration and lookup
type Registry struct {
	commands   map[string]Command
	aliases    map[string]string // alias -> command name
	categories []string
}

// NewRegistry creates a new command registry
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
	}
}

// Register registers a command with the registry
func (r *Registry) Register(cmd Command) error {
	// Register primary name
	r.commands[cmd.Name()] = cmd

	// Register aliases
	for _, alias := range cmd.Aliases() {
		r.aliases[alias] = cmd.Name()
	}

	// Track category if not already tracked
	if !r.hasCategory(cmd.Category()) {
		r.categories = append(r.categories, cmd.Category())
	}

	return nil
}

// Unregister removes a command from the registry
func (r *Registry) Unregister(name string) {
	cmd, exists := r.commands[name]
	if !exists {
		return
	}

	// Remove primary command
	delete(r.commands, name)

	// Remove aliases
	for _, alias := range cmd.Aliases() {
		delete(r.aliases, alias)
	}
}

// Lookup finds a command by name or alias
func (r *Registry) Lookup(name string) (Command, bool) {
	// Try direct name lookup first
	if cmd, ok := r.commands[name]; ok {
		return cmd, true
	}

	// Try alias lookup
	if realName, ok := r.aliases[name]; ok {
		if cmd, ok := r.commands[realName]; ok {
			return cmd, true
		}
	}

	return nil, false
}

// List returns all registered commands (excluding hidden)
func (r *Registry) List() []Command {
	var cmds []Command
	for _, cmd := range r.commands {
		if !cmd.Hidden() {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

// ListByCategory returns commands grouped by category
func (r *Registry) ListByCategory() map[string][]Command {
	result := make(map[string][]Command)
	for _, cmd := range r.commands {
		if cmd.Hidden() {
			continue
		}
		result[cmd.Category()] = append(result[cmd.Category()], cmd)
	}
	return result
}

// GetCategories returns all registered categories
func (r *Registry) GetCategories() []string {
	return r.categories
}

// Count returns the number of registered commands
func (r *Registry) Count() int {
	return len(r.commands)
}

// hasCategory checks if a category is already tracked
func (r *Registry) hasCategory(category string) bool {
	for _, c := range r.categories {
		if c == category {
			return true
		}
	}
	return false
}

// IsCommand checks if a given name is a registered command or alias
func (r *Registry) IsCommand(name string) bool {
	_, ok := r.commands[name]
	if ok {
		return true
	}
	_, ok = r.aliases[name]
	return ok
}

// ParseCommand extracts the command name from a text string
// Handles both "/command" and "command" formats
func ParseCommand(text string) (string, []string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}

	// Split into words
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", nil
	}

	// Remove leading slash if present
	cmdName := strings.TrimPrefix(parts[0], "/")

	// Return command name and remaining arguments
	if len(parts) > 1 {
		return cmdName, parts[1:]
	}
	return cmdName, nil
}

// NormalizeCommandName normalizes a command name by removing leading slash
// and converting to lowercase
func NormalizeCommandName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	return strings.ToLower(name)
}
