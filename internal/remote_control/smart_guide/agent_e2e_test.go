//go:build e2e
// +build e2e

package smart_guide

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-agentscope/pkg/agent"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// ============================================================================
// CONFIGURATION
// ============================================================================
const (
	REAL_APIKey       = "tingly-box-eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbGllbnRfaWQiOiJ0ZXN0LWNsaWVudCIsImV4cCI6MTc2NjQwMzQwNSwiaWF0IjoxNzY2MzE3MDA1fQ.AHtmsHxGGJ0jtzvrTZMHC3kfl3Os94HOhMA-zXFtHXQ"
	REAL_BaseURL      = "http://localhost:12580/tingly/anthropic"
	REAL_Model        = "tingly-box"
	REAL_ProviderUUID = "bfd637ca-e9d6-11f0-b967-aaf5c138276e"
)

// ============================================================================
// Test Helpers
// ============================================================================

// iterationTracker tracks ReAct loop behavior via hooks
type iterationTracker struct {
	mu              sync.Mutex
	modelResponses  []iterationRecord
	toolResults     []toolRecord
	loopComplete    *loopCompleteRecord
	maxIterReached  bool
	totalIterations int
}

type iterationRecord struct {
	Iteration      int
	MaxIterations  int
	ToolBlockCount int
	TextPreview    string
	Timestamp      time.Time
}

type toolRecord struct {
	Iteration int
	ToolName  string
	ToolID    string
	HasError  bool
	Timestamp time.Time
}

type loopCompleteRecord struct {
	IterationsUsed       int
	MaxIterationsReached bool
	Timestamp            time.Time
}

func newIterationTracker() *iterationTracker {
	return &iterationTracker{}
}

