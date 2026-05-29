package virtualserver

import (
	"context"
	"net/http/httptest"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/vmodel"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
)

// These tests consume the virtualserver's Anthropic stream through the OFFICIAL
// Anthropic SDK (Messages.NewStreaming + Message.Accumulate), rather than
// hand-parsing SSE. The SDK's accumulator enforces the real wire protocol —
// every content block must be bracketed by content_block_start / _stop around
// its deltas — so these act as a protocol-compliance guard for any real-SDK
// consumer (e.g. internal/afk). A bare content_block_delta with no preceding
// start makes Accumulate fail, which is exactly the regression we want caught
// here in vmodel rather than downstream.

// newSDKClient mounts the virtualserver at /v1 and returns an SDK client
// pointed at it (the SDK appends /v1/messages to the base URL).
func newSDKClient(t *testing.T, models ...anthropicvm.VirtualModel) sdk.Client {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := NewService()
	reg := svc.GetAnthropicRegistry()
	for _, m := range models {
		require.NoError(t, reg.Register(m))
	}

	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	return sdk.NewClient(
		option.WithBaseURL(srv.URL),
		option.WithAPIKey("test-key"),
	)
}

// accumulate drives a streaming request to completion through the SDK
// accumulator and returns the fully accumulated message. A protocol violation
// surfaces as a stream error here.
func accumulate(t *testing.T, client sdk.Client, model, prompt string) sdk.Message {
	t.Helper()
	stream := client.Messages.NewStreaming(context.Background(), sdk.MessageNewParams{
		Model:     sdk.Model(model),
		MaxTokens: 1024,
		Messages:  []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock(prompt))},
	})
	msg := sdk.Message{}
	for stream.Next() {
		require.NoError(t, msg.Accumulate(stream.Current()), "SDK accumulate rejected a stream event")
	}
	require.NoError(t, stream.Err(), "SDK stream error")
	return msg
}

// TestSDKStream_TextBlockBracketing verifies a text response streams as a
// protocol-valid start→delta(s)→stop sequence the SDK can accumulate.
func TestSDKStream_TextBlockBracketing(t *testing.T) {
	// The built-in static mock returns fixed text.
	client := newSDKClient(t) // default registry includes "virtual-claude-3"
	msg := accumulate(t, client, "virtual-claude-3", "hello")

	require.NotEmpty(t, msg.Content, "expected at least one content block")
	var text string
	for _, b := range msg.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	assert.Contains(t, text, "virtual Claude 3")
}

// TestSDKStream_ToolUseBlock verifies a tool_use response streams as a
// protocol-valid block the SDK can accumulate into a ToolUseBlock.
func TestSDKStream_ToolUseBlock(t *testing.T) {
	toolModel := anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID:   "sdk-tool-mock",
		Name: "sdk-tool-mock",
		ToolCall: &vmodel.ToolCallConfig{
			Name:      "get_weather",
			Arguments: map[string]interface{}{"city": "Paris"},
		},
	})
	client := newSDKClient(t, toolModel)
	msg := accumulate(t, client, "sdk-tool-mock", "weather in Paris?")

	assert.Equal(t, sdk.StopReasonToolUse, msg.StopReason)
	var sawTool bool
	for _, b := range msg.Content {
		if tu, ok := b.AsAny().(sdk.ToolUseBlock); ok {
			sawTool = true
			assert.Equal(t, "get_weather", tu.Name)
		}
	}
	assert.True(t, sawTool, "expected a tool_use block in the accumulated message")
}
