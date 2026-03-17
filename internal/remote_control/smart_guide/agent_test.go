package smart_guide

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-agentscope/pkg/agent"
	"github.com/tingly-dev/tingly-agentscope/pkg/memory"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/model"
	"github.com/tingly-dev/tingly-agentscope/pkg/model/mockmodel"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
)

// Helper function to extract text from tool response content
func extractTextFromContent(content []message.ContentBlock) string {
	var result string
	for _, block := range content {
		if tb, ok := block.(*message.TextBlock); ok {
			result += tb.Text
		}
	}
	return result
}

// MockModelClient wraps mockmodel.MockModel to implement agent.ModelClient
type MockModelClient struct {
	*mockmodel.MockModel
}

func (m *MockModelClient) Call(ctx context.Context, messages []*message.Msg, options *model.CallOptions) (*model.ChatResponse, error) {
	return m.MockModel.Call(ctx, messages, options)
}

func (m *MockModelClient) Stream(ctx context.Context, messages []*message.Msg, options *model.CallOptions) (<-chan *model.ChatResponseChunk, error) {
	return m.MockModel.Stream(ctx, messages, options)
}

func (m *MockModelClient) ModelName() string {
	return m.MockModel.ModelName()
}

func (m *MockModelClient) IsStreaming() bool {
	return m.MockModel.IsStreaming()
}

func TestNewTinglyBoxAgent_NilConfig(t *testing.T) {
	agent, err := NewTinglyBoxAgent(nil)
	assert.Nil(t, agent)
	assert.EqualError(t, err, "config is required")
}

func TestNewTinglyBoxAgent_DefaultConfig(t *testing.T) {
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "http://localhost:12580/tingly/_smart_guide",
		APIKey:           "test-api-key",
		Provider:         "test-provider",
		Model:            "claude-sonnet-4-6",
		GetStatusFunc: func(chatID string) (*StatusInfo, error) {
			return &StatusInfo{}, nil
		},
		UpdateProjectFunc: func(chatID string, projectPath string) error {
			return nil
		},
	}
	agent, err := NewTinglyBoxAgent(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, agent)
	assert.NotNil(t, agent.ReActAgent)
	assert.NotNil(t, agent.config)
	assert.NotNil(t, agent.executor)
	assert.NotNil(t, agent.toolkit)
}

