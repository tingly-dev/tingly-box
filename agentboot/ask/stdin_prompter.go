package ask

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// StdinPrompter implements Prompter using stdin/stdout
type StdinPrompter struct {
	// Debug enables verbose output
	Debug bool

	// Colors for terminal output (can be disabled by setting to empty strings)
	ColorReset  string
	ColorRed    string
	ColorGreen  string
	ColorYellow string
	ColorCyan   string

	// Registry for tool-specific handlers
	registry *ToolHandlerRegistry
}

// NewStdinPrompter creates a new StdinPrompter with default colors
func NewStdinPrompter() *StdinPrompter {
	return &StdinPrompter{
		ColorReset:  "\033[0m",
		ColorRed:    "\033[31m",
		ColorGreen:  "\033[32m",
		ColorYellow: "\033[33m",
		ColorCyan:   "\033[36m",
		registry:    NewToolHandlerRegistry(),
	}
}

// NewStdinPrompterDebug creates a new StdinPrompter with debug enabled
func NewStdinPrompterDebug() *StdinPrompter {
	p := NewStdinPrompter()
	p.Debug = true
	return p
}

// Prompt prompts the user via stdin for response
func (p *StdinPrompter) Prompt(ctx context.Context, req Request) (Result, error) {
	// Check if this is an AskUserQuestion tool - handle specially
	if req.ToolName == "AskUserQuestion" {
		return p.promptAskUserQuestion(ctx, req)
	}

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
		return Result{ID: req.ID, Approved: false}, ctx.Err()
	case r := <-resultChan:
		if r.err != nil {
			return Result{ID: req.ID, Approved: false}, r.err
		}

		switch r.response {
		case "y", "yes":
			return Result{
				ID:           req.ID,
				Approved:     true,
				UpdatedInput: req.Input,
			}, nil
		case "a", "always", "al":
			return Result{
				ID:           req.ID,
				Approved:     true,
				UpdatedInput: req.Input,
				Remember:     true,
			}, nil
		case "n", "no":
			return Result{ID: req.ID, Approved: false}, nil
		default:
			// Invalid response - treat as deny with message
			return Result{ID: req.ID, Approved: false}, nil
		}
	}
}

// promptAskUserQuestion handles AskUserQuestion tool with multi-option selection
func (p *StdinPrompter) promptAskUserQuestion(ctx context.Context, req Request) (Result, error) {
	questions, ok := req.Input["questions"].([]interface{})
	if !ok || len(questions) == 0 {
		return Result{
			ID:           req.ID,
			Approved:     true,
			UpdatedInput: req.Input,
		}, nil
	}

	// Display the questions and options
	fmt.Printf("\r%s[Question]%s\n", p.ColorYellow, p.ColorReset)

	answers := make(map[string]interface{})

	for i, q := range questions {
		question, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		questionText, _ := question["question"].(string)
		header, _ := question["header"].(string)

		if header != "" {
			fmt.Printf("\n%s[%s]%s\n", p.ColorCyan, header, p.ColorReset)
		}
		fmt.Printf("%s\n", questionText)

		// Show options
		options, ok := question["options"].([]interface{})
		if !ok || len(options) == 0 {
			continue
		}

		fmt.Printf("\nOptions:\n")
		for j, opt := range options {
			option, ok := opt.(map[string]interface{})
			if !ok {
				continue
			}
			label, _ := option["label"].(string)
			desc, _ := option["description"].(string)
			if desc != "" {
				fmt.Printf("  %s%d%s. %s - %s\n", p.ColorGreen, j+1, p.ColorReset, label, desc)
			} else {
				fmt.Printf("  %s%d%s. %s\n", p.ColorGreen, j+1, p.ColorReset, label)
			}
		}

		// Prompt for selection
		fmt.Printf("\n%sSelect option (1-%d) or type label: %s", p.ColorGreen, len(options), p.ColorReset)

		// Read user input
		type result struct {
			response string
			err      error
		}
		resultChan := make(chan result, 1)

		go func() {
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			response = strings.TrimSpace(response)
			resultChan <- result{response: response, err: err}
		}()

		// Wait for input or context cancellation
		select {
		case <-ctx.Done():
			return Result{ID: req.ID, Approved: false}, ctx.Err()
		case r := <-resultChan:
			if r.err != nil {
				return Result{ID: req.ID, Approved: false}, r.err
			}

			// Try to parse as number
			var selectedIndex int = -1
			var selectedLabel string

			// Try numeric parsing
			var num int
			if _, err := fmt.Sscanf(r.response, "%d", &num); err == nil {
				if num >= 1 && num <= len(options) {
					selectedIndex = num - 1
				}
			}

			// If not a valid number, try matching by label
			if selectedIndex < 0 {
				for j, opt := range options {
					if option, ok := opt.(map[string]interface{}); ok {
						if label, ok := option["label"].(string); ok {
							if strings.EqualFold(label, r.response) {
								selectedIndex = j
								break
							}
						}
					}
				}
			}

			// If still not found, use the raw input as label
			if selectedIndex < 0 {
				selectedLabel = r.response
			} else if selectedIndex >= 0 && selectedIndex < len(options) {
				if option, ok := options[selectedIndex].(map[string]interface{}); ok {
					if label, ok := option["label"].(string); ok {
						selectedLabel = label
					}
				}
			}

			// Store answer (using index as key for now)
			answers[fmt.Sprintf("%d", i)] = selectedLabel
		}
	}

	// Build updated input with answers
	updatedInput := make(map[string]interface{})
	for k, v := range req.Input {
		updatedInput[k] = v
	}
	updatedInput["answers"] = answers

	return Result{
		ID:           req.ID,
		Approved:     true,
		UpdatedInput: updatedInput,
	}, nil
}

// NoOpPrompter is a prompter that auto-approves everything
type NoOpPrompter struct{}

// NewNoOpPrompter creates a new NoOpPrompter
func NewNoOpPrompter() *NoOpPrompter {
	return &NoOpPrompter{}
}

// Prompt always approves without user interaction
func (p *NoOpPrompter) Prompt(ctx context.Context, req Request) (Result, error) {
	return Result{
		ID:           req.ID,
		Approved:     true,
		UpdatedInput: req.Input,
	}, nil
}

// DenyAllPrompter is a prompter that denies everything
type DenyAllPrompter struct{}

// NewDenyAllPrompter creates a new DenyAllPrompter
func NewDenyAllPrompter() *DenyAllPrompter {
	return &DenyAllPrompter{}
}

// Prompt always denies without user interaction
func (p *DenyAllPrompter) Prompt(ctx context.Context, req Request) (Result, error) {
	return Result{ID: req.ID, Approved: false}, nil
}

// Legacy adapters for backward compatibility

// StdinPrompterFromLegacy creates a StdinPrompter from legacy config
func StdinPrompterFromLegacy() *StdinPrompter {
	return NewStdinPrompter()
}

// ToLegacyUserPrompter creates a legacy-compatible prompter wrapper
func ToLegacyUserPrompter(p Prompter) UserPrompter {
	return &legacyPrompterWrapper{p: p}
}

// UserPrompter is the legacy interface for backward compatibility
type UserPrompter interface {
	PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error)
}

// legacyPrompterWrapper wraps a Prompter to implement UserPrompter
type legacyPrompterWrapper struct {
	p Prompter
}

// PromptPermission implements UserPrompter
func (w *legacyPrompterWrapper) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	askReq := FromPermissionRequest(req)
	result, err := w.p.Prompt(ctx, *askReq)
	if err != nil {
		return agentboot.PermissionResult{}, err
	}
	return result.ToPermissionResult(), nil
}
