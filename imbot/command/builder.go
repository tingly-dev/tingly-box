// Package command provides a simple, generic command management system for bots.
package command

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
