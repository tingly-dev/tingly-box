package transform

import "github.com/tingly-dev/tingly-box/internal/protocol/ops"

// ClaudeCodeVersionTransform carries the claude_code_version rule flag into
// the chain's Extra map so the vendor transform's Claude Code identity rewrite
// (ops.ApplyAnthropic{V1,Beta}MetadataTransform) can pick the profile. It is
// a pre-Vendor stage: it must run before VendorTransform and touches nothing
// else. Only added to the chain when the flag selects a non-legacy version.
type ClaudeCodeVersionTransform struct {
	version string
}

// NewClaudeCodeVersionTransform returns a transform stamping version.
func NewClaudeCodeVersionTransform(version string) *ClaudeCodeVersionTransform {
	return &ClaudeCodeVersionTransform{version: version}
}

func (t *ClaudeCodeVersionTransform) Name() string { return "claude_code_version" }

func (t *ClaudeCodeVersionTransform) Apply(ctx *TransformContext) error {
	if ctx.Extra == nil {
		ctx.Extra = map[string]interface{}{}
	}
	ctx.Extra[ops.ClaudeCodeVersionExtraKey] = t.version
	return nil
}
