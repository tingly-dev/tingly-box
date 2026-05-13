package processor

import (
	"context"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// visionClient is the small dependency VisionProxyProcessor needs to describe
// an image. The real adapter (RegisterAll wiring in server.go) wraps
// client.ClientPool and dispatches to the appropriate per-service client
// based on the chosen service's provider APIStyle. Tests substitute a fake.
//
// service is the upstream the processor picked from pctx.Services. The
// adapter uses it to resolve which client/provider to call. The fake ignores
// it and just returns canned text.
//
// Returning ("", nil) means "no description available" → fail-strip path.
// Returning a non-nil error is also fail-strip.
type visionClient interface {
	Describe(ctx context.Context, service *loadbalance.Service, mediaType, base64Data, remoteURL string) (string, error)
}

// providerResolver is the subset of routing.ProviderResolver this processor
// needs. Defined locally so the processor package does not depend on
// internal/server/routing.
type providerResolver interface {
	GetProviderByUUID(uuid string) (*typ.Provider, error)
}

// VisionProxyProcessor implements smartrouting.OpProcessor. When a smart-rule
// op carries this processor, the routing stage calls Process with the typed
// request; Process replaces every image content block with a text block
// containing the upstream's description (or a fail-strip marker), then
// returns nil so the pipeline continues.
type VisionProxyProcessor struct {
	Client   visionClient
	Resolver providerResolver
	Logger   *logrus.Logger
}

const imageUnavailableText = "[image: (description unavailable)]"

// imageHistoricalText replaces image blocks that appear in messages PRIOR to
// the latest one. Enabling the proxy implies the fallback model is
// text-only, so every image must be removed from the serialized request —
// but describing every historical image via the vision upstream would be
// prohibitively expensive (and is unnecessary: the model is rarely asked
// about images that aren't in the latest turn). Historical images are
// therefore stripped with a fixed marker, while only images in the latest
// message are sent through the vision upstream for description.
const imageHistoricalText = "[image: (omitted from history)]"

// Process mutates pctx.Request in place: every image block becomes a text
// block. On any failure (no usable service, vision client error, empty
// upstream response) the image is still removed so a downstream text-only
// model does not choke on an unsupported content block.
func (p *VisionProxyProcessor) Process(pctx *smartrouting.ProcessorContext) error {
	if pctx == nil || pctx.Request == nil {
		return nil
	}
	usable := p.pickUsableService(pctx.Services)
	ctx := pctx.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	switch req := pctx.Request.(type) {
	case *anthropic.BetaMessageNewParams:
		p.processBeta(ctx, req, usable)
	case *anthropic.MessageNewParams:
		p.processV1(ctx, req, usable)
	case *openai.ChatCompletionNewParams:
		p.processOpenAI(ctx, req, usable)
	default:
		// Unknown request shape — leave it alone.
	}
	return nil
}

func (p *VisionProxyProcessor) pickUsableService(services []*loadbalance.Service) *loadbalance.Service {
	for _, svc := range services {
		if svc == nil || !svc.Active {
			continue
		}
		if p.Resolver != nil {
			if _, err := p.Resolver.GetProviderByUUID(svc.Provider); err != nil {
				continue
			}
		}
		return svc
	}
	return nil
}

// describe calls the vision client when available, returning the marker text
// for the image's replacement block. Empty / errored responses become the
// fail-strip marker; success becomes "[image: <desc>]".
func (p *VisionProxyProcessor) describe(ctx context.Context, usable *loadbalance.Service, mediaType, b64, remoteURL string) string {
	if usable == nil || p.Client == nil {
		return imageUnavailableText
	}
	desc, err := p.Client.Describe(ctx, usable, mediaType, b64, remoteURL)
	if err != nil {
		if p.Logger != nil {
			p.Logger.Debugf("[vision_proxy] describe failed: %v", err)
		}
		return imageUnavailableText
	}
	if strings.TrimSpace(desc) == "" {
		return imageUnavailableText
	}
	return "Here is an [image] with message and is parsed into description [image: " + desc + "]"
}

// processBeta walks Beta messages and replaces every image content block
// with a text block. Images in messages PRIOR to the last one are stripped
// with a fixed historical marker (no vision call). Images in the LAST
// message are sent to the vision upstream for description.
func (p *VisionProxyProcessor) processBeta(ctx context.Context, req *anthropic.BetaMessageNewParams, usable *loadbalance.Service) {
	if len(req.Messages) == 0 {
		return
	}
	lastIdx := len(req.Messages) - 1
	historicalBlock := anthropic.BetaContentBlockParamUnion{
		OfText: &anthropic.BetaTextBlockParam{Text: imageHistoricalText},
	}
	for mi := 0; mi < lastIdx; mi++ {
		blocks := req.Messages[mi].Content
		for bi := range blocks {
			if blocks[bi].OfImage == nil {
				continue
			}
			blocks[bi] = historicalBlock
		}
	}
	blocks := req.Messages[lastIdx].Content
	for bi := range blocks {
		img := blocks[bi].OfImage
		if img == nil {
			continue
		}
		mediaType, b64, remoteURL := extractBetaImageSource(img)
		text := p.describe(ctx, usable, mediaType, b64, remoteURL)
		blocks[bi] = anthropic.BetaContentBlockParamUnion{
			OfText: &anthropic.BetaTextBlockParam{Text: text},
		}
	}
}

func (p *VisionProxyProcessor) processV1(ctx context.Context, req *anthropic.MessageNewParams, usable *loadbalance.Service) {
	if len(req.Messages) == 0 {
		return
	}
	lastIdx := len(req.Messages) - 1
	historicalBlock := anthropic.ContentBlockParamUnion{
		OfText: &anthropic.TextBlockParam{Text: imageHistoricalText},
	}
	for mi := 0; mi < lastIdx; mi++ {
		blocks := req.Messages[mi].Content
		for bi := range blocks {
			if blocks[bi].OfImage == nil {
				continue
			}
			blocks[bi] = historicalBlock
		}
	}
	blocks := req.Messages[lastIdx].Content
	for bi := range blocks {
		img := blocks[bi].OfImage
		if img == nil {
			continue
		}
		mediaType, b64, remoteURL := extractV1ImageSource(img)
		text := p.describe(ctx, usable, mediaType, b64, remoteURL)
		blocks[bi] = anthropic.ContentBlockParamUnion{
			OfText: &anthropic.TextBlockParam{Text: text},
		}
	}
}

func (p *VisionProxyProcessor) processOpenAI(ctx context.Context, req *openai.ChatCompletionNewParams, usable *loadbalance.Service) {
	if len(req.Messages) == 0 {
		return
	}
	lastIdx := len(req.Messages) - 1
	historicalPart := openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{Text: imageHistoricalText},
	}
	for mi := 0; mi < lastIdx; mi++ {
		um := req.Messages[mi].OfUser
		if um == nil {
			continue
		}
		parts := um.Content.OfArrayOfContentParts
		for pi := range parts {
			if parts[pi].OfImageURL == nil {
				continue
			}
			parts[pi] = historicalPart
		}
	}
	um := req.Messages[lastIdx].OfUser
	if um == nil {
		return
	}
	parts := um.Content.OfArrayOfContentParts
	for pi := range parts {
		ip := parts[pi].OfImageURL
		if ip == nil {
			continue
		}
		mediaType, b64, remoteURL := request.ParseImageURLToAnthropicSource(ip.ImageURL.URL)
		text := p.describe(ctx, usable, mediaType, b64, remoteURL)
		parts[pi] = openai.ChatCompletionContentPartUnionParam{
			OfText: &openai.ChatCompletionContentPartTextParam{Text: text},
		}
	}
}

func extractBetaImageSource(img *anthropic.BetaImageBlockParam) (mediaType, b64, remoteURL string) {
	if img == nil {
		return
	}
	if img.Source.OfBase64 != nil {
		return string(img.Source.OfBase64.MediaType), img.Source.OfBase64.Data, ""
	}
	if img.Source.OfURL != nil {
		return "", "", img.Source.OfURL.URL
	}
	return
}

func extractV1ImageSource(img *anthropic.ImageBlockParam) (mediaType, b64, remoteURL string) {
	if img == nil {
		return
	}
	if img.Source.OfBase64 != nil {
		return string(img.Source.OfBase64.MediaType), img.Source.OfBase64.Data, ""
	}
	if img.Source.OfURL != nil {
		return "", "", img.Source.OfURL.URL
	}
	return
}
