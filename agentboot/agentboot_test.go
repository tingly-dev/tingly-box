package agentboot

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// --- NewAgentService() ---

func TestNewAgentService_DefaultsApplied(t *testing.T) {
	svc, err := NewAgentService(Config{})
	require.NoError(t, err)
	assert.Equal(t, AgentTypeClaude, svc.config.DefaultAgent)
	assert.Equal(t, OutputFormatStreamJSON, svc.config.DefaultFormat)
	assert.Equal(t, 100, svc.config.StreamBufferSize)
}

func TestNewAgentService_DoesNotAssumeProviderSessionReader(t *testing.T) {
	svc, err := NewAgentService(Config{})
	require.NoError(t, err)
	assert.Nil(t, svc.sessionReader)
}

// --- Session API (AgentService) ---

func TestListSessions_WithoutReaderReturnsConfigurationError(t *testing.T) {
	svc, err := NewAgentService(Config{})
	require.NoError(t, err)

	_, err = svc.ListSessions(context.Background(), "/nonexistent/path/xyz", 10)
	assert.ErrorIs(t, err, errSessionReaderNotConfigured)
}

type testSessionReader struct {
	recent []common.SessionMetadata
}

func (r *testSessionReader) ListProjects(context.Context) ([]string, error) {
	return []string{"/project"}, nil
}

func (r *testSessionReader) ListSessions(context.Context, string) ([]common.SessionMetadata, error) {
	return r.recent, nil
}

func (r *testSessionReader) GetSession(context.Context, string) (*common.SessionMetadata, error) {
	if len(r.recent) == 0 {
		return nil, common.ErrSessionNotFound{}
	}
	return &r.recent[0], nil
}

func (r *testSessionReader) GetRecentSessions(context.Context, string, int) ([]common.SessionMetadata, error) {
	return r.recent, nil
}

func (r *testSessionReader) GetSessionEvents(context.Context, string, int, int) ([]common.SessionEvent, error) {
	return nil, nil
}

func (r *testSessionReader) GetSessionSummary(context.Context, string, int, int) (*common.SessionSummary, error) {
	return &common.SessionSummary{}, nil
}

func TestListSessions_WithReaderDelegates(t *testing.T) {
	reader := &testSessionReader{
		recent: []common.SessionMetadata{{SessionID: "session-1"}},
	}
	svc, err := NewAgentService(Config{}, WithSessionReader(reader))
	require.NoError(t, err)

	sessions, err := svc.ListSessions(context.Background(), "/project", 10)
	assert.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "session-1", sessions[0].SessionID)
}

func TestWithSessionReader_RejectsNil(t *testing.T) {
	_, err := NewAgentService(Config{}, WithSessionReader(nil))
	assert.Error(t, err)
}
