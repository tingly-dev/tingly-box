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
		{name: "enabled unregistered v1 identity", enabled: true, source: protocol.TypeAnthropicV1, target: protocol.TypeAnthropicV1, wantErr: true},
		{name: "enabled unsupported pair", enabled: true, source: protocol.TypeAnthropicBeta, target: protocol.TypeOpenAIChat, wantErr: true},
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
