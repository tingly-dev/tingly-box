package visionproxy

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// defaultDescribeCache is the process-wide describe cache used by
// VisionProxyProcessor when none is explicitly injected. Sharing one
// instance across all processors (rule-level and scenario-level both funnel
// through the same processor anyway, see .design/vision-proxy.md §4.1) keeps
// the capacity bound global instead of accidentally per-processor.
var defaultDescribeCache = newDescribeCache(defaultDescribeCacheCapacity)

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

	// cache maps (session, vision service, image content) to the
	// already-formatted replacement text — see describe_cache.go. Left nil
	// by zero-value construction (as most existing tests do) falls back to
	// defaultDescribeCache via the cache() accessor, so callers that build a
	// VisionProxyProcessor{} literal directly still get caching without
	// having to know about it.
	cache *describeCache
}

// describeCacheFor returns the cache this processor should use: an
// explicitly injected one (tests can set a small/fresh instance to isolate
// cases), or the shared process-wide default.
func (p *VisionProxyProcessor) describeCacheFor() *describeCache {
	if p.cache != nil {
		return p.cache
	}
	return defaultDescribeCache
}

// imageUnavailableText replaces an image when the vision proxy itself fails
// (no usable service, upstream error, empty description). The wording is a
// deliberate error report, not a neutral placeholder: the downstream model
// should understand the image was lost to a gateway-side failure and relay
// that to the user, so the user attributes the problem to the proxy — not to
// their own image or the client.
const imageUnavailableText = "[image error: the vision proxy failed to describe this image, so the gateway removed it from the request. This is a proxy-side failure — tell the user the image could not be processed by the vision proxy.]"

// imageHistoricalText replaces image blocks that appear in messages PRIOR to
// the latest one. Enabling the proxy implies the fallback model is
// text-only, so every image must be removed from the serialized request —
// but describing every historical image via the vision upstream would be
// prohibitively expensive (and is unnecessary: the model is rarely asked
// about images that aren't in the latest turn). Historical images are
// therefore stripped with a fixed marker, while only images in the latest
// message are sent through the vision upstream for description.
const imageHistoricalText = "[image: (omitted from history)]"

// describeConcurrency bounds how many vision upstream calls run in parallel
// for a single request. The upstream round-trip dominates proxy latency, so
// a latest message carrying several images (multi-screenshot tool results,
// image comparisons) should not pay N sequential round-trips; the bound
// keeps a pathological request from opening dozens of connections at once.
const describeConcurrency = 4

// imageRef is one image occurrence, captured while walking the request: the
// extracted source plus a splice callback that writes the replacement text
// back into the exact block the image came from, plus the cache key
// identifying this (session, vision service, image content) triple. Every
// ref targets a distinct slice slot, so splice calls are safe to run from
// concurrent goroutines without locking.
type imageRef struct {
	mediaType string
	b64       string
	remoteURL string
	cacheKey  visionCacheKey
	splice    func(text string)
}

// newVisionCacheKey builds the cache key for one image occurrence. provider
// and model come from usable (nil when no vision service is configured —
// still a valid, if useless, key: without a service nothing ever gets
// written to the cache, so it can only ever miss).
func newVisionCacheKey(session string, usable *loadbalance.Service, mediaType, b64, remoteURL string) visionCacheKey {
	var provider, model string
	if usable != nil {
		provider, model = usable.Provider, usable.Model
	}
	content := hashURLImage(remoteURL)
	if remoteURL == "" {
		content = hashBase64Image(mediaType, b64)
	}
	return visionCacheKey{session: session, provider: provider, model: model, content: content}
}

// newImageRef bundles one image occurrence's extracted source, its cache
// key, and the splice-back callback. Every collectXxx call site needs the
// same four fields together, so centralizing the construction here keeps
// them from drifting out of sync (e.g. a cache key built from a different
// mediaType/b64/remoteURL triple than the one actually described).
func newImageRef(session string, usable *loadbalance.Service, mediaType, b64, remoteURL string, splice func(text string)) imageRef {
	return imageRef{
		mediaType: mediaType,
		b64:       b64,
		remoteURL: remoteURL,
		cacheKey:  newVisionCacheKey(session, usable, mediaType, b64, remoteURL),
		splice:    splice,
	}
}

