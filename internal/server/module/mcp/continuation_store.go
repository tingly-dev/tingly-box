package mcp

import (
	"fmt"
	"sync"
	"time"

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
