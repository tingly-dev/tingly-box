package smartrouting

import (
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// ---------------------------------------------------------------------------
// Harness — tiny shared base64 PNG (1x1 black pixel)
// ---------------------------------------------------------------------------

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
const tinyPNGMediaType = "image/png"

// ---------------------------------------------------------------------------
// Harness — fakeOpProcessor
// ---------------------------------------------------------------------------

type processCall struct {
	RuleIndex int
	OpUUID    string
	N         int
}

type fakeOpProcessor struct {
	mu     sync.Mutex
	calls  []processCall
	mutate func(*ProcessorContext) error // optional per-call hook
}

func (f *fakeOpProcessor) Process(pctx *ProcessorContext) error {
	f.mu.Lock()
	f.calls = append(f.calls, processCall{
		RuleIndex: pctx.RuleIndex,
		OpUUID:    pctx.OpUUID,
		N:         len(f.calls) + 1,
	})
	f.mu.Unlock()
	if f.mutate != nil {
		return f.mutate(pctx)
	}
	return nil
}

func (f *fakeOpProcessor) snapshot() []processCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]processCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func registerFakeProcessor(t *testing.T, pos SmartOpPosition, op SmartOpOperation, fake *fakeOpProcessor) {
	t.Helper()
	RegisterProcessor(pos, op, fake)
	t.Cleanup(func() { UnregisterProcessor(pos, op) })
}

// ---------------------------------------------------------------------------
// Harness — fixture builders
// ---------------------------------------------------------------------------

func betaReqText(prompt string) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: prompt}},
				},
			},
		},
	}
}

func betaReqWithImage(prompt, b64 string) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: prompt}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      b64,
						MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
					}),
				},
			},
		},
	}
}

func v1ReqWithImage(prompt, b64 string) *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: prompt}},
					anthropic.NewImageBlock(anthropic.Base64ImageSourceParam{
						Data:      b64,
						MediaType: anthropic.Base64ImageSourceMediaType(tinyPNGMediaType),
					}),
				},
			},
		},
	}
}

func openaiReqWithImage(prompt, b64 string) *openai.ChatCompletionNewParams {
	dataURL := "data:" + tinyPNGMediaType + ";base64," + b64
	return &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("gpt-4o"),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
							{OfText: &openai.ChatCompletionContentPartTextParam{Text: prompt}},
							{OfImageURL: &openai.ChatCompletionContentPartImageParam{
								ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: dataURL},
							}},
						},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Harness — assertion helpers
// ---------------------------------------------------------------------------

// assertImageReplaced verifies that no image block remains in req and that
// at least one text block contains wantText. Type-switches over the three
// supported request shapes.
func assertImageReplaced(t *testing.T, req any, wantText string) {
	t.Helper()
	imgs, texts := walkContent(t, req)
	require.Equal(t, 0, imgs, "expected no image blocks remaining")
	joined := ""
	for _, s := range texts {
		joined += s + "\n"
	}
	require.Contains(t, joined, wantText, "expected text marker present")
}

// assertImagesUntouched verifies wantCount image blocks remain in req.
func assertImagesUntouched(t *testing.T, req any, wantCount int) {
	t.Helper()
	imgs, _ := walkContent(t, req)
	require.Equal(t, wantCount, imgs, "expected image count")
}

func walkContent(t *testing.T, req any) (images int, texts []string) {
	t.Helper()
	switch r := req.(type) {
	case *anthropic.BetaMessageNewParams:
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfImage != nil {
					images++
				}
				if b.OfText != nil {
					texts = append(texts, b.OfText.Text)
				}
			}
		}
	case *anthropic.MessageNewParams:
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfImage != nil {
					images++
				}
				if b.OfText != nil {
					texts = append(texts, b.OfText.Text)
				}
			}
		}
	case *openai.ChatCompletionNewParams:
		for _, m := range r.Messages {
			if m.OfUser == nil {
				continue
			}
			for _, p := range m.OfUser.Content.OfArrayOfContentParts {
				if p.OfImageURL != nil {
					images++
				}
				if p.OfText != nil {
					texts = append(texts, p.OfText.Text)
				}
			}
		}
	default:
		t.Fatalf("walkContent: unsupported request type %T", req)
	}
	return
}

// ---------------------------------------------------------------------------
// Tests — registry (Phase B)
// ---------------------------------------------------------------------------

func TestRegisterProcessor_StoresAndLooksUp(t *testing.T) {
	fake := &fakeOpProcessor{}
	registerFakeProcessor(t, PositionProxyVision, OpProxyVisionEnabled, fake)

	got, ok := LookupProcessor(PositionProxyVision, OpProxyVisionEnabled)
	require.True(t, ok)
	require.Same(t, OpProcessor(fake), got)
}

func TestLookupProcessor_MissingReturnsFalse(t *testing.T) {
	got, ok := LookupProcessor(PositionProxyVision, "nonexistent_op_xyz")
	require.False(t, ok)
	require.Nil(t, got)
}

