package smart_compact

import (
	"encoding/json"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// --- Test helpers ---

func betaToolUse(id, name, input string) anthropic.BetaContentBlockParamUnion {
	return anthropic.BetaContentBlockParamUnion{
		OfToolUse: &anthropic.BetaToolUseBlockParam{
			ID:    id,
			Name:  name,
			Input: []byte(input),
		},
	}
}

func betaToolResult(toolUseID, content string, isError bool) anthropic.BetaContentBlockParamUnion {
	result := &anthropic.BetaToolResultBlockParam{
		ToolUseID: toolUseID,
		Content: []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: content}},
		},
	}
	if isError {
		result.IsError = anthropic.Bool(true)
	}
	return anthropic.BetaContentBlockParamUnion{OfToolResult: result}
}

func betaUserMsg(blocks ...anthropic.BetaContentBlockParamUnion) anthropic.BetaMessageParam {
	return anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleUser,
		Content: blocks,
	}
}

func betaAssistantMsg(blocks ...anthropic.BetaContentBlockParamUnion) anthropic.BetaMessageParam {
	return anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: blocks,
	}
}

func betaText(text string) anthropic.BetaContentBlockParamUnion {
	return anthropic.BetaContentBlockParamUnion{
		OfText: &anthropic.BetaTextBlockParam{Text: text},
	}
}

func newBetaCtx(req *anthropic.BetaMessageNewParams) *transform.TransformContext {
	return transform.NewTransformContext(req)
}

// inputString converts BetaToolUseBlockParam.Input (any) to string for assertions.
func inputString(input any) string {
	switch v := input.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// --- DeduplicationTransform Tests ---

func TestDeduplicationTransform_Name(t *testing.T) {
	tr := NewDeduplicationTransform()
	assert.Equal(t, "deduplication", tr.Name())
}

func TestDeduplicationTransform_UniqueToolCalls_NoChange(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMsg(betaToolUse("c1", "read_file", `{"path":"/a.go"}`)),
			betaUserMsg(betaToolResult("c1", "content of a.go", false)),
			betaAssistantMsg(betaToolUse("c2", "read_file", `{"path":"/b.go"}`)),
			betaUserMsg(betaToolResult("c2", "content of b.go", false)),
		},
	}

	ctx := newBetaCtx(req)
	tr := NewDeduplicationTransform()
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	r1 := got.Messages[1].Content[0].OfToolResult
	r2 := got.Messages[3].Content[0].OfToolResult
	require.NotNil(t, r1)
	require.NotNil(t, r2)
	assert.Equal(t, "content of a.go", r1.Content[0].OfText.Text)
	assert.Equal(t, "content of b.go", r2.Content[0].OfText.Text)
}

func TestDeduplicationTransform_DuplicateToolCalls_KeepsLatest(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMsg(betaToolUse("c1", "read_file", `{"path":"/a.go"}`)),
			betaUserMsg(betaToolResult("c1", "old content", false)),
			betaAssistantMsg(betaToolUse("c2", "read_file", `{"path":"/a.go"}`)),
			betaUserMsg(betaToolResult("c2", "new content", false)),
		},
	}

	ctx := newBetaCtx(req)
	tr := NewDeduplicationTransform()
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	r1 := got.Messages[1].Content[0].OfToolResult
	r2 := got.Messages[3].Content[0].OfToolResult
	require.NotNil(t, r1)
	require.NotNil(t, r2)
	assert.Contains(t, r1.Content[0].OfText.Text, "[Output removed")
	assert.Equal(t, "new content", r2.Content[0].OfText.Text)
}

func TestDeduplicationTransform_MultipleRounds_KeepsOnlyLast(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMsg(betaToolUse("c1", "list_dir", `{"path":"/"}`)),
			betaUserMsg(betaToolResult("c1", "first listing", false)),
			betaAssistantMsg(betaToolUse("c2", "list_dir", `{"path":"/"}`)),
			betaUserMsg(betaToolResult("c2", "second listing", false)),
			betaAssistantMsg(betaToolUse("c3", "list_dir", `{"path":"/"}`)),
			betaUserMsg(betaToolResult("c3", "third listing", false)),
		},
	}

	ctx := newBetaCtx(req)
	tr := NewDeduplicationTransform()
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	r1 := got.Messages[1].Content[0].OfToolResult
	r2 := got.Messages[3].Content[0].OfToolResult
	r3 := got.Messages[5].Content[0].OfToolResult

	assert.Contains(t, r1.Content[0].OfText.Text, "[Output removed")
	assert.Contains(t, r2.Content[0].OfText.Text, "[Output removed")
	assert.Equal(t, "third listing", r3.Content[0].OfText.Text)
}

