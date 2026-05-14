package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestEndpointUsable(t *testing.T) {
	assert.True(t, endpointUsable(true, false, false))
	assert.True(t, endpointUsable(true, true, true))
	assert.False(t, endpointUsable(true, false, true))
	assert.False(t, endpointUsable(false, true, false))
}

func TestDefaultEndpointSelectionRespectsIncomingAPI(t *testing.T) {
	assert.Equal(t, protocol.TypeOpenAIChat, defaultEndpointSelection(IncomingAPIChat, "").Target)
	assert.Equal(t, protocol.TypeOpenAIResponses, defaultEndpointSelection(IncomingAPIResponses, "").Target)
}

func TestCanDowngradeResponsesToChatPlainRequest(t *testing.T) {
	ok, reason := CanDowngradeResponsesToChat(protocol.ResponseCreateRequest{})
	assert.True(t, ok)
	assert.Empty(t, reason)
}
