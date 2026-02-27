package permission

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// UserPrompter handles user interaction for permission requests
type UserPrompter interface {
	// PromptPermission prompts the user for permission decision
	// Returns: approved (whether to allow), remember (whether to remember this decision), error
	PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (approved bool, remember bool, err error)
}

// StdinPrompter implements UserPrompter using stdin/stdout
type StdinPrompter struct {
	// Debug enables verbose output
	Debug bool

	// Colors for terminal output (can be disabled by setting to empty strings)
	ColorReset  string
	ColorRed    string
	ColorGreen  string
	ColorYellow string
	ColorCyan   string
}

// NewStdinPrompter creates a new StdinPrompter with default colors
func NewStdinPrompter() *StdinPrompter {
	return &StdinPrompter{
		ColorReset:  "\033[0m",
		ColorRed:    "\033[31m",
		ColorGreen:  "\033[32m",
		ColorYellow: "\033[33m",
		ColorCyan:   "\033[36m",
	}
}

// NewStdinPrompterDebug creates a new StdinPrompter with debug enabled
func NewStdinPrompterDebug() *StdinPrompter {
	p := NewStdinPrompter()
	p.Debug = true
	return p
}

// PromptPermission prompts the user via stdin for permission decision
func (p *StdinPrompter) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (bool, bool, error) {
	// Display permission request
	fmt.Printf("\r%s[Tool Permission]%s Claude wants to use: %s%s\n",
		p.ColorYellow, p.ColorReset, p.ColorCyan, req.ToolName)

	// Show relevant input details
	if cmd, ok := req.Input["command"].(string); ok {
		fmt.Printf("%sCommand%s: %s\n", p.ColorCyan, p.ColorReset, cmd)
	} else if p.Debug {
		fmt.Printf("%sInput%s: %+v\n", p.ColorCyan, p.ColorReset, req.Input)
	}

	fmt.Printf("%sAllow?%s (y=yes/n=no/a=always): ", p.ColorGreen, p.ColorReset)

	// Read user input
	type result struct {
		response string
		err      error
	}
	resultChan := make(chan result, 1)

	go func() {
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		resultChan <- result{response: response, err: err}
	}()

	// Wait for input or context cancellation
	select {
	case <-ctx.Done():
		return false, false, ctx.Err()
	case r := <-resultChan:
		if r.err != nil {
			return false, false, r.err
		}

		switch r.response {
		case "y", "yes":
			return true, false, nil
		case "a", "always", "al":
			return true, true, nil
		case "n", "no":
			return false, false, nil
		default:
			// Invalid response - treat as deny with message
			return false, false, nil
		}
	}
}

// NoOpPrompter is a prompter that auto-approves everything
type NoOpPrompter struct{}

// NewNoOpPrompter creates a new NoOpPrompter
func NewNoOpPrompter() *NoOpPrompter {
	return &NoOpPrompter{}
}

// PromptPermission always approves without user interaction
func (p *NoOpPrompter) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (bool, bool, error) {
	return true, false, nil
}

// DenyAllPrompter is a prompter that denies everything
type DenyAllPrompter struct{}

// NewDenyAllPrompter creates a new DenyAllPrompter
func NewDenyAllPrompter() *DenyAllPrompter {
	return &DenyAllPrompter{}
}

// PromptPermission always denies without user interaction
func (p *DenyAllPrompter) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (bool, bool, error) {
	return false, false, nil
}