func TestDeduplicationTransform_DifferentParams_NoPruning(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMsg(betaToolUse("c1", "read_file", `{"path":"/a.go"}`)),
			betaUserMsg(betaToolResult("c1", "content a", false)),
			betaAssistantMsg(betaToolUse("c2", "read_file", `{"path":"/b.go"}`)),
			betaUserMsg(betaToolResult("c2", "content b", false)),
		},
	}

	ctx := newBetaCtx(req)
	tr := NewDeduplicationTransform()
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	r1 := got.Messages[1].Content[0].OfToolResult
	r2 := got.Messages[3].Content[0].OfToolResult
	assert.Equal(t, "content a", r1.Content[0].OfText.Text)
	assert.Equal(t, "content b", r2.Content[0].OfText.Text)
}

func TestDeduplicationTransform_EmptyMessages_NoError(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{Messages: []anthropic.BetaMessageParam{}}
	ctx := newBetaCtx(req)
	tr := NewDeduplicationTransform()
	assert.NoError(t, tr.Apply(ctx))
}

func TestDeduplicationTransform_NoToolCalls_NoChange(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaUserMsg(betaText("hello")),
			betaAssistantMsg(betaText("hi")),
		},
	}
	ctx := newBetaCtx(req)
	tr := NewDeduplicationTransform()
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	assert.Equal(t, "hello", got.Messages[0].Content[0].OfText.Text)
}

// --- PurgeErrorsTransform Tests ---

func TestPurgeErrorsTransform_Name(t *testing.T) {
	tr := NewPurgeErrorsTransform(4)
	assert.Equal(t, "purge_errors", tr.Name())
}

func TestPurgeErrorsTransform_RecentError_NotPruned(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMsg(betaToolUse("c1", "bash", `{"command":"bad_cmd"}`)),
			betaUserMsg(betaToolResult("c1", "command not found", true)),
			betaUserMsg(betaText("try again")),
		},
	}

	ctx := newBetaCtx(req)
	tr := NewPurgeErrorsTransform(4)
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	toolUseBlock := got.Messages[0].Content[0].OfToolUse
	require.NotNil(t, toolUseBlock)
	assert.Equal(t, `{"command":"bad_cmd"}`, inputString(toolUseBlock.Input))
}

func TestPurgeErrorsTransform_OldError_InputPruned(t *testing.T) {
	msgs := []anthropic.BetaMessageParam{
		betaAssistantMsg(betaToolUse("c1", "bash", `{"command":"bad_cmd"}`)),
		betaUserMsg(betaToolResult("c1", "error output", true)),
	}
	for i := 0; i < 4; i++ {
		msgs = append(msgs,
			betaAssistantMsg(betaText("working...")),
			betaUserMsg(betaText("ok")),
		)
	}

	req := &anthropic.BetaMessageNewParams{Messages: msgs}
	ctx := newBetaCtx(req)
	tr := NewPurgeErrorsTransform(4)
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	toolUseBlock := got.Messages[0].Content[0].OfToolUse
	require.NotNil(t, toolUseBlock)
	assert.Contains(t, inputString(toolUseBlock.Input), "[input removed")
}

func TestPurgeErrorsTransform_ErrorOutputPreserved(t *testing.T) {
	msgs := []anthropic.BetaMessageParam{
		betaAssistantMsg(betaToolUse("c1", "bash", `{"command":"bad_cmd"}`)),
		betaUserMsg(betaToolResult("c1", "permission denied", true)),
	}
	for i := 0; i < 4; i++ {
		msgs = append(msgs,
			betaAssistantMsg(betaText("working...")),
			betaUserMsg(betaText("ok")),
		)
	}

	req := &anthropic.BetaMessageNewParams{Messages: msgs}
	ctx := newBetaCtx(req)
	tr := NewPurgeErrorsTransform(4)
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	toolResult := got.Messages[1].Content[0].OfToolResult
	require.NotNil(t, toolResult)
	assert.Equal(t, "permission denied", toolResult.Content[0].OfText.Text)
}

