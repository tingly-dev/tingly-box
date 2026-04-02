package transform

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RequestUnionConstraint defines the exact set of request types accepted by the transform chain.
// This is a compile-time type constraint enforced via generic functions.
// Any attempt to pass a type not in this union will fail at compile time.
type RequestUnionConstraint interface {
	*anthropic.MessageNewParams | *anthropic.BetaMessageNewParams |
		*openai.ChatCompletionNewParams | *responses.ResponseNewParams |
		*protocol.GoogleRequest
}

// TransformConfig holds structured, type-safe configuration for the transform chain.
// Use With* option constructors to set these values.
type TransformConfig struct {
	// MaxTokens is the maximum output tokens allowed for the request.
	// Used by base transform when converting between protocols.
	MaxTokens int64

	// UserID is the OAuth user ID for authenticated requests.
	UserID string

	// Device is the device identifier (e.g., Claude Code device ID).
	Device string

	// OpenAIConfig holds OpenAI-specific configuration populated during transforms.
	OpenAIConfig *protocol.OpenAIConfig

	// ResponsesConfig holds Responses API-specific configuration populated during transforms.
	ResponsesConfig *protocol.OpenAIConfig
}

// TransformOption configures a TransformContext
type TransformOption func(*TransformContext)

// WithProviderURL sets the provider URL in the transform context.
func WithProviderURL(url string) TransformOption {
	return func(ctx *TransformContext) { ctx.ProviderURL = url }
}

// WithProviderType sets the provider type (e.g., "claude_code", "codex") in the transform context.
func WithProviderType(providerType string) TransformOption {
	return func(ctx *TransformContext) { ctx.ProviderType = providerType }
}

// WithScenarioFlags sets the scenario flags in the transform context.
func WithScenarioFlags(flags *typ.ScenarioFlags) TransformOption {
	return func(ctx *TransformContext) { ctx.ScenarioFlags = flags }
}

// WithStreaming sets the streaming flag in the transform context.
func WithStreaming(isStreaming bool) TransformOption {
	return func(ctx *TransformContext) { ctx.IsStreaming = isStreaming }
}

// WithExtra sets initial extra data in the transform context.
func WithExtra(extra map[string]interface{}) TransformOption {
	return func(ctx *TransformContext) { ctx.Extra = extra }
}

// WithMaxTokens sets the maximum output tokens in the transform config.
func WithMaxTokens(maxTokens int64) TransformOption {
	return func(ctx *TransformContext) { ctx.Config.MaxTokens = maxTokens }
}

// WithUserID sets the OAuth user ID in the transform config.
func WithUserID(userID string) TransformOption {
	return func(ctx *TransformContext) { ctx.Config.UserID = userID }
}

// WithDevice sets the device identifier in the transform config.
func WithDevice(device string) TransformOption {
	return func(ctx *TransformContext) { ctx.Config.Device = device }
}

// Transform defines the interface for a single transformation step
type Transform interface {
	// Name returns the unique identifier for this transform
	Name() string

	// Apply applies the transformation to the context
	// Returns an error if the transformation fails
	Apply(ctx *TransformContext) error
}

// TransformContext carries state through the transform chain
type TransformContext struct {
	SourceAPI protocol.APIType
	TargetAPI protocol.APIType

	RequestModel  string
	ResponseModel string

	// Request is the request being transformed.
	// Use SetRequest[T]() to update — only types satisfying RequestUnionConstraint are accepted.
	Request interface{}

	// ProviderURL identifies the provider (e.g., "api.deepseek.com")
	ProviderURL string

	// ProviderType identifies the OAuth provider type (e.g., "claude_code", "codex")
	// This is used for provider-specific model filtering
	ProviderType string

	// ScenarioFlags contains configuration flags for the scenario
	ScenarioFlags *typ.ScenarioFlags

	// IsStreaming indicates if this is a streaming request
	IsStreaming bool

	// OriginalRequest stores the original request before any transformations
	OriginalRequest interface{}

	// TransformSteps records the names of transforms that have been applied
	TransformSteps []string

	// Config holds structured, type-safe configuration for the transform chain.
	Config TransformConfig

	// Extra allows transforms to pass arbitrary data through the chain
	Extra map[string]interface{}
}

