package server

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
)

const pendingVirtualToolResultTTL = 10 * time.Minute

type virtualToolExecutionResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

type pendingVirtualToolResultBatch struct {
	Results   []virtualToolExecutionResult
	ExpiresAt time.Time
}

type pendingVirtualToolResultStore struct {
	mu    sync.Mutex
	items map[string]pendingVirtualToolResultBatch // key: external tool_use_id anchor
}

func newPendingVirtualToolResultStore() *pendingVirtualToolResultStore {
	return &pendingVirtualToolResultStore{
		items: make(map[string]pendingVirtualToolResultBatch),
	}
}

func (s *pendingVirtualToolResultStore) put(anchorToolUseID string, results []virtualToolExecutionResult) {
	if s == nil || anchorToolUseID == "" || len(results) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[anchorToolUseID] = pendingVirtualToolResultBatch{
		Results:   results,
		ExpiresAt: time.Now().Add(pendingVirtualToolResultTTL),
	}
}

func (s *pendingVirtualToolResultStore) pop(anchorToolUseID string) ([]virtualToolExecutionResult, bool) {
	if s == nil || anchorToolUseID == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[anchorToolUseID]
	if !ok {
		return nil, false
	}
	delete(s.items, anchorToolUseID)
	if time.Now().After(item.ExpiresAt) {
		return nil, false
	}
	return item.Results, true
}

func (s *Server) stashPendingVirtualToolResults(anchorToolUseIDs []string, results []virtualToolExecutionResult) {
	if s == nil || s.pendingVirtualToolResults == nil || len(anchorToolUseIDs) == 0 || len(results) == 0 {
		return
	}
	for _, anchor := range anchorToolUseIDs {
		if anchor == "" {
			continue
		}
		s.pendingVirtualToolResults.put(anchor, results)
	}
}

func (s *Server) injectPendingVirtualResultsAnthropicV1(req *anthropic.MessageNewParams) {
	if s == nil || s.pendingVirtualToolResults == nil || req == nil || len(req.Messages) == 0 {
		return
	}

	seenToolResultIDs := make(map[string]struct{})
	anchorIDs := make([]string, 0)
	for _, msg := range req.Messages {
		raw, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		content, _ := m["content"].([]any)
		for _, c := range content {
			block, _ := c.(map[string]any)
			if blockType, _ := block["type"].(string); blockType != "tool_result" {
				continue
			}
			toolUseID, _ := block["tool_use_id"].(string)
			if toolUseID != "" {
				anchorIDs = append(anchorIDs, toolUseID)
				seenToolResultIDs[toolUseID] = struct{}{}
			}
		}
	}

	pendingBlocks := make([]anthropic.ContentBlockParamUnion, 0)
	for _, anchorID := range anchorIDs {
		results, ok := s.pendingVirtualToolResults.pop(anchorID)
		if !ok {
			continue
		}
		for _, r := range results {
			if r.ToolUseID == "" {
				continue
			}
			if _, exists := seenToolResultIDs[r.ToolUseID]; exists {
				continue
			}
			seenToolResultIDs[r.ToolUseID] = struct{}{}
			pendingBlocks = append(pendingBlocks, anthropic.NewToolResultBlock(r.ToolUseID, r.Content, r.IsError))
		}
	}

	if len(pendingBlocks) == 0 {
		return
	}
	logrus.Debugf("[MCP-SSE-DEBUG] Injecting %d pending virtual tool_result block(s) into Anthropic V1 follow-up request", len(pendingBlocks))
	req.Messages = append(req.Messages, anthropic.NewUserMessage(pendingBlocks...))
}

func (s *Server) injectPendingVirtualResultsAnthropicBeta(req *anthropic.BetaMessageNewParams) {
	if s == nil || s.pendingVirtualToolResults == nil || req == nil || len(req.Messages) == 0 {
		return
	}

	seenToolResultIDs := make(map[string]struct{})
	anchorIDs := make([]string, 0)
	for _, msg := range req.Messages {
		raw, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		content, _ := m["content"].([]any)
		for _, c := range content {
			block, _ := c.(map[string]any)
			if blockType, _ := block["type"].(string); blockType != "tool_result" {
				continue
			}
			toolUseID, _ := block["tool_use_id"].(string)
			if toolUseID != "" {
				anchorIDs = append(anchorIDs, toolUseID)
				seenToolResultIDs[toolUseID] = struct{}{}
			}
		}
	}

	pendingBlocks := make([]anthropic.BetaContentBlockParamUnion, 0)
	for _, anchorID := range anchorIDs {
		results, ok := s.pendingVirtualToolResults.pop(anchorID)
		if !ok {
			continue
		}
		for _, r := range results {
			if r.ToolUseID == "" {
				continue
			}
			if _, exists := seenToolResultIDs[r.ToolUseID]; exists {
				continue
			}
			seenToolResultIDs[r.ToolUseID] = struct{}{}
			pendingBlocks = append(pendingBlocks, anthropic.NewBetaToolResultBlock(r.ToolUseID, r.Content, r.IsError))
		}
	}

	if len(pendingBlocks) == 0 {
		return
	}
	logrus.Debugf("[MCP-SSE-BETA-DEBUG] Injecting %d pending virtual tool_result block(s) into Anthropic Beta follow-up request", len(pendingBlocks))
	req.Messages = append(req.Messages, anthropic.NewBetaUserMessage(pendingBlocks...))
}

func (s *Server) injectPendingVirtualResultsOpenAI(req *openai.ChatCompletionNewParams) {
	if s == nil || s.pendingVirtualToolResults == nil || req == nil || len(req.Messages) == 0 {
		return
	}

	seenToolCallIDs := make(map[string]struct{})
	anchorIDs := make([]string, 0)
	for _, msg := range req.Messages {
		raw, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		role, _ := m["role"].(string)
		if role != "tool" {
			continue
		}
		toolCallID, _ := m["tool_call_id"].(string)
		if toolCallID == "" {
			continue
		}
		anchorIDs = append(anchorIDs, toolCallID)
		seenToolCallIDs[toolCallID] = struct{}{}
	}

	pendingMessages := make([]openai.ChatCompletionMessageParamUnion, 0)
	for _, anchorID := range anchorIDs {
		results, ok := s.pendingVirtualToolResults.pop(anchorID)
		if !ok {
			continue
		}
		for _, r := range results {
			if r.ToolUseID == "" {
				continue
			}
			if _, exists := seenToolCallIDs[r.ToolUseID]; exists {
				continue
			}
			seenToolCallIDs[r.ToolUseID] = struct{}{}
			pendingMessages = append(pendingMessages, openai.ToolMessage(r.Content, r.ToolUseID))
		}
	}

	if len(pendingMessages) == 0 {
		return
	}
	logrus.Debugf("[MCP-SSE-OPENAI-DEBUG] Injecting %d pending virtual tool message(s) into OpenAI follow-up request", len(pendingMessages))
	req.Messages = append(req.Messages, pendingMessages...)
}