func TestPurgeErrorsTransform_SuccessfulCall_NeverPruned(t *testing.T) {
	msgs := []anthropic.BetaMessageParam{
		betaAssistantMsg(betaToolUse("c1", "read_file", `{"path":"/a.go"}`)),
		betaUserMsg(betaToolResult("c1", "file content", false)),
	}
	for i := 0; i < 10; i++ {
		msgs = append(msgs,
			betaAssistantMsg(betaText("working...")),
			betaUserMsg(betaText("ok")),
		)
	}

	req := &anthropic.BetaMessageNewParams{Messages: msgs}
	ctx := newBetaCtx(req)
	tr := NewPurgeErrorsTransform(4)
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	toolUseBlock := got.Messages[0].Content[0].OfToolUse
	require.NotNil(t, toolUseBlock)
	assert.Equal(t, `{"path":"/a.go"}`, inputString(toolUseBlock.Input))
}

func TestPurgeErrorsTransform_ZeroGracePeriod_Disabled(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{
		Messages: []anthropic.BetaMessageParam{
			betaAssistantMsg(betaToolUse("c1", "bash", `{"command":"bad"}`)),
			betaUserMsg(betaToolResult("c1", "error", true)),
			betaUserMsg(betaText("try again")),
		},
	}

	ctx := newBetaCtx(req)
	tr := NewPurgeErrorsTransform(0)
	require.NoError(t, tr.Apply(ctx))

	got := ctx.Request.(*anthropic.BetaMessageNewParams)
	toolUseBlock := got.Messages[0].Content[0].OfToolUse
	require.NotNil(t, toolUseBlock)
	assert.Equal(t, `{"command":"bad"}`, inputString(toolUseBlock.Input))
}

func TestPurgeErrorsTransform_EmptyMessages_NoError(t *testing.T) {
	req := &anthropic.BetaMessageNewParams{Messages: []anthropic.BetaMessageParam{}}
	ctx := newBetaCtx(req)
	tr := NewPurgeErrorsTransform(4)
	assert.NoError(t, tr.Apply(ctx))
}

// --- Combined chain test ---

func TestStrategyChain_DeduplicationAndPurgeErrors_Combined(t *testing.T) {
	msgs := []anthropic.BetaMessageParam{
		// error call (will be purged after 4 turns)
		betaAssistantMsg(betaToolUse("e1", "bash", `{"command":"bad"}`)),
		betaUserMsg(betaToolResult("e1", "error output", true)),
		// first read_file (will be deduped)
		betaAssistantMsg(betaToolUse("c1", "read_file", `{"path":"/a.go"}`)),
		betaUserMsg(betaToolResult("c1", "old content", false)),
		// second read_file (latest — kept)
		betaAssistantMsg(betaToolUse("c2", "read_file", `{"path":"/a.go"}`)),
		betaUserMsg(betaToolResult("c2", "new content", false)),
	}
	// Add 2 more turns so error is 4 turns old
	msgs = append(msgs,
		betaAssistantMsg(betaText("working...")),
		betaUserMsg(betaText("ok")),
		betaAssistantMsg(betaText("working...")),
		betaUserMsg(betaText("ok")),
	)

	req := &anthropic.BetaMessageNewParams{Messages: msgs}
	ctx := newBetaCtx(req)

	chain := transform.NewTransformChain([]transform.Transform{
		NewDeduplicationTransform(),
		NewPurgeErrorsTransform(4),
	})
	_, err := chain.Execute(ctx)
	require.NoError(t, err)

	got := ctx.Request.(*anthropic.BetaMessageNewParams)

	errorToolUse := got.Messages[0].Content[0].OfToolUse
	assert.Contains(t, inputString(errorToolUse.Input), "[input removed")

	dupeResult := got.Messages[3].Content[0].OfToolResult
	assert.Contains(t, dupeResult.Content[0].OfText.Text, "[Output removed")

	latestResult := got.Messages[5].Content[0].OfToolResult
	assert.Equal(t, "new content", latestResult.Content[0].OfText.Text)
}
