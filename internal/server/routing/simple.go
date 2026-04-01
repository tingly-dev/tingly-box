package routing

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SimpleSelector provides a simplified API that mimics the old interface
// but uses the pipeline internally. This makes migration easier.
type SimpleSelector struct {
	selector *ServiceSelector
}

// NewSimpleSelector creates a simplified selector
func NewSimpleSelector(selector *ServiceSelector) *SimpleSelector {
	return &SimpleSelector{selector: selector}
}

// SelectService is a drop-in replacement for DetermineProviderAndModelWithScenario.
// It handles everything: session resolution, pipeline execution, provider validation.
//
// Migration is simple - just replace the method name:
//
// Before:
//
//	provider, service, err := s.DetermineProviderAndModelWithScenario(scenario, rule, req, sessionID)
//
// After:
//
//	provider, service, err := s.selector.SelectService(c, scenario, rule, req)
//
// sessionID is automatically resolved and stored in gin context.
func (s *SimpleSelector) SelectService(
	c *gin.Context,
	scenario typ.RuleScenario,
	rule *typ.Rule,
	req interface{},
) (*typ.Provider, *loadbalance.Service, error) {
	// Build context (session ID resolved internally)
	ctx := NewSelectionContext(rule, req, c, scenario)

	// Execute pipeline
	result, err := s.selector.Select(ctx)
	if err != nil {
		return nil, nil, err
	}

	if result.Provider == nil || result.Service == nil {
		return nil, nil, fmt.Errorf("selection returned nil result")
	}

	// Automatically store sessionID in gin context for downstream handlers
	c.Set("tracking_session_id", ctx.SessionID)

	// Store result metadata for observability
	c.Set("routing_source", result.Source)

	return result.Provider, result.Service, nil
}