// NewTransformContext creates a TransformContext with type-safe request validation.
// The generic type parameter T is constrained to RequestUnionConstraint, ensuring
// only valid request types can be used. Invalid types will cause a compile-time error.
//
// Example:
//
//	ctx := transform.NewTransformContext(&anthropicReq,
//	    transform.WithProviderURL("api.deepseek.com"),
//	    transform.WithStreaming(true),
//	)
func NewTransformContext[T RequestUnionConstraint](request T, opts ...TransformOption) *TransformContext {
	ctx := &TransformContext{
		Request:         request,
		OriginalRequest: request,
	}

	for _, opt := range opts {
		opt(ctx)
	}

	return ctx
}

// SetRequest updates the request in the context.
// Only types satisfying RequestUnionConstraint are accepted — passing any other type
// will result in a compile-time error.
func SetRequest[T RequestUnionConstraint](ctx *TransformContext, req T) {
	ctx.Request = req
}

// configExtraForMetadata builds a legacy extra map from Config fields
// for metadata transforms that still use map[string]any.
func (ctx *TransformContext) configExtraForMetadata() map[string]any {
	extra := map[string]any{}
	if ctx.Config.UserID != "" {
		extra["user_id"] = ctx.Config.UserID
	}
	if ctx.Config.Device != "" {
		extra["device"] = ctx.Config.Device
	}
	// Merge any existing Extra entries (for backward compat with cursor_compat etc.)
	for k, v := range ctx.Extra {
		extra[k] = v
	}
	return extra
}

// TransformChain manages an ordered sequence of transforms
type TransformChain struct {
	// transforms are the ordered transformation steps
	transforms []Transform
}

// NewTransformChain creates a new TransformChain with the given transforms
func NewTransformChain(transforms []Transform) *TransformChain {
	return &TransformChain{
		transforms: transforms,
	}
}

// Execute runs the transform chain on the provided context
// Transforms are executed in order, and each transform's name is recorded
// in TransformSteps. Returns the final TransformContext or an error if
// any transform fails with a descriptive error message.
func (c *TransformChain) Execute(ctx *TransformContext) (*TransformContext, error) {
	// Initialize TransformSteps if not already initialized
	if ctx.TransformSteps == nil {
		ctx.TransformSteps = []string{}
	}

	// Initialize Extra map if not already initialized
	if ctx.Extra == nil {
		ctx.Extra = make(map[string]interface{})
	}

	// Store the original request if not already stored
	if ctx.OriginalRequest == nil {
		ctx.OriginalRequest = ctx.Request
	}

	// Execute transforms in order
	for _, transform := range c.transforms {
		// DESIGN: we allow nil transform and ignore them, this is a design pattern to help upstream build chain
		if transform == nil {
			continue
		}

		// Record the transform name
		transformName := transform.Name()
		ctx.TransformSteps = append(ctx.TransformSteps, transformName)

		// Apply the transformation
		if err := transform.Apply(ctx); err != nil {
			// Return error with context about which transform failed
			return nil, fmt.Errorf("transform '%s' failed: %w", transformName, err)
		}
	}

	return ctx, nil
}

// Add appends a transform to the end of the chain
func (c *TransformChain) Add(transform Transform) {
	c.transforms = append(c.transforms, transform)
}

// Length returns the number of transforms in the chain
func (c *TransformChain) Length() int {
	return len(c.transforms)
}

// GetTransforms returns a copy of the transforms in the chain
func (c *TransformChain) GetTransforms() []Transform {
	transforms := make([]Transform, len(c.transforms))
	copy(transforms, c.transforms)
	return transforms
}