func TestRegisterProcessor_OverwriteContract(t *testing.T) {
	// Production contract: silently replace. Keeps server boot idempotent
	// across config reloads.
	first := &fakeOpProcessor{}
	second := &fakeOpProcessor{}

	const testOp SmartOpOperation = "harness_overwrite_op"
	RegisterProcessor(PositionProxyVision, testOp, first)
	t.Cleanup(func() { UnregisterProcessor(PositionProxyVision, testOp) })

	RegisterProcessor(PositionProxyVision, testOp, second)

	got, ok := LookupProcessor(PositionProxyVision, testOp)
	require.True(t, ok)
	require.Same(t, OpProcessor(second), got, "second registration must replace the first")
}

// ---------------------------------------------------------------------------
// Tests — op match (Phase B)
// ---------------------------------------------------------------------------

func TestEvaluateProxyVision_MatchesWhenImagePresent(t *testing.T) {
	reqCtx := ExtractContext(betaReqWithImage("describe", tinyPNGBase64))
	require.NotNil(t, reqCtx)

	rules := []SmartRouting{{
		Description: "vision proxy",
		Ops:         []SmartOp{{Position: PositionProxyVision, Operation: OpProxyVisionEnabled}},
		Services:    []*loadbalance.Service{{Provider: "p", Model: "m", Active: true}},
	}}
	r, err := NewRouter(rules)
	require.NoError(t, err)

	_, idx, matched, _ := r.Evaluate(reqCtx)
	require.True(t, matched, "image-bearing request must match proxy_vision op")
	require.Equal(t, 0, idx)
}

// TestEvaluateProxyVision_MatchesWhenImageOnlyInHistory verifies that the
// op matches even when the LATEST user message is text-only, as long as
// some earlier message has an image. The processor still needs to run so
// the historical image is stripped before the text-only downstream sees
// it. The literal "image in history only" case is the whole reason we
// match on HasImage instead of LatestContentType.
func TestEvaluateProxyVision_MatchesWhenImageOnlyInHistory(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "earlier turn"}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      tinyPNGBase64,
						MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
					}),
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "previous reply"}},
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "current question, no image"}},
				},
			},
		},
	}
	reqCtx := ExtractContext(req)
	require.NotNil(t, reqCtx)
	require.True(t, reqCtx.HasImage, "HasImage must reflect any image anywhere in the conversation")

	rules := []SmartRouting{{
		Description: "vision proxy",
		Ops:         []SmartOp{{Position: PositionProxyVision, Operation: OpProxyVisionEnabled}},
		Services:    []*loadbalance.Service{{Provider: "p", Model: "m", Active: true}},
	}}
	r, err := NewRouter(rules)
	require.NoError(t, err)

	_, _, matched, _ := r.Evaluate(reqCtx)
	require.True(t, matched, "historical image must trigger proxy_vision so the processor can clean it up")
}

// TestEvaluateProxyVision_MatchesWhenImageInAssistantMessage covers the
// rarer edge case where the image block lives in a non-user role (e.g.
// assistant or tool result). The processor walks every message so the
// matcher must too.
func TestEvaluateProxyVision_MatchesWhenImageInAssistantMessage(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "no image here"}},
				},
			},
			{
				Role: anthropic.BetaMessageParamRoleAssistant,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: "but I returned one"}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      tinyPNGBase64,
						MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
					}),
				},
			},
		},
	}
	reqCtx := ExtractContext(req)
	require.NotNil(t, reqCtx)
	require.True(t, reqCtx.HasImage, "image in assistant message must still be detected")

	rules := []SmartRouting{{
		Description: "vision proxy",
		Ops:         []SmartOp{{Position: PositionProxyVision, Operation: OpProxyVisionEnabled}},
		Services:    []*loadbalance.Service{{Provider: "p", Model: "m", Active: true}},
	}}
	r, err := NewRouter(rules)
	require.NoError(t, err)

	_, _, matched, _ := r.Evaluate(reqCtx)
	require.True(t, matched)
}

func TestEvaluateProxyVision_DoesNotMatchTextOnly(t *testing.T) {
	reqCtx := ExtractContext(betaReqText("hello"))
	require.NotNil(t, reqCtx)

	rules := []SmartRouting{{
		Description: "vision proxy",
		Ops:         []SmartOp{{Position: PositionProxyVision, Operation: OpProxyVisionEnabled}},
		Services:    []*loadbalance.Service{{Provider: "p", Model: "m", Active: true}},
	}}
	r, err := NewRouter(rules)
	require.NoError(t, err)

	_, _, matched, _ := r.Evaluate(reqCtx)
	require.False(t, matched, "text-only request must NOT match proxy_vision op")
}

// ---------------------------------------------------------------------------
// Sanity check — fixtures themselves (runs Phase A onward)
// ---------------------------------------------------------------------------

func TestHarness_Fixtures_ProduceWalkableContent(t *testing.T) {
	beta := betaReqWithImage("hi", tinyPNGBase64)
	assertImagesUntouched(t, beta, 1)

	v1 := v1ReqWithImage("hi", tinyPNGBase64)
	assertImagesUntouched(t, v1, 1)

	oai := openaiReqWithImage("hi", tinyPNGBase64)
	assertImagesUntouched(t, oai, 1)

	textOnly := betaReqText("hello")
	assertImagesUntouched(t, textOnly, 0)
}