func TestNewTinglyBoxAgent_NoAPIKey(t *testing.T) {
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "http://localhost:12580/tingly/_smart_guide",
		APIKey:           "", // Empty API key
		Provider:         "test-provider",
		Model:            "claude-sonnet-4-6",
	}
	agent, err := NewTinglyBoxAgent(cfg)
	assert.Nil(t, agent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestNewTinglyBoxAgent_NoBaseURL(t *testing.T) {
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "", // Empty BaseURL
		APIKey:           "test-api-key",
		Provider:         "test-provider",
		Model:            "claude-sonnet-4-6",
	}
	agent, err := NewTinglyBoxAgent(cfg)
	assert.Nil(t, agent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestNewTinglyBoxAgent_CustomToolExecutor(t *testing.T) {
	customExecutor := NewToolExecutor([]string{"foo", "bar"})
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "http://localhost:12580/tingly/_smart_guide",
		APIKey:           "test-api-key",
		Provider:         "test-provider",
		Model:            "claude-sonnet-4-6",
		ToolExecutor:     customExecutor,
	}
	agent, err := NewTinglyBoxAgent(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Same(t, customExecutor, agent.executor) // Should use the provided executor
}

func TestTinglyBoxAgent_ReplyWithContext(t *testing.T) {
	// Create a mock model using mockmodel
	mockModel := mockmodel.NewWithResponses("I understand your request about the project")
	defer mockModel.Reset()

	// Wrap in MockModelClient
	mockModelClient := &MockModelClient{MockModel: mockModel}

	// Create a dummy ReActAgent
	dummyReActAgent := agent.NewReActAgent(&agent.ReActAgentConfig{
		Name:         "test-agent",
		SystemPrompt: "test",
		Model:        mockModelClient,
		Toolkit:      tool.NewToolkit(), // Empty toolkit for this test
		Memory:       memory.NewHistory(10),
	})

	executor := NewToolExecutor([]string{})
	testAgent := &TinglyBoxAgent{
		ReActAgent: dummyReActAgent,
		executor:   executor,
	}

	// Test with ProjectPath in ToolContext
	toolCtx := &ToolContext{
		ChatID:      "test-chat",
		ProjectPath: "/tmp/test-project",
	}
	response, err := testAgent.ReplyWithContext(context.Background(), "test message", toolCtx)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "/tmp/test-project", executor.GetWorkingDirectory()) // Verify working directory updated

	// Test without ProjectPath in ToolContext
	executor.SetWorkingDirectory("") // Reset
	toolCtx = &ToolContext{
		ChatID: "test-chat",
	}
	response, err = testAgent.ReplyWithContext(context.Background(), "test message", toolCtx)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	// Working directory should remain unchanged from default (or whatever os.Getwd() returns)
	assert.NotEqual(t, "/tmp/test-project", executor.GetWorkingDirectory())
}

func TestTinglyBoxAgent_GetGreeting(t *testing.T) {
	agent := &TinglyBoxAgent{}
	assert.Equal(t, DefaultGreeting(), agent.GetGreeting())
}

func TestTinglyBoxAgent_GetExecutor(t *testing.T) {
	executor := NewToolExecutor([]string{})
	agent := &TinglyBoxAgent{executor: executor}
	assert.Same(t, executor, agent.GetExecutor())
}

func TestTinglyBoxAgent_GetToolkit(t *testing.T) {
	toolkit := tool.NewToolkit()
	testAgent := &TinglyBoxAgent{toolkit: toolkit}
	assert.Same(t, toolkit, testAgent.GetToolkit())
}

func TestTinglyBoxAgent_IsEnabled(t *testing.T) {
	// Enabled
	agent := &TinglyBoxAgent{config: &SmartGuideConfig{Enabled: true}}
	assert.True(t, agent.IsEnabled())

	// Disabled
	agent.config.Enabled = false
	assert.False(t, agent.IsEnabled())

	// Nil config
	agent.config = nil
	assert.False(t, agent.IsEnabled())
}

func TestTinglyBoxAgent_GetConfig(t *testing.T) {
	cfg := DefaultSmartGuideConfig()
	agent := &TinglyBoxAgent{config: cfg}
	assert.Same(t, cfg, agent.GetConfig())
}

func TestNewAgentFactory(t *testing.T) {
	cfg := DefaultSmartGuideConfig()
	factory := NewAgentFactory(cfg, "http://localhost:12580/tingly/_smart_guide", "test-api-key", "test-provider", "test-model")
	assert.NotNil(t, factory)
	assert.Same(t, cfg, factory.config)
	assert.Equal(t, "http://localhost:12580/tingly/_smart_guide", factory.baseURL)
	assert.Equal(t, "test-api-key", factory.apiKey)
}

func TestAgentFactory_CreateAgent(t *testing.T) {
	cfg := DefaultSmartGuideConfig()
	factory := NewAgentFactory(cfg, "http://localhost:12580/tingly/_smart_guide", "test-api-key", "test-provider", "test-model")

	getStatus := func(chatID string) (*StatusInfo, error) { return &StatusInfo{}, nil }
	getProject := func(chatID string) (string, bool, error) { return "", false, nil }
	updateProject := func(chatID string, projectPath string) error { return nil }

	testAgent, err := factory.CreateAgent(getStatus, getProject, updateProject)
	assert.NoError(t, err)
	assert.NotNil(t, testAgent)
	assert.NotNil(t, testAgent.ReActAgent)
	assert.NotNil(t, testAgent.config)
	assert.NotNil(t, testAgent.executor)
	assert.NotNil(t, testAgent.toolkit)
}

func TestToolExecutor_SetWorkingDirectory(t *testing.T) {
	executor := NewToolExecutor([]string{})

	// Initially empty
	assert.Equal(t, "", executor.GetWorkingDirectory())

	// Set directory
	executor.SetWorkingDirectory("/tmp/test")
	assert.Equal(t, "/tmp/test", executor.GetWorkingDirectory())

	// Change directory
	executor.SetWorkingDirectory("/home/user")
	assert.Equal(t, "/home/user", executor.GetWorkingDirectory())
}

func TestToolExecutor_ResolvePath(t *testing.T) {
	executor := NewToolExecutor([]string{})

	// Absolute path should be returned as-is
	absPath := executor.ResolvePath("/absolute/path")
	assert.Equal(t, "/absolute/path", absPath)

	// Relative path without working directory should use current directory
	executor.SetWorkingDirectory("")
	relPath := executor.ResolvePath("relative/path")
	// Should be a valid absolute path (we can't predict the exact value due to os.Getwd())
	// Just check it's not the same as input
	assert.NotEqual(t, "relative/path", relPath)

	// Relative path with working directory should be joined
	executor.SetWorkingDirectory("/home/user")
	relPath = executor.ResolvePath("project")
	assert.Equal(t, "/home/user/project", relPath)
}

func TestToolExecutor_GetAllowedCommands(t *testing.T) {
	executor := NewToolExecutor([]string{"ls", "pwd", "git"})
	commands := executor.GetAllowedCommands()

	assert.Len(t, commands, 3)
	assert.Contains(t, commands, "ls")
	assert.Contains(t, commands, "pwd")
	assert.Contains(t, commands, "git")
}

func TestToolExecutor_ExecuteBash_AllowedCommand(t *testing.T) {
	executor := NewToolExecutor([]string{"echo", "pwd"})

	ctx := context.Background()
	output, err := executor.ExecuteBash(ctx, "echo", "hello")

	assert.NoError(t, err)
	assert.Contains(t, output, "hello")
}

func TestToolExecutor_ExecuteBash_NotAllowedCommand(t *testing.T) {
	executor := NewToolExecutor([]string{"echo"})

	ctx := context.Background()
	_, err := executor.ExecuteBash(ctx, "rm", "-rf", "/")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestBashTool_Name(t *testing.T) {
	executor := NewToolExecutor([]string{"ls"})
	tool := NewBashTool(executor, []string{"ls"})
	assert.Equal(t, "bash", tool.Name())
}

func TestBashTool_Description(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewBashTool(executor, []string{})
	desc := tool.Description()
	assert.Contains(t, desc, "bash")
	assert.Contains(t, desc, "Allowed commands")
}

// Parameters() method removed in tool refactoring - tools now use typed params
// See RegisterTools() for the new registration pattern

func TestBashTool_Call_AllowedCommand(t *testing.T) {
	executor := NewToolExecutor([]string{"echo"})
	tool := NewBashTool(executor, []string{"echo"})

	ctx := context.Background()
	resp, err := tool.Call(ctx, BashParams{Command: "echo hello"})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "hello")
}

func TestBashTool_Call_NotAllowedCommand(t *testing.T) {
	executor := NewToolExecutor([]string{"echo"})
	tool := NewBashTool(executor, []string{"echo"})

	ctx := context.Background()
	resp, err := tool.Call(ctx, BashParams{Command: "rm -rf /"})

	assert.NoError(t, err) // Tool doesn't return error, just error message
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "not allowed")
}

