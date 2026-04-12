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

func TestNew_NoProjectsDir_StoreNil(t *testing.T) {
	ab, err := New(Config{})
	require.NoError(t, err)
	assert.Nil(t, ab.store, "store should be nil when ClaudeProjectsDir not set")
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

// --- Session API ---

func TestListRecentSessions_NoStore_ReturnsError(t *testing.T) {
	ab, err := New(Config{})
	require.NoError(t, err)

	_, err = ab.ListRecentSessions(context.Background(), "/some/path", 10)
	assert.Error(t, err)
}

func TestGetSessionSummary_NoStore_ReturnsError(t *testing.T) {
	ab, err := New(Config{})
	require.NoError(t, err)

	_, err = ab.GetSessionSummary(context.Background(), "session-id", 5, 5)
	assert.Error(t, err)
}

func TestListRecentSessions_WithStore_Delegates(t *testing.T) {
	dir := t.TempDir()
	ab, err := New(Config{ClaudeProjectsDir: dir})
	require.NoError(t, err)

	// Empty dir → returns empty slice, no error
	sessions, err := ab.ListRecentSessions(context.Background(), dir, 10)
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
