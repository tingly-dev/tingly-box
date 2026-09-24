package routing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type ctxKey struct{}

type ctxRecordingBalancer struct {
	gotCtx   context.Context
	plainHit bool
}

func (b *ctxRecordingBalancer) SelectService(rule *typ.Rule) (*loadbalance.Service, error) {
	b.plainHit = true
	return rule.Services[0], nil
}

func (b *ctxRecordingBalancer) SelectServiceCtx(ctx context.Context, rule *typ.Rule) (*loadbalance.Service, error) {
	b.gotCtx = ctx
	return rule.Services[0], nil
}

// The balancer's warnings belong on the request's timeline, so the stage
// must hand it the request context when the balancer can take one.
func TestSelectWithRequestContext_PrefersContextAwareBalancer(t *testing.T) {
	lb := &ctxRecordingBalancer{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "req-1")
	rule := &typ.Rule{Services: []*loadbalance.Service{{Provider: "p", Model: "m", Active: true}}}

	svc, err := selectWithRequestContext(lb, ctx, rule)
	require.NoError(t, err)
	assert.Equal(t, "m", svc.Model)
	assert.False(t, lb.plainHit)
	require.NotNil(t, lb.gotCtx)
	assert.Equal(t, "req-1", lb.gotCtx.Value(ctxKey{}))
}
