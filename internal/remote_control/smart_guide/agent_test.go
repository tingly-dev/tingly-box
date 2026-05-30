package smart_guide

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// NewTinglyBoxAgent construction / validation
// ============================================================================

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
		Model:            "claude-sonnet-4-6",
		GetStatusFunc: func(chatID string) (*StatusInfo, error) {
			return &StatusInfo{}, nil
		},
		UpdateProjectFunc: func(chatID string, projectPath string) error {
			return nil
		},
	}
	agent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.NotNil(t, agent.GetConfig())
	assert.NotNil(t, agent.GetExecutor())
}

func TestNewTinglyBoxAgent_NoModel(t *testing.T) {
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "http://localhost:12580/tingly/_smart_guide",
		APIKey:           "test-api-key",
		Model:            "", // missing model
	}
	agent, err := NewTinglyBoxAgent(cfg)
	assert.Nil(t, agent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestNewTinglyBoxAgent_NoAPIKey(t *testing.T) {
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "http://localhost:12580/tingly/_smart_guide",
		APIKey:           "", // Empty API key
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
		Model:            "claude-sonnet-4-6",
	}
	agent, err := NewTinglyBoxAgent(cfg)
	assert.Nil(t, agent)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestNewTinglyBoxAgent_NilSmartGuideConfigDefaults(t *testing.T) {
	cfg := &AgentConfig{
		BaseURL: "http://localhost:12580/tingly/_smart_guide",
		APIKey:  "test-api-key",
		Model:   "claude-sonnet-4-6",
	}
	agent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, agent)
	// A default config is filled in when none is provided.
	assert.NotNil(t, agent.GetConfig())
}

func TestNewTinglyBoxAgent_CustomToolExecutor(t *testing.T) {
	customExecutor := NewToolExecutor([]string{"foo", "bar"})
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "http://localhost:12580/tingly/_smart_guide",
		APIKey:           "test-api-key",
		Model:            "claude-sonnet-4-6",
		ToolExecutor:     customExecutor,
	}
	agent, err := NewTinglyBoxAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Same(t, customExecutor, agent.GetExecutor()) // Should use the provided executor
}

func TestNewTinglyBoxAgentWithSession_SeedsHistory(t *testing.T) {
	cfg := &AgentConfig{
		SmartGuideConfig: DefaultSmartGuideConfig(),
		BaseURL:          "http://localhost:12580/tingly/_smart_guide",
		APIKey:           "test-api-key",
		Model:            "claude-sonnet-4-6",
	}
	history := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hello")),
		{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("hi there")}},
	}
	agent, err := NewTinglyBoxAgentWithSession(cfg, history)
	require.NoError(t, err)
	require.NotNil(t, agent)
	assert.Len(t, agent.History(), 2)
	assert.Equal(t, "hi there", agent.LastAssistantText())
}

// ============================================================================
// Agent accessors
// ============================================================================

func TestTinglyBoxAgent_GetGreeting(t *testing.T) {
	agent := &TinglyBoxAgent{}
	assert.Equal(t, DefaultGreeting(), agent.GetGreeting())
}

func TestTinglyBoxAgent_GetExecutor(t *testing.T) {
	executor := NewToolExecutor([]string{})
	agent := &TinglyBoxAgent{executor: executor}
	assert.Same(t, executor, agent.GetExecutor())
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

func TestTinglyBoxAgent_LastAssistantText_Empty(t *testing.T) {
	agent := &TinglyBoxAgent{}
	assert.Equal(t, "", agent.LastAssistantText())
}

// ============================================================================
// CanCreateAgent
// ============================================================================

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

// ============================================================================
// SessionStore round-trip (anthropic-native message params)
// ============================================================================

func TestSessionStore_BlankDirDisabled(t *testing.T) {
	store, err := NewSessionStore("")
	require.NoError(t, err)
	assert.Nil(t, store, "blank dataDir should disable persistence")

	// nil store methods are safe no-ops.
	msgs, err := store.Load("chat-1")
	require.NoError(t, err)
	assert.Nil(t, msgs)
	require.NoError(t, store.Save("chat-1", nil))
	require.NoError(t, store.Delete("chat-1"))
}

func TestSessionStore_SaveLoadRoundTrip(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "sessions"))
	require.NoError(t, err)
	require.NotNil(t, store)

	// Loading an unknown chat returns empty, not an error.
	got, err := store.Load("unknown")
	require.NoError(t, err)
	assert.Empty(t, got)

	messages := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("what is 2+2?")),
		{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("4")}},
	}
	require.NoError(t, store.Save("chat-1", messages))

	loaded, err := store.Load("chat-1")
	require.NoError(t, err)
	require.Len(t, loaded, 2)

	// The SDK param types carry constant-typed fields that default differently
	// between in-memory construction and JSON unmarshal, so compare the JSON
	// encodings (the wire shape the model API consumes) rather than the structs.
	wantJSON, err := json.Marshal(messages)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(loaded)
	require.NoError(t, err)
	assert.JSONEq(t, string(wantJSON), string(gotJSON), "stored history should round-trip losslessly")

	assert.Equal(t, anthropic.BetaMessageParamRoleUser, loaded[0].Role)
	assert.Equal(t, anthropic.BetaMessageParamRoleAssistant, loaded[1].Role)
}

func TestSessionStore_Delete(t *testing.T) {
	store, err := NewSessionStore(filepath.Join(t.TempDir(), "sessions"))
	require.NoError(t, err)
	require.NotNil(t, store)

	messages := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
	}
	require.NoError(t, store.Save("chat-2", messages))

	loaded, err := store.Load("chat-2")
	require.NoError(t, err)
	require.Len(t, loaded, 1)

	require.NoError(t, store.Delete("chat-2"))

	loaded, err = store.Load("chat-2")
	require.NoError(t, err)
	assert.Empty(t, loaded, "history should be gone after delete")

	// Deleting a non-existent chat is not an error.
	require.NoError(t, store.Delete("chat-does-not-exist"))
}
