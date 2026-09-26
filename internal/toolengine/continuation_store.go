package toolengine

import (
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const continuationTTL = 10 * time.Minute

type continuationItem struct {
	Segment   any
	ExpiresAt time.Time
}

type continuationStore struct {
	mu    sync.Mutex
	items map[string]continuationItem
}

func newContinuationStore() *continuationStore {
	return &continuationStore{
		items: make(map[string]continuationItem),
	}
}

// continuationKey scopes a stored mixed round to one session and provider.
// The store is process-wide, so without a session there is no safe scope:
// the empty key disables storing and resuming.
func continuationKey(sessionID typ.SessionID, providerUUID string, adapterID string) string {
	if sessionID.Value == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s|%s|%s", sessionID.Source, sessionID.Value, providerUUID, adapterID)
}

func (s *continuationStore) put(key string, segment any) {
	if s == nil || key == "" || segment == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = continuationItem{
		Segment:   segment,
		ExpiresAt: time.Now().Add(continuationTTL),
	}
}

func (s *continuationStore) pop(key string) (any, bool) {
	if s == nil || key == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[key]
	if !ok {
		return nil, false
	}
	delete(s.items, key)
	if time.Now().After(item.ExpiresAt) {
		return nil, false
	}
	return item.Segment, true
}

// popAnswered removes and returns the segment under key only when request is
// its follow-up (see continuationAnswered). Any other request leaves it for
// the follow-up, with its original expiry.
func (s *continuationStore) popAnswered(key string, request any) (any, bool) {
	if s == nil || key == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(item.ExpiresAt) {
		delete(s.items, key)
		return nil, false
	}
	if !continuationAnswered(item.Segment, request) {
		return nil, false
	}
	delete(s.items, key)
	return item.Segment, true
}

// continuationAnswered reports whether request carries a tool result for one
// of the calls in the segment's stored assistant turn, i.e. whether it is the
// client's follow-up to that mixed round.
func continuationAnswered(segment, request any) bool {
	calls := map[string]bool{}
	switch seg := segment.(type) {
	case []anthropic.MessageParam:
		req, ok := request.(*anthropic.MessageNewParams)
		if !ok || len(seg) == 0 {
			return false
		}
		for _, block := range seg[0].Content {
			if block.OfToolUse != nil {
				calls[block.OfToolUse.ID] = true
			}
		}
		for _, message := range req.Messages {
			for _, block := range message.Content {
				if block.OfToolResult != nil && calls[block.OfToolResult.ToolUseID] {
					return true
				}
			}
		}
	case []anthropic.BetaMessageParam:
		req, ok := request.(*anthropic.BetaMessageNewParams)
		if !ok || len(seg) == 0 {
			return false
		}
		for _, block := range seg[0].Content {
			if block.OfToolUse != nil {
				calls[block.OfToolUse.ID] = true
			}
		}
		for _, message := range req.Messages {
			for _, block := range message.Content {
				if block.OfToolResult != nil && calls[block.OfToolResult.ToolUseID] {
					return true
				}
			}
		}
	case []openai.ChatCompletionMessageParamUnion:
		req, ok := request.(*openai.ChatCompletionNewParams)
		if !ok || len(seg) == 0 || seg[0].OfAssistant == nil {
			return false
		}
		for _, call := range seg[0].OfAssistant.ToolCalls {
			if call.OfFunction != nil {
				calls[call.OfFunction.ID] = true
			}
		}
		for _, message := range req.Messages {
			if message.OfTool != nil && calls[message.OfTool.ToolCallID] {
				return true
			}
		}
	}
	return false
}

var mixedContinuationStore = newContinuationStore()

func StoreOpenAIContinuationSegment(sessionID typ.SessionID, providerUUID string, segment []openai.ChatCompletionMessageParamUnion) {
	key := continuationKey(sessionID, providerUUID, "openai-chat")
	mixedContinuationStore.put(key, segment)
}

// PopOpenAIContinuationSegment returns the stored mixed round that req
// follows up on, if any.
func PopOpenAIContinuationSegment(sessionID typ.SessionID, providerUUID string, req *openai.ChatCompletionNewParams) ([]openai.ChatCompletionMessageParamUnion, bool) {
	key := continuationKey(sessionID, providerUUID, "openai-chat")
	seg, ok := mixedContinuationStore.popAnswered(key, req)
	if !ok {
		return nil, false
	}
	messages, ok := seg.([]openai.ChatCompletionMessageParamUnion)
	if !ok || len(messages) == 0 {
		return nil, false
	}
	return messages, true
}