func (t *iterationTracker) registerHooks(ag *TinglyBoxAgent) error {
	// Hook 1: Track each model response
	err := ag.ReActAgent.RegisterHook(types.HookTypeLoopModelResponse, "e2e_model_response",
		agent.LoopModelResponseHookFunc(func(ctx context.Context, a agent.Agent, msg *message.Msg, hookCtx *agent.LoopModelResponseContext) error {
			t.mu.Lock()
			defer t.mu.Unlock()

			textContent := msg.GetTextContent()
			preview := textContent
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}

			t.modelResponses = append(t.modelResponses, iterationRecord{
				Iteration:      hookCtx.Iteration,
				MaxIterations:  hookCtx.MaxIterations,
				ToolBlockCount: hookCtx.ToolBlocksCount,
				TextPreview:    preview,
				Timestamp:      time.Now(),
			})
			t.totalIterations = hookCtx.Iteration + 1
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("register model response hook: %w", err)
	}

	// Hook 2: Track tool executions
	err = ag.ReActAgent.RegisterHook(types.HookTypeLoopToolResult, "e2e_tool_result",
		agent.LoopToolResultHookFunc(func(ctx context.Context, a agent.Agent, msg *message.Msg, hookCtx *agent.LoopToolResultContext) error {
			t.mu.Lock()
			defer t.mu.Unlock()

			t.toolResults = append(t.toolResults, toolRecord{
				Iteration: hookCtx.Iteration,
				ToolName:  hookCtx.ToolName,
				ToolID:    hookCtx.ToolID,
				HasError:  hookCtx.Error != nil,
				Timestamp: time.Now(),
			})
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("register tool result hook: %w", err)
	}

	// Hook 3: Track loop completion
	err = ag.ReActAgent.RegisterHook(types.HookTypeLoopComplete, "e2e_loop_complete",
		agent.LoopCompleteHookFunc(func(ctx context.Context, a agent.Agent, msg *message.Msg, hookCtx *agent.LoopCompleteContext) error {
			t.mu.Lock()
			defer t.mu.Unlock()

			t.loopComplete = &loopCompleteRecord{
				IterationsUsed:       hookCtx.IterationsUsed,
				MaxIterationsReached: hookCtx.MaxIterationsReached,
			}
			t.maxIterReached = hookCtx.MaxIterationsReached
			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("register loop complete hook: %w", err)
	}

	return nil
}

func (t *iterationTracker) report(tb testing.TB) {
	t.mu.Lock()
	defer t.mu.Unlock()

	tb.Logf("━━━ Iteration Tracker Report ━━━")
	tb.Logf("Total model responses: %d", len(t.modelResponses))
	tb.Logf("Total tool executions: %d", len(t.toolResults))

	for i, r := range t.modelResponses {
		tb.Logf("  [iter %d/%d] model response #%d: toolBlocks=%d, text=%q",
			r.Iteration, r.MaxIterations, i, r.ToolBlockCount, r.TextPreview)
	}

	for _, r := range t.toolResults {
		errStr := "ok"
		if r.HasError {
			errStr = "ERROR"
		}
		tb.Logf("  [iter %d] tool: %s (id=%s) → %s", r.Iteration, r.ToolName, r.ToolID, errStr)
	}

	if t.loopComplete != nil {
		tb.Logf("  Loop complete: iterationsUsed=%d, maxReached=%v",
			t.loopComplete.IterationsUsed, t.loopComplete.MaxIterationsReached)
	} else {
		tb.Logf("  ⚠ Loop complete hook was NOT called")
	}

	if t.maxIterReached {
		tb.Logf("  ⚠ MAX ITERATIONS REACHED - agent was forced to stop")
	}
	tb.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func (t *iterationTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.modelResponses = nil
	t.toolResults = nil
	t.loopComplete = nil
	t.maxIterReached = false
	t.totalIterations = 0
}

// createTestConfig creates a standard test config
func createTestConfig() *AgentConfig {
	return &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          REAL_BaseURL,
		APIKey:           REAL_APIKey,
		Provider:         REAL_ProviderUUID,
		Model:            REAL_Model,
		GetStatusFunc: func(chatID string) (*StatusInfo, error) {
			return &StatusInfo{
				CurrentAgent:   "@tb",
				SessionID:      "test-session",
				ProjectPath:    "/tmp/test-project",
				WorkingDir:     "/tmp",
				HasRunningTask: false,
				Whitelisted:    true,
			}, nil
		},
		UpdateProjectFunc: func(chatID string, projectPath string) error {
			return nil
		},
	}
}

// ============================================================================
// Test: Basic Agent Execution (Original)
// ============================================================================

// TestRealAgentExecution tests the agent with real model calls.
// To run: go test -v -tags=e2e -run TestRealAgentExecution ./internal/remote_control/smart_guide/
func TestRealAgentExecution(t *testing.T) {
	cfg := createTestConfig()

	testAgent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err, "Agent creation should succeed")
	require.NotNil(t, testAgent, "Agent should not be nil")

	t.Logf("✓ Agent created successfully with model: %s", REAL_Model)
	t.Logf("✓ Available tools: %d", len(testAgent.GetToolkit().GetSchemas()))

	ctx := context.Background()
	toolCtx := &ToolContext{
		ChatID:      "test-chat-real",
		ProjectPath: "/tmp",
	}

	testCases := []struct {
		name    string
		message string
	}{
		{
			name:    "Simple greeting",
			message: "Hello, can you help me?",
		},
		{
			name:    "Tool use - get status",
			message: "What's the current status?",
		},
		{
			name:    "Simple question",
			message: "What is the capital of France?",
		},
		{
			name:    "Tool use - ls command",
			message: "Please list the files in current directory with ls command",
		},
		{
			name:    "Multiple tools",
			message: "Show current directory with pwd, then list files with ls",
		},
		{
			name:    "Read file",
			message: "Read go.mod file",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Sending: %s", tc.message)

			response, err := testAgent.ReplyWithContext(ctx, tc.message, toolCtx)

			assert.NoError(t, err, "Request should complete without errors")
			assert.NotNil(t, response, "Response should not be nil")

			if response != nil {
				content := response.Content
				assert.NotEmpty(t, content, "Response should have content")

				responseText := response.GetTextContent()
				t.Logf("Response length: %d chars", len(responseText))

				if len(responseText) > 200 {
					t.Logf("Response preview: %s...", responseText[:200])
				} else {
					t.Logf("Response: %s", responseText)
				}

				t.Logf("✓ Test case '%s' completed successfully", tc.name)
			}
		})
	}

	t.Logf("✓ All e2e tests passed - agent is working correctly")
}

// ============================================================================
// Test: Shutdown Diagnosis - Tracks iterations to find why agent stops
// ============================================================================

// TestAgentShutdownDiagnosis runs commands that may cause agent shutdown,
// tracking iteration counts, tool usage, and completion reason.
// To run: go test -v -tags=e2e -run TestAgentShutdownDiagnosis ./internal/remote_control/smart_guide/
func TestAgentShutdownDiagnosis(t *testing.T) {
	cfg := createTestConfig()

	testAgent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)

	// Setup iteration tracker
	tracker := newIterationTracker()
	require.NoError(t, tracker.registerHooks(testAgent))

	ctx := context.Background()
	toolCtx := &ToolContext{
		ChatID:      "test-shutdown-diag",
		ProjectPath: "/tmp",
	}

	testCases := []struct {
		name    string
		message string
		desc    string // why this test matters
	}{
		{
			name:    "Single command",
			message: "Run: ls /tmp",
			desc:    "Baseline: single tool call, should complete normally",
		},
		{
			name:    "Multi-step: git info",
			message: "Check git version, then show current directory, then list files",
			desc:    "Multi-tool: 3 sequential commands, tests iteration usage",
		},
		{
			name:    "Multi-step: file exploration",
			message: "List files in /tmp, then show the current working directory with pwd, then check if /etc/hosts exists with ls -la /etc/hosts, then read the first 5 lines of /etc/hosts with head -5 /etc/hosts",
			desc:    "Heavy tool use: 4+ commands, may approach MaxIterations=5",
		},
		{
			name:    "Error recovery: invalid path",
			message: "List files in /nonexistent/path/that/does/not/exist",
			desc:    "Error case: command fails, does agent handle gracefully?",
		},
		{
			name:    "Complex: read and analyze",
			message: "Read /etc/hosts file and tell me how many lines it has. Use bash to count with wc -l.",
			desc:    "Multi-step reasoning: read + analyze, may use multiple iterations",
		},
		{
			name:    "Non-allowlisted command",
			message: "Run the command: python3 --version",
			desc:    "Non-allowlisted: should trigger approval or fail, tests error path",
		},
		{
			name:    "Chained commands",
			message: "Run: echo hello && echo world && echo done",
			desc:    "Command chaining: tests && chain handling",
		},
		{
			name:    "Write then read",
			message: "Write 'hello test' to /tmp/e2e_test_file.txt, then read it back to verify",
			desc:    "Write+read cycle: tests file tool interaction",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracker.reset()

			t.Logf("━━━ %s ━━━", tc.name)
			t.Logf("Description: %s", tc.desc)
			t.Logf("Message: %s", tc.message)

			startTime := time.Now()
			response, err := testAgent.ReplyWithContext(ctx, tc.message, toolCtx)
			duration := time.Since(startTime)

			t.Logf("Duration: %v", duration)

			// Report tracker data regardless of success/failure
			tracker.report(t)

			if err != nil {
				t.Logf("⚠ ERROR: %v", err)
				// Don't fail - we want to see the tracker report
				return
			}

			if response == nil {
				t.Logf("⚠ Response is nil")
				return
			}

			responseText := response.GetTextContent()
			t.Logf("Response length: %d chars", len(responseText))
			if len(responseText) > 300 {
				t.Logf("Response preview: %s...", responseText[:300])
			} else {
				t.Logf("Response: %s", responseText)
			}

			// Key diagnostic assertions
			if tracker.maxIterReached {
				t.Logf("⚠ SHUTDOWN CAUSE: MaxIterations (%d) reached - agent was forced to stop",
					cfg.SmartGuideConfig.MaxIterations)
			}

			if tracker.loopComplete == nil {
				t.Logf("⚠ SHUTDOWN CAUSE: Loop complete hook was not called - possible crash or unexpected exit")
			}
		})
	}
}

// ============================================================================
// Test: MaxIterations Boundary
// ============================================================================

// TestMaxIterationsBoundary tests behavior at different MaxIterations settings.
// To run: go test -v -tags=e2e -run TestMaxIterationsBoundary ./internal/remote_control/smart_guide/
func TestMaxIterationsBoundary(t *testing.T) {
	iterationLimits := []int{3, 5, 8}

	// A message that requires multiple tool calls
	message := "Do the following steps: 1) pwd, 2) ls /tmp, 3) echo hello, 4) cat /etc/hostname, 5) date"

	for _, maxIter := range iterationLimits {
		t.Run(fmt.Sprintf("MaxIterations_%d", maxIter), func(t *testing.T) {
			cfg := createTestConfig()
			cfg.SmartGuideConfig.MaxIterations = maxIter

			testAgent, err := NewTinglyBoxAgent(cfg)
			require.NoError(t, err)

			tracker := newIterationTracker()
			require.NoError(t, tracker.registerHooks(testAgent))

			ctx := context.Background()
			toolCtx := &ToolContext{
				ChatID:      fmt.Sprintf("test-maxiter-%d", maxIter),
				ProjectPath: "/tmp",
			}

			t.Logf("Testing with MaxIterations=%d", maxIter)
			t.Logf("Message: %s", message)

			response, err := testAgent.ReplyWithContext(ctx, message, toolCtx)

			tracker.report(t)

			if err != nil {
				t.Logf("Error: %v", err)
			}

			if response != nil {
				responseText := response.GetTextContent()
				if len(responseText) > 200 {
					t.Logf("Response preview: %s...", responseText[:200])
				} else {
					t.Logf("Response: %s", responseText)
				}
			}

			if tracker.maxIterReached {
				t.Logf("⚠ MaxIterations=%d was NOT enough for this task", maxIter)
			} else {
				t.Logf("✓ MaxIterations=%d was sufficient (used %d iterations)",
					maxIter, tracker.totalIterations)
			}
		})
	}
}

// ============================================================================
// Test: Context Cancellation
// ============================================================================

// TestContextCancellation tests that context cancellation properly stops the agent.
// To run: go test -v -tags=e2e -run TestContextCancellation ./internal/remote_control/smart_guide/
func TestContextCancellation(t *testing.T) {
	cfg := createTestConfig()

	testAgent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)

	tracker := newIterationTracker()
	require.NoError(t, tracker.registerHooks(testAgent))

	toolCtx := &ToolContext{
		ChatID:      "test-cancel",
		ProjectPath: "/tmp",
	}

	// Cancel after 3 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	message := "Do the following slowly: 1) list /tmp, 2) list /etc, 3) list /var, 4) list /usr, 5) list /opt"

	t.Logf("Testing context cancellation (3s timeout)")
	t.Logf("Message: %s", message)

	startTime := time.Now()
	response, err := testAgent.ReplyWithContext(ctx, message, toolCtx)
	duration := time.Since(startTime)

	t.Logf("Duration: %v", duration)
	tracker.report(t)

	if err != nil {
		t.Logf("Error (expected for cancellation): %v", err)
		if ctx.Err() != nil {
			t.Logf("✓ Context was cancelled: %v", ctx.Err())
		}
	} else {
		t.Logf("Agent completed before timeout (response=%v)", response != nil)
	}
}

