package request

import (
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
)

// missingToolOutputPlaceholder is the output synthesized for a function_call
// that has no function_call_output anywhere in the input.
//
// Codex (and other Responses clients) replay history where a call never
// produced an output — e.g. the user interrupted the turn while the tool was
// running. Chat Completions providers (DeepSeek, OpenAI) and Anthropic reject
// an assistant tool call that is not answered, so the gateway fills the gap
// rather than forward a broken sequence.
const missingToolOutputPlaceholder = "[tool call aborted: no output was recorded for this call]"

// RepairResponsesToolCalls rewrites a Responses API input item list so that
// the tool-call invariants required by every downstream protocol hold:
//
//  1. every function_call is immediately followed (after its sibling calls
//     from the same assistant turn) by exactly one function_call_output;
//  2. no function_call_output stands without a preceding function_call.
//
// The Responses API itself only correlates calls and outputs by call_id: it
// has no adjacency rule, does not require every call to be answered, and
// OpenAI even accepts a standalone function_call_output. Codex relies on all
// three (interrupted turns, automation/heartbeat/cross-thread injections).
// Chat Completions ("An assistant message with 'tool_calls' must be followed
// by tool messages responding to each 'tool_call_id'", "Messages with role
// 'tool' must be a response to a preceding message with 'tool_calls'") and
// Anthropic ("tool_use ids were found without tool_result blocks immediately
// after", "unexpected tool_use_id") enforce the stricter shape, so the
// Responses→Chat and Responses→Anthropic converters both run this first.
//
// Repairs applied, in order of the input:
//   - outputs are moved to sit right after the call group that requested them;
//   - a call with no output gets a synthesized output carrying
//     missingToolOutputPlaceholder;
//   - an output with no call (missing call_id, call trimmed from history, or
//     a duplicate answer) is rewritten as a user message that quotes the
//     output, so its content survives without a dangling tool message;
//   - a function_call whose call_id repeats an earlier call_id (corrupted or
//     replayed history — call_id must be unique) is dropped: every
//     downstream protocol rejects or mispairs two tool_calls/tool_use
//     entries sharing one id, so re-emitting the repeat would only relocate
//     the collision instead of repairing it.
//
// Every other item passes through unchanged, in its original relative order.
// See .design/protocol-responses.md for the live provider probes.
func RepairResponsesToolCalls(items responses.ResponseInputParam) responses.ResponseInputParam {
	if len(items) == 0 {
		return items
	}

	// Index the first output for every call_id, and the set of call_ids that
	// have a function_call anywhere in the input.
	outputByCall := make(map[string]*responses.ResponseInputItemFunctionCallOutputParam)
	callIDs := make(map[string]bool)
	for _, item := range items {
		switch {
		case !param.IsOmitted(item.OfFunctionCall):
			callIDs[item.OfFunctionCall.CallID] = true
		case !param.IsOmitted(item.OfFunctionCallOutput):
			id := item.OfFunctionCallOutput.CallID.Value
			if id == "" {
				continue
			}
			if _, dup := outputByCall[id]; !dup {
				outputByCall[id] = item.OfFunctionCallOutput
			}
		}
	}

	out := make(responses.ResponseInputParam, 0, len(items))
	emitted := make(map[*responses.ResponseInputItemFunctionCallOutputParam]bool)
	callIDSeen := make(map[string]bool, len(callIDs))
	var pendingCalls []responses.ResponseInputItemUnionParam

	// flush emits the accumulated call group followed by one output per call.
	flush := func() {
		if len(pendingCalls) == 0 {
			return
		}
		out = append(out, pendingCalls...)
		for _, call := range pendingCalls {
			id := call.OfFunctionCall.CallID
			if output, ok := outputByCall[id]; ok && !emitted[output] {
				out = append(out, responses.ResponseInputItemUnionParam{OfFunctionCallOutput: output})
				emitted[output] = true
				continue
			}
			logrus.Debugf("RepairResponsesToolCalls: function_call %q (%s) has no function_call_output; synthesizing placeholder", id, call.OfFunctionCall.Name)
			out = append(out, responses.ResponseInputItemUnionParam{
				OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
					CallID: param.NewOpt(id),
					Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
						OfString: param.NewOpt(missingToolOutputPlaceholder),
					},
				},
			})
		}
		pendingCalls = nil
	}

	for _, item := range items {
		if !param.IsOmitted(item.OfFunctionCall) {
			id := item.OfFunctionCall.CallID
			// A call_id must be unique: every downstream protocol rejects (or
			// silently mispairs) two tool_calls/tool_use entries sharing one
			// id in the same turn. A repeat is corrupted/replayed history
			// (the class of input this function targets), not a legitimate
			// second call, so it is dropped rather than re-emitted — keeping
			// it would just relocate the collision instead of repairing it.
			if id != "" && callIDSeen[id] {
				logrus.Debugf("RepairResponsesToolCalls: duplicate function_call call_id %q; dropping repeat", id)
				continue
			}
			callIDSeen[id] = true
			pendingCalls = append(pendingCalls, item)
			continue
		}

		// Any other item ends the assistant's tool-call turn.
		flush()

		if param.IsOmitted(item.OfFunctionCallOutput) {
			out = append(out, item)
			continue
		}

		output := item.OfFunctionCallOutput
		if emitted[output] {
			continue // already placed right after its call
		}
		id := output.CallID.Value
		if id != "" && callIDs[id] && outputByCall[id] == output {
			continue // its call comes later in the input; emitted at that flush
		}
		logrus.Debugf("RepairResponsesToolCalls: function_call_output call_id=%q name=%q has no matching function_call; rewriting as user message", id, output.Name.Value)
		out = append(out, orphanOutputToUserMessage(output))
	}
	flush()

	return out
}

