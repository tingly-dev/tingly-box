package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	continuationTTL      = 10 * time.Minute
	maxContinuationItems = 256
)

type continuationItem struct {
	Segment     any
	ExpectedIDs map[string]struct{}
	ExpiresAt   time.Time
}

type continuationStore struct {
	mu    sync.Mutex
	items map[string][]continuationItem
}

func newContinuationStore() *continuationStore {
	return &continuationStore{items: make(map[string][]continuationItem)}
}

func continuationKey(sessionID typ.SessionID, providerUUID string, adapterID string) string {
	// An IP address is not a conversation identity. Persisting continuation
	// state under it can splice requests from different clients behind one NAT.
	if sessionID.IsEmpty() || sessionID.IsIPFallback() || providerUUID == "" || adapterID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s|%s|%s", sessionID.Source, sessionID.Value, providerUUID, adapterID)
}

func (s *continuationStore) put(key string, segment any, expectedIDs []string) {
	if s == nil || key == "" || segment == nil {
		return
	}
	expected := stringSet(expectedIDs)
	if len(expected) == 0 {
		return
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(now)
	for s.sizeLocked() >= maxContinuationItems {
		s.evictOldestLocked()
	}
	s.items[key] = append(s.items[key], continuationItem{
		Segment:     segment,
		ExpectedIDs: expected,
		ExpiresAt:   now.Add(continuationTTL),
	})
}

func (s *continuationStore) pop(key string, request any) (any, bool) {
	if s == nil || key == "" {
		return nil, false
	}
	resultIDs := continuationResultIDs(request)
	if len(resultIDs) == 0 {
		return nil, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(time.Now())
	items := s.items[key]
	for index, item := range items {
		if !containsAll(resultIDs, item.ExpectedIDs) {
			continue
		}
		items = append(items[:index], items[index+1:]...)
		if len(items) == 0 {
			delete(s.items, key)
		} else {
			s.items[key] = items
		}
		return item.Segment, true
	}
	return nil, false
}

func (s *continuationStore) sweepLocked(now time.Time) {
	for key, items := range s.items {
		kept := items[:0]
		for _, item := range items {
			if now.Before(item.ExpiresAt) {
				kept = append(kept, item)
			}
		}
		if len(kept) == 0 {
			delete(s.items, key)
		} else {
			s.items[key] = kept
		}
	}
}

func (s *continuationStore) sizeLocked() int {
	total := 0
	for _, items := range s.items {
		total += len(items)
	}
	return total
}

func (s *continuationStore) evictOldestLocked() {
	var oldestKey string
	oldestIndex := -1
	var oldestExpiry time.Time
	for key, items := range s.items {
		for index, item := range items {
			if oldestIndex < 0 || item.ExpiresAt.Before(oldestExpiry) {
				oldestKey, oldestIndex, oldestExpiry = key, index, item.ExpiresAt
			}
		}
	}
	if oldestIndex < 0 {
		return
	}
	items := s.items[oldestKey]
	items = append(items[:oldestIndex], items[oldestIndex+1:]...)
	if len(items) == 0 {
		delete(s.items, oldestKey)
	} else {
		s.items[oldestKey] = items
	}
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func containsAll(actual, expected map[string]struct{}) bool {
	for id := range expected {
		if _, ok := actual[id]; !ok {
			return false
		}
	}
	return true
}

// continuationResultIDs extracts only results in the current trailing client
// turn. Looking through the whole history could match a stale result from an
// earlier turn and consume an unrelated continuation.
func continuationResultIDs(request any) map[string]struct{} {
	raw, err := json.Marshal(request)
	if err != nil {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	if messages, ok := root["messages"].([]any); ok {
		return trailingMessageResultIDs(messages)
	}
	if input, ok := root["input"].([]any); ok {
		return trailingResponsesResultIDs(input)
	}
	return nil
}

func trailingMessageResultIDs(messages []any) map[string]struct{} {
	result := make(map[string]struct{})
	for index := len(messages) - 1; index >= 0; index-- {
		message, ok := messages[index].(map[string]any)
		if !ok {
			break
		}
		role, _ := message["role"].(string)
		if role == "tool" {
			if id, _ := message["tool_call_id"].(string); id != "" {
				result[id] = struct{}{}
			}
			continue
		}
		if index != len(messages)-1 {
			break
		}
		content, ok := message["content"].([]any)
		if !ok {
			break
		}
		for _, value := range content {
			block, _ := value.(map[string]any)
			if block["type"] != "tool_result" {
				continue
			}
			if id, _ := block["tool_use_id"].(string); id != "" {
				result[id] = struct{}{}
			}
		}
		break
	}
	return result
}

func trailingResponsesResultIDs(input []any) map[string]struct{} {
	result := make(map[string]struct{})
	for index := len(input) - 1; index >= 0; index-- {
		item, ok := input[index].(map[string]any)
		if !ok || item["type"] != "function_call_output" {
			break
		}
		if id, _ := item["call_id"].(string); id != "" {
			result[id] = struct{}{}
		}
	}
	return result
}

var mixedContinuationStore = newContinuationStore()

func StoreOpenAIContinuationSegment(sessionID typ.SessionID, providerUUID string, segment []openai.ChatCompletionMessageParamUnion, externalIDs []string) {
	key := continuationKey(sessionID, providerUUID, "openai-chat")
	mixedContinuationStore.put(key, segment, externalIDs)
}

func PopOpenAIContinuationSegment(sessionID typ.SessionID, providerUUID string, request *openai.ChatCompletionNewParams) ([]openai.ChatCompletionMessageParamUnion, bool) {
	key := continuationKey(sessionID, providerUUID, "openai-chat")
	seg, ok := mixedContinuationStore.pop(key, request)
	if !ok {
		return nil, false
	}
	messages, ok := seg.([]openai.ChatCompletionMessageParamUnion)
	if !ok || len(messages) == 0 {
		return nil, false
	}
	return messages, true
}

// ProviderBetaContinuationStore adapts the shared bounded, single-consume
// mixed continuation store to the Beta-native ToolLoop Stage.
type ProviderBetaContinuationStore struct {
	providerUUID string
}

func NewProviderBetaContinuationStore(providerUUID string) *ProviderBetaContinuationStore {
	return &ProviderBetaContinuationStore{providerUUID: providerUUID}
}

func (s *ProviderBetaContinuationStore) Pop(ctx context.Context, request *anthropic.BetaMessageNewParams) ([]anthropic.BetaMessageParam, bool) {
	if s == nil {
		return nil, false
	}
	key := continuationKey(typ.GetSessionID(ctx), s.providerUUID, "anthropic-beta")
	segment, ok := mixedContinuationStore.pop(key, request)
	if !ok {
		return nil, false
	}
	messages, ok := segment.([]anthropic.BetaMessageParam)
	if !ok || len(messages) == 0 {
		return nil, false
	}
	return messages, true
}

func (s *ProviderBetaContinuationStore) Put(ctx context.Context, segment []anthropic.BetaMessageParam, externalIDs []string) {
	if s == nil || len(segment) == 0 {
		return
	}
	key := continuationKey(typ.GetSessionID(ctx), s.providerUUID, "anthropic-beta")
	mixedContinuationStore.put(key, append([]anthropic.BetaMessageParam(nil), segment...), externalIDs)
}
