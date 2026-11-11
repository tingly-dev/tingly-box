package mutate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"github.com/tingly-dev/tingly-box/internal/protocol/ops"
)

func TestMaskAnthropicV1SystemRoleCredentials(t *testing.T) {
	const secret = "test-secret-not-a-real-credential"
	const alias = "TINGLY_CRED_test"
	var req anthropic.MessageNewParams
	if err := json.Unmarshal([]byte(`{"model":"test","max_tokens":32,"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"ready"},{"role":"system","content":[{"type":"text","text":"first"},{"type":"text","text":"test-secret-not-a-real-credential"}]}]}`), &req); err != nil {
		t.Fatal(err)
	}
	changed, tailChanged := MaskAnthropicV1RequestCredentials(&req, []guardrailscore.ProtectedCredential{{
		ID: "test", Secret: secret, AliasToken: alias,
	}}, guardrailscore.NewCredentialMaskState())
	if !changed || !tailChanged {
		t.Fatalf("mask result = (%v, %v), want (true, true)", changed, tailChanged)
	}
	if got := req.Messages[2].OfSystem.Content[1].Text; got != alias {
		t.Fatalf("system text = %q, want alias %q", got, alias)
	}
	if got := req.Messages[2].OfSystem.Content[0].Text; got != "first" {
		t.Fatalf("unmatched system text changed: %q", got)
	}
	// Production masks credentials before the compatibility role rewrite.
	ops.ApplyClaudeCodeCompatRoleRewrite(&req)
	wire, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), secret) || !strings.Contains(string(wire), alias) {
		t.Fatalf("rewritten request did not retain credential masking: %s", wire)
	}
}
