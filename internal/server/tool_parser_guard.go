package server

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func requestUsesToolParser(req *openai.ChatCompletionNewParams) bool {
	if req == nil {
		return false
	}
	if len(req.Tools) > 0 {
		return true
	}
	if req.ToolChoice.OfAuto.Value != "" {
		return true
	}
	if req.ToolChoice.OfAllowedTools != nil {
		return true
	}
	if req.ToolChoice.OfFunctionToolChoice != nil {
		return true
	}
	if req.ToolChoice.OfCustomToolChoice != nil {
		return true
	}
	return false
}

func (s *Server) enforceToolParserSupport(c *gin.Context, provider *typ.Provider, modelID string, req *openai.ChatCompletionNewParams) bool {
	if provider == nil || req == nil {
		return true
	}
	if provider.APIStyle != protocol.APIStyleOpenAI {
		return true
	}
	if !requestUsesToolParser(req) {
		return true
	}

	supported, known, errMsg := s.getToolParserCapability(provider, modelID)
	if known && !supported {
		if errMsg == "" {
			errMsg = "Tool parser is not supported by the selected provider/model."
		}
		logrus.Warnf("Tool parser unsupported for provider=%s model=%s: %s", provider.Name, modelID, errMsg)
	}
	return true
}

func (s *Server) getToolParserCapability(provider *typ.Provider, modelID string) (supported bool, known bool, errMsg string) {
	if provider == nil {
		return false, false, ""
	}

	ap := NewAdaptiveProbe(s)
	capability, err := ap.GetModelCapability(provider.UUID, modelID)
	if err != nil {
		// Trigger async probe refresh
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
			defer cancel()
			ap.ProbeModelEndpoints(ctx, ModelProbeRequest{
				ProviderUUID: provider.UUID,
				ModelID:      modelID,
			})
		}()
		return false, false, ""
	}

	if !capability.ToolParserChecked {
		return false, false, ""
	}
	return capability.SupportsToolParser, true, capability.ToolParserError
}
