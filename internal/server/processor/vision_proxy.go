package processor

import (
	"context"
	"fmt"
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
//
// Successfully described images also append their raw description (the
// upstream model's output, before being wrapped into the request-side text
// block) to pctx.Descriptions. The outer handler picks these up and feeds
// them to outputinjector so the descriptions become visible on the response
// side too — see internal/server/vision_proxy.go's applyVisionProxy.
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
		p.processBeta(ctx, req, usable, &pctx.Descriptions)
	case *anthropic.MessageNewParams:
		p.processV1(ctx, req, usable, &pctx.Descriptions)
	case *openai.ChatCompletionNewParams:
		p.processOpenAI(ctx, req, usable, &pctx.Descriptions)
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

// describe calls the vision client when available and returns the
// replacement text for the image block. Failures (no service, upstream
// error, empty response) collapse to the fail-strip marker so the
// downstream text-only model never sees an unsupported content block.
//
// Logging deliberately uses no "source" field — that would push the
// entry down MultiLogger's explicit-source branch and skip the
// request_id auto-injection that drives per-request log aggregation.
// See .design/vision-proxy-scenario.md §9.3.
// describe returns (request-side replacement text, raw description) for an
// image. The replacement is what goes back into the request's text block
// (downstream sees this); the raw description is the upstream model's plain
// output, used for response-side injection. On failure the replacement is
// the fail-strip marker and the raw description is "" — both meaning
// "nothing to surface in the response".
func (p *VisionProxyProcessor) describe(ctx context.Context, usable *loadbalance.Service, mediaType, b64, remoteURL string) (replacement, rawDesc string) {
	base := logrus.WithContext(ctx).WithField("component", "vision_proxy")
	if usable == nil || p.Client == nil {
		base.Warn("vision proxy: no usable service or client; stripping image")
		return imageUnavailableText, ""
	}
	log := base.WithFields(logrus.Fields{
		"vision_provider": usable.Provider,
		"vision_model":    usable.Model,
		"media_type":      mediaType,
	})
	desc, err := p.Client.Describe(ctx, usable, mediaType, b64, remoteURL)
	if err != nil {
		log.WithError(err).Warn("vision proxy: describe failed; stripping image")
		return imageUnavailableText, ""
	}
	if strings.TrimSpace(desc) == "" {
		log.Warn("vision proxy: empty description; stripping image")
		return imageUnavailableText, ""
	}
	log.WithField("description", truncateForLog(desc, 200)).Info("vision proxy: image described")
	return "Here is an [image] with message and is parsed into description [image: " + desc + "]", desc
}

// truncateForLog clips long descriptions so a noisy upstream cannot blow up
// the log line. The "…(+N)" suffix records how much was elided.
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("…(+%d)", len(s)-max)
}

// processBeta replaces every image block in every message with a text
// block: described via the vision upstream for the latest message,
// stripped with imageHistoricalText for earlier ones. Image blocks
// occur both at the top level and nested inside tool_result.Content
// (see .design/vision-proxy-scenario.md §9.1) — walkBetaContent handles
// both shapes.
// recordDesc appends a non-empty raw description to the sink so the
// outer handler can forward them via outputinjector. Centralized so the
// nil-sink case (legacy callers / unit tests) is handled in one place.
func recordDesc(sink *[]string, desc string) {
	if sink == nil || desc == "" {
		return
	}
	*sink = append(*sink, desc)
}

func (p *VisionProxyProcessor) processBeta(ctx context.Context, req *anthropic.BetaMessageNewParams, usable *loadbalance.Service, sink *[]string) {
	if len(req.Messages) == 0 {
		return
	}
	lastIdx := len(req.Messages) - 1
	for mi := range req.Messages {
		p.walkBetaContent(ctx, req.Messages[mi].Content, usable, mi == lastIdx, sink)
	}
}

func (p *VisionProxyProcessor) walkBetaContent(ctx context.Context, blocks []anthropic.BetaContentBlockParamUnion, usable *loadbalance.Service, isLast bool, sink *[]string) {
	for bi := range blocks {
		if blocks[bi].OfImage != nil {
			text, desc := p.betaReplacementText(ctx, blocks[bi].OfImage, usable, isLast)
			recordDesc(sink, desc)
			blocks[bi] = anthropic.BetaContentBlockParamUnion{
				OfText: &anthropic.BetaTextBlockParam{Text: text},
			}
			continue
		}
		if tr := blocks[bi].OfToolResult; tr != nil {
			inner := tr.Content
			for ii := range inner {
				if inner[ii].OfImage == nil {
					continue
				}
				text, desc := p.betaReplacementText(ctx, inner[ii].OfImage, usable, isLast)
				recordDesc(sink, desc)
				inner[ii] = anthropic.BetaToolResultBlockParamContentUnion{
					OfText: &anthropic.BetaTextBlockParam{Text: text},
				}
			}
		}
	}
}

func (p *VisionProxyProcessor) betaReplacementText(ctx context.Context, img *anthropic.BetaImageBlockParam, usable *loadbalance.Service, isLast bool) (replacement, rawDesc string) {
	if !isLast {
		return imageHistoricalText, ""
	}
	mediaType, b64, remoteURL := extractBetaImageSource(img)
	return p.describe(ctx, usable, mediaType, b64, remoteURL)
}

func (p *VisionProxyProcessor) processV1(ctx context.Context, req *anthropic.MessageNewParams, usable *loadbalance.Service, sink *[]string) {
	if len(req.Messages) == 0 {
		return
	}
	lastIdx := len(req.Messages) - 1
	for mi := range req.Messages {
		p.walkV1Content(ctx, req.Messages[mi].Content, usable, mi == lastIdx, sink)
	}
}

func (p *VisionProxyProcessor) walkV1Content(ctx context.Context, blocks []anthropic.ContentBlockParamUnion, usable *loadbalance.Service, isLast bool, sink *[]string) {
	for bi := range blocks {
		if blocks[bi].OfImage != nil {
			text, desc := p.v1ReplacementText(ctx, blocks[bi].OfImage, usable, isLast)
			recordDesc(sink, desc)
			blocks[bi] = anthropic.ContentBlockParamUnion{
				OfText: &anthropic.TextBlockParam{Text: text},
			}
			continue
		}
		if tr := blocks[bi].OfToolResult; tr != nil {
			inner := tr.Content
			for ii := range inner {
				if inner[ii].OfImage == nil {
					continue
				}
				text, desc := p.v1ReplacementText(ctx, inner[ii].OfImage, usable, isLast)
				recordDesc(sink, desc)
				inner[ii] = anthropic.ToolResultBlockParamContentUnion{
					OfText: &anthropic.TextBlockParam{Text: text},
				}
			}
		}
	}
}

func (p *VisionProxyProcessor) v1ReplacementText(ctx context.Context, img *anthropic.ImageBlockParam, usable *loadbalance.Service, isLast bool) (replacement, rawDesc string) {
	if !isLast {
		return imageHistoricalText, ""
	}
	mediaType, b64, remoteURL := extractV1ImageSource(img)
	return p.describe(ctx, usable, mediaType, b64, remoteURL)
}

func (p *VisionProxyProcessor) processOpenAI(ctx context.Context, req *openai.ChatCompletionNewParams, usable *loadbalance.Service, sink *[]string) {
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
		text, desc := p.describe(ctx, usable, mediaType, b64, remoteURL)
		recordDesc(sink, desc)
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
