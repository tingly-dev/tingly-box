package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol/msgparam"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// These tests exercise the whole rename path against a real HTTP server: the
// request body Anthropic would actually receive, and the stream the client
// actually reads back. The unit tests cover each site in isolation; this is the
// check that the plan reaches the wire and is undone on the way home.

const wireMetadataUserID = `{"device_id":"dev1","account_uuid":"acc1","session_id":"550e8400-e29b-41d4-a716-446655440000"}`

// betaToolUseStream is a minimal but well-formed Anthropic SSE stream whose one
// content block is a tool_use naming the *renamed* tool.
func betaToolUseStream(outboundName string) string {
	blocks := []struct {
		event   string
		payload map[string]any
	}{
		{"message_start", map[string]any{"type": "message_start", "message": map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model": "claude-sonnet-4-6", "content": []any{},
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		}}},
		{"content_block_start", map[string]any{"type": "content_block_start", "index": 0,
			"content_block": map[string]any{
				"type": "tool_use", "id": "toolu_1", "name": outboundName, "input": map[string]any{},
			}}},
		{"content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}},
		{"message_delta", map[string]any{"type": "message_delta",
			"delta": map[string]any{"stop_reason": "tool_use"},
			"usage": map[string]any{"output_tokens": 5}}},
		{"message_stop", map[string]any{"type": "message_stop"}},
	}
	out := ""
	for _, b := range blocks {
		out += sseLine(b.event, b.payload)
	}
	return out
}

// newWireServer serves the given SSE body and records the request body it saw.
func newWireServer(t *testing.T, body string) (*httptest.Server, *[]byte) {
	t.Helper()
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &captured
}

func newWireClient(t *testing.T, baseURL string) *ClaudeClient {
	t.Helper()
	provider := newOAuthProvider()
	provider.APIBase = baseURL
	c, err := NewClaudeClient(context.Background(), provider, "", typ.SessionID{Value: "s"})
	require.NoError(t, err)
	return c
}

// wireToolNames pulls tools[].name out of a serialized request body.
func wireToolNames(t *testing.T, body []byte) []string {
	t.Helper()
	var parsed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	names := make([]string, 0, len(parsed.Tools))
	for _, tool := range parsed.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestWire_BetaStreaming_RenamesEverySiteAndRestoresTheStream(t *testing.T) {
	srv, captured := newWireServer(t, betaToolUseStream("ReadFile"))
	c := newWireClient(t, srv.URL)

	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Tools: []anthropic.BetaToolUnionParam{
			{OfTool: &anthropic.BetaToolParam{Name: "read_file"}},
			{OfTool: &anthropic.BetaToolParam{Name: "mcp__github__get_pull_request"}},
			{OfTool: &anthropic.BetaToolParam{Name: "tingly_box_mcp__webtools__mcp_web_search"}},
		},
		ToolChoice: anthropic.BetaToolChoiceParamOfTool("read_file"),
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("go")),
			msgparam.NewBeta("assistant", []anthropic.BetaContentBlockParamUnion{{
				OfToolUse: &anthropic.BetaToolUseBlockParam{
					ID: "toolu_0", Name: "read_file", Input: map[string]any{},
				},
			}}),
		},
	}
	req.Metadata.UserID = param.NewOpt(wireMetadataUserID)

	stream := c.BetaMessagesNewStreaming(context.Background(), req)
	var streamedToolName string
	for stream.Next() {
		if ev := stream.Current(); ev.Type == "content_block_start" {
			streamedToolName = ev.ContentBlock.Name
		}
	}
	require.NoError(t, stream.Err())
	require.NotEmpty(t, *captured, "server never saw a request body")

	body := string(*captured)

	// tools[]: the bare name folds, both MCP namespaces pass through verbatim.
	assert.Equal(t, []string{
		"ReadFile",
		"mcp__github__get_pull_request",
		"tingly_box_mcp__webtools__mcp_web_search",
	}, wireToolNames(t, *captured))

	// tool_choice and the prior turn's tool_use agree with tools[].
	assert.Contains(t, body, `"tool_choice":{"name":"ReadFile","type":"tool"}`)
	assert.Contains(t, body, `"id":"toolu_0","input":{},"name":"ReadFile","type":"tool_use"`)

	// The whole point: no snake_case spelling of the renamed tool survives
	// anywhere in the outbound body.
	assert.NotContains(t, body, "read_file")

	// ...and the client still sees its own name coming back off the stream.
	assert.Equal(t, "read_file", streamedToolName)
}

// TestWire_V1Streaming_RenamesEverySiteAndRestoresTheStream is the v1 twin of
// the Beta wire test, kept compact: the full assertion set runs once above.
func TestWire_V1Streaming_RenamesEverySiteAndRestoresTheStream(t *testing.T) {
	srv, captured := newWireServer(t, betaToolUseStream("ReadFile"))
	c := newWireClient(t, srv.URL)

	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Tools: []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{Name: "read_file"}},
			{OfTool: &anthropic.ToolParam{Name: "mcp__github__get_pull_request"}},
		},
		ToolChoice: anthropic.ToolChoiceParamOfTool("read_file"),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("go")),
			msgparam.New("assistant", []anthropic.ContentBlockParamUnion{{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID: "toolu_0", Name: "read_file", Input: map[string]any{},
				},
			}}),
		},
	}
	req.Metadata.UserID = param.NewOpt(wireMetadataUserID)

	stream := c.MessagesNewStreaming(context.Background(), req)
	var streamedToolName string
	for stream.Next() {
		if ev := stream.Current(); ev.Type == "content_block_start" {
			streamedToolName = ev.ContentBlock.Name
		}
	}
	require.NoError(t, stream.Err())

	body := string(*captured)
	assert.Equal(t, []string{"ReadFile", "mcp__github__get_pull_request"}, wireToolNames(t, *captured))
	assert.Contains(t, body, `"tool_choice":{"name":"ReadFile","type":"tool"}`)
	assert.Contains(t, body, `"id":"toolu_0","input":{},"name":"ReadFile","type":"tool_use"`)
	assert.NotContains(t, body, "read_file")
	assert.Equal(t, "read_file", streamedToolName)
}

// TestWire_MCPOnlyToolset_LeavesBodyUntouched pins the no-op case: a toolset
// that is entirely MCP produces no rename plan at all, so no middleware is
// attached and the body goes out exactly as the client wrote it.
func TestWire_MCPOnlyToolset_LeavesBodyUntouched(t *testing.T) {
	srv, captured := newWireServer(t, betaToolUseStream("mcp__github__get_pull_request"))
	c := newWireClient(t, srv.URL)

	names := []string{
		"mcp__github__get_pull_request",
		"mcp__linear__create_issue",
		"tingly_box_mcp__advisor__advisor",
	}
	tools := make([]anthropic.BetaToolUnionParam, 0, len(names))
	for _, n := range names {
		tools = append(tools, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{Name: n}})
	}
	req := &anthropic.BetaMessageNewParams{
		Model:     anthropic.Model("claude-sonnet-4-6"),
		MaxTokens: 512,
		Tools:     tools,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("go"))},
	}
	req.Metadata.UserID = param.NewOpt(wireMetadataUserID)

	stream := c.BetaMessagesNewStreaming(context.Background(), req)
	var streamedToolName string
	for stream.Next() {
		if ev := stream.Current(); ev.Type == "content_block_start" {
			streamedToolName = ev.ContentBlock.Name
		}
	}
	require.NoError(t, stream.Err())

	assert.Equal(t, names, wireToolNames(t, *captured))
	assert.Equal(t, "mcp__github__get_pull_request", streamedToolName)
}
