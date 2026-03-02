// Package permission provides backward-compatible type aliases for the ask package.
// Deprecated: Use github.com/tingly-dev/tingly-box/agentboot/ask instead.
package permission

import (
	"context"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
)

// Type aliases for backward compatibility
//
// Deprecated: Use ask.Request instead.
type PermissionRequest = agentboot.PermissionRequest

// Deprecated: Use ask.Result instead.
type PermissionResult = agentboot.PermissionResult

// Deprecated: Use ask.Response instead.
type UserResponse = ask.Response

// Deprecated: Use ask.Mode instead.
type PermissionMode = ask.Mode

// Mode constants for backward compatibility
const (
	PermissionModeAuto   = ask.ModeAuto
	PermissionModeManual = ask.ModeManual
	PermissionModeSkip   = ask.ModeSkip
)

// Handler is the main interface for permission handling.
// Deprecated: Use ask.Handler instead.
type Handler interface {
	CanUseTool(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error)
	SetMode(scopeID string, mode agentboot.PermissionMode) error
	GetMode(scopeID string) (agentboot.PermissionMode, error)
	SubmitDecision(requestID string, approved bool, reason string) error
	GetPendingRequests() []agentboot.PermissionRequest
	RecordDecision(req agentboot.PermissionRequest, response agentboot.PermissionResponse) error
	SetUserPrompter(UserPrompter)
}

// HandlerAdapter wraps an ask.Handler to implement the legacy Handler interface
type HandlerAdapter struct {
	handler *ask.DefaultHandler
}

// NewHandlerAdapter creates a new HandlerAdapter from an ask.Handler
func NewHandlerAdapter(h *ask.DefaultHandler) *HandlerAdapter {
	return &HandlerAdapter{handler: h}
}

// CanUseTool implements Handler
func (a *HandlerAdapter) CanUseTool(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	askReq := ask.FromPermissionRequest(req)
	result, err := a.handler.Ask(ctx, *askReq)
	if err != nil {
		return agentboot.PermissionResult{}, err
	}
	return result.ToPermissionResult(), nil
}

// SetMode implements Handler
func (a *HandlerAdapter) SetMode(scopeID string, mode agentboot.PermissionMode) error {
	return a.handler.SetMode(scopeID, ask.Mode(mode))
}

// GetMode implements Handler
func (a *HandlerAdapter) GetMode(scopeID string) (agentboot.PermissionMode, error) {
	mode, err := a.handler.GetMode(scopeID)
	return agentboot.PermissionMode(mode), err
}

// SubmitDecision implements Handler
func (a *HandlerAdapter) SubmitDecision(requestID string, approved bool, reason string) error {
	return a.handler.SubmitResult(requestID, ask.Result{
		ID:       requestID,
		Approved: approved,
		Reason:   reason,
	})
}

// GetPendingRequests implements Handler
func (a *HandlerAdapter) GetPendingRequests() []agentboot.PermissionRequest {
	pending := a.handler.GetPendingRequests()
	result := make([]agentboot.PermissionRequest, len(pending))
	for i, req := range pending {
		result[i] = req.ToPermissionRequest()
	}
	return result
}

// RecordDecision implements Handler
func (a *HandlerAdapter) RecordDecision(req agentboot.PermissionRequest, response agentboot.PermissionResponse) error {
	// No-op for now - decision recording is internal
	return nil
}

// SetUserPrompter implements Handler
func (a *HandlerAdapter) SetUserPrompter(p UserPrompter) {
	a.handler.SetPrompter(&userPrompterWrapper{p: p})
}

// UserPrompter handles user interaction for permission requests.
// Deprecated: Use ask.Prompter instead.
type UserPrompter interface {
	PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error)
}

// userPrompterWrapper wraps a UserPrompter to implement ask.Prompter
type userPrompterWrapper struct {
	p UserPrompter
}

// Prompt implements ask.Prompter
func (w *userPrompterWrapper) Prompt(ctx context.Context, req ask.Request) (ask.Result, error) {
	permReq := req.ToPermissionRequest()
	result, err := w.p.PromptPermission(ctx, permReq)
	if err != nil {
		return ask.Result{}, err
	}
	return ask.Result{
		ID:           req.ID,
		Approved:     result.Approved,
		Reason:       result.Reason,
		UpdatedInput: result.UpdatedInput,
		Remember:     result.Remember,
	}, nil
}

// ToolHandler is the interface for tool-specific handlers.
// Deprecated: Use ask.ToolHandler instead.
type ToolHandler = ask.ToolHandler

// ToolPromptBuilder builds prompts for tool permission requests.
// Deprecated: Use ask.ToolPromptBuilder instead.
type ToolPromptBuilder = ask.ToolPromptBuilder

// ToolResponseParser parses user responses into permission results.
// Deprecated: Use ask.ToolResponseParser instead.
type ToolResponseParser = ask.ToolResponseParser

// ToolHandlerRegistry manages tool-specific handlers.
// Deprecated: Use ask.ToolHandlerRegistry instead.
type ToolHandlerRegistry = ask.ToolHandlerRegistry

