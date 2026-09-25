package transform

import "github.com/tingly-dev/tingly-box/internal/protocol/ops"

// ClaudeCodeVersionTransform stamps the claude_code_version flag into
// ctx.Extra for the vendor transform's Claude Code identity rewrite. Runs
// pre-Vendor, only when a native profile is selected.
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
