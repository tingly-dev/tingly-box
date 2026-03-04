package mock

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// mockToolNames are the tool names used in mock permission requests
var mockToolNames = []string{
	"mock_tool_read",
	"mock_tool_write",
	"mock_tool_execute",
	"mock_tool_search",
	"mock_tool_analyze",
}

// Agent implements the agentboot.Agent interface for testing purposes.
// It simulates agent behavior by repeatedly requesting user permission confirmations.
type Agent struct {
	config        Config
	abConfig      agentboot.Config
	defaultFormat agentboot.OutputFormat
	mu            sync.RWMutex
}

// NewAgent creates a new mock agent with the given configuration
func NewAgent(config Config) *Agent {
	config = config.Merge(DefaultConfig())
	return &Agent{
		config:        config,
		defaultFormat: agentboot.OutputFormatStreamJSON,
	}
}

// NewAgentWithConfig creates a new mock agent with both mock and agentboot configs
func NewAgentWithConfig(mockConfig Config, abConfig agentboot.Config) *Agent {
	agent := NewAgent(mockConfig)
	agent.abConfig = abConfig
	return agent
}

// Execute runs the mock agent, simulating permission request cycles
func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.Result, error) {
	startTime := time.Now()
	logrus.Infof("[MockAgent] Starting execution with prompt: %s", truncatePrompt(prompt))

	var events []agentboot.Event
	sessionID := opts.SessionID
	if sessionID == "" {
		sessionID = uuid.NewString()[:8]
	}

	// Generate session init event
	initMsg := agentboot.NewInitMessage(agentboot.AgentTypeMockAgent, sessionID, a.config.MaxIterations)
	events = append(events, initMsg.ToEvent())

	// Send via handler if available
	if opts.Handler != nil {
		if err := opts.Handler.OnMessage(initMsg); err != nil {
			opts.Handler.OnError(err)
		}
	}

	// Process through iterations
	for step := 1; step <= a.config.MaxIterations; step++ {
		select {
		case <-ctx.Done():
			logrus.Infof("[MockAgent] Context cancelled at step %d", step)
			resultMsg := agentboot.NewResultMessage(agentboot.AgentTypeMockAgent, sessionID, "cancelled", "Context cancelled by user")
			events = append(events, resultMsg.ToEvent())

			if opts.Handler != nil {
				opts.Handler.OnComplete(&agentboot.CompletionResult{
					Success:    false,
					DurationMS: time.Since(startTime).Milliseconds(),
					SessionID:  sessionID,
					Error:      "Context cancelled",
				})
			}

			return a.buildResult(events, startTime, sessionID), ctx.Err()
		default:
		}

		// Generate tool name for this step
		toolName := mockToolNames[(step-1)%len(mockToolNames)]

		// Check if this should be an AskUserQuestion request
		var isAskUserQuestion bool
		var askInput map[string]interface{}

		if a.config.AskUserQuestionFrequency > 0 && step%a.config.AskUserQuestionFrequency == 0 {
			isAskUserQuestion = true
			askInput = map[string]interface{}{
				"questions": []map[string]interface{}{
					{
						"question": fmt.Sprintf("Mock question %d of %d", step, a.config.MaxIterations),
						"header":   "Mock",
						"options": []map[string]interface{}{
							{
								"label":       "Option A",
								"description": "First option",
							},
							{
								"label":       "Option B",
								"description": "Second option",
							},
							{
								"label":       "Option C",
								"description": "Third option",
							},
						},
					},
				},
			}
		}

		var approved bool

		if isAskUserQuestion {
			// Handle AskUserQuestion
			req := agentboot.AskRequest{
				ID:        uuid.NewString()[:8],
				Type:      "tool_use",
				AgentType: agentboot.AgentTypeMockAgent,
				Platform:  opts.Platform,
				ChatID:    opts.ChatID,
				SessionID: sessionID,
				ToolName:  "AskUserQuestion",
				Input:     askInput,
			}

			// Create unified permission request message
			permReqMsg := agentboot.NewPermissionRequestMessage(
				agentboot.AgentTypeMockAgent, sessionID, req.ID, req.ToolName, req.Input, "Mock AskUserQuestion",
			)
			permReqMsg.Step = step
			permReqMsg.Total = a.config.MaxIterations
			events = append(events, permReqMsg.ToEvent())

			// Send via handler if available
			if opts.Handler != nil {
				if err := opts.Handler.OnMessage(permReqMsg); err != nil {
					opts.Handler.OnError(err)
				}
			}

			// Get decision
			if a.config.AutoApprove {
				approved = true
			} else if opts.Handler != nil {
				result, e := opts.Handler.OnAsk(ctx, req)
				if e != nil {
					logrus.Errorf("[MockAgent] Ask handler error: %v", e)
					approved = false
				} else {
					approved = result.Approved
				}
			} else {
				approved = true // Default to approved if no handler
			}

			// Create permission result message
			if approved {
				permResultMsg := agentboot.NewPermissionResultMessage(
					agentboot.AgentTypeMockAgent, sessionID, req.ID, true, "Approved",
				)
				events = append(events, permResultMsg.ToEvent())
			} else {
				permResultMsg := agentboot.NewPermissionResultMessage(
					agentboot.AgentTypeMockAgent, sessionID, req.ID, false, "Denied",
				)
				events = append(events, permResultMsg.ToEvent())

				resultMsg := agentboot.NewResultMessage(
					agentboot.AgentTypeMockAgent, sessionID, "ask_denied",
					fmt.Sprintf("AskUserQuestion denied at step %d", step),
				)
				events = append(events, resultMsg.ToEvent())

				if opts.Handler != nil {
					opts.Handler.OnComplete(&agentboot.CompletionResult{
						Success:    false,
						DurationMS: time.Since(startTime).Milliseconds(),
						SessionID:  sessionID,
						Error:      "AskUserQuestion denied",
					})
				}

				return a.buildResult(events, startTime, sessionID), nil
			}
		} else {
			// Regular permission request
			req := agentboot.PermissionRequest{
				RequestID: uuid.NewString()[:8],
				AgentType: agentboot.AgentTypeMockAgent,
				ToolName:  toolName,
				Input: map[string]interface{}{
					"step":      step,
					"total":     a.config.MaxIterations,
					"prompt":    truncatePrompt(prompt),
					"command":   fmt.Sprintf("mock_command --step %d --input %q", step, truncatePrompt(prompt)),
					"_chat_id":  opts.ChatID,
					"_platform": opts.Platform,
				},
				Reason:    fmt.Sprintf("Mock permission request %d of %d", step, a.config.MaxIterations),
				Timestamp: time.Now(),
				SessionID: sessionID,
			}

			// Create unified permission request message
			permReqMsg := agentboot.NewPermissionRequestMessage(
				agentboot.AgentTypeMockAgent, sessionID, req.RequestID, req.ToolName, req.Input, req.Reason,
			)
			permReqMsg.Step = step
			permReqMsg.Total = a.config.MaxIterations
			events = append(events, permReqMsg.ToEvent())

			// Send via handler if available
			if opts.Handler != nil {
				if err := opts.Handler.OnMessage(permReqMsg); err != nil {
					opts.Handler.OnError(err)
				}
			}

			// Get permission decision using the new OnApproval method
			if a.config.AutoApprove {
				approved = true
			} else if opts.Handler != nil {
				result, e := opts.Handler.OnApproval(ctx, req)
				if e != nil {
					logrus.Errorf("[MockAgent] Permission handler error: %v", e)
					approved = false
				} else {
					approved = result.Approved
				}
			} else {
				approved = true // Default to approved if no handler
			}

			// Handle permission response
			if !approved {
				logrus.Infof("[MockAgent] Permission denied at step %d", step)

				// Create unified permission result message (denied)
				permResultMsg := agentboot.NewPermissionResultMessage(
					agentboot.AgentTypeMockAgent, sessionID, req.RequestID, false, "Denied",
				)
				events = append(events, permResultMsg.ToEvent())

				// Send result event
				resultMsg := agentboot.NewResultMessage(
					agentboot.AgentTypeMockAgent, sessionID, "permission_denied",
					fmt.Sprintf("Permission denied at step %d", step),
				)
				events = append(events, resultMsg.ToEvent())

				if opts.Handler != nil {
					opts.Handler.OnComplete(&agentboot.CompletionResult{
						Success:    false,
						DurationMS: time.Since(startTime).Milliseconds(),
						SessionID:  sessionID,
						Error:      "Permission denied",
					})
				}

				return a.buildResult(events, startTime, sessionID), nil
			}

			// Permission approved - create unified result message
			permResultMsg := agentboot.NewPermissionResultMessage(
				agentboot.AgentTypeMockAgent, sessionID, req.RequestID, true, "Approved",
			)
			events = append(events, permResultMsg.ToEvent())
		}

		// Generate assistant response
		responseText := a.formatResponse(step, prompt)
		assistantMsg := agentboot.NewAssistantMessage(agentboot.AgentTypeMockAgent, sessionID, responseText)
		events = append(events, assistantMsg.ToEvent())

		// Send via handler if available
		if opts.Handler != nil {
			if err := opts.Handler.OnMessage(assistantMsg); err != nil {
				opts.Handler.OnError(err)
			}
		}

		// Add delay between steps (except for the last step)
		if step < a.config.MaxIterations {
			select {
			case <-time.After(a.config.StepDelay):
				// Continue to next step
			case <-ctx.Done():
				logrus.Infof("[MockAgent] Context cancelled during delay at step %d", step)

				if opts.Handler != nil {
					opts.Handler.OnComplete(&agentboot.CompletionResult{
						Success:    false,
						DurationMS: time.Since(startTime).Milliseconds(),
						SessionID:  sessionID,
						Error:      "Context cancelled",
					})
				}

				return a.buildResult(events, startTime, sessionID), ctx.Err()
			}
		}
	}

	// All iterations completed successfully
	resultMsg := agentboot.NewResultMessage(
		agentboot.AgentTypeMockAgent, sessionID, "success", "Mock agent completed all iterations",
	)
	events = append(events, resultMsg.ToEvent())

	// Notify handler of completion
	if opts.Handler != nil {
		opts.Handler.OnComplete(&agentboot.CompletionResult{
			Success:    true,
			DurationMS: time.Since(startTime).Milliseconds(),
			SessionID:  sessionID,
		})
	}

	return a.buildResult(events, startTime, sessionID), nil
}