func TestBashTool_Call_CDAllowed(t *testing.T) {
	executor := NewToolExecutor([]string{"cd", "ls", "pwd"})
	tool := NewBashTool(executor, []string{"cd", "ls", "pwd"})

	ctx := context.Background()
	// cd is now allowed in bash (uses shell chaining)
	resp, err := tool.Call(ctx, BashParams{Command: "cd /tmp && pwd"})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "/tmp")
}

func TestGetStatusTool_Name(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewGetStatusTool(executor, nil)
	assert.Equal(t, "get_status", tool.Name())
}

func TestGetStatusTool_Description(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewGetStatusTool(executor, nil)
	desc := tool.Description()
	assert.Contains(t, desc, "status")
}

func TestGetStatusTool_Call_NoCallback(t *testing.T) {
	executor := NewToolExecutor([]string{})
	executor.SetWorkingDirectory("/tmp")
	tool := NewGetStatusTool(executor, nil)

	ctx := context.Background()
	resp, err := tool.Call(ctx, GetStatusParams{})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "/tmp")
}

func TestGetStatusTool_Call_WithCallback(t *testing.T) {
	executor := NewToolExecutor([]string{})
	executor.SetWorkingDirectory("/home/user/project")

	getStatus := func(chatID string) (*StatusInfo, error) {
		return &StatusInfo{
			CurrentAgent:   "@tb",
			SessionID:      "session-123",
			ProjectPath:    "/home/user/project",
			WorkingDir:     "/home/user/project",
			HasRunningTask: false,
			Whitelisted:    true,
		}, nil
	}
	tool := NewGetStatusTool(executor, getStatus)

	ctx := context.Background()
	resp, err := tool.Call(ctx, GetStatusParams{ChatID: "chat-123"})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "@tb")
	assert.Contains(t, text, "session-123")
	assert.Contains(t, text, "true")
}

func TestChangeDirTool_Name(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewChangeDirTool(executor, nil)
	assert.Equal(t, "change_workdir", tool.Name())
}

