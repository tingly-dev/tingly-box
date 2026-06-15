package processor

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ---------------------------------------------------------------------------
// Harness — fakes
// ---------------------------------------------------------------------------

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
const tinyPNGMediaType = "image/png"

type describeCall struct {
	MediaType string
	Base64    string
	URL       string
}

type fakeVisionClient struct {
	mu        sync.Mutex
	calls     []describeCall
	responses []string // canned response per call index; "" => empty description
	errAt     map[int]error
}

func newFakeVisionClient(responses ...string) *fakeVisionClient {
	return &fakeVisionClient{
		responses: responses,
		errAt:     make(map[int]error),
	}
}

func (f *fakeVisionClient) failCall(idx int, err error) {
	f.errAt[idx] = err
}

func (f *fakeVisionClient) Describe(_ context.Context, _ *loadbalance.Service, mediaType, b64, url string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := len(f.calls)
	f.calls = append(f.calls, describeCall{MediaType: mediaType, Base64: b64, URL: url})
	if err, ok := f.errAt[idx]; ok {
		return "", err
	}
	if idx < len(f.responses) {
		return f.responses[idx], nil
	}
	return "", nil
}

func (f *fakeVisionClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeProviderResolver struct {
	providers map[string]*typ.Provider
}

func newFakeProviderResolver(ps ...*typ.Provider) *fakeProviderResolver {
	m := make(map[string]*typ.Provider, len(ps))
	for _, p := range ps {
		m[p.UUID] = p
	}
	return &fakeProviderResolver{providers: m}
}

func (f *fakeProviderResolver) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	if p, ok := f.providers[uuid]; ok {
		return p, nil
	}
	return nil, errors.New("provider not found: " + uuid)
}

// ---------------------------------------------------------------------------
// Harness — fixtures (request builders + service builder)
// ---------------------------------------------------------------------------

func mkProvider(uuid string) *typ.Provider {
	return &typ.Provider{UUID: uuid, Name: uuid, Enabled: true}
}

func mkService(provider string, active bool) *loadbalance.Service {
	return &loadbalance.Service{Provider: provider, Model: "vision-model", Weight: 1, Active: active}
}

func mkProcessor(t *testing.T, vc visionClient, providers ...*typ.Provider) *VisionProxyProcessor {
	t.Helper()
	return &VisionProxyProcessor{
		Client:   vc,
		Resolver: newFakeProviderResolver(providers...),
	}
}

func mkPctx(req any, services ...*loadbalance.Service) *smartrouting.ProcessorContext {
	return &smartrouting.ProcessorContext{
		Ctx:       context.Background(),
		Request:   req,
		RuleIndex: 0,
		OpUUID:    "test-op",
		Services:  services,
	}
}

func betaReqWithImages(prompt string, imageBase64 ...string) *anthropic.BetaMessageNewParams {
	blocks := []anthropic.BetaContentBlockParamUnion{
		{OfText: &anthropic.BetaTextBlockParam{Text: prompt}},
	}
	for _, b64 := range imageBase64 {
		blocks = append(blocks, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      b64,
			MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
		}))
	}
	return &anthropic.BetaMessageNewParams{
		Model: anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: []anthropic.BetaMessageParam{
			{Role: anthropic.BetaMessageParamRoleUser, Content: blocks},
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

// betaMessage builds a Beta user/assistant message with one optional image
// followed by a text prompt. Used to assemble multi-turn fixtures for the
// historical-strip tests.
func betaMessage(role anthropic.BetaMessageParamRole, text string, imageB64 string) anthropic.BetaMessageParam {
	var blocks []anthropic.BetaContentBlockParamUnion
	if text != "" {
		blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
			OfText: &anthropic.BetaTextBlockParam{Text: text},
		})
	}
	if imageB64 != "" {
		blocks = append(blocks, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      imageB64,
			MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
		}))
	}
	return anthropic.BetaMessageParam{Role: role, Content: blocks}
}

func betaReqWithMessages(msgs ...anthropic.BetaMessageParam) *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:    anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: msgs,
	}
}

