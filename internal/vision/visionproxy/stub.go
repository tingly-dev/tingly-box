// Test doubles for the vision proxy plugin, reused both by this package's
// own tests and by other packages' tests that need a real Service wired
// through a VisionProxyProcessor (e.g. handler-ordering regression tests).
package visionproxy

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// StubVisionClient returns a canned description; implements the processor's
// (unexported) visionClient interface structurally.
type StubVisionClient struct{ Desc string }

// Describe echoes the service model so tests can assert WHICH service was used.
func (s StubVisionClient) Describe(_ context.Context, svc *loadbalance.Service, _, _, _ string) (string, error) {
	if svc != nil {
		return s.Desc + " via " + svc.Model, nil
	}
	return s.Desc, nil
}

// StubResolver implements the processor's (unexported) providerResolver so
// the configured service is treated as usable.
type StubResolver struct{}

// GetProviderByUUID always returns a usable stub provider.
func (StubResolver) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	return &typ.Provider{UUID: uuid, Name: "stub"}, nil
}

// NewProcessor builds a visionproxy.VisionProxyProcessor wired to the stub
// client/resolver above, echoing "<desc> via <model>" for every described image.
// Gets its own describe cache (rather than falling back to
// defaultDescribeCache) so callers building several stub processors in the
// same test binary don't silently share cached descriptions across them.
func NewProcessor() *VisionProxyProcessor {
	return &VisionProxyProcessor{
		Client:   StubVisionClient{Desc: "desc"},
		Resolver: StubResolver{},
		cache:    newDescribeCache(defaultDescribeCacheCapacity),
	}
}

// ScenarioExt builds the Extensions map a ScenarioConfig would carry for a
// scenario-level vision proxy service.
func ScenarioExt(provider, model string) map[string]interface{} {
	return map[string]interface{}{
		constant.ExtensionVisionProxyService: map[string]interface{}{
			"provider": provider,
			"model":    model,
		},
	}
}

// RuleWithService builds a *typ.Rule carrying a rule-level vision proxy service.
func RuleWithService(provider, model string) *typ.Rule {
	return &typ.Rule{Flags: typ.RuleFlags{
		VisionProxyService: &typ.VisionProxyService{Provider: provider, Model: model},
	}}
}