func TestChangeDirTool_Description(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewChangeDirTool(executor, nil)
	desc := tool.Description()
	assert.Contains(t, desc, "directory")
}

func TestChangeDirTool_Call_Success(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewChangeDirTool(executor, nil)

	ctx := context.Background()
	resp, err := tool.Call(ctx, ChangeDirParams{Path: "/tmp"})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "Changed directory")
	assert.Equal(t, "/tmp", executor.GetWorkingDirectory())
}

func TestChangeDirTool_Call_RelativePath(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewChangeDirTool(executor, nil)

	ctx := context.Background()
	// Use /tmp which exists
	executor.SetWorkingDirectory("/tmp")
	resp, err := tool.Call(ctx, ChangeDirParams{Path: ".."})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	// Should change to parent of /tmp
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "Changed directory")
	// Parent of /tmp is /
	assert.Equal(t, "/", executor.GetWorkingDirectory())
}

func TestChangeDirTool_Call_EmptyPath(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewChangeDirTool(executor, nil)

	ctx := context.Background()
	resp, err := tool.Call(ctx, ChangeDirParams{})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "required")
}

func TestChangeDirTool_Call_NotADirectory(t *testing.T) {
	executor := NewToolExecutor([]string{})
	tool := NewChangeDirTool(executor, nil)

	ctx := context.Background()
	// Use /etc/passwd which exists but is not a directory
	resp, err := tool.Call(ctx, ChangeDirParams{Path: "/etc/passwd"})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "not a directory")
}

func TestHandoffToCCTool_Name(t *testing.T) {
	tool := NewHandoffToCCTool()
	assert.Equal(t, "handoff_to_cc", tool.Name())
}

func TestHandoffToCCTool_Description(t *testing.T) {
	tool := NewHandoffToCCTool()
	desc := tool.Description()
	assert.Contains(t, desc, "Claude Code")
}

func TestHandoffToCCTool_Call(t *testing.T) {
	tool := NewHandoffToCCTool()

	ctx := context.Background()
	resp, err := tool.Call(ctx, map[string]any{})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	text := extractTextFromContent(resp.Content)
	assert.Equal(t, "HANDOFF_TO_CC", text)
}

func TestBashTool_ComplexCommands(t *testing.T) {
	executor := NewToolExecutor([]string{"echo", "sh", "cat"})
	tool := NewBashTool(executor, []string{"echo", "sh", "cat"})

	ctx := context.Background()

	// Test command with quotes
	resp, err := tool.Call(ctx, BashParams{Command: "echo 'hello world'"})
	assert.NoError(t, err)
	text := extractTextFromContent(resp.Content)
	assert.Contains(t, text, "hello world")

	// Test command with pipe (if shell supports it)
	resp, err = tool.Call(ctx, BashParams{Command: "echo 'test' | cat"})
	assert.NoError(t, err)
	text = extractTextFromContent(resp.Content)
	assert.Contains(t, text, "test")

	// Test command with redirect
	resp, err = tool.Call(ctx, BashParams{Command: "echo 'redirect test' > /dev/null && echo 'success'"})
	assert.NoError(t, err)
	text = extractTextFromContent(resp.Content)
	assert.Contains(t, text, "success")
}

// CanCreateAgent tests

func TestCanCreateAgent_EmptyBaseURL(t *testing.T) {
	result := CanCreateAgent("", "api-key", "provider-uuid", "model-id")
	assert.False(t, result, "Should return false when BaseURL is empty")
}

func TestCanCreateAgent_EmptyAPIKey(t *testing.T) {
	result := CanCreateAgent("http://localhost:12580/tingly/_smart_guide", "", "provider-uuid", "model-id")
	assert.False(t, result, "Should return false when APIKey is empty")
}

func TestCanCreateAgent_EmptyProvider(t *testing.T) {
	result := CanCreateAgent("http://localhost:12580/tingly/_smart_guide", "api-key", "", "model-id")
	assert.False(t, result, "Should return false when provider is empty")
}

func TestCanCreateAgent_EmptyModel(t *testing.T) {
	result := CanCreateAgent("http://localhost:12580/tingly/_smart_guide", "api-key", "provider-uuid", "")
	assert.False(t, result, "Should return false when model is empty")
}

func TestCanCreateAgent_Success(t *testing.T) {
	result := CanCreateAgent("http://localhost:12580/tingly/_smart_guide", "api-key", "provider-uuid", "model-id")
	assert.True(t, result, "Should return true when all required values are provided")
}