// ============================================================================
// Test: ExecuteWithHandler (Full Pipeline)
// ============================================================================

// TestExecuteWithHandlerFlow tests the full ExecuteWithHandler pipeline
// which is what bot_agent.go actually uses.
// To run: go test -v -tags=e2e -run TestExecuteWithHandlerFlow ./internal/remote_control/smart_guide/
func TestExecuteWithHandlerFlow(t *testing.T) {
	cfg := createTestConfig()

	testAgent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)

	toolCtx := &ToolContext{
		ChatID:      "test-handler-flow",
		ProjectPath: "/tmp",
	}
	testAgent.GetExecutor().SetWorkingDirectory("/tmp")

	// Track messages received by handler
	var handlerMu sync.Mutex
	var messages []interface{}
	var completionResult *agentboot.CompletionResult
	var handlerErrors []error

	handler := &testMessageHandler{
		onMessageFn: func(msg interface{}) {
			handlerMu.Lock()
			defer handlerMu.Unlock()
			messages = append(messages, msg)
		},
		onCompleteFn: func(result *agentboot.CompletionResult) {
			handlerMu.Lock()
			defer handlerMu.Unlock()
			completionResult = result
		},
		onErrorFn: func(err error) {
			handlerMu.Lock()
			defer handlerMu.Unlock()
			handlerErrors = append(handlerErrors, err)
		},
	}

	testCases := []struct {
		name    string
		message string
	}{
		{
			name:    "Simple greeting via handler",
			message: "Hello!",
		},
		{
			name:    "Tool use via handler",
			message: "List files in /tmp",
		},
		{
			name:    "Multi-step via handler",
			message: "Show pwd, then list files, then echo test done",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset tracking
			handlerMu.Lock()
			messages = nil
			completionResult = nil
			handlerErrors = nil
			handlerMu.Unlock()

			t.Logf("Message: %s", tc.message)

			ctx := context.Background()
			result, err := testAgent.ExecuteWithHandler(ctx, tc.message, toolCtx, handler)

			handlerMu.Lock()
			msgCount := len(messages)
			errCount := len(handlerErrors)
			hasCompletion := completionResult != nil
			handlerMu.Unlock()

			t.Logf("Result: exitCode=%d, err=%v", result.ExitCode, err)
			t.Logf("Handler: messages=%d, errors=%d, hasCompletion=%v", msgCount, errCount, hasCompletion)

			if result.Output != "" {
				preview := result.Output
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				t.Logf("Output: %s", preview)
			}

			// Key assertions
			assert.NoError(t, err, "ExecuteWithHandler should not error")
			assert.Equal(t, 0, result.ExitCode, "Exit code should be 0")
			assert.True(t, hasCompletion, "OnComplete should have been called")

			if errCount > 0 {
				t.Logf("⚠ Handler received %d errors", errCount)
				handlerMu.Lock()
				for i, e := range handlerErrors {
					t.Logf("  error[%d]: %v", i, e)
				}
				handlerMu.Unlock()
			}

			// Log message details
			handlerMu.Lock()
			for i, msg := range messages {
				if msgMap, ok := msg.(map[string]interface{}); ok {
					t.Logf("  msg[%d]: type=%v", i, msgMap["type"])
				} else {
					t.Logf("  msg[%d]: %T", i, msg)
				}
			}
			handlerMu.Unlock()
		})
	}
}

