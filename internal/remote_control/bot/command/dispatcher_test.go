package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
)

// mockHandler is a mock implementation of command.Handler for testing
type mockHandler struct {
	sendTextFunc           func(chatID, text string) error
	getProjectPathFunc     func(chatID string) (string, error)
	setProjectPathFunc     func(chatID, path string) error
	stopExecutionFunc      func(chatID string) bool
	getCurrentAgentFunc    func(chatID string) (string, error)
	getVerboseFunc         func(chatID string) bool
	setVerboseFunc         func(chatID string, enabled bool)
	isWhitelistedFunc      func(groupID string) bool
	getBashCwdFunc         func(chatID string) (string, error)
	setBashCwdFunc         func(chatID, path string) error
	getDefaultProjectPath  func() string
	getBashAllowlistFunc   func() map[string]struct{}
	listProjectPathsFunc   func(ownerID, platform string) ([]string, error)
	verifyAndPairFunc      func(botUUID, chatID, senderID, platform, code string) error
	clearSessionFunc       func(chatID, agentType string) error
	findOrCreateSession    func(chatID, agentType, projectPath string) (*SessionInfo, error)
	updatePermissionMode   func(sessionID, mode string) error
	getSessionFunc         func(chatID, agentType, projectPath string) (*SessionInfo, error)
}

func (m *mockHandler) SendText(chatID, text string) error {
	if m.sendTextFunc != nil {
		return m.sendTextFunc(chatID, text)
	}
	return nil
}

func (m *mockHandler) GetProjectPath(chatID string) (string, error) {
	if m.getProjectPathFunc != nil {
		return m.getProjectPathFunc(chatID)
	}
	return "", nil
}

func (m *mockHandler) SetProjectPath(chatID, path string) error {
	if m.setProjectPathFunc != nil {
		return m.setProjectPathFunc(chatID, path)
	}
	return nil
}

func (m *mockHandler) GetProjectPathForGroup(chatID, platform string) (string, bool) {
	return "", false
}

func (m *mockHandler) StopExecution(chatID string) bool {
	if m.stopExecutionFunc != nil {
		return m.stopExecutionFunc(chatID)
	}
	return false
}

func (m *mockHandler) GetCurrentAgent(chatID string) (string, error) {
	if m.getCurrentAgentFunc != nil {
		return m.getCurrentAgentFunc(chatID)
	}
	return "tingly-box", nil
}

func (m *mockHandler) SetVerbose(chatID string, enabled bool) {
	if m.setVerboseFunc != nil {
		m.setVerboseFunc(chatID, enabled)
	}
}

func (m *mockHandler) GetVerbose(chatID string) bool {
	if m.getVerboseFunc != nil {
		return m.getVerboseFunc(chatID)
	}
	return true
}

func (m *mockHandler) IsWhitelisted(groupID string) bool {
	if m.isWhitelistedFunc != nil {
		return m.isWhitelistedFunc(groupID)
	}
	return false
}

func (m *mockHandler) AddToWhitelist(groupID, platform, userID string) error {
	return nil
}

func (m *mockHandler) GetBashCwd(chatID string) (string, error) {
	if m.getBashCwdFunc != nil {
		return m.getBashCwdFunc(chatID)
	}
	return "", nil
}

func (m *mockHandler) SetBashCwd(chatID, path string) error {
	if m.setBashCwdFunc != nil {
		return m.setBashCwdFunc(chatID, path)
	}
	return nil
}

func (m *mockHandler) ResolveChatID(input string) (string, error) {
	return input, nil
}

func (m *mockHandler) GetDefaultProjectPath() string {
	if m.getDefaultProjectPath != nil {
		return m.getDefaultProjectPath()
	}
	return ""
}

func (m *mockHandler) GetBashAllowlist() map[string]struct{} {
	if m.getBashAllowlistFunc != nil {
		return m.getBashAllowlistFunc()
	}
	return make(map[string]struct{})
}

func (m *mockHandler) ListProjectPaths(ownerID, platform string) ([]string, error) {
	if m.listProjectPathsFunc != nil {
		return m.listProjectPathsFunc(ownerID, platform)
	}
	return []string{}, nil
}

func (m *mockHandler) VerifyAndPair(botUUID, chatID, senderID, platform, code string) error {
	if m.verifyAndPairFunc != nil {
		return m.verifyAndPairFunc(botUUID, chatID, senderID, platform, code)
	}
	return nil
}

func (m *mockHandler) ClearSession(chatID, agentType string) error {
	if m.clearSessionFunc != nil {
		return m.clearSessionFunc(chatID, agentType)
	}
	return nil
}

func (m *mockHandler) FindOrCreateSession(chatID, agentType, projectPath string) (*SessionInfo, error) {
	if m.findOrCreateSession != nil {
		return m.findOrCreateSession(chatID, agentType, projectPath)
	}
	return &SessionInfo{}, nil
}

