package mcp

import (
	"context"
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

func continuationKey(sessionID typ.SessionID, providerUUID string, adapterID string) string {
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

var mixedContinuationStore = newContinuationStore()

func StoreOpenAIContinuationSegment(sessionID typ.SessionID, providerUUID string, segment []openai.ChatCompletionMessageParamUnion) {
	key := continuationKey(sessionID, providerUUID, "openai-chat")
	mixedContinuationStore.put(key, segment)
}

func PopOpenAIContinuationSegment(sessionID typ.SessionID, providerUUID string) ([]openai.ChatCompletionMessageParamUnion, bool) {
	key := continuationKey(sessionID, providerUUID, "openai-chat")
	seg, ok := mixedContinuationStore.pop(key)
	if !ok {
		return nil, false
	}
	messages, ok := seg.([]openai.ChatCompletionMessageParamUnion)
	if !ok || len(messages) == 0 {
		return nil, false
	}
	return messages, true
}

// ProviderBetaContinuationStore adapts the existing bounded, single-consume
// mixed continuation store to the Beta-native ToolLoop Stage. Binding the
// provider UUID in the instance keeps provider routing out of the Stage.
type ProviderBetaContinuationStore struct {
	providerUUID string
}

func NewProviderBetaContinuationStore(providerUUID string) *ProviderBetaContinuationStore {
	return &ProviderBetaContinuationStore{providerUUID: providerUUID}
}

func (s *ProviderBetaContinuationStore) Pop(ctx context.Context) ([]anthropic.BetaMessageParam, bool) {
	if s == nil {
		return nil, false
	}
	key := continuationKey(typ.GetSessionID(ctx), s.providerUUID, "anthropic-beta")
	segment, ok := mixedContinuationStore.pop(key)
	if !ok {
		return nil, false
	}
	messages, ok := segment.([]anthropic.BetaMessageParam)
	if !ok || len(messages) == 0 {
		return nil, false
	}
	return messages, true
}

func (s *ProviderBetaContinuationStore) Put(ctx context.Context, segment []anthropic.BetaMessageParam) {
	if s == nil || len(segment) == 0 {
		return
	}
	key := continuationKey(typ.GetSessionID(ctx), s.providerUUID, "anthropic-beta")
	mixedContinuationStore.put(key, append([]anthropic.BetaMessageParam(nil), segment...))
}
