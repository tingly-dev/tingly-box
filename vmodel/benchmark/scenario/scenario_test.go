package scenario

import (
	"encoding/json"
	"testing"
)

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

// TestDecisionResponse_RandomButRight pins the decision mock's contract: the
// answer varies across calls (it is not a fixed echo a passthrough bug could
// satisfy by accident), yet every body is internally consistent — the chosen
// option is the argmax of its own probability map and usage is non-zero.
func TestDecisionResponse_RandomButRight(t *testing.T) {
	builder := decisionResponse()
	answers := make(map[string]bool)
	for i := 0; i < 40; i++ {
		status, body := builder.NonStream()
		if status != 200 {
			t.Fatalf("call %d: status = %d", i, status)
		}
		var parsed struct {
			Answers struct {
				QRoute struct {
					Answer string `json:"answer"`
				} `json:"q_route"`
			} `json:"answers"`
			Probabilities map[string]map[string]float64 `json:"probabilities"`
			Usage         struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("call %d: body is not JSON: %v", i, err)
		}
		answer := parsed.Answers.QRoute.Answer
		probs := parsed.Probabilities["q_route"]
		if _, ok := probs[answer]; !ok {
			t.Fatalf("call %d: answer %q missing from probabilities %v", i, answer, probs)
		}
		best, bestP := "", -1.0
		for opt, p := range probs {
			if p > bestP {
				best, bestP = opt, p
			}
		}
		if answer != best {
			t.Fatalf("call %d: answer %q is not the argmax (%q=%.2f)", i, answer, best, bestP)
		}
		if parsed.Usage.InputTokens <= 0 || parsed.Usage.OutputTokens <= 0 {
			t.Fatalf("call %d: usage is zero: %+v", i, parsed.Usage)
		}
		answers[answer] = true
	}
	if len(answers) < 2 {
		t.Errorf("expected randomized answers across 40 calls, got only %v", answers)
	}
}
