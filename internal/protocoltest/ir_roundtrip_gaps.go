package protocoltest

import "strings"

// IR round-trip losses found by H3 (ir_roundtrip_test.go), by category. Each
// is something Beta, the Stage's IR, does not yet carry between an OpenAI
// source and an OpenAI target; OpenAI-source cutovers wait for these to be
// cleared (by carrying the fact across the IR) or explicitly accepted.
var irGapCategories = map[string]string{
	"I1": "IR sets an output-token limit (max_tokens / max_output_tokens 4096) the client did not ask for, and replaces the client's max_completion_tokens",
	"I2": "sampling and request metadata are dropped (temperature, top_p, stop, seed, presence/frequency penalty, user)",
	"I3": "structured output (response_format / text.format json_schema) is dropped",
	"I4": "reasoning effort (reasoning_effort / reasoning.effort) is dropped",
	"I5": "developer messages are dropped",
	"I6": "input image detail is dropped",
	"R1": "response, output item and function call IDs are minted anew instead of carried from the provider",
	"R2": "the Responses object type is normalized to \"response\" instead of carried from the provider",
	"R3": "a streamed length stop (finish_reason length) arrives as stop",
	"R4": "when the provider streams no usage, the IR's estimated usage differs from the direct path's",
}

var irGaps = map[string][]string{
	"TestIRRoundTripRequest/openai_chat->openai_chat/basic":                  {"I1"},
	"TestIRRoundTripRequest/openai_chat->openai_chat/developer":              {"I1", "I5"},
	"TestIRRoundTripRequest/openai_chat->openai_chat/image":                  {"I1"},
	"TestIRRoundTripRequest/openai_chat->openai_chat/reasoning":              {"I1", "I4"},
	"TestIRRoundTripRequest/openai_chat->openai_chat/response_format":        {"I1", "I3"},
	"TestIRRoundTripRequest/openai_chat->openai_chat/sampling":               {"I1", "I2"},
	"TestIRRoundTripRequest/openai_chat->openai_chat/tool_history":           {"I1"},
	"TestIRRoundTripRequest/openai_chat->openai_chat/tools":                  {"I1"},
	"TestIRRoundTripRequest/openai_chat->openai_responses/reasoning":         {"I4"},
	"TestIRRoundTripRequest/openai_chat->openai_responses/sampling":          {"I2"},
	"TestIRRoundTripRequest/openai_responses->openai_chat/sampling":          {"I2"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/basic":        {"I1"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/image":        {"I1", "I6"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/reasoning":    {"I1", "I4"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/sampling":     {"I2"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/string_input": {"I1"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/text_format":  {"I1", "I3"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/tool_history": {"I1"},
	"TestIRRoundTripRequest/openai_responses->openai_responses/tools":        {"I1"},

	"TestIRRoundTripResponse/openai_chat->openai_chat/incomplete/nonstream":                {"R1"},
	"TestIRRoundTripResponse/openai_chat->openai_chat/incomplete/stream":                   {"R3"},
	"TestIRRoundTripResponse/openai_chat->openai_chat/multi_turn/nonstream":                {"R1"},
	"TestIRRoundTripResponse/openai_chat->openai_chat/text/nonstream":                      {"R1"},
	"TestIRRoundTripResponse/openai_chat->openai_chat/thinking/nonstream":                  {"R1"},
	"TestIRRoundTripResponse/openai_chat->openai_chat/tool_result/nonstream":               {"R1"},
	"TestIRRoundTripResponse/openai_chat->openai_chat/tool_use/nonstream":                  {"R1"},
	"TestIRRoundTripResponse/openai_chat->openai_responses/incomplete/stream":              {"R3"},
	"TestIRRoundTripResponse/openai_responses->openai_chat/multi_turn/stream":              {"R4"},
	"TestIRRoundTripResponse/openai_responses->openai_chat/streaming_text/stream":          {"R4"},
	"TestIRRoundTripResponse/openai_responses->openai_chat/streaming_tool_use/stream":      {"R4"},
	"TestIRRoundTripResponse/openai_responses->openai_chat/text/stream":                    {"R4"},
	"TestIRRoundTripResponse/openai_responses->openai_chat/thinking/stream":                {"R4"},
	"TestIRRoundTripResponse/openai_responses->openai_chat/tool_result/stream":             {"R4"},
	"TestIRRoundTripResponse/openai_responses->openai_chat/tool_use/stream":                {"R4"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/incomplete/nonstream":      {"R1", "R2"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/multi_turn/nonstream":      {"R1", "R2"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/streaming_tool_use/stream": {"R1"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/text/nonstream":            {"R1", "R2"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/thinking/nonstream":        {"R1", "R2"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/tool_result/nonstream":     {"R1", "R2"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/tool_use/nonstream":        {"R1", "R2"},
	"TestIRRoundTripResponse/openai_responses->openai_responses/tool_use/stream":           {"R1"},
}

var _ = func() bool {
	for name, ids := range irGaps {
		reasons := make([]string, len(ids))
		for i, id := range ids {
			reasons[i] = id + ": " + irGapCategories[id]
		}
		registerKnownGaps(KnownGap{ID: strings.Join(ids, "+"), Reason: strings.Join(reasons, "; ")}, name)
	}
	return true
}()
