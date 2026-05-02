// Package command provides a simple, generic command management system for bots.
package command

import "github.com/tingly-dev/tingly-box/imbot/core"

// CommandBuilder provides a fluent API for command definition.
type CommandBuilder struct {
	cmd Command
}

// NewCommand creates a new command builder.
// The ID should be unique, name is the primary command name (without slash).
func NewCommand(id, name, description string) *CommandBuilder {
	return &CommandBuilder{
		cmd: Command{
			ID:          id,
			Name:        name,
			Description: description,
			Aliases:     make([]string, 0),
		},
	}
}

// WithHandler sets the handler function for the command.
func (b *CommandBuilder) WithHandler(handler CommandHandler) *CommandBuilder {
	b.cmd.Handler = handler
	return b
}

// WithAliases adds command aliases.
func (b *CommandBuilder) WithAliases(aliases ...string) *CommandBuilder {
	b.cmd.Aliases = append(b.cmd.Aliases, aliases...)
	return b
}

// WithCategory sets the command category.
func (b *CommandBuilder) WithCategory(category string) *CommandBuilder {
	b.cmd.Category = category
	return b
}

// WithPriority sets the display priority (higher = first).
func (b *CommandBuilder) WithPriority(priority int) *CommandBuilder {
	b.cmd.Priority = priority
	return b
}

// Hidden marks the command as hidden from menus.
func (b *CommandBuilder) Hidden() *CommandBuilder {
	b.cmd.Hidden = true
	return b
}

// WithPlatforms restricts the command to specific platforms.
// If not called, the command is available on all platforms.
func (b *CommandBuilder) WithPlatforms(platforms ...core.Platform) *CommandBuilder {
	b.cmd.Platforms = append(b.cmd.Platforms, platforms...)
	return b
}

// WithMenuLabel sets the display text for menus (optional).
func (b *CommandBuilder) WithMenuLabel(label string) *CommandBuilder {
	b.cmd.MenuLabel = label
	return b
}

// WithMenuIcon sets the icon for platforms that support it (e.g., Feishu/Lark).
func (b *CommandBuilder) WithMenuIcon(icon string) *CommandBuilder {
	b.cmd.MenuIcon = icon
	return b
}

// WithMenuGroup sets the group for organizing related commands in menus.
func (b *CommandBuilder) WithMenuGroup(group string) *CommandBuilder {
	b.cmd.MenuGroup = group
	return b
}

// Build validates and returns the command.
func (b *CommandBuilder) Build() (Command, error) {
	if err := b.cmd.Validate(); err != nil {
		return Command{}, err
	}
	return b.cmd, nil
}

// MustBuild panics if validation fails.
// Useful for package-level command definitions.
func (b *CommandBuilder) MustBuild() Command {
	cmd, err := b.Build()
	if err != nil {
		panic(err)
	}
	return cmd
}