// NewToolHandlerRegistry creates a new registry with default handlers.
// Deprecated: Use ask.NewToolHandlerRegistry instead.
func NewToolHandlerRegistry() *ToolHandlerRegistry {
	return ask.NewToolHandlerRegistry()
}

// PermissionConfig holds permission handler configuration.
// Deprecated: Use ask.Config instead.
type PermissionConfig struct {
	DefaultMode       agentboot.PermissionMode
	Timeout           time.Duration
	EnableWhitelist   bool
	Whitelist         []string
	Blacklist         []string
	RememberDecisions bool
	DecisionDuration  time.Duration
}

// NewDefaultHandler creates a new permission handler with the given config.
// Deprecated: Use ask.NewHandler instead.
func NewDefaultHandler(config PermissionConfig) Handler {
	askConfig := ask.Config{
		DefaultMode:       ask.Mode(config.DefaultMode),
		Timeout:           config.Timeout,
		EnableWhitelist:   config.EnableWhitelist,
		Whitelist:         config.Whitelist,
		Blacklist:         config.Blacklist,
		RememberDecisions: config.RememberDecisions,
		DecisionDuration:  config.DecisionDuration,
	}
	return NewHandlerAdapter(ask.NewHandler(askConfig))
}

// NewDefaultHandlerFromAgentboot creates a new permission handler from agentboot.PermissionConfig.
// This is a convenience function for migration.
func NewDefaultHandlerFromAgentboot(config agentboot.PermissionConfig) Handler {
	return NewDefaultHandler(PermissionConfig{
		DefaultMode:       config.DefaultMode,
		Timeout:           config.Timeout,
		EnableWhitelist:   config.EnableWhitelist,
		Whitelist:         config.Whitelist,
		Blacklist:         config.Blacklist,
		RememberDecisions: config.RememberDecisions,
		DecisionDuration:  config.DecisionDuration,
	})
}

// StdinPrompterWrapper wraps ask.StdinPrompter to implement UserPrompter
type StdinPrompterWrapper struct {
	*ask.StdinPrompter
}

// PromptPermission implements UserPrompter
func (w *StdinPrompterWrapper) PromptPermission(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	askReq := ask.FromPermissionRequest(req)
	result, err := w.StdinPrompter.Prompt(ctx, *askReq)
	if err != nil {
		return agentboot.PermissionResult{}, err
	}
	return result.ToPermissionResult(), nil
}

// StdinPrompter implements UserPrompter using stdin/stdout.
// Deprecated: Use ask.StdinPrompter instead.
type StdinPrompter = StdinPrompterWrapper

// NewStdinPrompter creates a new StdinPrompter with default colors.
// Deprecated: Use ask.NewStdinPrompter instead.
func NewStdinPrompter() *StdinPrompter {
	return &StdinPrompterWrapper{StdinPrompter: ask.NewStdinPrompter()}
}

// NewStdinPrompterDebug creates a new StdinPrompter with debug enabled.
// Deprecated: Use ask.NewStdinPrompterDebug instead.
func NewStdinPrompterDebug() *StdinPrompter {
	return &StdinPrompterWrapper{StdinPrompter: ask.NewStdinPrompterDebug()}
}

// NoOpPrompter is a prompter that auto-approves everything.
// Deprecated: Use ask.NoOpPrompter instead.
type NoOpPrompter = ask.NoOpPrompter

// NewNoOpPrompter creates a new NoOpPrompter.
// Deprecated: Use ask.NewNoOpPrompter instead.
func NewNoOpPrompter() *NoOpPrompter {
	return ask.NewNoOpPrompter()
}

// DenyAllPrompter is a prompter that denies everything.
// Deprecated: Use ask.DenyAllPrompter instead.
type DenyAllPrompter = ask.DenyAllPrompter

// NewDenyAllPrompter creates a new DenyAllPrompter.
// Deprecated: Use ask.NewDenyAllPrompter instead.
func NewDenyAllPrompter() *DenyAllPrompter {
	return ask.NewDenyAllPrompter()
}

// AskUserQuestionHandler handles the AskUserQuestion tool.
// Deprecated: Use ask.AskUserQuestionHandler instead.
type AskUserQuestionHandler = ask.AskUserQuestionHandler

// NewAskUserQuestionHandler creates a new AskUserQuestionHandler.
// Deprecated: Use ask.NewAskUserQuestionHandler instead.
func NewAskUserQuestionHandler() *AskUserQuestionHandler {
	return ask.NewAskUserQuestionHandler()
}

// DefaultToolHandler is the fallback handler for tools without specific handlers.
// Deprecated: Use ask.DefaultToolHandler instead.
type DefaultToolHandler = ask.DefaultToolHandler

// NewDefaultToolHandler creates a new DefaultToolHandler.
// Deprecated: Use ask.NewDefaultToolHandler instead.
func NewDefaultToolHandler() *DefaultToolHandler {
	return ask.NewDefaultToolHandler()
}
