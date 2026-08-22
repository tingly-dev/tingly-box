package probe

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/visionproxy"
)

// These tests pin the vision-axis request shapes at the builder level: the
// same builders feed real probes AND the cURL generator, so an image dropped
// here would silently gut both. The tool-channel Chat case is the builder-level
// regression for issue #1606 (image_url part in role:"tool" content).

// marshalToMap round-trips any params value through JSON for shape assertions.
func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

// findImageURL walks a Chat content-part array and returns the first
// image_url.url value.
func findImageURL(t *testing.T, parts []any) string {
	t.Helper()
	for _, rawPart := range parts {
		part, _ := rawPart.(map[string]any)
		if part["type"] != "image_url" {
			continue
		}
		img, ok := part["image_url"].(map[string]any)
		require.True(t, ok, "image_url payload must be an object, got %v", part)
		url, _ := img["url"].(string)
		return url
	}
	return ""
}

func TestBuildOpenAIChatParams_VisionUser(t *testing.T) {
	m := marshalToMap(t, buildOpenAIChatParams(probeParams{Model: "m", Vision: VisonUser}))
	msgs := m["messages"].([]any)
	require.Len(t, msgs, 1, "vision probes replace the echo system message")

	user := msgs[0].(map[string]any)
	assert.Equal(t, "user", user["role"])
	parts, ok := user["content"].([]any)
	require.True(t, ok)
	assert.Equal(t, visionproxy.FixtureDataURL, findImageURL(t, parts))
}

func TestBuildOpenAIChatParams_VisionTool(t *testing.T) {
	m := marshalToMap(t, buildOpenAIChatParams(probeParams{Model: "m", Vision: VisionTool}))
	msgs := m["messages"].([]any)
	require.Len(t, msgs, 3, "user ask + assistant tool call + tool result")

	assistant := msgs[1].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	require.Len(t, calls, 1)
	call := calls[0].(map[string]any)
	assert.Equal(t, visionproxy.ToolCallID, call["id"])

	tool := msgs[2].(map[string]any)
	assert.Equal(t, "tool", tool["role"])
	assert.Equal(t, visionproxy.ToolCallID, tool["tool_call_id"])
	parts, ok := tool["content"].([]any)
	require.True(t, ok, "tool content must stay a content-part array, got %v", tool["content"])
	assert.Equal(t, visionproxy.FixtureDataURL, findImageURL(t, parts),
		"issue #1606 regression: image_url payload must survive in tool content")

	tools, _ := m["tools"].([]any)
	require.Len(t, tools, 1, "the synthetic capture tool must be declared")
}

func TestBuildOpenAIResponsesParams_VisionUser(t *testing.T) {
	m := marshalToMap(t, buildOpenAIResponsesParams(probeParams{Model: "m", Vision: VisonUser}))
	assert.Nil(t, m["instructions"], "echo instruction must be dropped for vision probes")

	input := m["input"].([]any)
	require.Len(t, input, 1)
	msg := input[0].(map[string]any)
	parts := msg["content"].([]any)
	var url string
	for _, rawPart := range parts {
		part, _ := rawPart.(map[string]any)
		if part["type"] == "input_image" {
			url, _ = part["image_url"].(string)
		}
	}
	assert.Equal(t, visionproxy.FixtureDataURL, url)
}

func TestBuildOpenAIResponsesParams_VisionTool(t *testing.T) {
	m := marshalToMap(t, buildOpenAIResponsesParams(probeParams{Model: "m", Vision: VisionTool}))
	input := m["input"].([]any)
	require.Len(t, input, 3, "user ask + function_call + function_call_output")

	fco := input[2].(map[string]any)
	assert.Equal(t, "function_call_output", fco["type"])
	assert.Equal(t, visionproxy.ToolCallID, fco["call_id"])
	items, ok := fco["output"].([]any)
	require.True(t, ok, "output must be the structured item list, got %v", fco["output"])
	var url string
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if item["type"] == "input_image" {
			url, _ = item["image_url"].(string)
		}
	}
	assert.Equal(t, visionproxy.FixtureDataURL, url)

	tools, _ := m["tools"].([]any)
	require.Len(t, tools, 1)
}

func TestBuildAnthropicMessageParams_VisionUser(t *testing.T) {
	m := marshalToMap(t, buildAnthropicMessageParams(probeParams{Model: "m", Vision: VisonUser}, false))
	assert.Nil(t, m["system"], "echo instruction must be dropped for vision probes")

	msgs := m["messages"].([]any)
	require.Len(t, msgs, 1)
	blocks := msgs[0].(map[string]any)["content"].([]any)
	var data string
	for _, rawBlock := range blocks {
		block, _ := rawBlock.(map[string]any)
		if block["type"] == "image" {
			source, _ := block["source"].(map[string]any)
			data, _ = source["data"].(string)
		}
	}
	assert.Equal(t, visionproxy.FixturePNGBase64, data)
}

func TestBuildAnthropicMessageParams_VisionTool(t *testing.T) {
	m := marshalToMap(t, buildAnthropicMessageParams(probeParams{Model: "m", Vision: VisionTool}, false))
	msgs := m["messages"].([]any)
	require.Len(t, msgs, 3, "user ask + assistant tool_use + tool_result")

	result := msgs[2].(map[string]any)
	blocks := result["content"].([]any)
	toolResult := blocks[0].(map[string]any)
	assert.Equal(t, "tool_result", toolResult["type"])
	assert.Equal(t, visionproxy.ToolCallID, toolResult["tool_use_id"])
	var data string
	for _, rawPart := range toolResult["content"].([]any) {
		part, _ := rawPart.(map[string]any)
		if part["type"] == "image" {
			source, _ := part["source"].(map[string]any)
			data, _ = source["data"].(string)
		}
	}
	assert.Equal(t, visionproxy.FixturePNGBase64, data)

	tools, _ := m["tools"].([]any)
	require.Len(t, tools, 1)
}

func TestBuildAnthropicMessageParams_VisionKeepsClaudeCodeHeader(t *testing.T) {
	m := marshalToMap(t, buildAnthropicMessageParams(probeParams{Model: "m", Vision: VisonUser}, true))
	system, ok := m["system"].([]any)
	require.True(t, ok, "Claude Code header must survive vision probes")
	require.Len(t, system, 1, "only the CC header — no echo instruction")
}

func TestProbeGoogleGenerate_VisionRejected(t *testing.T) {
	_, err := probeGoogleGenerate(context.Background(), nil, probeParams{Model: "m", Vision: VisonUser})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vision probes are not supported")
}

func TestValidateE2ERequest_VisionAxis(t *testing.T) {
	base := func() *E2ERequest {
		return &E2ERequest{TargetType: E2ETargetProvider, ProviderUUID: "u", Model: "m"}
	}
	for _, v := range []VisionChannel{"", VisionNone, VisonUser, VisionTool} {
		req := base()
		req.Vision = v
		assert.NoError(t, ValidateE2ERequest(req), "vision=%q should validate", v)
	}
	req := base()
	req.Vision = "banana"
	require.Error(t, ValidateE2ERequest(req))
}
