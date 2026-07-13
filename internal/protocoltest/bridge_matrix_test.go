package protocoltest

import "testing"

func TestDefaultBridgeMatrix(t *testing.T) {
	t.Parallel()

	results := DefaultBridgeMatrix().ExecuteAll()
	if len(results) != 24 {
		t.Fatalf("result count = %d, want 24", len(results))
	}
	for _, result := range results {
		if !result.Passed {
			t.Errorf("%s failed: %+v", result.Name, result.Errors)
		}
		if result.Skipped {
			t.Errorf("%s unexpectedly skipped: %s", result.Name, result.SkipReason)
		}
		if result.Response == nil {
			t.Errorf("%s has nil semantic response", result.Name)
		}
	}
}

func TestBridgeMatrixFiltersAndBatch(t *testing.T) {
	t.Parallel()

	results := DefaultBridgeMatrix().
		OnlyScenarios("tool_result").
		OnlySources("anthropic_beta").
		OnlyTargets("openai_chat").
		OnlyStreaming(true).
		WithBatchCount(3).
		ExecuteAll()
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	result := results[0]
	if !result.Passed || result.BatchCount != 3 || result.BatchPassed != 3 {
		t.Fatalf("batch result = %+v", result)
	}
	if result.Name != "bridges/tool_result/anthropic_beta/openai_chat/stream" {
		t.Fatalf("result name = %q", result.Name)
	}
}