// formatResponse generates a mock response text
func (a *Agent) formatResponse(step int, prompt string) string {
	text := a.config.ResponseTemplate
	text = strings.ReplaceAll(text, "{step}", fmt.Sprintf("%d", step))
	text = strings.ReplaceAll(text, "{total}", fmt.Sprintf("%d", a.config.MaxIterations))
	text = strings.ReplaceAll(text, "{prompt}", truncatePrompt(prompt))
	return text
}

// buildResult constructs the final result
func (a *Agent) buildResult(events []agentboot.Event, startTime time.Time, sessionID string) *agentboot.Result {
	return &agentboot.Result{
		Output:   "", // Text output is empty for stream-json mode
		ExitCode: 0,
		Error:    "",
		Duration: time.Since(startTime),
		Format:   a.defaultFormat,
		Events:   events,
		Metadata: map[string]interface{}{
			"session_id":     sessionID,
			"agent_type":     "mock",
			"max_iterations": a.config.MaxIterations,
		},
	}
}

// IsAvailable always returns true for mock agent
func (a *Agent) IsAvailable() bool {
	return true
}

// Type returns the mock agent type
func (a *Agent) Type() agentboot.AgentType {
	return agentboot.AgentTypeMockAgent
}

// SetDefaultFormat sets the default output format
func (a *Agent) SetDefaultFormat(format agentboot.OutputFormat) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.defaultFormat = format
}

// GetDefaultFormat returns the current default format
func (a *Agent) GetDefaultFormat() agentboot.OutputFormat {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.defaultFormat
}

// SetMaxIterations configures the maximum number of iterations
func (a *Agent) SetMaxIterations(max int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if max > 0 {
		a.config.MaxIterations = max
	}
}

// SetStepDelay configures the delay between steps
func (a *Agent) SetStepDelay(delay time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if delay > 0 {
		a.config.StepDelay = delay
	}
}

// SetAutoApprove configures auto-approval mode
func (a *Agent) SetAutoApprove(autoApprove bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.AutoApprove = autoApprove
}

// SetAskUserQuestionFrequency configures how often to send AskUserQuestion requests
func (a *Agent) SetAskUserQuestionFrequency(freq int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if freq > 0 {
		a.config.AskUserQuestionFrequency = freq
	}
}

// truncatePrompt truncates a prompt for display purposes
func truncatePrompt(prompt string) string {
	const maxLen = 50
	if len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen-3] + "..."
}