// ============================================================================
// Test: Multi-Turn Conversation (Session Persistence)
// ============================================================================

// TestMultiTurnConversation tests that context is maintained across turns.
// To run: go test -v -tags=e2e -run TestMultiTurnConversation ./internal/remote_control/smart_guide/
func TestMultiTurnConversation(t *testing.T) {
	cfg := createTestConfig()

	testAgent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)

	tracker := newIterationTracker()
	require.NoError(t, tracker.registerHooks(testAgent))

	ctx := context.Background()
	toolCtx := &ToolContext{
		ChatID:      "test-multi-turn",
		ProjectPath: "/tmp",
	}

	turns := []struct {
		name    string
		message string
	}{
		{name: "Turn 1", message: "Remember: my project name is TestProject123"},
		{name: "Turn 2", message: "What is my project name?"},
		{name: "Turn 3", message: "List files in /tmp"},
		{name: "Turn 4", message: "Now tell me what you just did and what my project name is"},
	}

	for i, turn := range turns {
		t.Run(turn.name, func(t *testing.T) {
			tracker.reset()

			t.Logf("[Turn %d] %s", i+1, turn.message)

			response, err := testAgent.ReplyWithContext(ctx, turn.message, toolCtx)

			if err != nil {
				t.Logf("⚠ Error on turn %d: %v", i+1, err)
				tracker.report(t)
				return
			}

			if response != nil {
				text := response.GetTextContent()
				if len(text) > 200 {
					t.Logf("Response: %s...", text[:200])
				} else {
					t.Logf("Response: %s", text)
				}
			}

			tracker.report(t)
		})
	}
}

// ============================================================================
// Test Message Handler (implements agentboot.MessageHandler for testing)
// ============================================================================

// Compile-time check that testMessageHandler implements agentboot.MessageHandler
var _ agentboot.MessageHandler = (*testMessageHandler)(nil)

type testMessageHandler struct {
	onMessageFn  func(interface{})
	onCompleteFn func(*agentboot.CompletionResult)
	onErrorFn    func(error)
}

func (h *testMessageHandler) OnMessage(msg interface{}) error {
	if h.onMessageFn != nil {
		h.onMessageFn(msg)
	}
	return nil
}

func (h *testMessageHandler) OnComplete(result *agentboot.CompletionResult) {
	if h.onCompleteFn != nil {
		h.onCompleteFn(result)
	}
}

func (h *testMessageHandler) OnError(err error) {
	if h.onErrorFn != nil {
		h.onErrorFn(err)
	}
}

func (h *testMessageHandler) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	// Auto-approve all in tests
	return agentboot.PermissionResult{Approved: true, Reason: "auto-approved in test"}, nil
}

func (h *testMessageHandler) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	return agentboot.AskResult{Response: "test response"}, nil
}
