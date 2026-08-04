package ask

import (
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

func TestAskUserQuestionHandler_BuildPrompt(t *testing.T) {
	h := NewAskUserQuestionHandler()

	prompt := h.BuildPrompt(questionRequest())
	assert.Contains(t, prompt, "Which color?")
	assert.Contains(t, prompt, "Red")
	assert.Contains(t, prompt, "Option 2")

	empty := h.BuildPrompt(Request{Input: map[string]interface{}{}})
	assert.Contains(t, empty, "No questions provided")
}

func TestAskUserQuestionHandler_ParseResponse(t *testing.T) {
	h := NewAskUserQuestionHandler()
	req := questionRequest()

	// 0-based index from a keyboard callback.
	res, err := h.ParseResponse(req, Response{Type: "button", Data: "1"})
	require.NoError(t, err)
	assert.True(t, res.Approved)
	answers := res.UpdatedInput["answers"].(map[string]interface{})
	assert.Equal(t, "Blue", answers["Which color?"])

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

// --- default permission handler ---

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
