package processor

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
//
// Descriptions emitted during a Process call are also pushed onto a
// DescriptionCollector stashed in ProcessorContext.Extras under
// ExtrasKeyVisionDescriptions (when present). The response-side injector
// consumes that slice to prepend the same descriptions to the model's
// reply — preserving them in the client-side transcript across turns.
type VisionProxyProcessor struct {
	Client   visionClient
	Resolver providerResolver
}

// wrapVisionDescription produces the canonical request- and response-side
// replacement text for a single image description. Leading/trailing \n
// keep the fragment stable when concatenated with surrounding text or
// content parts.
func wrapVisionDescription(body string) string {
	return "\n<image-description>" + body + "</image-description>\n"
}

// imageHistoricalText replaces image blocks that appear in messages PRIOR to
// the latest one. Enabling the proxy implies the fallback model is
// text-only, so every image must be removed from the serialized request —
// but describing every historical image via the vision upstream would be
// prohibitively expensive (and is unnecessary: the model is rarely asked
// about images that aren't in the latest turn). Historical images are
// therefore stripped with a fixed marker, while only images in the latest
// message are sent through the vision upstream for description.
var imageHistoricalText = wrapVisionDescription("(omitted from history)")

// imageUnavailableText is the fail-strip marker emitted when the vision
// upstream returns an error or an empty description.
var imageUnavailableText = wrapVisionDescription("(description unavailable)")

// ExtrasKeyVisionDescriptions is the smartrouting.ProcessorContext.Extras
// key under which VisionProxyProcessor stores every description it
// emits during a request. Downstream layers (the protocol-level stream
// hook and the non-stream gin middleware) read this list to inject the
// same descriptions into the model's response, so the assistant
// transcript carries them naturally into the next turn's history.
const ExtrasKeyVisionDescriptions = "vision_proxy.descriptions"

// DescriptionCollector accumulates the description bodies (unwrapped)
// emitted by a single pass of the vision proxy.
// BuildVisionDescriptionPrefix wraps them at consumption time.
type DescriptionCollector struct {
	items []string
}

// Append records one description body. Nil-safe so describe() can call
// it unconditionally regardless of whether a collector was installed.
func (d *DescriptionCollector) Append(body string) {
	if d == nil {
		return
	}
	d.items = append(d.items, body)
}

// Snapshot returns a copy of the collected description bodies in order.
func (d *DescriptionCollector) Snapshot() []string {
	if d == nil || len(d.items) == 0 {
		return nil
	}
	out := make([]string, len(d.items))
	copy(out, d.items)
	return out
}

// BuildVisionDescriptionPrefix turns description bodies into the text
// the response-side injector prepends before the model's first text:
//
//	<image-description>d1</image-description>
//	<image-description>d2</image-description>
//
//	...model output...
//
// Returns "" for an empty input so callers can treat the no-image case
// as a no-op.
func BuildVisionDescriptionPrefix(bodies []string) string {
	if len(bodies) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, b := range bodies {
		sb.WriteString("<image-description>")
		sb.WriteString(b)
		sb.WriteString("</image-description>\n")
	}
	sb.WriteByte('\n')
	return sb.String()
}

// ctxKeyCollector is the per-Process context key used to thread the
// DescriptionCollector down to describe() without rippling it through
// every walk/replacement signature. Resolved once at Process entry and
// read at each describe() leaf; nil-safe (Append on a nil collector is
// a no-op).
type ctxKeyCollectorType struct{}

var ctxKeyCollector = ctxKeyCollectorType{}

func collectorFromCtx(ctx context.Context) *DescriptionCollector {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(ctxKeyCollector).(*DescriptionCollector)
	return v
}

// resolveCollector pulls the collector out of pctx.Extras under
// ExtrasKeyVisionDescriptions if present; otherwise returns nil (and
// describe()'s appends become no-ops). The caller — server.applyVisionProxy
// — owns construction so the lifetime is per-request.
func resolveCollector(pctx *smartrouting.ProcessorContext) *DescriptionCollector {
	if pctx == nil || pctx.Extras == nil {
		return nil
	}
	c, _ := pctx.Extras[ExtrasKeyVisionDescriptions].(*DescriptionCollector)
	return c
}

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
	if coll := resolveCollector(pctx); coll != nil {
		ctx = context.WithValue(ctx, ctxKeyCollector, coll)
	}

	switch req := pctx.Request.(type) {
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
	coll := collectorFromCtx(ctx)
	base := logrus.WithContext(ctx).WithField("component", "vision_proxy")
	const unavailableBody = "(description unavailable)"
	if usable == nil || p.Client == nil {
		base.Warn("vision proxy: no usable service or client; stripping image")
		coll.Append(unavailableBody)
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
		coll.Append(unavailableBody)
		return imageUnavailableText
	}
	if strings.TrimSpace(desc) == "" {
		log.Warn("vision proxy: empty description; stripping image")
		coll.Append(unavailableBody)
		return imageUnavailableText
	}
	log.WithField("description", truncateForLog(desc, 200)).Info("vision proxy: image described")
	coll.Append(desc)
	return wrapVisionDescription(desc)
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
