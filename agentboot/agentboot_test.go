package agentboot

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// --- New() ---

func TestNew_DefaultsApplied(t *testing.T) {
	ab, err := New(Config{})
	require.NoError(t, err)
	assert.Equal(t, AgentTypeClaude, ab.config.DefaultAgent)
	assert.Equal(t, OutputFormatStreamJSON, ab.config.DefaultFormat)
	assert.Equal(t, 100, ab.config.StreamBufferSize)
}

func TestNew_NoProjectsDir_StoreDefaultsToHome(t *testing.T) {
	ab, err := New(Config{})
	require.NoError(t, err)
	assert.NotNil(t, ab.store, "store should default to ~/.claude/projects when ClaudeProjectsDir not set")
}

func TestNew_InvalidProjectsDir_StoreStillInitialized(t *testing.T) {
	// Store accepts any path — validation happens lazily on first use
	ab, err := New(Config{ClaudeProjectsDir: "/nonexistent/path/xyz"})
	assert.NoError(t, err)
	assert.NotNil(t, ab.store, "store is initialized regardless; dir is validated on use")
}

func TestNew_ValidProjectsDir_StoreInitialized(t *testing.T) {
	dir := t.TempDir()
	ab, err := New(Config{ClaudeProjectsDir: dir})
	require.NoError(t, err)
	assert.NotNil(t, ab.store, "store should be initialized when valid dir provided")
}

// --- Session API (AgentService) ---

func TestListSessions_DefaultStore_NoError(t *testing.T) {
	svc, err := NewAgentService(Config{})
	require.NoError(t, err)

	// Default store points at ~/.claude/projects; an unknown path inside it
	// returns an empty slice (not an error), matching the on-disk store
	// behavior for missing project dirs.
	_, err = svc.ListSessions(context.Background(), "/nonexistent/path/xyz", 10)
	assert.NoError(t, err)
}

func TestGetSessionSummary_DefaultStore_ReturnsNotFound(t *testing.T) {
	svc, err := NewAgentService(Config{})
	require.NoError(t, err)

	// Store is configured; the session simply doesn't exist on disk.
	_, err = svc.GetSessionSummary(context.Background(), "definitely-not-a-real-session-id", 5, 5)
	assert.Error(t, err)
}

func TestListSessions_WithStore_Delegates(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewAgentService(Config{ClaudeProjectsDir: dir})
	require.NoError(t, err)

	// Empty dir → returns empty slice, no error
	sessions, err := svc.ListSessions(context.Background(), dir, 10)
	assert.NoError(t, err)
	assert.Empty(t, sessions)
}

// --- ResumeSession ---

func TestResumeSession_ReturnsOptionsWithSessionID(t *testing.T) {
	ab, err := New(Config{})
	require.NoError(t, err)

	opts := ab.ResumeSession("my-session-123")
	assert.Equal(t, "my-session-123", opts.SessionID)
	assert.True(t, opts.Resume)
}

// --- Event type unification ---

// agentboot.Event must be assignable to/from common.Event without conversion.
// This test fails to compile if the types are not unified (alias).
func TestEvent_IsCommonEvent(t *testing.T) {
	now := time.Now()
	ce := common.Event{
		Type:      "assistant",
		Data:      map[string]interface{}{"key": "val"},
		Timestamp: now,
		Raw:       `{"type":"assistant"}`,
	}

	// Direct assignment: only works if Event = common.Event (alias, not copy)
	var ae Event = ce
	assert.Equal(t, ce.Type, ae.Type)
	assert.Equal(t, ce.Raw, ae.Raw)

	// Slice assignability
	events := []common.Event{ce}
	var resultEvents []Event = events
	assert.Len(t, resultEvents, 1)
}
