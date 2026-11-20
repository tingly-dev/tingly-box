package agentboot

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// plainTransport is an agent that reads nothing while a turn runs.
type plainTransport struct{ AgentTransport }

// midTurnTransport is one that does.
type midTurnTransport struct{ plainTransport }

func (midTurnTransport) EncodeMidTurnMessage(prompt string) any {
	return map[string]any{"type": "user", "content": prompt}
}

func TestInject_AgentWithoutMidTurnInputRefuses(t *testing.T) {
	var wire bytes.Buffer
	s := &persistentSession{state: SessionStateRunning, transport: plainTransport{}, encoder: protocol.NewEncoder(&wire)}
	assert.False(t, s.CanInject())
	assert.ErrorIs(t, s.Inject(context.Background(), "now"), ErrInjectUnsupported)
	assert.Empty(t, wire.String(), "a refused message must not reach the process")
}

func TestInject_AgentWithMidTurnInputWritesIt(t *testing.T) {
	var wire bytes.Buffer
	s := &persistentSession{state: SessionStateRunning, transport: midTurnTransport{}, encoder: protocol.NewEncoder(&wire)}
	assert.True(t, s.CanInject())
	require.NoError(t, s.Inject(context.Background(), "now"))
	assert.Contains(t, wire.String(), `"content":"now"`)

	s.state = SessionStateIdle
	assert.ErrorIs(t, s.Inject(context.Background(), "late"), ErrNoTurnInFlight)
}
