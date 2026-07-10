package typ

import (
	"encoding/json"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// TestTacticUnmarshal_RemovedAdaptiveFallsBackToRandom pins the decode
// contract for configs persisted before the adaptive tactic was removed:
// the "adaptive" name (and its old params) must decode cleanly to Random
// instead of erroring or shifting to another tactic.
func TestTacticUnmarshal_RemovedAdaptiveFallsBackToRandom(t *testing.T) {
	raw := `{"type":"adaptive","params":{"latency_weight":0.5,"token_weight":0.2,"scoring_mode":"weighted_sum"}}`

	var tc Tactic
	if err := json.Unmarshal([]byte(raw), &tc); err != nil {
		t.Fatalf("legacy adaptive config must still decode: %v", err)
	}
	if tc.Type != loadbalance.TacticRandom {
		t.Fatalf("Type = %v, want TacticRandom", tc.Type)
	}
	if _, ok := tc.Params.(*RandomParams); !ok {
		t.Fatalf("Params = %T, want *RandomParams", tc.Params)
	}
	if got := tc.Instantiate().GetType(); got != loadbalance.TacticRandom {
		t.Fatalf("Instantiate().GetType() = %v, want TacticRandom", got)
	}
}

// TestTacticUnmarshal_RemovedAdaptiveIntSlot pins the integer backward-compat
// path: raw int 6 (the removed adaptive enum slot) must instantiate as the
// random fallback and serialize as "random".
func TestTacticUnmarshal_RemovedAdaptiveIntSlot(t *testing.T) {
	var tc Tactic
	if err := json.Unmarshal([]byte(`{"type":6}`), &tc); err != nil {
		t.Fatalf("legacy int tactic type must still decode: %v", err)
	}
	if got := tc.Instantiate().GetType(); got != loadbalance.TacticRandom {
		t.Fatalf("Instantiate().GetType() = %v, want TacticRandom", got)
	}
	if got := tc.Type.String(); got != "random" {
		t.Fatalf("Type.String() = %q, want %q", got, "random")
	}
}
