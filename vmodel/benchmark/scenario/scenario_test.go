package scenario

import "testing"

func TestAllScenarios_Registered(t *testing.T) {
	names := make(map[string]bool)
	for _, s := range AllScenarios() {
		names[s.Name] = true
	}
	for _, name := range []string{
		"text", "tool_use", "tool_result", "thinking",
		"multi_turn", "streaming_text", "streaming_tool_use", "error",
	} {
		if !names[name] {
			t.Errorf("scenario %q must be registered", name)
		}
	}
}

func TestScenario_Text(t *testing.T) {
	s := TextScenario()
	if s.Name != "text" {
		t.Fatalf("name: got %q", s.Name)
	}
	if len(s.Tags) == 0 || len(s.Assertions) == 0 || s.MockResponses == nil {
		t.Fatal("text scenario missing tags/assertions/responses")
	}
}

func TestScenario_ToolUse_AllFormatsHaveNonStream(t *testing.T) {
	s := ToolUseScenario()
	for _, f := range []ResponseFormat{FormatOpenAIChat, FormatAnthropic, FormatGoogle} {
		if s.MockResponses[f].NonStream == nil {
			t.Errorf("tool_use scenario missing NonStream for %q", f)
		}
	}
}

func TestScenario_Error_NonOKStatus(t *testing.T) {
	s := ErrorScenario()
	if s.Name != "error" {
		t.Fatalf("name: got %q", s.Name)
	}
	status, _ := s.MockResponses[FormatOpenAIChat].NonStream()
	if status == 200 {
		t.Fatalf("error scenario should not return 200, got %d", status)
	}
}

func TestScenario_StreamingText_TerminatesWithDONE(t *testing.T) {
	s := StreamingTextScenario()
	events := s.MockResponses[FormatOpenAIChat].Stream()
	if len(events) == 0 {
		t.Fatal("expected stream events")
	}
	if last := events[len(events)-1]; last != "data: [DONE]" {
		t.Fatalf("stream should end with [DONE], got %q", last)
	}
}

func TestBuildErrorFromSpec_RateLimit(t *testing.T) {
	spec := GetErrorSpec("virtual-fail-429")
	b := BuildErrorFromSpec(FormatOpenAIChat, spec)
	if b.NonStream == nil {
		t.Fatal("expected a NonStream builder for the 429 spec")
	}
	status, body := b.NonStream()
	if status != 429 {
		t.Fatalf("status: got %d, want 429", status)
	}
	if len(body) == 0 {
		t.Fatal("expected an error body")
	}
}
