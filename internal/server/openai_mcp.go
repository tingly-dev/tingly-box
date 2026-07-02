package server

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	typ "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
)

func (s *Server) streamOpenAIChatToAnthropicV1WithMCP(
	c *gin.Context,
	provider *typ.Provider,
	req *openai.ChatCompletionNewParams,
	actualModel string,
	responseModel string,
	recorder *ProtocolRecorder,
) {
	for round := 0; round < 3; round++ {
		wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
		fc := forwarding.NewForwardContext(c.Request.Context(), provider)
		streamResp, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			stream.SendStreamingError(c, protocol.TypeAnthropicV1, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		var hooks *stream.OpenAIToAnthropicMCPHooks
		if hasDeclaredMCPTools(req) && s.mcpEnabled() {
			hooks = s.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		hc := protocol.NewHandleContext(c, responseModel)
		usage, err := stream.HandleOpenAIToAnthropicStreamResponseWithMCPHooks(hc, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, protocol.TypeAnthropicV1, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}
		s.trackUsageWithTokenUsage(c, usage, nil)
		return
	}
	stream.SendInternalError(c, protocol.TypeAnthropicV1, "MCP stream continuation exceeded max rounds")
	if recorder != nil {
		recorder.RecordError(stream.ErrMCPStreamContinue)
	}
}

func (s *Server) streamOpenAIChatToAnthropicBetaWithMCP(
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
		wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
		fc := forwarding.NewForwardContext(c.Request.Context(), provider)
		streamResp, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			stream.SendStreamingError(c, protocol.TypeAnthropicBeta, err)
			if streamRec != nil {
				streamRec.RecordError(err)
			}
			return
		}

		var hooks *stream.OpenAIToAnthropicMCPHooks
		if hasDeclaredMCPTools(req) && s.mcpEnabled() {
			hooks = s.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		hc := protocol.NewHandleContext(c, responseModel)
		usage, err := stream.HandleOpenAIToAnthropicBetaStreamWithMCPHooks(hc, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, protocol.TypeAnthropicBeta, err.Error())
			if streamRec != nil {
				streamRec.RecordError(err)
			}
			return
		}
		s.trackUsageWithTokenUsage(c, usage, nil)
		if streamRec != nil {
			streamRec.Finish(responseModel, usage)
			streamRec.RecordResponse(provider, actualModel)
		}
		return
	}
	stream.SendInternalError(c, protocol.TypeAnthropicBeta, "MCP stream continuation exceeded max rounds")
	if streamRec != nil {
		streamRec.RecordError(stream.ErrMCPStreamContinue)
	}
}

func hasOnlyMCPToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) bool {
	if len(toolCalls) == 0 {
		return false
	}
	for _, tc := range toolCalls {
		if !runtime.IsMCPToolName(tc.Function.Name) {
			return false
		}
	}
	return true
}

func hasDeclaredMCPTools(req *openai.ChatCompletionNewParams) bool {
	if req == nil || len(req.Tools) == 0 {
		return false
	}
	for _, t := range req.Tools {
		fn := t.GetFunction()
		if fn == nil {
			continue
		}
		if runtime.IsMCPToolName(fn.Name) {
			return true
		}
	}
	return false
}