// orphanOutputToUserMessage rewrites a function_call_output that cannot be
// paired with a function_call as a plain user message quoting the output.
// Plain user text carries no cross-reference constraint in any protocol,
// which is why this is preferred over synthesizing a fake tool call (whose
// tool may not even be declared).
func orphanOutputToUserMessage(output *responses.ResponseInputItemFunctionCallOutputParam) responses.ResponseInputItemUnionParam {
	label := output.Name.Value
	if label == "" {
		label = output.CallID.Value
	}
	if label == "" {
		label = "unknown"
	}
	header := fmt.Sprintf("[tool output: %s]", label)

	var texts []string
	var content responses.ResponseInputMessageContentListParam
	cacheBreakpoint := false
	if !param.IsOmitted(output.Output.OfString) {
		texts = append(texts, output.Output.OfString.Value)
	} else {
		for _, part := range output.Output.OfResponseFunctionCallOutputItemArray {
			switch {
			case part.OfInputText != nil:
				texts = append(texts, part.OfInputText.Text)
				cacheBreakpoint = cacheBreakpoint || !param.IsOmitted(part.OfInputText.PromptCacheBreakpoint)
			case part.OfInputImage != nil && part.OfInputImage.ImageURL.Valid():
				content = append(content, responses.ResponseInputContentUnionParam{
					OfInputImage: &responses.ResponseInputImageParam{
						Detail:   responses.ResponseInputImageDetailAuto,
						ImageURL: part.OfInputImage.ImageURL,
					},
				})
			}
		}
	}

	text := header
	if joined := strings.Join(texts, ""); joined != "" {
		text += "\n" + joined
	}
	textPart := &responses.ResponseInputTextParam{Text: text}
	if cacheBreakpoint {
		textPart.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
	}
	content = append(responses.ResponseInputMessageContentListParam{{OfInputText: textPart}}, content...)

	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Type:    responses.EasyInputMessageTypeMessage,
			Role:    responses.EasyInputMessageRoleUser,
			Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: content},
		},
	}
}
