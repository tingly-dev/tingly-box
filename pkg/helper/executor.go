package helper

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Request sent to helper via stdin
type Request struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Provider        string   `json:"provider"`
	IDs             []string `json:"ids"`
}

// Response received from helper via stdout
type Response struct {
	ProtocolVersion int               `json:"protocolVersion"`
	Values          map[string]string `json:"values"`
	Errors          map[string]Error  `json:"errors,omitempty"`
}

// Error represents an error from the helper
type Error struct {
	Message string `json:"message"`
}

// Config for helper execution
type Config struct {
	Command    string
	Args       []string
	TimeoutMs  int
	Env        map[string]string
	PassEnv    []string
	SimpleMode bool
}

// Executor runs external helper commands to fetch credentials
type Executor struct {
	config Config
}

// NewExecutor creates a new helper executor
func NewExecutor(config Config) *Executor {
	if config.TimeoutMs == 0 {
		config.TimeoutMs = 5000
	}
	return &Executor{config: config}
}

// Fetch retrieves credentials from the helper
func (e *Executor) Fetch(ctx context.Context, provider string) (string, error) {
	timeout := time.Duration(e.config.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(ctx, e.config.Command, e.config.Args...)

	// Build environment
	cmd.Env = os.Environ()
	for _, key := range e.config.PassEnv {
		if val, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, val))
		}
	}
	for key, val := range e.config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, val))
	}

	// Build request
	req := Request{
		ProtocolVersion: 1,
		Provider:        provider,
		IDs:             []string{"token"},
	}
	reqJSON, _ := json.Marshal(req)
	cmd.Stdin = strings.NewReader(string(reqJSON))

	// Execute
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("helper timed out after %dms", e.config.TimeoutMs)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("helper execution failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("helper execution failed: %w", err)
	}

	// Parse response
	if e.config.SimpleMode {
		// Plain text response
		return strings.TrimSpace(string(output)), nil
	}

	// JSON response
	var resp Response
	if err := json.Unmarshal(output, &resp); err != nil {
		return "", fmt.Errorf("invalid helper response: %w", err)
	}

	if resp.ProtocolVersion != 1 {
		return "", fmt.Errorf("unsupported protocol version: %d", resp.ProtocolVersion)
	}

	if errDetail, ok := resp.Errors["token"]; ok {
		return "", fmt.Errorf("helper error: %s", errDetail.Message)
	}

	token, ok := resp.Values["token"]
	if !ok {
		return "", fmt.Errorf("helper did not return token")
	}

	return token, nil
}

// TestConfig contains configuration for testing a helper
type TestConfig struct {
	Command    string
	Args       []string
	TimeoutMs  int
	Env        map[string]string
	PassEnv    []string
	SimpleMode bool
}

// TestResult contains the result of a helper test
type TestResult struct {
	Success bool   `json:"success"`
	Preview string `json:"preview"` // Masked token preview
	Error   string `json:"error,omitempty"`
}

// TestHelper executes a helper command and returns a masked preview
func TestHelper(ctx context.Context, config TestConfig) TestResult {
	executor := NewExecutor(Config{
		Command:    config.Command,
		Args:       config.Args,
		TimeoutMs:  config.TimeoutMs,
		Env:        config.Env,
		PassEnv:    config.PassEnv,
		SimpleMode: config.SimpleMode,
	})

	token, err := executor.Fetch(ctx, "test")
	if err != nil {
		return TestResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	// Mask token for preview
	preview := maskToken(token)

	return TestResult{
		Success: true,
		Preview: preview,
	}
}

// maskToken creates a masked preview of a token
func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	// Show first 4 and last 4 characters
	return token[:4] + "..." + token[len(token)-4:]
}