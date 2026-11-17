package vmodel_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/anthropic"
	"github.com/tingly-dev/tingly-box/vmodel/openai"
)

func TestMockSpecFormatsAndModels(t *testing.T) {
	yaml := []byte("id: file-model\ncontent: hello\ndelay: 5ms\nusage:\n  prompt_tokens: 17\ntool_call:\n  name: search\n  arguments:\n    query: test\n")
	js := []byte(`{"id":"file-model","content":"hello","delay":"5ms","usage":{"prompt_tokens":17},"tool_call":{"name":"search","arguments":{"query":"test"}}}`)
	a, err := vmodel.DecodeMockSpec(yaml)
	if err != nil {
		t.Fatal(err)
	}
	b, err := vmodel.DecodeMockSpec(js)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || a.Delay != 5*time.Millisecond || a.Name != a.ID {
		t.Fatalf("inconsistent specs: %+v / %+v", a, b)
	}
	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := vmodel.DecodeMockSpec(encoded)
	if err != nil || !reflect.DeepEqual(a, roundTrip) {
		t.Fatalf("JSON round trip: %+v, %v", roundTrip, err)
	}
	o := openai.NewMockModel(a)
	ar := anthropic.NewMockModel(b)
	or, err := o.HandleOpenAIChat(&protocol.OpenAIChatCompletionRequest{})
	if err != nil || len(or.ToolCalls) != 1 || or.ToolCalls[0].Name != "search" {
		t.Fatalf("OpenAI response: %+v, %v", or, err)
	}
	anr, err := ar.HandleAnthropic(&protocol.AnthropicBetaMessagesRequest{})
	if err != nil || len(anr.Content) != 2 || anr.Content[1].OfToolUse.Name != "search" {
		t.Fatalf("Anthropic response: %+v, %v", anr, err)
	}
}

func TestMockSpecRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		"", "null", "[]", "content: hello", "id: x\nunknown: true", "id: x\nid: y",
		"id: x\n---\nid: y", `{"id":"x","id":"y"}`, `{"id":"x"} {"id":"y"}`,
		"id: x\ndelay: -1ms", "id: x\ndelay: fast", "id: x\ndelay: null",
		"id: x\ntool_call:\n  arguments: {}", "id: x\nusage:\n  promt_tokens: 1",
		"id: x\nerror:\n  stage: 2", "id: x\nerror:\n  status: 200",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := vmodel.DecodeMockSpec([]byte(input)); err == nil {
				t.Fatalf("accepted %q", input)
			}
		})
	}
}
