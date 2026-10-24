package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// Approving a tool call can also grant a directory for the rest of the
// session; the wire shape is the one the CLI accepted when run for real
// (ref/live-control.md).
func TestTransport_ApprovalAddsDirectories(t *testing.T) {
	tr := NewTransport()
	wire := tr.EncodeControlResponse("r1", agentboot.ApprovalResponse{
		Approved:       true,
		AddDirectories: []string{"/data/other"},
	}, map[string]any{"file_path": "/data/other/a.txt"}).(map[string]any)

	inner := wire["response"].(map[string]any)["response"].(map[string]any)
	assert.Equal(t, "allow", inner["behavior"])
	assert.Equal(t, []map[string]any{{
		"type": "addDirectories", "directories": []string{"/data/other"}, "destination": "session",
	}}, inner["updatedPermissions"])

	plain := tr.EncodeControlResponse("r2", agentboot.ApprovalResponse{Approved: true}, nil).(map[string]any)
	assert.NotContains(t, plain["response"].(map[string]any)["response"], "updatedPermissions")
}

func TestTransport_EncodesLiveControlRequests(t *testing.T) {
	tr := NewTransport()
	req := func(r agentboot.ControlRequest) any {
		return tr.EncodeControlRequest("c1", r).(map[string]any)["request"]
	}
	assert.Equal(t, map[string]any{"subtype": "set_model", "model": "m"}, req(agentboot.SetModelRequest{Model: "m"}))
	assert.Equal(t, map[string]any{"subtype": "set_model", "model": "default"}, req(agentboot.SetModelRequest{}))
	assert.Equal(t, map[string]any{"subtype": "set_permission_mode", "mode": "plan"}, req(agentboot.SetPermissionModeRequest{Mode: "plan"}))
}

func TestTransport_DecodesControlResponses(t *testing.T) {
	tr := NewTransport()
	ok := protocol.Event{Type: "control_response", Data: map[string]any{"response": map[string]any{"subtype": "success", "request_id": "c1"}}}
	id, err, isResp := tr.DecodeControlResponse(ok)
	assert.True(t, isResp)
	assert.Equal(t, "c1", id)
	assert.NoError(t, err)

	bad := protocol.Event{Type: "control_response", Data: map[string]any{"response": map[string]any{"subtype": "error", "request_id": "c2", "error": "nope"}}}
	id, err, isResp = tr.DecodeControlResponse(bad)
	assert.True(t, isResp)
	assert.Equal(t, "c2", id)
	var rejected *agentboot.ControlRejectedError
	assert.ErrorAs(t, err, &rejected)

	_, _, isResp = tr.DecodeControlResponse(protocol.Event{Type: "assistant"})
	assert.False(t, isResp)
}