func v1Message(role anthropic.MessageParamRole, text string, imageB64 string) anthropic.MessageParam {
	var blocks []anthropic.ContentBlockParamUnion
	if text != "" {
		blocks = append(blocks, anthropic.ContentBlockParamUnion{
			OfText: &anthropic.TextBlockParam{Text: text},
		})
	}
	if imageB64 != "" {
		blocks = append(blocks, anthropic.NewImageBlock(anthropic.Base64ImageSourceParam{
			Data:      imageB64,
			MediaType: anthropic.Base64ImageSourceMediaType(tinyPNGMediaType),
		}))
	}
	return anthropic.MessageParam{Role: role, Content: blocks}
}

func v1ReqWithMessages(msgs ...anthropic.MessageParam) *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:    anthropic.Model("claude-3-5-sonnet-latest"),
		Messages: msgs,
	}
}

func openaiUserMessageWithImage(text, imageB64 string) openai.ChatCompletionMessageParamUnion {
	parts := []openai.ChatCompletionContentPartUnionParam{
		{OfText: &openai.ChatCompletionContentPartTextParam{Text: text}},
	}
	if imageB64 != "" {
		dataURL := "data:" + tinyPNGMediaType + ";base64," + imageB64
		parts = append(parts, openai.ChatCompletionContentPartUnionParam{
			OfImageURL: &openai.ChatCompletionContentPartImageParam{
				ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: dataURL},
			},
		})
	}
	return openai.ChatCompletionMessageParamUnion{
		OfUser: &openai.ChatCompletionUserMessageParam{
			Content: openai.ChatCompletionUserMessageParamContentUnion{
				OfArrayOfContentParts: parts,
			},
		},
	}
}

// responsesMessageItem builds an OpenAI Responses-API input item
// (EasyInputMessageParam variant) with a text part and an optional image
// part. Used to assemble multi-item fixtures for the historical-strip
// tests on the Responses path.
func responsesMessageItem(role responses.EasyInputMessageRole, text, imageB64 string) responses.ResponseInputItemUnionParam {
	parts := responses.ResponseInputMessageContentListParam{
		{OfInputText: &responses.ResponseInputTextParam{Text: text}},
	}
	if imageB64 != "" {
		dataURL := "data:" + tinyPNGMediaType + ";base64," + imageB64
		parts = append(parts, responses.ResponseInputContentUnionParam{
			OfInputImage: &responses.ResponseInputImageParam{ImageURL: param.NewOpt(dataURL)},
		})
	}
	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role:    role,
			Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: parts},
		},
	}
}

// responsesInputMessageItem builds the alternate ResponseInputItemMessageParam
// (`OfInputMessage`) variant — content list inline, no Easy wrapper. The
// SDK uses this for system/developer/user messages that were already
// serialised once.
func responsesInputMessageItem(role, text, imageB64 string) responses.ResponseInputItemUnionParam {
	parts := responses.ResponseInputMessageContentListParam{
		{OfInputText: &responses.ResponseInputTextParam{Text: text}},
	}
	if imageB64 != "" {
		dataURL := "data:" + tinyPNGMediaType + ";base64," + imageB64
		parts = append(parts, responses.ResponseInputContentUnionParam{
			OfInputImage: &responses.ResponseInputImageParam{ImageURL: param.NewOpt(dataURL)},
		})
	}
	return responses.ResponseInputItemUnionParam{
		OfInputMessage: &responses.ResponseInputItemMessageParam{
			Role:    role,
			Content: parts,
		},
	}
}

func responsesReqWithItems(items ...responses.ResponseInputItemUnionParam) *responses.ResponseNewParams {
	return &responses.ResponseNewParams{
		Model: "gpt-5",
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: items},
	}
}

// countImages returns the number of remaining image blocks across all
// supported request shapes; -1 for unsupported. Images inside tool_result
// blocks count too — that path is part of the proxy contract.
func countImages(req any) int {
	switch r := req.(type) {
	case *anthropic.BetaMessageNewParams:
		n := 0
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfImage != nil {
					n++
				}
				if b.OfToolResult != nil {
					for _, inner := range b.OfToolResult.Content {
						if inner.OfImage != nil {
							n++
						}
					}
				}
			}
		}
		return n
	case *anthropic.MessageNewParams:
		n := 0
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfImage != nil {
					n++
				}
				if b.OfToolResult != nil {
					for _, inner := range b.OfToolResult.Content {
						if inner.OfImage != nil {
							n++
						}
					}
				}
			}
		}
		return n
	case *openai.ChatCompletionNewParams:
		n := 0
		for _, m := range r.Messages {
			if m.OfUser == nil {
				continue
			}
			for _, p := range m.OfUser.Content.OfArrayOfContentParts {
				if p.OfImageURL != nil {
					n++
				}
			}
		}
		return n
	case *responses.ResponseNewParams:
		n := 0
		for _, item := range r.Input.OfInputItemList {
			n += countResponsesImagesInItem(item)
		}
		return n
	}
	return -1
}

