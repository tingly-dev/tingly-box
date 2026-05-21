package routing

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

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
	c.Set("tracking_session_id", ctx.SessionID.String())

	// Store result metadata for observability
	c.Set("routing_source", result.Source)

	// Add debug headers when X-TBE-Debug-Routing is enabled
	debugHeader := c.GetHeader("X-TBE-Debug-Routing")
	logrus.Debugf("[routing-debug] X-TBE-Debug-Routing header = %q", debugHeader)
	if debugHeader == "1" {
		providerName := result.Provider.Name
		modelName := result.Service.Model
		source := result.Source
		c.Header("X-TBE-Selected-Provider", providerName)
		c.Header("X-TBE-Selected-Provider-UUID", result.Provider.UUID)
		c.Header("X-TBE-Selected-Model", modelName)
		c.Header("X-TBE-Routing-Source", source)
		if result.MatchedSmartRuleIndex >= 0 {
			c.Header("X-TBE-Matched-Smart-Rule", fmt.Sprintf("%d", result.MatchedSmartRuleIndex))
		}
		logrus.Infof("[routing-debug] Set debug headers: provider=%s model=%s source=%s", providerName, modelName, source)
	}

	return result.Provider, result.Service, nil
}

// SelectServiceForEmbeddings is a variant of SelectService for embedding requests.
// Embedding requests don't carry chat-style context, so content-based smart routing
// is skipped (load balancing, affinity, and health filters still apply).
func (s *SimpleSelector) SelectServiceForEmbeddings(
	c *gin.Context,
	scenario typ.RuleScenario,
	rule *typ.Rule,
) (*typ.Provider, *loadbalance.Service, error) {
	return s.SelectService(c, scenario, rule, nil)
}

// SelectServiceForImageGeneration is a variant of SelectService for image generation
// requests. Image generation requests don't carry chat-style context, so content-based
// smart routing is skipped (load balancing, affinity, and health filters still apply).
func (s *SimpleSelector) SelectServiceForImageGeneration(
	c *gin.Context,
	scenario typ.RuleScenario,
	rule *typ.Rule,
) (*typ.Provider, *loadbalance.Service, error) {
	return s.SelectService(c, scenario, rule, nil)
}

// SelectServiceForTranslation is a variant of SelectService for translation
// requests. Translation requests don't carry chat-style context, so
// content-based smart routing is skipped.
func (s *SimpleSelector) SelectServiceForTranslation(
	c *gin.Context,
	scenario typ.RuleScenario,
	rule *typ.Rule,
) (*typ.Provider, *loadbalance.Service, error) {
	return s.SelectService(c, scenario, rule, nil)
}
