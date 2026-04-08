package stream

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamAnthropicV1SingleMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	raw := `{
	  "id":"msg_v1_1",
	  "content":[{"type":"text","text":"hello from v1"}],
	  "model":"claude-3-5-sonnet",
	  "role":"assistant",
	  "stop_reason":"end_turn",
	  "stop_sequence":"",
	  "type":"message",
	  "usage":{"input_tokens":12,"output_tokens":7}
	}`

	var resp anthropic.Message
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := StreamAnthropicV1SingleMessage(c, &resp, "proxy-model")
	require.NoError(t, err)

	body := w.Body.String()
	require.Contains(t, body, "event:message_start")
	require.Contains(t, body, "event:content_block_start")
	require.Contains(t, body, "event:content_block_delta")
	require.Contains(t, body, "hello from v1")
	require.Contains(t, body, "event:message_delta")
	require.Contains(t, body, "event:message_stop")
	require.Contains(t, body, `"model":"proxy-model"`)
}

func TestStreamAnthropicBetaSingleMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	raw := `{
	  "id":"msg_beta_1",
	  "content":[{"type":"text","text":"hello from beta"}],
	  "context_management":{"applied":null},
	  "model":"claude-3-7-sonnet",
	  "role":"assistant",
	  "stop_reason":"end_turn",
	  "stop_sequence":"",
	  "type":"message",
	  "usage":{"input_tokens":20,"output_tokens":9}
	}`

	var resp anthropic.BetaMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := StreamAnthropicBetaSingleMessage(c, &resp, "proxy-beta-model")
	require.NoError(t, err)

	body := w.Body.String()
	require.Contains(t, body, "event:message_start")
	require.Contains(t, body, "event:content_block_start")
	require.Contains(t, body, "event:content_block_delta")
	require.Contains(t, body, "hello from beta")
	require.Contains(t, body, "event:message_delta")
	require.Contains(t, body, "event:message_stop")
	require.Contains(t, body, `"model":"proxy-beta-model"`)
	// Beta emitter also appends a simple trailing data event with message_stop type.
	require.True(t, strings.Contains(body, `"type":"message_stop"`))
}
