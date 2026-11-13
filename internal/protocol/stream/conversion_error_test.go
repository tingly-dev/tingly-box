package stream

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesConversionRetainsFailureCode(t *testing.T) {
	for _, route := range []string{"chat", "anthropic"} {
		for _, eventType := range []string{"error", "response.failed"} {
			t.Run(route+"/"+eventType, func(t *testing.T) {
				event := map[string]any{"type": eventType, "code": "context_length_exceeded", "param": "input", "message": "too long"}
				if eventType == "response.failed" {
					event["response"] = map[string]any{"error": map[string]any{"code": "context_length_exceeded", "message": "too long"}}
				}
				var c StreamConverter
				if route == "chat" {
					c = NewOpenAIResponsesToChatConverter(newResponsesIterFromEvents([]map[string]any{event}), "model", false)
				} else {
					c = NewOpenAIResponsesToAnthropicConverter(context.Background(), newResponsesIterFromEvents([]map[string]any{event}), "model")
				}
				found := false
				for {
					e, done, err := c.Next()
					if err != nil || done {
						break
					}
					raw, _ := json.Marshal(e)
					if v, ok := AsAnthropicEvent(e); ok {
						raw = []byte(v.RawJSON())
					}
					if gjson.GetBytes(raw, "error.code").Exists() {
						found = true
						require.Equal(t, "context_length_exceeded", gjson.GetBytes(raw, "error.code").String())
						require.Equal(t, "too long", gjson.GetBytes(raw, "error.message").String())
					}
				}
				require.True(t, found)
			})
		}
	}
}

func TestChatAnthropicDoesNotSalvageExplicitFailureAfterFinish(t *testing.T) {
	for _, beta := range []bool{false, true} {
		raw := `data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}` + "\n\n" + `data: {"error":{"code":"context_length_exceeded","message":"usage tail failed"}}` + "\n\n"
		var c StreamConverter
		if beta {
			c = NewOpenAIChatToAnthropicBetaConverter(newChatChunkStream(raw), "model", nil)
		} else {
			c = NewOpenAIChatToAnthropicV1Converter(newChatChunkStream(raw), "model", nil)
		}
		for {
			e, done, err := c.Next()
			if err != nil {
				require.ErrorContains(t, err, "usage tail failed")
				require.Equal(t, "context_length_exceeded", BuildErrorEvent(err, "stream_failed")["error"].(map[string]interface{})["code"])
				break
			}
			require.False(t, done)
			v, _ := AsAnthropicEvent(e)
			require.NotEqual(t, "message_stop", v.Type)
		}
	}
}
