package server

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/toolruntime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const maxRuntimeFollowUpRounds = 3

func (s *Server) continueAnthropicRuntimeTools(ctx context.Context, provider *typ.Provider, originalReq *anthropic.MessageNewParams, initialResp *anthropic.Message) (*anthropic.Message, error) {
	currentReq := *originalReq
	currentResp := initialResp

	for range maxRuntimeFollowUpRounds {
		followReq, hasRuntimeCalls := s.buildAnthropicRuntimeFollowUp(ctx, provider, &currentReq, currentResp)
		if !hasRuntimeCalls {
			return currentResp, nil
		}
		wrapper := s.clientPool.GetAnthropicClient(provider, string(followReq.Model))
		fc := NewForwardContext(ctx, provider)
		nextResp, cancel, err := ForwardAnthropicV1(fc, wrapper, *followReq)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, err
		}
		currentReq = *followReq
		currentResp = nextResp
	}

	return currentResp, fmt.Errorf("tool runtime follow-up exceeded %d rounds", maxRuntimeFollowUpRounds)
}

func (s *Server) continueAnthropicBetaRuntimeTools(ctx context.Context, provider *typ.Provider, originalReq *anthropic.BetaMessageNewParams, initialResp *anthropic.BetaMessage) (*anthropic.BetaMessage, error) {
	currentReq := *originalReq
	currentResp := initialResp

	for range maxRuntimeFollowUpRounds {
		followReq, hasRuntimeCalls := s.buildAnthropicBetaRuntimeFollowUp(ctx, provider, &currentReq, currentResp)
		if !hasRuntimeCalls {
			return currentResp, nil
		}
		wrapper := s.clientPool.GetAnthropicClient(provider, string(followReq.Model))
		fc := NewForwardContext(ctx, provider)
		nextResp, cancel, err := ForwardAnthropicV1Beta(fc, wrapper, *followReq)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, err
		}
		currentReq = *followReq
		currentResp = nextResp
	}

	return currentResp, fmt.Errorf("beta tool runtime follow-up exceeded %d rounds", maxRuntimeFollowUpRounds)
}

func (s *Server) continueGoogleAnthropicRuntimeTools(ctx context.Context, provider *typ.Provider, originalReq *anthropic.MessageNewParams, initialResp *genai.GenerateContentResponse, responseModel string) (*genai.GenerateContentResponse, error) {
	currentReq := *originalReq
	currentResp := initialResp

	for range maxRuntimeFollowUpRounds {
		anthropicResp := nonstream.ConvertGoogleToAnthropicResponse(currentResp, responseModel)
		followReq, hasRuntimeCalls := s.buildAnthropicRuntimeFollowUp(ctx, provider, &currentReq, &anthropicResp)
		if !hasRuntimeCalls {
			return currentResp, nil
		}

		model, googleReq, cfg := request.ConvertAnthropicToGoogleRequest(followReq, 0)
		wrapper := s.clientPool.GetGoogleClient(provider, model)
		fc := NewForwardContext(ctx, provider)
		nextResp, cancel, err := ForwardGoogle(fc, wrapper, model, googleReq, cfg)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, err
		}
		currentReq = *followReq
		currentResp = nextResp
	}

	return currentResp, fmt.Errorf("google anthropic tool runtime follow-up exceeded %d rounds", maxRuntimeFollowUpRounds)
}

func (s *Server) continueGoogleAnthropicBetaRuntimeTools(ctx context.Context, provider *typ.Provider, originalReq *anthropic.BetaMessageNewParams, initialResp *genai.GenerateContentResponse, responseModel string) (*genai.GenerateContentResponse, error) {
	currentReq := *originalReq
	currentResp := initialResp

	for range maxRuntimeFollowUpRounds {
		anthropicResp := nonstream.ConvertGoogleToAnthropicBetaResponse(currentResp, responseModel)
		followReq, hasRuntimeCalls := s.buildAnthropicBetaRuntimeFollowUp(ctx, provider, &currentReq, &anthropicResp)
		if !hasRuntimeCalls {
			return currentResp, nil
		}

		model, googleReq, cfg := request.ConvertAnthropicBetaToGoogleRequest(followReq, 0)
		wrapper := s.clientPool.GetGoogleClient(provider, model)
		fc := NewForwardContext(ctx, provider)
		nextResp, cancel, err := ForwardGoogle(fc, wrapper, model, googleReq, cfg)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, err
		}
		currentReq = *followReq
		currentResp = nextResp
	}

	return currentResp, fmt.Errorf("google anthropic beta tool runtime follow-up exceeded %d rounds", maxRuntimeFollowUpRounds)
}

func (s *Server) buildAnthropicRuntimeFollowUp(ctx context.Context, provider *typ.Provider, originalReq *anthropic.MessageNewParams, resp *anthropic.Message) (*anthropic.MessageNewParams, bool) {
	if resp == nil {
		return originalReq, false
	}
	newMessages := append([]anthropic.MessageParam{}, originalReq.Messages...)
	newMessages = append(newMessages, resp.ToParam())

	hasRuntimeCalls := false
	for _, block := range resp.Content {
		if block.Type != "tool_use" || !s.toolRuntime.IsRuntimeTool(provider, block.Name) {
			continue
		}
		hasRuntimeCalls = true
		result := s.toolRuntime.ExecuteTool(ctx, provider, block.Name, string(block.Input))
		content := result.Content
		if result.IsError {
			content = result.Error
		}
		newMessages = append(newMessages, anthropic.NewUserMessage(
			toolruntime.CreateAnthropicToolResultBlock(block.ID, content, result.IsError),
		))
	}
	if !hasRuntimeCalls {
		return originalReq, false
	}
	followReq := *originalReq
	followReq.Messages = newMessages
	return &followReq, true
}

func (s *Server) buildAnthropicBetaRuntimeFollowUp(ctx context.Context, provider *typ.Provider, originalReq *anthropic.BetaMessageNewParams, resp *anthropic.BetaMessage) (*anthropic.BetaMessageNewParams, bool) {
	if resp == nil {
		return originalReq, false
	}
	newMessages := append([]anthropic.BetaMessageParam{}, originalReq.Messages...)
	newMessages = append(newMessages, resp.ToParam())

	hasRuntimeCalls := false
	for _, block := range resp.Content {
		if block.Type != "tool_use" || !s.toolRuntime.IsRuntimeTool(provider, block.Name) {
			continue
		}
		hasRuntimeCalls = true
		result := s.toolRuntime.ExecuteTool(ctx, provider, block.Name, string(block.Input))
		content := result.Content
		if result.IsError {
			content = result.Error
		}
		newMessages = append(newMessages, anthropic.NewBetaUserMessage(
			toolruntime.CreateAnthropicBetaToolResultBlock(block.ID, content, result.IsError),
		))
	}
	if !hasRuntimeCalls {
		return originalReq, false
	}
	followReq := *originalReq
	followReq.Messages = newMessages
	return &followReq, true
}
