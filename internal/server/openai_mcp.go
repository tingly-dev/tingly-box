package server

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/server/recording"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (ph *ProtocolHandler) StreamOpenAIChatToAnthropicV1WithMCP(
	c *gin.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	actualModel string,
	responseModel string,
	recorder *recording.ProtocolRecorder,
) {
	for round := 0; round < 3; round++ {
		wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
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
		if HasDeclaredMCPTools(req) && ph.mcpEnabled() {
			hooks = ph.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		hc := protocol.NewHandleContext(c, responseModel)
		usage, err := stream.HandleOpenAIToAnthropicStreamResponseWithMCPHooks(hc, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			ph.trackUsageWithTokenUsage(c, usage, err)
			if !c.Writer.Written() {
				stream.SendStreamingError(c, err)
			}
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}
		ph.trackUsageWithTokenUsage(c, usage, nil)
		return
	}
	stream.SendInternalError(c, "MCP stream continuation exceeded max rounds")
	if recorder != nil {
		recorder.RecordError(stream.ErrMCPStreamContinue)
	}
}

func (ph *ProtocolHandler) StreamOpenAIChatToAnthropicBetaWithMCP(
	c *gin.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	actualModel string,
	responseModel string,
	recorder *recording.ProtocolRecorder,
) {
	streamRec := recording.NewStreamRecorder(recorder)
	if streamRec != nil {
		streamRec.SetupStreamRecorderInContext(c)
	}

	for round := 0; round < 3; round++ {
		wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
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
		if HasDeclaredMCPTools(req) && ph.mcpEnabled() {
			hooks = ph.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		hc := protocol.NewHandleContext(c, responseModel)
		usage, err := stream.HandleOpenAIToAnthropicBetaStreamWithMCPHooks(hc, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			ph.trackUsageWithTokenUsage(c, usage, err)
			if !c.Writer.Written() {
				stream.SendStreamingError(c, err)
			}
			if streamRec != nil {
				streamRec.RecordError(err)
			}
			return
		}
		ph.trackUsageWithTokenUsage(c, usage, nil)
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
