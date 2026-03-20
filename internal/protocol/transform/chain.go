package transform

import (
	"fmt"
	"reflect"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

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
	// Request is the request being transformed
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

	// Extra allows transforms to pass arbitrary data through the chain
	Extra map[string]interface{}
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
	// Validate request type: must be a pointer type for consistency
	if err := validateRequestPointerType(ctx.Request); err != nil {
		return nil, fmt.Errorf("invalid request type: %w", err)
	}

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

// validateRequestPointerType ensures the request is a pointer type
// This is required for consistent behavior across the transform chain
func validateRequestPointerType(req interface{}) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	reqType := reflect.TypeOf(req)
	if reqType.Kind() != reflect.Ptr {
		return fmt.Errorf("request must be a pointer type, got %T (value type)", req)
	}

	return nil
}
