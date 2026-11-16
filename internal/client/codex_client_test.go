package client

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Codex's ResponseItem::Reasoning has an optional id. The Go SDK models it as
// required and serializes a missing id as ""; that must not cost us the opaque
// history needed by stateless replay and remote compaction.
func TestCodexDefaultsPreserveEncryptedReasoning(t *testing.T) {
	for _, id := range []string{"", `,"id":""`, `,"id":"rs/bad"`, `,"id":null`, `,"id":"rs_valid"`} {
		t.Run(id, func(t *testing.T) {
			body := `{"model":"gpt-5-codex","input":[{"type":"reasoning","summary":[],"encrypted_content":"opaque-reasoning","provider_metadata":{"keep":true}` + id + `},{"type":"compaction","encrypted_content":"opaque-compaction"},{"type":"compaction_trigger"}]}`
			var req responses.ResponseNewParams
			require.NoError(t, json.Unmarshal([]byte(body), &req))
			// Preserve existing SDK extras when adding the required-ID escape hatch.
			req.Input.OfInputItemList[0].OfReasoning.SetExtraFields(map[string]any{"provider_metadata": map[string]any{"keep": true}})
			for range 2 { // Defaults also need to be safe when applied again.
				applyCodexDefaultsToParams(&req)
				raw, err := json.Marshal(req)
				require.NoError(t, err)
				wire, err := (&codexRoundTripper{}).filterField(raw)
				require.NoError(t, err)
				input := gjson.GetBytes(wire, "input").Array()
				require.Len(t, input, 3)
				require.Equal(t, "reasoning", input[0].Get("type").String())
				require.Equal(t, "opaque-reasoning", input[0].Get("encrypted_content").String())
				require.True(t, input[0].Get("provider_metadata.keep").Bool())
				if id == `,"id":"rs_valid"` {
					require.Equal(t, "rs_valid", input[0].Get("id").String())
				} else {
					require.False(t, input[0].Get("id").Exists())
				}
				require.Equal(t, "opaque-compaction", input[1].Get("encrypted_content").String())
				require.Equal(t, "compaction_trigger", input[2].Get("type").String())
			}
		})
	}
}

func TestApplyCodexDefaultsToParams_FastSuffix(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol:fast"}

	applyCodexDefaultsToParams(&req)

	if req.Model != "gpt-5.6-sol" {
		t.Fatalf("expected model to be stripped to %q, got %q", "gpt-5.6-sol", req.Model)
	}
	if req.ServiceTier != responses.ResponseNewParamsServiceTierPriority {
		t.Fatalf("expected service tier %q, got %q", responses.ResponseNewParamsServiceTierPriority, req.ServiceTier)
	}
}

func TestApplyCodexDefaultsToParams_NoFastSuffix(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol"}

	applyCodexDefaultsToParams(&req)

	if req.Model != "gpt-5.6-sol" {
		t.Fatalf("expected model to remain %q, got %q", "gpt-5.6-sol", req.Model)
	}
	if req.ServiceTier != "" {
		t.Fatalf("expected empty service tier, got %q", req.ServiceTier)
	}
}

// TestApplyCodexDefaultsToParams_ServiceTierSurvivesWireBody guards against the
// request body being silently stripped of "service_tier" by the second body-shaping
// pass in codexRoundTripper.filterField (a deny-list JSON filter applied to the
// already-marshaled body, separate from the SDK struct marshaling).
func TestApplyCodexDefaultsToParams_ServiceTierSurvivesWireBody(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol:fast"}
	applyCodexDefaultsToParams(&req)

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	rt := &codexRoundTripper{}
	filtered, err := rt.filterField(raw)
	if err != nil {
		t.Fatalf("filterField failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(filtered, &out); err != nil {
		t.Fatalf("unmarshal filtered body: %v", err)
	}

	if out["service_tier"] != "priority" {
		t.Fatalf("expected service_tier=priority in final wire body, got %v (full body: %s)", out["service_tier"], filtered)
	}
	if out["model"] != "gpt-5.6-sol" {
		t.Fatalf("expected model=gpt-5.6-sol in final wire body, got %v", out["model"])
	}
}
