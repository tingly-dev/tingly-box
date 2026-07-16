package visionproxy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// VisionClient is the small dependency VisionProxyProcessor needs to describe
// an image. The real adapter (NewServiceFromPool wiring in service.go) wraps
// client.ClientPool and dispatches to the appropriate per-service client
// based on the chosen service's provider APIStyle. Tests substitute a fake.
//
// service is the upstream Process picked from the services it was given. The
// adapter uses it to resolve which client/provider to call. The fake ignores
// it and just returns canned text.
//
// Returning ("", nil) means "no description available" → fail-strip path.
// Returning a non-nil error is also fail-strip.
//
// Describe may be called concurrently — the processor fans out up to
// describeConcurrency goroutines per request — so implementations must be
// safe for concurrent use.
type VisionClient interface {
	Describe(ctx context.Context, service *loadbalance.Service, mediaType, base64Data, remoteURL string) (string, error)
}

// poolVisionClient is the production VisionClient. It dispatches each
// describe call to the appropriate per-service client obtained from the
// shared ClientPool. Supports Anthropic-style and OpenAI-style providers;
// unknown styles return an error and the processor falls back to the
// fail-strip marker.
type poolVisionClient struct {
	pool     *client.ClientPool
	resolver providerResolver
	prompt   string
}

const defaultVisionPrompt = "Describe this image concisely; output plain text only."
const defaultVisionMaxTokens = 256

// NewPoolVisionClient builds the production vision client backed by the
// shared SDK pool. resolver is typically the routing.ProviderResolver
// implementation (server config). logger may be nil.
func NewPoolVisionClient(pool *client.ClientPool, resolver providerResolver) VisionClient {
	return &poolVisionClient{
		pool:     pool,
		resolver: resolver,
		prompt:   defaultVisionPrompt,
	}
}

func (a *poolVisionClient) Describe(ctx context.Context, service *loadbalance.Service, mediaType, b64Data, remoteURL string) (string, error) {
	if service == nil {
		return "", errors.New("vision adapter: nil service")
	}
	if a.pool == nil || a.resolver == nil {
		return "", errors.New("vision adapter: pool or resolver not configured")
	}
	provider, err := a.resolver.GetProviderByUUID(service.Provider)
	if err != nil || provider == nil {
		return "", fmt.Errorf("vision adapter: resolve provider %q: %w", service.Provider, err)
	}

	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		return a.describeViaAnthropic(ctx, provider, service.Model, mediaType, b64Data, remoteURL)
	case protocol.APIStyleOpenAI:
		return a.describeViaOpenAI(ctx, provider, service.Model, mediaType, b64Data, remoteURL)
	default:
		return "", fmt.Errorf("vision adapter: api_style %q not supported", provider.APIStyle)
	}
}

func (a *poolVisionClient) describeViaAnthropic(ctx context.Context, provider *typ.Provider, model, mediaType, b64Data, remoteURL string) (string, error) {
	var imageBlock anthropic.BetaContentBlockParamUnion
	switch {
	case b64Data != "":
		imageBlock = anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      b64Data,
			MediaType: anthropic.BetaBase64ImageSourceMediaType(mediaType),
		})
	case remoteURL != "":
		imageBlock = anthropic.NewBetaImageBlock(anthropic.BetaURLImageSourceParam{URL: remoteURL})
	default:
		return "", errors.New("vision adapter: no image source")
	}

	c := a.pool.GetAnthropicClient(ctx, provider, model)
	if c == nil {
		return "", errors.New("vision adapter: pool returned nil anthropic client")
	}
	// Streaming variant: most providers (especially proxy gateways and
	// regional Anthropic-compatible vendors) require the streaming endpoint
	// for vision requests. Use the shared SDK assembler to fold the events
	// back into a *BetaMessage so we surface a non-streaming result without
	// hand-rolling the accumulation logic.
	stream := c.BetaMessagesNewStreaming(ctx, &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: defaultVisionMaxTokens,
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					imageBlock,
					{OfText: &anthropic.BetaTextBlockParam{Text: a.prompt}},
				},
			},
		},
	})
	defer stream.Close()
	asm := assembler.NewAnthropicBetaSDKAssembler()
	for stream.Next() {
		if err := asm.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("vision adapter: accumulate beta stream: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	msg := asm.Finish()
	var sb strings.Builder
	for _, b := range msg.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// describeViaOpenAI calls the OpenAI ChatCompletions endpoint with one user
// message containing the image (as a data: URL for base64, or the remote URL
// as-is) plus the describe prompt. The first non-empty assistant text is
// returned; empty/error responses bubble up to the processor's fail-strip.
func (a *poolVisionClient) describeViaOpenAI(ctx context.Context, provider *typ.Provider, model, mediaType, b64Data, remoteURL string) (string, error) {
	imageURL, err := openAIImageURL(mediaType, b64Data, remoteURL)
	if err != nil {
		return "", err
	}

	c := a.pool.GetOpenAIClient(ctx, provider, model)
	if c == nil {
		return "", errors.New("vision adapter: pool returned nil openai client")
	}
	// Streaming variant: many OpenAI-compatible vision endpoints
	// (Qwen-VL, GLM-4V, custom proxies, etc.) only support streaming for
	// multimodal inputs. Use the shared SDK assembler to fold the chunks
	// back into a *ChatCompletion.
	stream := c.ChatCompletionsNewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		MaxTokens: openai.Int(defaultVisionMaxTokens),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfArrayOfContentParts: []openai.ChatCompletionContentPartUnionParam{
							{OfImageURL: &openai.ChatCompletionContentPartImageParam{
								ImageURL: openai.ChatCompletionContentPartImageImageURLParam{URL: imageURL},
							}},
							{OfText: &openai.ChatCompletionContentPartTextParam{Text: a.prompt}},
						},
					},
				},
			},
		},
	})
	defer stream.Close()
	asm := assembler.NewOpenAIStreamAssembler()
	for stream.Next() {
		if !asm.AddChunk(stream.Current()) {
			return "", errors.New("vision adapter: openai stream accumulator rejected chunk")
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	resp := asm.Finish()
	for _, ch := range resp.Choices {
		if text := strings.TrimSpace(ch.Message.Content); text != "" {
			logrus.Debugf("openai: image description: %s", text)
			return text, nil
		}
	}

	return "", nil
}

// openAIImageURL builds the URL string OpenAI's image_url content part
// expects: a data: URL for base64 sources, or the passthrough remote URL.
// Mirrors betaImageBlockToOpenAIURL so both conversion paths render
// identically when the same image is sent through different transports.
func openAIImageURL(mediaType, b64Data, remoteURL string) (string, error) {
	switch {
	case b64Data != "":
		mt := mediaType
		if mt == "" {
			mt = "image/png"
		}
		return "data:" + mt + ";base64," + b64Data, nil
	case remoteURL != "":
		return remoteURL, nil
	default:
		return "", errors.New("vision adapter: no image source")
	}
}
