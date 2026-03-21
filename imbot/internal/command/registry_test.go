// Package command provides a simple, generic command management system for bots.
package command

import (
	"context"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		cmd     Command
		wantErr bool
	}{
		{
			name: "valid command",
			cmd: Command{
				ID:          "test",
				Name:        "test",
				Description: "test command",
				Handler:     func(ctx *HandlerContext, args []string) error { return nil },
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			cmd: Command{
				Name:        "test",
				Description: "test command",
				Handler:     func(ctx *HandlerContext, args []string) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "missing name",
			cmd: Command{
				ID:          "test",
				Description: "test command",
				Handler:     func(ctx *HandlerContext, args []string) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "missing handler",
			cmd: Command{
				ID:          "test",
				Name:        "test",
				Description: "test command",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegister(t *testing.T) {
	registry := NewRegistry()

	cmd := Command{
		ID:          "test",
		Name:        "test",
		Description: "test command",
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	}

	err := registry.Register(cmd)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Check that command is registered
	if got, ok := registry.Get("test"); !ok {
		t.Error("Get() should return true")
	} else if got.ID != "test" {
		t.Errorf("Get() ID = %v, want test", got.ID)
	}

	// Check that Count is updated
	if got := registry.Count(); got != 1 {
		t.Errorf("Count() = %v, want 1", got)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	registry := NewRegistry()

	cmd1 := Command{
		ID:          "test",
		Name:        "test",
		Description: "test command",
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	}
	cmd2 := Command{
		ID:          "test2",
		Name:        "test", // same name
		Description: "test command 2",
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	}

	_ = registry.Register(cmd1)
	err := registry.Register(cmd2)
	if err == nil {
		t.Error("Register() with duplicate name should return error")
	}
}

func TestGet(t *testing.T) {
	registry := NewRegistry()

	cmd := Command{
		ID:          "test",
		Name:        "test",
		Aliases:     []string{"t", "testing"},
		Description: "test command",
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	}

	_ = registry.Register(cmd)

	// Test by primary name
	if got, ok := registry.Get("test"); !ok {
		t.Error("Get(test) should return true")
	} else if got.Name != "test" {
		t.Errorf("Get(test) Name = %v, want test", got.Name)
	}

	// Test by alias
	if got, ok := registry.Get("t"); !ok {
		t.Error("Get(t) should return true")
	} else if got.Name != "test" {
		t.Errorf("Get(t) Name = %v, want test", got.Name)
	}

	// Test non-existent
	if _, ok := registry.Get("nonexistent"); ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestMatch(t *testing.T) {
	registry := NewRegistry()

	handler := func(ctx *HandlerContext, args []string) error { return nil }

	cmd := Command{
		ID:          "test",
		Name:        "test",
		Aliases:     []string{"t"},
		Description: "test command",
		Handler:     handler,
	}

	_ = registry.Register(cmd)

	// Test match by name
	got, ok := registry.Match("test")
	if !ok {
		t.Error("Match(test) should return true")
	}
	if got == nil {
		t.Error("Match(test) handler should not be nil")
	}

	// Test match by alias
	got, ok = registry.Match("t")
	if !ok {
		t.Error("Match(t) should return true")
	}
	if got == nil {
		t.Error("Match(t) handler should not be nil")
	}

	// Test non-existent
	_, ok = registry.Match("nonexistent")
	if ok {
		t.Error("Match(nonexistent) should return false")
	}
}

func TestCommandMatch(t *testing.T) {
	cmd := Command{
		ID:          "test",
		Name:        "test",
		Aliases:     []string{"t", "testing"},
		Description: "test command",
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	}

	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{"primary name", "test", true},
		{"alias 1", "t", true},
		{"alias 2", "testing", true},
		{"non-existent", "nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cmd.Match(tt.arg); got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllNames(t *testing.T) {
	cmd := Command{
		ID:          "test",
		Name:        "test",
		Aliases:     []string{"t", "testing"},
		Description: "test command",
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	}

	names := cmd.AllNames()
	if len(names) != 3 {
		t.Errorf("AllNames() = %v, want 3", len(names))
	}

	// Check that primary name is first
	if names[0] != "test" {
		t.Errorf("AllNames()[0] = %v, want test", names[0])
	}
}

func TestBuilder(t *testing.T) {
	handler := func(ctx *HandlerContext, args []string) error { return nil }

	cmd, err := NewCommand("test", "test", "test command").
		WithAliases("t", "testing").
		WithHandler(handler).
		WithCategory("test").
		WithPriority(10).
		Hidden().
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if cmd.ID != "test" {
		t.Errorf("ID = %v, want test", cmd.ID)
	}
	if cmd.Name != "test" {
		t.Errorf("Name = %v, want test", cmd.Name)
	}
	if len(cmd.Aliases) != 2 {
		t.Errorf("Aliases length = %v, want 2", len(cmd.Aliases))
	}
	if cmd.Category != "test" {
		t.Errorf("Category = %v, want test", cmd.Category)
	}
	if cmd.Priority != 10 {
		t.Errorf("Priority = %v, want 10", cmd.Priority)
	}
	if !cmd.Hidden {
		t.Error("Hidden should be true")
	}
}

func TestBuilderMustBuild(t *testing.T) {
	handler := func(ctx *HandlerContext, args []string) error { return nil }

	// Should not panic
	cmd := NewCommand("test", "test", "test command").
		WithHandler(handler).
		MustBuild()

	if cmd.ID != "test" {
		t.Errorf("ID = %v, want test", cmd.ID)
	}

	// Should panic with invalid command
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBuild() with invalid command should panic")
		}
	}()

	NewCommand("", "test", "test command").MustBuild()
}

func TestHandlerContext(t *testing.T) {
	// Mock bot for testing
	mockBot := &mockBot{}

	ctx := NewHandlerContext(mockBot, "chat123", "user456", "telegram").
		WithText("test message").
		WithDirectMessage(true).
		WithMessageID("msg789")

	if ctx.ChatID != "chat123" {
		t.Errorf("ChatID = %v, want chat123", ctx.ChatID)
	}
	if ctx.SenderID != "user456" {
		t.Errorf("SenderID = %v, want user456", ctx.SenderID)
	}
	if ctx.Text != "test message" {
		t.Errorf("Text = %v, want test message", ctx.Text)
	}
	if !ctx.IsDirectMessage {
		t.Error("IsDirectMessage should be true")
	}
	if ctx.MessageID != "msg789" {
		t.Errorf("MessageID = %v, want msg789", ctx.MessageID)
	}

	cloned := ctx.Clone()
	if cloned.ChatID != ctx.ChatID {
		t.Error("Clone() should copy ChatID")
	}
}

func TestPrioritySorting(t *testing.T) {
	registry := NewRegistry()

	commands := []Command{
		{ID: "low", Name: "low", Description: "low", Handler: func(ctx *HandlerContext, args []string) error { return nil }, Priority: 1},
		{ID: "high", Name: "high", Description: "high", Handler: func(ctx *HandlerContext, args []string) error { return nil }, Priority: 100},
		{ID: "medium", Name: "medium", Description: "medium", Handler: func(ctx *HandlerContext, args []string) error { return nil }, Priority: 50},
	}

	for _, cmd := range commands {
		_ = registry.Register(cmd)
	}

	all := registry.All()
	if len(all) != 3 {
		t.Fatalf("All() length = %v, want 3", len(all))
	}

	if all[0].ID != "high" {
		t.Errorf("All()[0] ID = %v, want high", all[0].ID)
	}
	if all[1].ID != "medium" {
		t.Errorf("All()[1] ID = %v, want medium", all[1].ID)
	}
	if all[2].ID != "low" {
		t.Errorf("All()[2] ID = %v, want low", all[2].ID)
	}
}

func TestHiddenCommands(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(Command{
		ID:          "visible",
		Name:        "visible",
		Description: "visible command",
		Hidden:      false,
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	})

	_ = registry.Register(Command{
		ID:          "hidden",
		Name:        "hidden",
		Description: "hidden command",
		Hidden:      true,
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	})

	// Hidden commands should still be findable via Get
	if _, ok := registry.Get("hidden"); !ok {
		t.Error("Get() should find hidden commands")
	}

	// All() returns all commands (including hidden)
	all := registry.All()
	if len(all) != 2 {
		t.Errorf("All() should return 2 commands, got %d", len(all))
	}

	// ForPlatform() filters out hidden commands
	visible := registry.ForPlatform("telegram")
	if len(visible) != 1 {
		t.Errorf("ForPlatform() should return 1 command (no hidden), got %d", len(visible))
	}
	for _, cmd := range visible {
		if cmd.Hidden {
			t.Error("ForPlatform() should not include hidden commands")
		}
	}
}

func TestBuildHelpText(t *testing.T) {
	registry := NewRegistry()

	_ = registry.Register(Command{
		ID:          "help",
		Name:        "help",
		Description: "Show help",
		Category:    "session",
		Priority:    100,
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	})

	_ = registry.Register(Command{
		ID:          "cd",
		Name:        "cd",
		Description: "Change directory",
		Category:    "project",
		Priority:    90,
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	})

	_ = registry.Register(Command{
		ID:          "secret",
		Name:        "secret",
		Description: "Secret command",
		Category:    "system",
		Hidden:      true,
		Handler:     func(ctx *HandlerContext, args []string) error { return nil },
	})

	text := registry.BuildHelpText(true)
	if text == "" {
		t.Error("BuildHelpText() returned empty string")
	}

	// Hidden command should not be in help text
	if contains(text, "secret") {
		t.Error("BuildHelpText() should not include hidden commands")
	}

	// Visible commands should be in help text
	if !contains(text, "/help") {
		t.Error("BuildHelpText() should include /help")
	}
	if !contains(text, "/cd") {
		t.Error("BuildHelpText() should include /cd")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockBot is a minimal mock for testing
type mockBot struct{}

func (m *mockBot) UUID() string                         { return "mock-uuid" }
func (m *mockBot) Connect(ctx context.Context) error    { return nil }
func (m *mockBot) Disconnect(ctx context.Context) error { return nil }
func (m *mockBot) IsConnected() bool                    { return true }
func (m *mockBot) SendMessage(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	return &core.SendResult{}, nil
}
func (m *mockBot) SendText(ctx context.Context, target string, text string) (*core.SendResult, error) {
	return &core.SendResult{}, nil
}
func (m *mockBot) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return &core.SendResult{}, nil
}
func (m *mockBot) React(ctx context.Context, messageID string, emoji string) error      { return nil }
func (m *mockBot) EditMessage(ctx context.Context, messageID string, text string) error { return nil }
func (m *mockBot) DeleteMessage(ctx context.Context, messageID string) error            { return nil }
func (m *mockBot) ChunkText(text string) []string                                       { return []string{text} }
func (m *mockBot) ValidateTextLength(text string) error                                 { return nil }
func (m *mockBot) GetMessageLimit() int                                                 { return 4000 }
func (m *mockBot) Status() *core.BotStatus                                              { return &core.BotStatus{} }
func (m *mockBot) PlatformInfo() *core.PlatformInfo                                     { return &core.PlatformInfo{} }
func (m *mockBot) OnMessage(handler func(core.Message))                                 {}
func (m *mockBot) OnError(handler func(error))                                          {}
func (m *mockBot) OnConnected(handler func())                                           {}
func (m *mockBot) OnDisconnected(handler func())                                        {}
func (m *mockBot) OnReady(handler func())                                               {}
func (m *mockBot) Close() error                                                         { return nil }
