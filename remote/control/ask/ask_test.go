package ask

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// --- normalize ---

func TestNormalizeQuestions_Shapes(t *testing.T) {
	q := map[string]any{"question": "pick one"}

	// Canonical shape passes through.
	assert.Equal(t, []map[string]any{q}, NormalizeQuestions([]map[string]any{q}))

	// []interface{} of maps is coerced.
	got := NormalizeQuestions([]interface{}{q, "not-a-map"})
	require.Len(t, got, 1)
	assert.Equal(t, "pick one", got[0]["question"])

	// Anything else yields nil.
	assert.Nil(t, NormalizeQuestions(nil))
	assert.Nil(t, NormalizeQuestions("bogus"))
	assert.Nil(t, NormalizeQuestions(map[string]any{}))
}

// --- registry ---

func TestToolHandlerRegistry_Routing(t *testing.T) {
	registry := NewToolHandlerRegistry()

	// AskUserQuestion routes to its dedicated handler.
	h := registry.FindHandler("AskUserQuestion", nil)
	require.NotNil(t, h)
	assert.IsType(t, &AskUserQuestionHandler{}, h)

	// Everything else falls back to the default handler.
	h = registry.FindHandler("Bash", nil)
	require.NotNil(t, h)
	assert.IsType(t, &DefaultToolHandler{}, h)

	// Both built-ins implement prompt building and response parsing.
	assert.NotNil(t, registry.FindPromptBuilder("AskUserQuestion", nil))
	assert.NotNil(t, registry.FindResponseParser("Bash", nil))
}

// --- AskUserQuestionHandler ---

func questionRequest() Request {
	return Request{
		ID:       "req-1",
		Type:     TypeQuestion,
		ToolName: "AskUserQuestion",
		Input: map[string]interface{}{
			"questions": []interface{}{
				map[string]any{
					"question": "Which color?",
					"header":   "Color",
					"options": []interface{}{
						map[string]any{"label": "Red", "description": "warm"},
						map[string]any{"label": "Blue"},
					},
				},
			},
		},
	}
}

func multiQuestionRequest() Request {
	return Request{
		ID:       "req-multi",
		Type:     TypeQuestion,
		ToolName: "AskUserQuestion",
		Input: map[string]interface{}{
			"questions": []interface{}{
				map[string]any{
					"question": "Which color?",
					"options": []interface{}{
						map[string]any{"label": "Red"},
						map[string]any{"label": "Blue"},
					},
				},
				map[string]any{
					"question": "Which size?",
					"options": []interface{}{
						map[string]any{"label": "Small"},
						map[string]any{"label": "Large"},
					},
				},
			},
		},
	}
}

func TestAskUserQuestionHandler_BuildPrompt(t *testing.T) {
	h := NewAskUserQuestionHandler()

	prompt := h.BuildPrompt(questionRequest(), true)
	assert.Contains(t, prompt, "Which color?")
	assert.Contains(t, prompt, "Red")
	assert.Contains(t, prompt, "Option 2")
	assert.Contains(t, prompt, "Click a button below to select")

	empty := h.BuildPrompt(Request{Input: map[string]interface{}{}}, true)
	assert.Contains(t, empty, "No questions provided")
}

// TestAskUserQuestionHandler_BuildPrompt_NoKeyboard covers the text-only
// fallback: the reply instruction must lead the message (so it survives a
// chat client's notification-preview truncation), the question/options must
// render exactly once (not duplicated by a separately-appended instructions
// block), and the misleading "click a button" trailer must not appear since
// there is no button to click.
func TestAskUserQuestionHandler_BuildPrompt_NoKeyboard(t *testing.T) {
	h := NewAskUserQuestionHandler()

	prompt := h.BuildPrompt(questionRequest(), false)

	instrIdx := strings.Index(prompt, "Reply with the option number")
	require.GreaterOrEqual(t, instrIdx, 0, "reply instruction must be present")
	questionIdx := strings.Index(prompt, "Which color?")
	require.GreaterOrEqual(t, questionIdx, 0)
	assert.Less(t, instrIdx, questionIdx, "reply instruction must come before the question, not after")

	assert.NotContains(t, prompt, "Click a button below to select")
	assert.Equal(t, 1, strings.Count(prompt, "Which color?"), "question must render exactly once, not duplicated")
	assert.Equal(t, 1, strings.Count(prompt, "Red"), "options must render exactly once, not duplicated")
}

func TestAskUserQuestionHandler_ParseResponse(t *testing.T) {
	h := NewAskUserQuestionHandler()
	req := questionRequest()

	// 1-based number, matching the "Option 1"/"Option 2" labels BuildPrompt
	// shows the user — the only shape this ever actually sees, since a
	// button click resolves its index from the callback payload directly
	// and never reaches ParseResponse.
	res, err := h.ParseResponse(req, Response{Type: "text", Data: "1"})
	require.NoError(t, err)
	assert.True(t, res.Approved)
	answers := res.UpdatedInput["answers"].(map[string]interface{})
	assert.Equal(t, "Red", answers["Which color?"])

	// Label match, case-insensitive.
	res, err = h.ParseResponse(req, Response{Type: "text", Data: "red"})
	require.NoError(t, err)
	assert.True(t, res.Approved)
	answers = res.UpdatedInput["answers"].(map[string]interface{})
	assert.Equal(t, "Red", answers["Which color?"])

	// Empty selection is rejected without error.
	res, err = h.ParseResponse(req, Response{Type: "text", Data: "  "})
	require.NoError(t, err)
	assert.False(t, res.Approved)
}

