package protocoltest

import (
	"strings"
	"testing"
)

func TestDefaultBridgeMatrix(t *testing.T) {
	t.Parallel()

	results := DefaultBridgeMatrix().ExecuteAll()
	if len(results) != 42 {
		t.Fatalf("result count = %d, want 42", len(results))
	}
	chainResults := 0
	for _, result := range results {
		if strings.HasPrefix(result.Name, "bridges/chain/") {
			chainResults++
		}
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
	if chainResults != 6 {
		t.Fatalf("chain result count = %d, want 6", chainResults)
	}
}

func TestBridgeMatrixConcreteChainFiltersAndBatch(t *testing.T) {
	t.Parallel()

	results := DefaultBridgeMatrix().
		OnlyScenarios("tool_result").
		OnlySources("openai_chat").
		OnlyTargets("openai_chat").
		OnlyStreaming(true).
		WithBatchCount(3).
		ExecuteAll()
	if len(results) != 2 {
		t.Fatalf("result count = %d, want identity plus concrete chain", len(results))
	}
	for _, result := range results {
		if !result.Passed || result.BatchCount != 3 || result.BatchPassed != 3 {
			t.Fatalf("batch result = %+v", result)
		}
		if strings.HasPrefix(result.Name, "bridges/chain/") {
			if result.Name != "bridges/chain/chat_beta_stage_chat/tool_result/openai_chat/openai_chat/stream" {
				t.Fatalf("chain result name = %q", result.Name)
			}
			return
		}
	}
	t.Fatal("concrete chain result not found")
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