func countResponsesImagesInItem(item responses.ResponseInputItemUnionParam) int {
	n := 0
	if item.OfMessage != nil {
		for _, p := range item.OfMessage.Content.OfInputItemContentList {
			if p.OfInputImage != nil {
				n++
			}
		}
	}
	if item.OfInputMessage != nil {
		for _, p := range item.OfInputMessage.Content {
			if p.OfInputImage != nil {
				n++
			}
		}
	}
	return n
}

func collectText(req any) string {
	out := ""
	switch r := req.(type) {
	case *anthropic.BetaMessageNewParams:
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfText != nil {
					out += b.OfText.Text + "\n"
				}
				if b.OfToolResult != nil {
					for _, inner := range b.OfToolResult.Content {
						if inner.OfText != nil {
							out += inner.OfText.Text + "\n"
						}
					}
				}
			}
		}
	case *anthropic.MessageNewParams:
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfText != nil {
					out += b.OfText.Text + "\n"
				}
				if b.OfToolResult != nil {
					for _, inner := range b.OfToolResult.Content {
						if inner.OfText != nil {
							out += inner.OfText.Text + "\n"
						}
					}
				}
			}
		}
	case *openai.ChatCompletionNewParams:
		for _, m := range r.Messages {
			if m.OfUser == nil {
				continue
			}
			for _, p := range m.OfUser.Content.OfArrayOfContentParts {
				if p.OfText != nil {
					out += p.OfText.Text + "\n"
				}
			}
		}
	case *responses.ResponseNewParams:
		for _, item := range r.Input.OfInputItemList {
			if item.OfMessage != nil {
				for _, p := range item.OfMessage.Content.OfInputItemContentList {
					if p.OfInputText != nil {
						out += p.OfInputText.Text + "\n"
					}
				}
			}
			if item.OfInputMessage != nil {
				for _, p := range item.OfInputMessage.Content {
					if p.OfInputText != nil {
						out += p.OfInputText.Text + "\n"
					}
				}
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests — vision proxy contract (Phase C makes these green)
// ---------------------------------------------------------------------------

func TestVisionProxy_AnthropicBeta_SuccessReplacesImageWithDescription(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("a red apple on a white plate")
	p := mkProcessor(t, fake, prov)

	req := betaReqWithImages("What's in the picture?", tinyPNGBase64)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 1, fake.callCount(), "vision client called once")
	require.Equal(t, 0, countImages(req), "no image blocks remain")
	require.Contains(t, collectText(req), "a red apple on a white plate", "description spliced into text")
}

func TestVisionProxy_AnthropicV1_Success(t *testing.T) {
	prov := mkProvider("anthropic-v1")
	fake := newFakeVisionClient("a blue sky")
	p := mkProcessor(t, fake, prov)

	req := v1ReqWithImage("describe", tinyPNGBase64)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 0, countImages(req))
	require.Contains(t, collectText(req), "a blue sky")
}

func TestVisionProxy_OpenAI_Success(t *testing.T) {
	prov := mkProvider("openai-vision")
	fake := newFakeVisionClient("a cat sitting on a mat")
	p := mkProcessor(t, fake, prov)

	req := openaiReqWithImage("what is this?", tinyPNGBase64)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 0, countImages(req))
	require.Contains(t, collectText(req), "a cat sitting on a mat")
}

func TestVisionProxy_VisionCallError_StripImageWithUnavailableMarker(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("")
	fake.failCall(0, errors.New("upstream timeout"))
	p := mkProcessor(t, fake, prov)

	req := betaReqWithImages("describe", tinyPNGBase64)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx), "Process must not surface the upstream error — fail-strip semantics")
	require.Equal(t, 0, countImages(req), "image stripped despite upstream failure")
	require.Contains(t, collectText(req), "description unavailable", "fail-strip marker present")
}