// Process mutates req in place: every image block becomes a text block. On
// any failure (no usable service, vision client error, empty upstream
// response) the image is still removed so a downstream text-only model does
// not choke on an unsupported content block. services is the candidate
// upstream pool — the first active, resolvable service is used; pass a
// single already-resolved service for the common case. sessionID scopes the
// describe cache — see describe_cache.go and .design/vision-proxy.md §10 for
// why a session boundary, not just image content, decides cache reuse.
func (p *VisionProxyProcessor) Process(ctx context.Context, req any, services []*loadbalance.Service, sessionID typ.SessionID) error {
	if req == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Resolved once, up front: both the describe fan-out and every cache key
	// built during the walk need the same (session, service) pair.
	usable := p.pickUsableService(services)
	cache := p.describeCacheFor()
	session := sessionID.String()

	// Phase 1 — walk the request: images whose cache key already has a
	// description get it spliced in immediately; historical images that
	// miss fall back to the fixed marker (no upstream cost); latest-message
	// images that miss are collected for the describe fan-out.
	var refs []imageRef
	switch req := req.(type) {
	case *anthropic.BetaMessageNewParams:
		refs = collectBeta(req, session, usable, cache)
	case *anthropic.MessageNewParams:
		refs = collectV1(req, session, usable, cache)
	case *openai.ChatCompletionNewParams:
		refs = collectOpenAI(req, session, usable, cache)
	case *responses.ResponseNewParams:
		refs = collectResponses(req, session, usable, cache)
	default:
		// Unknown request shape — leave it alone.
		return nil
	}
	if len(refs) == 0 {
		return nil
	}

	// Phase 2 — describe the collected images (concurrently when there is
	// more than one) and splice the text back in.
	p.describeAll(ctx, usable, cache, refs)
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

// describeAll resolves every collected ref via the vision upstream and
// splices the replacement text into the request. The semaphore is acquired
// BEFORE each goroutine is spawned, so describeConcurrency bounds live
// goroutines as well as in-flight upstream calls. A successful description
// is written to cache before splicing so the next occurrence of the same
// (session, service, image) key — retry, failover, repeated tool call — can
// skip the upstream call entirely; fail-strip results are never cached (see
// FR5 in the spec: a transient failure must not be remembered as permanent).
func (p *VisionProxyProcessor) describeAll(ctx context.Context, usable *loadbalance.Service, cache *describeCache, refs []imageRef) {
	sem := make(chan struct{}, describeConcurrency)
	var wg sync.WaitGroup
	for _, r := range refs {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			text := p.safeDescribe(ctx, usable, r)
			if text != imageUnavailableText {
				cache.put(r.cacheKey, text)
			}
			r.splice(text)
		}()
	}
	wg.Wait()
}

// safeDescribe is describe plus panic containment. It runs on goroutines
// describeAll spawns, outside the HTTP handler's recovery middleware — a
// panicking vision client (SDK edge case, malformed upstream stream) must
// fail-strip one image, not crash the process.
func (p *VisionProxyProcessor) safeDescribe(ctx context.Context, usable *loadbalance.Service, r imageRef) (text string) {
	defer func() {
		if rec := recover(); rec != nil {
			logrus.WithContext(ctx).WithField("component", "vision_proxy").
				WithField("panic", rec).Error("vision proxy: describe panicked; stripping image")
			text = imageUnavailableText
		}
	}()
	return p.describe(ctx, usable, r.mediaType, r.b64, r.remoteURL)
}

