package server

import (
	"testing"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
)

func TestProtocolStageSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		source  protocol.APIType
		target  protocol.APIType
		want    bool
		wantErr bool
	}{
		{name: "disabled supported pair", source: protocol.TypeOpenAIChat, target: protocol.TypeAnthropicBeta},
		{name: "enabled supported pair", enabled: true, source: protocol.TypeOpenAIChat, target: protocol.TypeAnthropicBeta, want: true},
		{name: "enabled implicit identity pair", enabled: true, source: protocol.TypeOpenAIChat, target: protocol.TypeOpenAIChat, wantErr: true},
		{name: "enabled registered beta identity", enabled: true, source: protocol.TypeAnthropicBeta, target: protocol.TypeAnthropicBeta, want: true},
		{name: "enabled registered v1 identity", enabled: true, source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, want: true},
		{name: "enabled beta to chat", enabled: true, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, want: true},
		{name: "enabled v1 to chat", enabled: true, source: protocol.TypeAnthropicV1, target: protocol.TypeOpenAIChat, want: true},
		{name: "enabled registered responses identity", enabled: true, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIResponses, want: true},
		{name: "enabled responses to beta", enabled: true, source: protocol.TypeOpenAIResponses, target: protocol.TypeAnthropicBeta, want: true},
		{name: "enabled responses to chat", enabled: true, source: protocol.TypeOpenAIResponses, target: protocol.TypeOpenAIChat, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewProtocolStageSelector(tt.enabled).ShouldUseStage(tt.source, tt.target, stage.AllBridgeCapabilities)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("ShouldUseStage() = %v, %v; want %v, error=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