func TestVisionProxy_MultipleImages_AllReplacedInOrder(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("first description", "second description", "third description")
	p := mkProcessor(t, fake, prov)

	req := betaReqWithImages("compare these", tinyPNGBase64, tinyPNGBase64, tinyPNGBase64)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 3, fake.callCount(), "one describe call per image")
	require.Equal(t, 0, countImages(req))
	text := collectText(req)
	require.Contains(t, text, "first description")
	require.Contains(t, text, "second description")
	require.Contains(t, text, "third description")
}

func TestVisionProxy_NoImages_NoOp(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient()
	p := mkProcessor(t, fake, prov)

	req := betaReqWithImages("just text") // no image args
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 0, fake.callCount(), "no images → no vision calls")
	require.Equal(t, 0, countImages(req))
}

func TestVisionProxy_EmptyDescription_StripImageWithUnavailableMarker(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("") // explicit empty response
	p := mkProcessor(t, fake, prov)

	req := betaReqWithImages("describe", tinyPNGBase64)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 0, countImages(req))
	require.Contains(t, collectText(req), "description unavailable",
		"empty upstream response treated as fail-strip")
}

// TestVisionProxy_HistoricalImages_StrippedWithoutDescribe verifies the
// two-responsibility split: images in the LAST message go through the
// vision upstream; images in older messages are replaced with a fixed
// historical marker without any describe call. Calling Describe on every
// historical image would be cost-prohibitive and is unnecessary because
// the model rarely needs to reason about images outside the current turn.
func TestVisionProxy_HistoricalImages_StrippedWithoutDescribe(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("latest description")
	p := mkProcessor(t, fake, prov)

	req := betaReqWithMessages(
		betaMessage(anthropic.BetaMessageParamRoleUser, "earlier turn", tinyPNGBase64),
		betaMessage(anthropic.BetaMessageParamRoleAssistant, "previous reply", ""),
		betaMessage(anthropic.BetaMessageParamRoleUser, "second user turn", tinyPNGBase64),
		betaMessage(anthropic.BetaMessageParamRoleUser, "current question", tinyPNGBase64),
	)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 1, fake.callCount(),
		"only the LAST message's image triggers a describe call; historical images are stripped without upstream cost")
	require.Equal(t, 0, countImages(req), "all images removed from the request")

	text := collectText(req)
	require.Contains(t, text, "latest description", "latest image was described")
	// Historical images get the fixed omitted marker, not the description text.
	require.Contains(t, text, "omitted from history", "historical images carry the omitted marker")
}

// TestVisionProxy_HistoricalImages_V1AndOpenAI covers v1 and OpenAI request
// shapes with the same split contract.
func TestVisionProxy_HistoricalImages_V1AndOpenAI(t *testing.T) {
	t.Run("v1", func(t *testing.T) {
		prov := mkProvider("anthropic-v1")
		fake := newFakeVisionClient("v1 latest desc")
		p := mkProcessor(t, fake, prov)

		req := v1ReqWithMessages(
			v1Message(anthropic.MessageParamRoleUser, "old turn", tinyPNGBase64),
			v1Message(anthropic.MessageParamRoleUser, "current", tinyPNGBase64),
		)
		pctx := mkPctx(req, mkService(prov.UUID, true))

		require.NoError(t, p.Process(pctx))
		require.Equal(t, 1, fake.callCount(), "describe only the latest")
		require.Equal(t, 0, countImages(req))
		text := collectText(req)
		require.Contains(t, text, "v1 latest desc")
		require.Contains(t, text, "omitted from history")
	})
	t.Run("openai", func(t *testing.T) {
		prov := mkProvider("openai-vision")
		fake := newFakeVisionClient("openai latest desc")
		p := mkProcessor(t, fake, prov)

		req := &openai.ChatCompletionNewParams{
			Model: openai.ChatModel("gpt-4o"),
			Messages: []openai.ChatCompletionMessageParamUnion{
				openaiUserMessageWithImage("old turn", tinyPNGBase64),
				openaiUserMessageWithImage("current", tinyPNGBase64),
			},
		}
		pctx := mkPctx(req, mkService(prov.UUID, true))

		require.NoError(t, p.Process(pctx))
		require.Equal(t, 1, fake.callCount(), "describe only the latest")
		require.Equal(t, 0, countImages(req))
		text := collectText(req)
		require.Contains(t, text, "openai latest desc")
		require.Contains(t, text, "omitted from history")
	})
}

