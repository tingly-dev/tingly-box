package server

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (ah *AIHandler) StreamOpenAIChatToAnthropicV1WithMCP(
	c *gin.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	actualModel string,
	responseModel string,
	recorder *ProtocolRecorder,
) {
	for round := 0; round < 3; round++ {
		wrapper := ah.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
		fc := forwarding.NewForwardContext(c.Request.Context(), provider)
		streamResp, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			stream.SendStreamingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		var hooks *stream.OpenAIToAnthropicMCPHooks
		if HasDeclaredMCPTools(req) && ah.mcpEnabled() {
			hooks = ah.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		hc := protocol.NewHandleContext(c, responseModel)
		usage, err := stream.HandleOpenAIToAnthropicStreamResponseWithMCPHooks(hc, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			ah.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}
		ah.trackUsageWithTokenUsage(c, usage, nil)
		return
	}
	stream.SendInternalError(c, "MCP stream continuation exceeded max rounds")
	if recorder != nil {
		recorder.RecordError(stream.ErrMCPStreamContinue)
	}
}

func (ah *AIHandler) StreamOpenAIChatToAnthropicBetaWithMCP(
	c *gin.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	actualModel string,
	responseModel string,
	recorder *ProtocolRecorder,
) {
	streamRec := newStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	for round := 0; round < 3; round++ {
		wrapper := ah.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
		fc := forwarding.NewForwardContext(c.Request.Context(), provider)
		streamResp, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			stream.SendStreamingError(c, err)
			if streamRec != nil {
				streamRec.RecordError(err)
			}
			return
		}

		var hooks *stream.OpenAIToAnthropicMCPHooks
		if HasDeclaredMCPTools(req) && ah.mcpEnabled() {
			hooks = ah.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		hc := protocol.NewHandleContext(c, responseModel)
		usage, err := stream.HandleOpenAIToAnthropicBetaStreamWithMCPHooks(hc, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			ah.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if streamRec != nil {
				streamRec.RecordError(err)
			}
			return
		}
		ah.trackUsageWithTokenUsage(c, usage, nil)
		if streamRec != nil {
			streamRec.Finish(responseModel, usage)
			streamRec.RecordResponse(provider, actualModel)
		}
		return
	}
	stream.SendInternalError(c, "MCP stream continuation exceeded max rounds")
	if streamRec != nil {
		streamRec.RecordError(stream.ErrMCPStreamContinue)
	}
}

func hasOnlyMCPToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) bool {
	if len(toolCalls) == 0 {
		return false
	}
	for _, tc := range toolCalls {
		if !mcpruntime.IsMCPToolName(tc.Function.Name) {
			return false
		}
	}
	return true
}

// HasDeclaredMCPTools reports whether req declares any MCP-named tool in its
// OpenAI Chat tool list.
func HasDeclaredMCPTools(req *openai.ChatCompletionNewParams) bool {
	if req == nil || len(req.Tools) == 0 {
		return false
	}
	for _, t := range req.Tools {
		fn := t.GetFunction()
		if fn == nil {
			continue
		}
		if mcpruntime.IsMCPToolName(fn.Name) {
			return true
		}
	}
	return false
}