// describe calls the vision client when available and returns the
// replacement text for the image block. Failures (no service, upstream
// error, empty response) collapse to the fail-strip marker so the
// downstream text-only model never sees an unsupported content block.
//
// Logging deliberately uses no "source" field — that would push the
// entry down MultiLogger's explicit-source branch and skip the
// request_id auto-injection that drives per-request log aggregation.
// See .design/vision-proxy.md §6.3.
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

// spliceOrCollect is the single decision point for what happens to an image
// block. Every image is checked against cache first, regardless of position
// — a hit is spliced in immediately whether the image is in the latest
// message or history, no upstream call either way. On a miss, images outside
// the latest message get the fixed historical marker spliced in immediately
// — no upstream call, no ref. Latest-message misses are appended to refs for
// the describe fan-out.
func spliceOrCollect(cache *describeCache, refs []imageRef, isLast bool, ref imageRef) []imageRef {
	if text, ok := cache.get(ref.cacheKey); ok {
		ref.splice(text)
		return refs
	}
	if !isLast {
		ref.splice(imageHistoricalText)
		return refs
	}
	return append(refs, ref)
}

// collectBeta walks every message of a Beta request. Image blocks occur both
// at the top level and nested inside tool_result.Content (see
// .design/vision-proxy.md §6.1) — both shapes are handled.
func collectBeta(req *anthropic.BetaMessageNewParams, session string, usable *loadbalance.Service, cache *describeCache) []imageRef {
	var refs []imageRef
	lastIdx := len(req.Messages) - 1
	for mi := range req.Messages {
		isLast := mi == lastIdx
		blocks := req.Messages[mi].Content
		for bi := range blocks {
			if img := blocks[bi].OfImage; img != nil {
				mediaType, b64, remoteURL := extractBetaImageSource(img)
				refs = spliceOrCollect(cache, refs, isLast, newImageRef(session, usable, mediaType, b64, remoteURL, func(text string) {
					blocks[bi] = anthropic.BetaContentBlockParamUnion{
						OfText: &anthropic.BetaTextBlockParam{Text: text},
					}
				}))
				continue
			}
			tr := blocks[bi].OfToolResult
			if tr == nil {
				continue
			}
			inner := tr.Content
			for ii := range inner {
				img := inner[ii].OfImage
				if img == nil {
					continue
				}
				mediaType, b64, remoteURL := extractBetaImageSource(img)
				refs = spliceOrCollect(cache, refs, isLast, newImageRef(session, usable, mediaType, b64, remoteURL, func(text string) {
					inner[ii] = anthropic.BetaToolResultBlockParamContentUnion{
						OfText: &anthropic.BetaTextBlockParam{Text: text},
					}
				}))
			}
		}
	}
	return refs
}

// collectV1 mirrors collectBeta for the v1 Messages API types.
func collectV1(req *anthropic.MessageNewParams, session string, usable *loadbalance.Service, cache *describeCache) []imageRef {
	var refs []imageRef
	lastIdx := len(req.Messages) - 1
	for mi := range req.Messages {
		isLast := mi == lastIdx
		blocks := req.Messages[mi].Content
		for bi := range blocks {
			if img := blocks[bi].OfImage; img != nil {
				mediaType, b64, remoteURL := extractV1ImageSource(img)
				refs = spliceOrCollect(cache, refs, isLast, newImageRef(session, usable, mediaType, b64, remoteURL, func(text string) {
					blocks[bi] = anthropic.ContentBlockParamUnion{
						OfText: &anthropic.TextBlockParam{Text: text},
					}
				}))
				continue
			}
			tr := blocks[bi].OfToolResult
			if tr == nil {
				continue
			}
			inner := tr.Content
			for ii := range inner {
				img := inner[ii].OfImage
				if img == nil {
					continue
				}
				mediaType, b64, remoteURL := extractV1ImageSource(img)
				refs = spliceOrCollect(cache, refs, isLast, newImageRef(session, usable, mediaType, b64, remoteURL, func(text string) {
					inner[ii] = anthropic.ToolResultBlockParamContentUnion{
						OfText: &anthropic.TextBlockParam{Text: text},
					}
				}))
			}
		}
	}
	return refs
}

