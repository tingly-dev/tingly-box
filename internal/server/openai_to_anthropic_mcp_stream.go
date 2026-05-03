package server

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
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
			stream.SendStreamingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		var hooks *stream.OpenAIToAnthropicMCPHooks
		if hasDeclaredMCPTools(req) && s.mcpEnabled() {
			hooks = s.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		usage, err := stream.HandleOpenAIToAnthropicStreamResponseWithMCPHooks(c, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}
		s.trackUsageWithTokenUsage(c, usage, nil)
		return
	}
	stream.SendInternalError(c, "MCP stream continuation exceeded max rounds")
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
		streamRec.SetupStreamRecorderInContext(c, "stream_event_recorder")
	}

	for round := 0; round < 3; round++ {
		wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, req.Model)
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
		if hasDeclaredMCPTools(req) && s.mcpEnabled() {
			hooks = s.buildOpenAIToAnthropicMCPHooks(c.Request.Context(), provider.UUID, req)
		}
		usage, err := stream.HandleOpenAIToAnthropicBetaStreamWithMCPHooks(c, req, streamResp, responseModel, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			s.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if streamRec != nil {
				streamRec.RecordError(err)
			}
			return
		}
		s.trackUsageWithTokenUsage(c, usage, nil)
		if streamRec != nil {
			streamRec.Finish(responseModel, usage.InputTokens, usage.OutputTokens)
			streamRec.RecordResponse(provider, actualModel)
		}
		return
	}
	stream.SendInternalError(c, "MCP stream continuation exceeded max rounds")
	if streamRec != nil {
		streamRec.RecordError(stream.ErrMCPStreamContinue)
	}
}
