package mcp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestContinuationStoreOnlyConsumesMatchingCurrentToolResults(t *testing.T) {
	store := newContinuationStore()
	key := continuationKey(typ.SessionID{Source: typ.SessionSourceHeader, Value: "session"}, "provider", "anthropic-beta")
	store.put(key, "first", []string{"toolu-first"})
	store.put(key, "second", []string{"toolu-second"})

	unrelated := &anthropic.BetaMessageNewParams{Messages: []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock("toolu-other", "no", false)),
	}}
	if _, ok := store.pop(key, unrelated); ok {
		t.Fatal("unrelated tool result consumed a continuation")
	}

	second := &anthropic.BetaMessageNewParams{Messages: []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock("toolu-second", "yes", false)),
	}}
	if got, ok := store.pop(key, second); !ok || got != "second" {
		t.Fatalf("matched continuation = %#v, ok=%v", got, ok)
	}
	if len(store.items[key]) != 1 || store.items[key][0].Segment != "first" {
		t.Fatalf("unmatched continuation was removed: %#v", store.items[key])
	}
}

func TestContinuationStoreRejectsIPFallbackAndSweepsExpiredItems(t *testing.T) {
	if key := continuationKey(typ.SessionID{Source: typ.SessionSourceIP, Value: "127.0.0.1"}, "provider", "anthropic-beta"); key != "" {
		t.Fatalf("IP fallback produced continuation key %q", key)
	}

	store := newContinuationStore()
	store.items["expired"] = []continuationItem{{
		Segment:     "old",
		ExpectedIDs: stringSet([]string{"toolu-old"}),
		ExpiresAt:   time.Now().Add(-time.Second),
	}}
	store.put("active", "new", []string{"toolu-new"})
	if _, ok := store.items["expired"]; ok {
		t.Fatal("expired continuation was not swept on write")
	}
}

func TestContinuationStoreIsBoundedAndRecognizesOpenAIToolResults(t *testing.T) {
	store := newContinuationStore()
	for index := 0; index < maxContinuationItems+10; index++ {
		store.put("key", index, []string{fmt.Sprintf("call-%d", index)})
	}
	if got := store.sizeLocked(); got != maxContinuationItems {
		t.Fatalf("continuation count = %d, want %d", got, maxContinuationItems)
	}

	request := &openai.ChatCompletionNewParams{Messages: []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage("before"),
		openai.ToolMessage("result", "call-42"),
	}}
	if ids := continuationResultIDs(request); len(ids) != 1 {
		t.Fatalf("OpenAI trailing result IDs = %#v", ids)
	} else if _, ok := ids["call-42"]; !ok {
		t.Fatalf("OpenAI trailing result IDs = %#v", ids)
	}
}

func TestProviderBetaContinuationStoreRequiresExplicitSession(t *testing.T) {
	store := NewProviderBetaContinuationStore("provider")
	segment := []anthropic.BetaMessageParam{{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("stored")},
	}}
	request := &anthropic.BetaMessageNewParams{Messages: []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaToolResultBlock("toolu-external", "ok", false)),
	}}
	store.Put(context.Background(), segment, []string{"toolu-external"})
	if _, ok := store.Pop(context.Background(), request); ok {
		t.Fatal("empty session persisted a continuation")
	}
}
