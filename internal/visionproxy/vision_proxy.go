package visionproxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// providerResolver is the subset of routing.ProviderResolver this package
// needs. Defined locally so this package does not depend on
// internal/server/routing.
type providerResolver interface {
	GetProviderByUUID(uuid string) (*typ.Provider, error)
}

// VisionProxyProcessor rewrites a typed request in place: every image
// content block becomes a text block, either the vision upstream's
// description (latest message) or a fixed omitted-marker (history). It is
// the image-rewriting engine behind Service.Apply — see service.go for the
// rule/scenario resolution that picks the upstream and invokes Process.
type VisionProxyProcessor struct {
	Client   VisionClient
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

// Process mutates req in place: every image block becomes a text block. On
// any failure (no usable service, vision client error, empty upstream
// response) the image is still removed so a downstream text-only model does
// not choke on an unsupported content block. services is the candidate
// upstream pool — the first active, resolvable service is used; pass a
// single already-resolved service for the common case.
func (p *VisionProxyProcessor) Process(ctx context.Context, req any, services []*loadbalance.Service) error {
	if req == nil {
		return nil
	}
	usable := p.pickUsableService(services)
	if ctx == nil {
		ctx = context.Background()
	}

	switch req := req.(type) {
	case *anthropic.BetaMessageNewParams:
		p.processBeta(ctx, req, usable)
	case *anthropic.MessageNewParams:
		p.processV1(ctx, req, usable)
	case *openai.ChatCompletionNewParams:
		p.processOpenAI(ctx, req, usable)
	case *responses.ResponseNewParams:
		p.processResponses(ctx, req, usable)
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
func (p *VisionProxyProcessor) describe(ctx context.Context, usable *loadbalance.Service, mediaType, b64, remoteURL string) string {
	base := logrus.WithContext(ctx).WithField("component", "vision_proxy")
	if usable == nil || p.Client == nil {
		base.Warn("vision proxy: no usable service or client; stripping image")
		return imageUnavailableText
	}
	log := base.WithFields(logrus.Fields{
		"vision_provider": usable.Provider,
		"vision_model":    usable.Model,
		"media_type":      mediaType,
	})
	desc, err := p.Client.Describe(ctx, usable, mediaType, b64, remoteURL)
	if err != nil {
		log.WithError(err).Warn("vision proxy: describe failed; stripping image")
		return imageUnavailableText
	}
	if strings.TrimSpace(desc) == "" {
		log.Warn("vision proxy: empty description; stripping image")
		return imageUnavailableText
	}
	log.WithField("description", truncateForLog(desc, 200)).Info("vision proxy: image described")
	return "Here is an [image] with message and is parsed into description [image: " + desc + "]"
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
func (p *VisionProxyProcessor) processBeta(ctx context.Context, req *anthropic.BetaMessageNewParams, usable *loadbalance.Service) {
	if len(req.Messages) == 0 {
		return
	}
	lastIdx := len(req.Messages) - 1
	for mi := range req.Messages {
		p.walkBetaContent(ctx, req.Messages[mi].Content, usable, mi == lastIdx)
	}
}

func (p *VisionProxyProcessor) walkBetaContent(ctx context.Context, blocks []anthropic.BetaContentBlockParamUnion, usable *loadbalance.Service, isLast bool) {
	for bi := range blocks {
		if blocks[bi].OfImage != nil {
			blocks[bi] = anthropic.BetaContentBlockParamUnion{
				OfText: &anthropic.BetaTextBlockParam{Text: p.betaReplacementText(ctx, blocks[bi].OfImage, usable, isLast)},
			}
			continue
		}
		if tr := blocks[bi].OfToolResult; tr != nil {
			inner := tr.Content
			for ii := range inner {
				if inner[ii].OfImage == nil {
					continue
				}
				inner[ii] = anthropic.BetaToolResultBlockParamContentUnion{
					OfText: &anthropic.BetaTextBlockParam{Text: p.betaReplacementText(ctx, inner[ii].OfImage, usable, isLast)},
				}
			}
		}
	}
}

func (p *VisionProxyProcessor) betaReplacementText(ctx context.Context, img *anthropic.BetaImageBlockParam, usable *loadbalance.Service, isLast bool) string {
	if !isLast {
		return imageHistoricalText
	}
	mediaType, b64, remoteURL := extractBetaImageSource(img)
	return p.describe(ctx, usable, mediaType, b64, remoteURL)
}

func (p *VisionProxyProcessor) processV1(ctx context.Context, req *anthropic.MessageNewParams, usable *loadbalance.Service) {
	if len(req.Messages) == 0 {
		return
	}
	lastIdx := len(req.Messages) - 1
	for mi := range req.Messages {
		p.walkV1Content(ctx, req.Messages[mi].Content, usable, mi == lastIdx)
	}
}

func (p *VisionProxyProcessor) walkV1Content(ctx context.Context, blocks []anthropic.ContentBlockParamUnion, usable *loadbalance.Service, isLast bool) {
	for bi := range blocks {
		if blocks[bi].OfImage != nil {
			blocks[bi] = anthropic.ContentBlockParamUnion{
				OfText: &anthropic.TextBlockParam{Text: p.v1ReplacementText(ctx, blocks[bi].OfImage, usable, isLast)},
			}
			continue
		}
		if tr := blocks[bi].OfToolResult; tr != nil {
			inner := tr.Content
			for ii := range inner {
				if inner[ii].OfImage == nil {
					continue
				}
				inner[ii] = anthropic.ToolResultBlockParamContentUnion{
					OfText: &anthropic.TextBlockParam{Text: p.v1ReplacementText(ctx, inner[ii].OfImage, usable, isLast)},
				}
			}
		}
	}
}

func (p *VisionProxyProcessor) v1ReplacementText(ctx context.Context, img *anthropic.ImageBlockParam, usable *loadbalance.Service, isLast bool) string {
	if !isLast {
		return imageHistoricalText
	}
	mediaType, b64, remoteURL := extractV1ImageSource(img)
	return p.describe(ctx, usable, mediaType, b64, remoteURL)
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

// processResponses mirrors processBeta/processV1 for the OpenAI Responses
// API: it walks every input item's content list, replacing each
// `input_image` part with a text part. The latest item gets a real
// description via the vision upstream; earlier items get the fixed
// historical marker. Same rationale as processBeta — historical images
// are too costly to describe and rarely needed for the current turn.
//
// Shapes handled: ResponseInputItemUnionParam.OfMessage (EasyInputMessageParam,
// with EasyInputMessageContentUnionParam.OfInputItemContentList) and
// ResponseInputItemUnionParam.OfInputMessage (ResponseInputItemMessageParam,
// content list inline). Tool/function/output items don't carry
// input_image parts in the current SDK union and are left alone.
func (p *VisionProxyProcessor) processResponses(ctx context.Context, req *responses.ResponseNewParams, usable *loadbalance.Service) {
	items := req.Input.OfInputItemList
	if len(items) == 0 {
		return
	}
	lastIdx := len(items) - 1
	for mi := range items {
		p.walkResponsesItem(ctx, &items[mi], usable, mi == lastIdx)
	}
}

func (p *VisionProxyProcessor) walkResponsesItem(ctx context.Context, item *responses.ResponseInputItemUnionParam, usable *loadbalance.Service, isLast bool) {
	if item.OfMessage != nil {
		p.walkResponsesContentList(ctx, item.OfMessage.Content.OfInputItemContentList, usable, isLast)
		return
	}
	if item.OfInputMessage != nil {
		p.walkResponsesContentList(ctx, item.OfInputMessage.Content, usable, isLast)
		return
	}
}

func (p *VisionProxyProcessor) walkResponsesContentList(ctx context.Context, list responses.ResponseInputMessageContentListParam, usable *loadbalance.Service, isLast bool) {
	for i := range list {
		img := list[i].OfInputImage
		if img == nil {
			continue
		}
		text := p.responsesReplacementText(ctx, img, usable, isLast)
		list[i] = responses.ResponseInputContentUnionParam{
			OfInputText: &responses.ResponseInputTextParam{Text: text},
		}
	}
}

func (p *VisionProxyProcessor) responsesReplacementText(ctx context.Context, img *responses.ResponseInputImageParam, usable *loadbalance.Service, isLast bool) string {
	if !isLast {
		return imageHistoricalText
	}
	url := ""
	if img.ImageURL.Valid() {
		url = img.ImageURL.Value
	}
	mediaType, b64, remoteURL := request.ParseImageURLToAnthropicSource(url)
	return p.describe(ctx, usable, mediaType, b64, remoteURL)
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
