package nonstream

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"
)

func TestResponsesToChatRetainsVisibleReasoningSummary(t *testing.T) {
	var resp responses.Response
	require.NoError(t, json.Unmarshal([]byte(`{"id":"r1","status":"completed","output":[{"type":"reasoning","id":"rs1","summary":[{"type":"summary_text","text":"visible summary"}]}]}`), &resp))
	chat := ConvertResponsesToOpenAIChat(&resp, "model")
	require.Equal(t, "visible summary", chat.Choices[0].Message.ReasoningContent)
}

func TestResponsesChatNonstreamIncompleteToolCallKeepsTruncation(t *testing.T) {
	var resp responses.Response
	require.NoError(t, json.Unmarshal([]byte(`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"function_call","id":"fc1","call_id":"call1","name":"lookup","arguments":"{\"id\":"}]}`), &resp))
	require.Equal(t, "length", ConvertResponsesToOpenAIChat(&resp, "model").Choices[0].FinishReason)
}
