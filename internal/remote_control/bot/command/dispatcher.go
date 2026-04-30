package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tingly-dev/tingly-box/imbot"
)

// Dispatcher handles command execution and routing
type Dispatcher struct {
	registry *Registry
	handler  Handler
}

// NewDispatcher creates a new command dispatcher
func NewDispatcher(handler Handler) *Dispatcher {
	return &Dispatcher{
		registry: NewRegistry(),
		handler:  handler,
	}
}

// Register registers a command with the dispatcher
func (d *Dispatcher) Register(cmd Command) error {
	return d.registry.Register(cmd)
}

// Handle processes a text message and executes the command if found
// Returns (handled, error) where:
// - handled is true if the text was a command and was processed
// - error is any error that occurred during command execution
func (d *Dispatcher) Handle(ctx *Context, text string) (bool, error) {
	cmdName, args := ParseCommand(text)

	// Check if this is a registered command
	if !d.registry.IsCommand(cmdName) {
		return false, nil
	}

	// Lookup the command
	cmd, ok := d.registry.Lookup(cmdName)
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrNotFound, cmdName)
	}

	// Set args in context
	ctx.Args = args

	// Execute the command
	err := cmd.Handler(ctx, d.handler)
	if err != nil {
		return true, fmt.Errorf("command %s failed: %w", cmdName, err)
	}

	return true, nil
}

// BuildHelpText generates formatted help text for all commands
func (d *Dispatcher) BuildHelpText(platform imbot.Platform) string {
	var builder strings.Builder

	builder.WriteString("📚 **Available Commands**\n\n")

	// Group commands by category
	categories := d.registry.ListByCategory()

	// Sort categories for consistent output
	sortedCategories := make([]string, 0, len(categories))
	for cat := range categories {
		sortedCategories = append(sortedCategories, cat)
	}
	sort.Strings(sortedCategories)

	// Build help text for each category
	for _, category := range sortedCategories {
		commands := categories[category]

		// Sort commands by priority (descending)
		sort.Slice(commands, func(i, j int) bool {
			return commands[i].Priority() > commands[j].Priority()
		})

		// Category header
		builder.WriteString(fmt.Sprintf("**%s**\n", strings.ToUpper(category)))

		// Command list
		for _, cmd := range commands {
			var aliases string
			if len(cmd.Aliases()) > 0 {
				aliases = fmt.Sprintf(" (%s)", strings.Join(cmd.Aliases(), ", "))
			}
			builder.WriteString(fmt.Sprintf("  /%s%s - %s\n", cmd.Name(), aliases, cmd.Description()))
		}
		builder.WriteString("\n")
	}

	return builder.String()
}

// BuildHelpTextForCategory generates help text for a specific category
func (d *Dispatcher) BuildHelpTextForCategory(category string) (string, error) {
	commands := d.registry.ListByCategory()

	cmds, ok := commands[category]
	if !ok || len(cmds) == 0 {
		return "", fmt.Errorf("category not found: %s", category)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("**%s**\n\n", strings.ToUpper(category)))

	// Sort by priority
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Priority() > cmds[j].Priority()
	})

	for _, cmd := range cmds {
		var aliases string
		if len(cmd.Aliases()) > 0 {
			aliases = fmt.Sprintf(" (%s)", strings.Join(cmd.Aliases(), ", "))
		}
		builder.WriteString(fmt.Sprintf("  /%s%s - %s\n", cmd.Name(), aliases, cmd.Description()))
	}

	return builder.String(), nil
}

// GetRegistry returns the command registry (for testing/inspection)
func (d *Dispatcher) GetRegistry() *Registry {
	return d.registry
}

// GetCommand returns a command by name (for testing/inspection)
func (d *Dispatcher) GetCommand(name string) (Command, bool) {
	return d.registry.Lookup(name)
}

// ListCommands returns all registered commands
func (d *Dispatcher) ListCommands() []Command {
	return d.registry.List()
}

// CommandCount returns the number of registered commands
func (d *Dispatcher) CommandCount() int {
	return d.registry.Count()
}
