package ops

import (
	"github.com/openai/openai-go/v3/responses"
)

// ApplyResponsesProviderTransforms applies provider-specific transformations
// to an OpenAI Responses request — the Responses-shape sibling of
// ApplyProviderTransforms.
//
// Today that is only the prompt-cache default-deny: the same OpenAI-only
// fields stripOpenAIPromptCacheFields removes from Chat requests exist on the
// Responses shape, and a strict-schema Responses-compatible vendor rejects
// them just the same (#1561, following #1548). Codex strips them again at its
// own boundary (sanitizeCodexPromptCacheJSON), so this is a no-op there.
func ApplyResponsesProviderTransforms(req *responses.ResponseNewParams, providerURL string) *responses.ResponseNewParams {
	host, _ := SplitProviderHostPath(providerURL)
	if !supportsExplicitPromptCache(host) {
		stripResponsesPromptCacheFields(req)
	}
	return req
}

// stripResponsesPromptCacheFields removes the OpenAI-only prompt-cache fields
// from a Responses request: top-level prompt_cache_options and
// prompt_cache_retention, and prompt_cache_breakpoint on every input_text /
// input_image / input_file part — in message content and in function and
// custom tool call outputs. prompt_cache_key is kept, for the reason given on
// stripOpenAIPromptCacheFields.
//
// Every field carries omitzero, so zeroing it omits the key without a JSON
// round-trip that would drop extra fields.
func stripResponsesPromptCacheFields(req *responses.ResponseNewParams) {
	req.PromptCacheOptions = responses.ResponseNewParamsPromptCacheOptions{}
	req.PromptCacheRetention = ""

	for i := range req.Input.OfInputItemList {
		item := &req.Input.OfInputItemList[i]
		switch {
		case item.OfMessage != nil:
			stripResponsesContentBreakpoints(item.OfMessage.Content.OfInputItemContentList)
		case item.OfInputMessage != nil:
			stripResponsesContentBreakpoints(item.OfInputMessage.Content)
		case item.OfFunctionCallOutput != nil:
			for j := range item.OfFunctionCallOutput.Output.OfResponseFunctionCallOutputItemArray {
				part := &item.OfFunctionCallOutput.Output.OfResponseFunctionCallOutputItemArray[j]
				switch {
				case part.OfInputText != nil:
					part.OfInputText.PromptCacheBreakpoint = responses.ResponseInputTextContentPromptCacheBreakpointParam{}
				case part.OfInputImage != nil:
					part.OfInputImage.PromptCacheBreakpoint = responses.ResponseInputImageContentPromptCacheBreakpointParam{}
				case part.OfInputFile != nil:
					part.OfInputFile.PromptCacheBreakpoint = responses.ResponseInputFileContentPromptCacheBreakpointParam{}
				}
			}
		case item.OfCustomToolCallOutput != nil:
			for j := range item.OfCustomToolCallOutput.Output.OfOutputContentList {
				part := &item.OfCustomToolCallOutput.Output.OfOutputContentList[j]
				switch {
				case part.OfInputText != nil:
					part.OfInputText.PromptCacheBreakpoint = responses.ResponseInputTextPromptCacheBreakpointParam{}
				case part.OfInputImage != nil:
					part.OfInputImage.PromptCacheBreakpoint = responses.ResponseInputImagePromptCacheBreakpointParam{}
				case part.OfInputFile != nil:
					part.OfInputFile.PromptCacheBreakpoint = responses.ResponseInputFilePromptCacheBreakpointParam{}
				}
			}
		}
	}
}

func stripResponsesContentBreakpoints(parts responses.ResponseInputMessageContentListParam) {
	for i := range parts {
		part := &parts[i]
		switch {
		case part.OfInputText != nil:
			part.OfInputText.PromptCacheBreakpoint = responses.ResponseInputTextPromptCacheBreakpointParam{}
		case part.OfInputImage != nil:
			part.OfInputImage.PromptCacheBreakpoint = responses.ResponseInputImagePromptCacheBreakpointParam{}
		case part.OfInputFile != nil:
			part.OfInputFile.PromptCacheBreakpoint = responses.ResponseInputFilePromptCacheBreakpointParam{}
		}
	}
}
