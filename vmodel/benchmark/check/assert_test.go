package check

import "testing"

func TestAssertContentContains(t *testing.T) {
	r := &RoundTripResult{Content: "The weather in Paris is sunny."}
	if err := AssertContentContains("Paris").Check(r); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	if err := AssertContentContains("Tokyo").Check(r); err == nil {
		t.Fatal("expected failure for missing substring")
	}
}

func TestAssertHasToolCallsAndArgs(t *testing.T) {
	r := &RoundTripResult{
		ToolCalls: []ToolCallResult{
			{ID: "call_1", Name: "get_weather", Arguments: `{"location":"Paris","unit":"celsius"}`},
		},
	}
	if err := AssertHasToolCalls(1).Check(r); err != nil {
		t.Fatalf("has_tool_calls: %v", err)
	}
	if err := AssertToolCallName(0, "get_weather").Check(r); err != nil {
		t.Fatalf("tool_call_name: %v", err)
	}
	if err := AssertToolCallArgs(0, "location", "Paris").Check(r); err != nil {
		t.Fatalf("tool_call_args: %v", err)
	}
	if err := AssertToolCallArgs(0, "location", "Tokyo").Check(r); err == nil {
		t.Fatal("expected failure for wrong arg value")
	}
	// Out-of-range index must error, not panic.
	if err := AssertToolCallName(1, "get_weather").Check(r); err == nil {
		t.Fatal("expected out-of-range failure")
	}
}

func TestAssertUsageNonZero(t *testing.T) {
	if err := AssertUsageNonZero().Check(&RoundTripResult{Usage: &TokenUsage{InputTokens: 10}}); err != nil {
		t.Fatalf("expected pass: %v", err)
	}
	if err := AssertUsageNonZero().Check(&RoundTripResult{Usage: &TokenUsage{}}); err == nil {
		t.Fatal("expected failure for zero usage")
	}
	if err := AssertUsageNonZero().Check(&RoundTripResult{Usage: nil}); err == nil {
		t.Fatal("expected failure for nil usage")
	}
	// Streaming skips the check.
	if err := AssertUsageNonZero().Check(&RoundTripResult{IsStreaming: true}); err != nil {
		t.Fatalf("streaming should skip usage check: %v", err)
	}
}

func TestAssertHTTPStatusVariants(t *testing.T) {
	r := &RoundTripResult{HTTPStatus: 429}
	if err := AssertHTTPStatus(429).Check(r); err != nil {
		t.Fatalf("http_status: %v", err)
	}
	if err := AssertHTTPStatusAtLeast(400).Check(r); err != nil {
		t.Fatalf("http_status_at_least: %v", err)
	}
	if err := AssertHTTPStatus(200).Check(r); err == nil {
		t.Fatal("expected mismatch failure")
	}
}

func TestAssertFinishReasonOneOf(t *testing.T) {
	r := &RoundTripResult{FinishReason: "max_tokens"}
	if err := AssertFinishReasonOneOf("length", "max_tokens", "incomplete").Check(r); err != nil {
		t.Fatalf("finish_reason_one_of: %v", err)
	}
	if err := AssertFinishReasonOneOf("stop").Check(r); err == nil {
		t.Fatal("expected failure for unaccepted finish reason")
	}
}
