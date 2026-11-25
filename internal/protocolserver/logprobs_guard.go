package protocolserver

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Logprobs are an OpenAI-family parameter. Anthropic Messages and Google
// generateContent have no place for them, and the converters cannot invent an
// answer, so a request that asks for logprobs on one of those providers is
// rejected up front with an explicit 400. Silently forwarding without the
// parameter (the old behaviour) made a client believe it had probabilities
// while the response's "logprobs": null was the only hint it had not.

// logprobsRequestedChat reports whether a Chat request asks for token
// logprobs. Chat needs the boolean logprobs=true; top_logprobs is only valid
// alongside it, but a stray top_logprobs still means the client wanted them.
func logprobsRequestedChat(req *openai.ChatCompletionNewParams) bool {
	if req == nil {
		return false
	}
	return req.Logprobs.Value || !param.IsOmitted(req.TopLogprobs)
}

// logprobsRequestedResponses reports whether a Responses request asks for
// token logprobs: Responses expresses the request through include, with the
// per-position count on top_logprobs.
func logprobsRequestedResponses(req *responses.ResponseNewParams) bool {
	if req == nil {
		return false
	}
	return !param.IsOmitted(req.TopLogprobs) ||
		slices.Contains(req.Include, responses.ResponseIncludableMessageOutputTextLogprobs)
}

// targetCarriesLogprobs reports whether the provider protocol can express
// logprobs at all. Chat carries them natively; Responses carries them through
// include=message.output_text.logprobs. Anthropic and Google cannot.
func targetCarriesLogprobs(target protocol.APIType) bool {
	return target == protocol.TypeOpenAIChat || target == protocol.TypeOpenAIResponses
}

// rejectUnsupportedLogprobs writes an explicit 400 when a client asked for
// logprobs but the resolved provider protocol cannot carry them. It returns
// true when it handled (rejected) the request.
func rejectUnsupportedLogprobs(c *gin.Context, target protocol.APIType, enabled bool) bool {
	if !enabled || targetCarriesLogprobs(target) {
		return false
	}
	writeErrorResponse(c, http.StatusBadRequest, ErrorResponse{
		Error: ErrorDetail{
			Message: fmt.Sprintf(
				"logprobs/top_logprobs is not supported by this provider: %s has no logprobs field, and the gateway will not forward the request without it. Remove logprobs and top_logprobs, or route to an OpenAI Chat/Responses provider.",
				target,
			),
			Type: "invalid_request_error",
			Code: "logprobs_unsupported",
		},
	})
	return true
}
