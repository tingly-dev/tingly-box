package tool

import "context"

type usageIdentityKey struct{}

// UsageIdentity survives timeout and hook context wrappers during tool rounds.
// It contains attribution only, never conversation content or credentials.
type UsageIdentity struct {
	UserID   string
	TeamID   string
	Scenario string
	RuleUUID string
}

func WithUsageIdentity(ctx context.Context, identity UsageIdentity) context.Context {
	return context.WithValue(ctx, usageIdentityKey{}, identity)
}

func GetUsageIdentity(ctx context.Context) UsageIdentity {
	identity, _ := ctx.Value(usageIdentityKey{}).(UsageIdentity)
	return identity
}
