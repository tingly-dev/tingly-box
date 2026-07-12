package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestMigrate20260712_DropsUnsupportedPartitions verifies that a persisted
// rule carrying a removed smart-routing position (tool_use) loses ONLY that
// partition: siblings keep routing and the rule stays editable, instead of
// the whole router failing validation at request time.
func TestMigrate20260712_DropsUnsupportedPartitions(t *testing.T) {
	svc := []*loadbalance.Service{{Provider: "p", Model: "m", Weight: 1, Active: true}}
	stale := smartrouting.SmartRouting{
		Description: "tool traffic",
		Ops:         []smartrouting.SmartOp{{Position: "tool_use", Operation: "equals", Value: "x"}},
		Services:    svc,
	}
	valid := smartrouting.SmartRouting{
		Description: "big context",
		Ops:         []smartrouting.SmartOp{{Position: smartrouting.PositionToken, Operation: smartrouting.OpTokenGe, Value: "50000"}},
		Services:    svc,
	}

	c := &Config{Rules: []typ.Rule{
		{UUID: "mixed", SmartEnabled: true, SmartRouting: []smartrouting.SmartRouting{stale, valid}},
		{UUID: "only-stale", SmartEnabled: true, SmartRouting: []smartrouting.SmartRouting{stale}},
		{UUID: "clean", SmartEnabled: true, SmartRouting: []smartrouting.SmartRouting{valid}},
	}}

	migrate20260712(c)

	mixed := c.Rules[0]
	assert.Len(t, mixed.SmartRouting, 1, "stale partition dropped, sibling kept")
	assert.Equal(t, "big context", mixed.SmartRouting[0].Description)
	assert.True(t, mixed.SmartEnabled)

	onlyStale := c.Rules[1]
	assert.Empty(t, onlyStale.SmartRouting)
	assert.False(t, onlyStale.SmartEnabled, "smart routing disabled when no partition survives")

	clean := c.Rules[2]
	assert.Len(t, clean.SmartRouting, 1, "untouched rule keeps its partition")
	assert.True(t, clean.SmartEnabled)
}
