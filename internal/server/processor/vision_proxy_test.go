package processor

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
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

func (f *fakeVisionClient) Describe(_ context.Context, mediaType, b64, url string) (string, error) {
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
		Logger:   logrus.New(),
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

// countImages returns the number of remaining image blocks across all
// supported request shapes; -1 for unsupported.
func countImages(req any) int {
	switch r := req.(type) {
	case *anthropic.BetaMessageNewParams:
		n := 0
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfImage != nil {
					n++
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
	}
	return -1
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
			}
		}
	case *anthropic.MessageNewParams:
		for _, m := range r.Messages {
			for _, b := range m.Content {
				if b.OfText != nil {
					out += b.OfText.Text + "\n"
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