// TestAskUserQuestionHandler_ParseResponse_MultiQuestion guards the exact
// promise BuildPrompt makes for a multi-question fallback ("reply with
// answers in order, e.g. `1 2 1`"): every question must get its answer from
// the matching positional token, not just the first one.
func TestAskUserQuestionHandler_ParseResponse_MultiQuestion(t *testing.T) {
	h := NewAskUserQuestionHandler()
	req := multiQuestionRequest()

	res, err := h.ParseResponse(req, Response{Type: "text", Data: "2 1"})
	require.NoError(t, err)
	assert.True(t, res.Approved)
	answers := res.UpdatedInput["answers"].(map[string]interface{})
	assert.Equal(t, "Blue", answers["Which color?"])
	assert.Equal(t, "Small", answers["Which size?"])

	// Fewer tokens than questions: the answered ones still land, the rest
	// are simply missing (not corrupted by misapplied tokens).
	res, err = h.ParseResponse(req, Response{Type: "text", Data: "1"})
	require.NoError(t, err)
	answers = res.UpdatedInput["answers"].(map[string]interface{})
	assert.Equal(t, "Red", answers["Which color?"])
	assert.NotContains(t, answers, "Which size?")
}

// --- default permission handler ---

// TestBuildDefaultPrompt_NoKeyboard asserts the reply instructions lead the
// message (before the tool/args detail) when the platform has no keyboard,
// so they survive a chat client's notification-preview truncation, and are
// absent when the platform does have one (the keyboard is the instruction).
func TestBuildDefaultPrompt_NoKeyboard(t *testing.T) {
	req := Request{ID: "req-3", ToolName: "Bash", Input: map[string]interface{}{"command": "ls"}}

	withKeyboard := BuildDefaultPrompt(req, true)
	assert.NotContains(t, withKeyboard, "Reply to approve or deny")

	noKeyboard := BuildDefaultPrompt(req, false)
	instrIdx := strings.Index(noKeyboard, "Reply to approve or deny")
	require.GreaterOrEqual(t, instrIdx, 0, "reply instructions must be present")
	toolIdx := strings.Index(noKeyboard, "Tool: `Bash`")
	require.GreaterOrEqual(t, toolIdx, 0)
	assert.Less(t, instrIdx, toolIdx, "reply instructions must come before the tool detail, not after")
}

func TestParseDefaultResponse(t *testing.T) {
	req := Request{ID: "req-2", Input: map[string]interface{}{"command": "ls"}}

	res, err := ParseDefaultResponse(req, Response{Data: "allow"})
	require.NoError(t, err)
	assert.True(t, res.Approved)
	assert.False(t, res.Remember)

	res, err = ParseDefaultResponse(req, Response{Data: "always"})
	require.NoError(t, err)
	assert.True(t, res.Approved)
	assert.True(t, res.Remember)

	res, err = ParseDefaultResponse(req, Response{Data: "deny"})
	require.NoError(t, err)
	assert.False(t, res.Approved)

	res, err = ParseDefaultResponse(req, Response{Data: "gibberish"})
	require.NoError(t, err)
	assert.False(t, res.Approved)
}

func TestParseTextResponse_MatchesPermissionOptions(t *testing.T) {
	approved, remember, ok := ParseTextResponse(" YES ")
	assert.True(t, ok)
	assert.True(t, approved)
	assert.False(t, remember)

	approved, remember, ok = ParseTextResponse("always")
	assert.True(t, ok)
	assert.True(t, approved)
	assert.True(t, remember)

	_, _, ok = ParseTextResponse("maybe")
	assert.False(t, ok)
}

func TestFindPermissionByAction(t *testing.T) {
	opt := FindPermissionByAction("always")
	require.NotNil(t, opt)
	assert.True(t, opt.Approved)
	assert.True(t, opt.Remember)

	assert.Nil(t, FindPermissionByAction("nope"))
}

func TestFormatPermissionInstructions_ListsEveryOption(t *testing.T) {
	instructions := FormatPermissionInstructions()
	for _, opt := range PermissionOptions {
		assert.Contains(t, instructions, opt.Label)
	}
}

// --- agentboot event bridge ---

func TestFromApprovalEvent_And_ToApprovalResponse(t *testing.T) {
	ev := agentboot.ApprovalRequestEvent{
		ID:        "perm-1",
		SessionID: "sess-1",
		AgentType: agentboot.AgentTypeClaude,
		ToolName:  "Bash",
		Input:     map[string]any{"_chat_id": "chat-9", "_platform": "telegram"},
		Reason:    "runs a command",
	}

	req := FromApprovalEvent(ev)
	assert.Equal(t, "perm-1", req.ID)
	assert.Equal(t, TypePermission, req.Type)
	assert.Equal(t, "Bash", req.ToolName)
	// Chat context falls back to the _chat_id/_platform input fields.
	assert.Equal(t, "chat-9", req.ChatID)
	assert.Equal(t, "telegram", req.Platform)

	res := Result{ID: "perm-1", Approved: true, Remember: true, Reason: "ok",
		UpdatedInput: map[string]interface{}{"command": "ls"}}
	resp := res.ToApprovalResponse()
	assert.True(t, resp.Approved)
	assert.Equal(t, "ok", resp.Reason)
	assert.Equal(t, "ls", resp.UpdatedInput["command"])
}