// collectOpenAI walks a Chat Completions request. Image parts ride in user
// messages and — since the fork widened tool-result content from text-only to
// the full part union (#1609) — in tool messages too. Missing the tool channel
// leaves the image to be forwarded verbatim, which text-only providers reject
// outright (z.ai code 1210) instead of describing.
func collectOpenAI(req *openai.ChatCompletionNewParams, session string, usable *loadbalance.Service, cache *describeCache) []imageRef {
	var refs []imageRef
	lastIdx := len(req.Messages) - 1
	for mi := range req.Messages {
		var parts []openai.ChatCompletionContentPartUnionParam
		switch {
		case req.Messages[mi].OfUser != nil:
			parts = req.Messages[mi].OfUser.Content.OfArrayOfContentParts
		case req.Messages[mi].OfTool != nil:
			parts = req.Messages[mi].OfTool.Content.OfArrayOfContentParts
		default:
			continue
		}
		isLast := mi == lastIdx
		for pi := range parts {
			ip := parts[pi].OfImageURL
			if ip == nil {
				continue
			}
			mediaType, b64, remoteURL := request.ParseImageURLToAnthropicSource(ip.ImageURL.URL)
			refs = spliceOrCollect(cache, refs, isLast, newImageRef(session, usable, mediaType, b64, remoteURL, func(text string) {
				parts[pi] = openai.ChatCompletionContentPartUnionParam{
					OfText: &openai.ChatCompletionContentPartTextParam{Text: text},
				}
			}))
		}
	}
	return refs
}

// collectResponses mirrors collectBeta/collectV1 for the OpenAI Responses
// API: it walks every input item's content list, replacing each
// `input_image` part with a text part.
//
// Shapes handled: ResponseInputItemUnionParam.OfMessage (EasyInputMessageParam,
// with EasyInputMessageContentUnionParam.OfInputItemContentList) and
// ResponseInputItemUnionParam.OfInputMessage (ResponseInputItemMessageParam,
// content list inline). Tool/function/output items don't carry
// input_image parts in the current SDK union and are left alone.
func collectResponses(req *responses.ResponseNewParams, session string, usable *loadbalance.Service, cache *describeCache) []imageRef {
	items := req.Input.OfInputItemList
	var refs []imageRef
	lastIdx := len(items) - 1
	for mi := range items {
		var list responses.ResponseInputMessageContentListParam
		switch {
		case items[mi].OfMessage != nil:
			list = items[mi].OfMessage.Content.OfInputItemContentList
		case items[mi].OfInputMessage != nil:
			list = items[mi].OfInputMessage.Content
		default:
			continue
		}
		isLast := mi == lastIdx
		for ci := range list {
			img := list[ci].OfInputImage
			if img == nil {
				continue
			}
			mediaType, b64, remoteURL := request.ParseImageURLToAnthropicSource(img.ImageURL.Or(""))
			refs = spliceOrCollect(cache, refs, isLast, newImageRef(session, usable, mediaType, b64, remoteURL, func(text string) {
				list[ci] = responses.ResponseInputContentUnionParam{
					OfInputText: &responses.ResponseInputTextParam{Text: text},
				}
			}))
		}
	}
	return refs
}

func extractBetaImageSource(img *anthropic.BetaImageBlockParam) (mediaType, b64, remoteURL string) {
	if img.Source.OfBase64 != nil {
		return string(img.Source.OfBase64.MediaType), img.Source.OfBase64.Data, ""
	}
	if img.Source.OfURL != nil {
		return "", "", img.Source.OfURL.URL
	}
	return
}

func extractV1ImageSource(img *anthropic.ImageBlockParam) (mediaType, b64, remoteURL string) {
	if img.Source.OfBase64 != nil {
		return string(img.Source.OfBase64.MediaType), img.Source.OfBase64.Data, ""
	}
	if img.Source.OfURL != nil {
		return "", "", img.Source.OfURL.URL
	}
	return
}
