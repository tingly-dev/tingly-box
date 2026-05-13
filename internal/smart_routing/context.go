package smartrouting

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// ServiceCapacityInfo holds seat-capacity info for a single service.
// Capacity is ModelCapacity (configured seat limit); 0 means unlimited.
// ActiveCount is the number of active affinity sessions currently locked to this service.
type ServiceCapacityInfo struct {
	ServiceID   string
	Capacity    int // seat limit (ModelCapacity); 0 = unlimited
	ActiveCount int // active affinity sessions
}

// RequestContext holds extracted request data for evaluation
type RequestContext struct {
	Model             string
	ThinkingEnabled   bool
	SystemMessages    []string
	UserMessages      []string
	ToolUses          []string
	LatestRole        string // Latest message role (user, assistant, tool, function, etc.)
	LatestContentType string
	// HasImage is true when ANY message in the conversation (any role,
	// any position — not just the latest) contains an image content block.
	// proxy_vision uses this because its responsibilities include cleaning
	// historical images from the request before the text-only downstream
	// model sees them — so a rule that only matched on the latest message
	// would let historical images slip through.
	HasImage        bool
	EstimatedTokens int

	// ClaudeCodeRequestKind is one of "main", "subagent", "compact" — populated by the
	// SmartRoutingStage only when the request scenario is claude_code. Empty otherwise.
	ClaudeCodeRequestKind string

	// Service runtime characteristics — populated by SmartRoutingStage before router evaluation.
	// These fields are set per-rule inside evaluateRule to avoid cross-rule contamination.
	ServiceStats    []loadbalance.ServiceStats // TTFT / latency snapshots
	ServiceCapacity []ServiceCapacityInfo      // seat utilization info
}

// GetLatestUserMessage returns the latest user message
func (rc *RequestContext) GetLatestUserMessage() string {
	if len(rc.UserMessages) == 0 {
		return ""
	}
	return rc.UserMessages[len(rc.UserMessages)-1]
}

// CombineMessages combines all messages of a type into a single string
func (rc *RequestContext) CombineMessages(messages []string) string {
	return strings.Join(messages, "\n")
}

// ExtractContext is the canonical entry point for building a RequestContext.
// It funnels every supported wire protocol through the existing protocol/request
// converters into the Anthropic Beta shape, then runs a single extractor against
// it. Returns nil for unrecognised request types.
func ExtractContext(req interface{}) *RequestContext {
	switch r := req.(type) {
	case *anthropic.BetaMessageNewParams:
		return ExtractContextFromBetaRequest(r)
	case *anthropic.MessageNewParams:
		return ExtractContextFromBetaRequest(request.ConvertAnthropicV1ToBetaRequest(r))
	case *openai.ChatCompletionNewParams:
		return ExtractContextFromBetaRequest(request.ConvertOpenAIToAnthropicRequest(r, 0))
	default:
		logrus.Debugf("[smart_routing] unknown request type %T, cannot extract context", req)
		return nil
	}
}

// ExtractContextFromBetaRequest extracts RequestContext from an Anthropic beta messages request
func ExtractContextFromBetaRequest(req *anthropic.BetaMessageNewParams) *RequestContext {
	if req == nil {
		return nil
	}
	ctx := &RequestContext{
		Model:           string(req.Model),
		ThinkingEnabled: req.Thinking.OfEnabled != nil,
	}

	if req.System != nil {
		for _, s := range req.System {
			if s.Text != "" {
				ctx.SystemMessages = append(ctx.SystemMessages, s.Text)
			}
		}
	}

	if req.Messages != nil {
		for _, msg := range req.Messages {
			ctx.LatestRole = string(msg.Role)

			// HasImage tracks images across every role so proxy_vision
			// (which cleans historical images) matches when the image
			// lives in an assistant message or tool result — not only
			// when it's in a user message.
			if hasImageInBetaContent(msg.Content) {
				ctx.HasImage = true
			}

			if string(msg.Role) != "user" {
				continue
			}

			contentStr, toolUses := extractBetaContent(msg.Content)

			if contentStr != "" {
				ctx.UserMessages = append(ctx.UserMessages, contentStr)
			}
			if ctx.HasImage {
				ctx.LatestContentType = "image"
			}

			ctx.ToolUses = append(ctx.ToolUses, toolUses...)
		}
	}

	allContent := strings.Join(append(ctx.SystemMessages, ctx.UserMessages...), "\n")
	ctx.EstimatedTokens = EstimateTokens(allContent)

	return ctx
}

// extractBetaContent extracts string content and tool uses from Beta content blocks
func extractBetaContent(content []anthropic.BetaContentBlockParamUnion) (string, []string) {
	var parts []string
	var tools []string

	for _, blockUnion := range content {
		switch {
		case blockUnion.OfText != nil:
			parts = append(parts, blockUnion.OfText.Text)
		case blockUnion.OfImage != nil:
			parts = append(parts, "[image]")
		case blockUnion.OfToolUse != nil:
			tools = append(tools, blockUnion.OfToolUse.Name)
		}
	}

	return strings.Join(parts, "\n"), tools
}

// hasImageInBetaContent checks if content contains image
func hasImageInBetaContent(content []anthropic.BetaContentBlockParamUnion) bool {
	for _, blockUnion := range content {
		if blockUnion.OfImage != nil {
			return true
		}
	}
	return false
}
