package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestAlternateOpenAIProtocol(t *testing.T) {
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIChat); got != protocol.TypeOpenAIResponses {
		t.Errorf("alternate of chat = %v, want responses", got)
	}
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIResponses); got != protocol.TypeOpenAIChat {
		t.Errorf("alternate of responses = %v, want chat", got)
	}
}

func TestIncomingToTarget(t *testing.T) {
	if got := incomingToTarget(IncomingAPIChat); got != protocol.TypeOpenAIChat {
		t.Errorf("incoming chat → %v, want chat", got)
	}
	if got := incomingToTarget(IncomingAPIResponses); got != protocol.TypeOpenAIResponses {
		t.Errorf("incoming responses → %v, want responses", got)
	}
}