func TestVisionProxy_NoUsableService_StripImagesAndReturnNil(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("never called")
	p := mkProcessor(t, fake, prov)

	// Service is inactive → no usable service in pctx.Services.
	req := betaReqWithImages("describe", tinyPNGBase64)
	pctx := mkPctx(req, mkService(prov.UUID, false))

	require.NoError(t, p.Process(pctx), "must not error when no usable service")
	require.Equal(t, 0, fake.callCount(), "no service → no vision call")
	require.Equal(t, 0, countImages(req), "image stripped so downstream still works")
}

// ---------------------------------------------------------------------------
// Tests — OpenAI Responses API path
// ---------------------------------------------------------------------------

// TestVisionProxy_Responses_Success covers the happy path: a single input
// item with an image on the latest turn is replaced by described text.
// This is the regression test for the DeepSeek `unknown variant
// 'image_url', expected 'text'` failure originating from /v1/responses.
func TestVisionProxy_Responses_Success(t *testing.T) {
	prov := mkProvider("openai-vision")
	fake := newFakeVisionClient("a yellow rubber duck")
	p := mkProcessor(t, fake, prov)

	req := responsesReqWithItems(
		responsesMessageItem(responses.EasyInputMessageRoleUser, "what is this?", tinyPNGBase64),
	)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 1, fake.callCount())
	require.Equal(t, 0, countImages(req), "no input_image parts remain")
	require.Contains(t, collectText(req), "a yellow rubber duck")
}

// TestVisionProxy_Responses_InputMessageVariant verifies the second
// content-carrying union arm (ResponseInputItemMessageParam, via
// OfInputMessage) is also walked.
func TestVisionProxy_Responses_InputMessageVariant(t *testing.T) {
	prov := mkProvider("openai-vision")
	fake := newFakeVisionClient("a chart with three bars")
	p := mkProcessor(t, fake, prov)

	req := responsesReqWithItems(
		responsesInputMessageItem("user", "interpret this chart", tinyPNGBase64),
	)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 0, countImages(req))
	require.Contains(t, collectText(req), "a chart with three bars")
}

// TestVisionProxy_Responses_HistoricalImagesStripped enforces the same
// "describe latest, marker for history" split that the Anthropic /
// Chat paths implement. Without this, a multi-turn Responses request
// would re-describe every prior image — prohibitively expensive.
func TestVisionProxy_Responses_HistoricalImagesStripped(t *testing.T) {
	prov := mkProvider("openai-vision")
	fake := newFakeVisionClient("latest description")
	p := mkProcessor(t, fake, prov)

	req := responsesReqWithItems(
		responsesMessageItem(responses.EasyInputMessageRoleUser, "earlier turn", tinyPNGBase64),
		responsesMessageItem(responses.EasyInputMessageRoleAssistant, "previous reply", ""),
		responsesMessageItem(responses.EasyInputMessageRoleUser, "current question", tinyPNGBase64),
	)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 1, fake.callCount(),
		"only the LAST item's image triggers a describe call; historical images use the marker")
	require.Equal(t, 0, countImages(req))

	text := collectText(req)
	require.Contains(t, text, "latest description")
	require.Contains(t, text, "omitted from history")
}

// TestVisionProxy_Responses_MarshalNoImageURL is a serialization-level
// regression: even if a new SDK arm is added that we forgot to walk,
// the marshalled JSON must contain neither `"input_image"` nor
// `"image_url"` after the proxy runs. This is the surface that DeepSeek
// actually validates against.
func TestVisionProxy_Responses_MarshalNoImageURL(t *testing.T) {
	prov := mkProvider("openai-vision")
	fake := newFakeVisionClient("ok")
	p := mkProcessor(t, fake, prov)

	req := responsesReqWithItems(
		responsesMessageItem(responses.EasyInputMessageRoleUser, "older", tinyPNGBase64),
		responsesInputMessageItem("user", "newer", tinyPNGBase64),
	)
	pctx := mkPctx(req, mkService(prov.UUID, true))

	require.NoError(t, p.Process(pctx))
	require.Equal(t, 0, countImages(req))

	body, err := req.MarshalJSON()
	require.NoError(t, err)
	require.NotContains(t, string(body), `"input_image"`)
	require.NotContains(t, string(body), `"image_url"`)
}