func (m *mockHandler) UpdatePermissionMode(sessionID, mode string) error {
	if m.updatePermissionMode != nil {
		return m.updatePermissionMode(sessionID, mode)
	}
	return nil
}

func (m *mockHandler) GetSession(chatID, agentType, projectPath string) (*SessionInfo, error) {
	if m.getSessionFunc != nil {
		return m.getSessionFunc(chatID, agentType, projectPath)
	}
	return &SessionInfo{}, nil
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		cmd      string
		args     []string
	}{
		{
			name: "simple command",
			text: "/help",
			cmd:  "help",
			args: nil,
		},
		{
			name: "command with args",
			text: "/cd my-project",
			cmd:  "cd",
			args: []string{"my-project"},
		},
		{
			name: "command with multiple args",
			text: "/bash cd /tmp",
			cmd:  "bash",
			args: []string{"cd", "/tmp"},
		},
		{
			name: "command without slash",
			text: "help",
			cmd:  "help",
			args: nil,
		},
		{
			name: "empty text",
			text: "",
			cmd:  "",
			args: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args := ParseCommand(tt.text)
			if cmd != tt.cmd {
				t.Errorf("ParseCommand() cmd = %v, want %v", cmd, tt.cmd)
			}
			if !equalStringSlice(args, tt.args) {
				t.Errorf("ParseCommand() args = %v, want %v", args, tt.args)
			}
		})
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNormalizeCommandName(t *testing.T) {
	tests := []struct {
		name string
		input string
		want string
	}{
		{
			name: "with slash",
			input: "/help",
			want: "help",
		},
		{
			name: "uppercase",
			input: "/HELP",
			want: "help",
		},
		{
			name: "mixed case",
			input: "/Cd",
			want: "cd",
		},
		{
			name: "with spaces",
			input: "  /help  ",
			want: "help",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCommandName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeCommandName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	// Create a test command with handler
	cmd := NewBuilder("test-cmd", "test", "Test command").
		WithCategory("test").
		WithPriority(10).
		WithHandler(func(ctx *Context, h Handler) error {
			return nil
		}).
		Build()

	// Register the command
	err := registry.Register(cmd)
	if err != nil {
		t.Fatalf("Failed to register command: %v", err)
	}

	// Lookup the command
	got, ok := registry.Lookup("test")
	if !ok {
		t.Fatal("Command not found")
	}

	if got.ID() != "test-cmd" {
		t.Errorf("got.ID() = %v, want %v", got.ID(), "test-cmd")
	}

	// Check count
	if count := registry.Count(); count != 1 {
		t.Errorf("Count() = %v, want %v", count, 1)
	}
}

func TestDispatcher(t *testing.T) {
	handler := &mockHandler{
		sendTextFunc: func(chatID, text string) error {
			return nil
		},
	}

	dispatcher := NewDispatcher(handler)

	// Register a simple test command
	cmd := NewBuilder("test", "test", "Test command").
		WithHandler(func(ctx *Context, h Handler) error {
			return h.SendText(ctx.ChatID, "test response")
		}).
		Build()

	dispatcher.Register(cmd)

	// Create a test context
	ctx := &Context{
		ChatID:   "test-chat",
		SenderID: "test-user",
		Platform: imbot.PlatformTelegram,
		IsDirect: true,
	}

	// Test handling the command
	handled, err := dispatcher.Handle(ctx, "/test")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if !handled {
		t.Error("Handle() = false, want true")
	}

	// Test non-command
	handled, err = dispatcher.Handle(ctx, "not a command")
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if handled {
		t.Error("Handle() = true for non-command, want false")
	}
}

func TestDispatcher_BuildHelpText(t *testing.T) {
	handler := &mockHandler{}
	dispatcher := NewDispatcher(handler)

	// Register some test commands
	dispatcher.Register(NewBuilder("cmd1", "help", "Help command").
		WithCategory("session").
		WithPriority(100).
		WithHandler(func(ctx *Context, h Handler) error {
			return nil
		}).
		Build())

	dispatcher.Register(NewBuilder("cmd2", "cd", "Change directory").
		WithCategory("project").
		WithPriority(90).
		WithHandler(func(ctx *Context, h Handler) error {
			return nil
		}).
		Build())

	// Build help text
	helpText := dispatcher.BuildHelpText(imbot.PlatformTelegram)

	if helpText == "" {
		t.Error("BuildHelpText() returned empty string")
	}

	// Check that categories are present
	if !contains(helpText, "SESSION") {
		t.Error("Help text missing SESSION category")
	}
	if !contains(helpText, "PROJECT") {
		t.Error("Help text missing PROJECT category")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
