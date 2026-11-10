package protocol

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamErrorEvent builds the error openai-go returns for an in-stream event
// whose payload has a top-level "error" object, as the Codex backend sends.
func streamErrorEvent(errorObject string) *ssestream.StreamError {
	data := `{"type":"error","sequence_number":9,"error":` + errorObject + "}\n"
	return &ssestream.StreamError{
		Message: "received error while streaming: " + errorObject,
		Event:   ssestream.Event{Type: "error", Data: []byte(data)},
	}
}

func codeError(code string) *ssestream.StreamError {
	return streamErrorEvent(fmt.Sprintf(`{"type":"invalid_request","code":%q,"message":"flagged","param":null}`, code))
}

func TestAsContentPolicyBlock(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string // "" = not a block
	}{
		{"cyber_policy", codeError("cyber_policy"), "cyber_policy"},
		{"bio_policy", codeError("bio_policy"), "bio_policy"},
		{"misalignment_policy_violation", codeError("misalignment_policy_violation"), "misalignment_policy_violation"},
		{"wrapped by CodexClient", fmt.Errorf("stream error: %w", codeError("cyber_policy")), "cyber_policy"},
		{"wrapped by EmptyResponseError", &EmptyResponseError{Err: codeError("cyber_policy")}, "cyber_policy"},
		{"transient code", codeError("server_error"), ""},
		{"policy-like code outside the allowlist", codeError("invalid_prompt"), ""},
		{"no code", streamErrorEvent(`{"message":"boom"}`), ""},
		{"HTTP 400 SDK error is a different path", &openai.Error{StatusCode: 400, Code: "cyber_policy"}, ""},
		{"plain error", errors.New("boom"), ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block, ok := AsContentPolicyBlock(tc.err)
			assert.Equal(t, tc.want != "", ok)
			assert.Equal(t, tc.want, block.Code)
		})
	}
}

func TestAsContentPolicyBlock_Fields(t *testing.T) {
	long := strings.Repeat("x", 600)
	block, ok := AsContentPolicyBlock(streamErrorEvent(
		`{"type":"invalid_request","code":"misalignment_policy_violation","message":"` + long + `","misalignment":{"category":"x"},"param":"input"}`))
	require.True(t, ok)
	assert.Equal(t, "invalid_request", block.Type)
	assert.Equal(t, truncateClientDetail(long), block.Message, "client message is capped like every relayed vendor message")
	assert.Less(t, len(block.Message), len(long))
	assert.JSONEq(t, `{"category":"x"}`, string(block.Misalignment))

	plain, ok := AsContentPolicyBlock(codeError("cyber_policy"))
	require.True(t, ok)
	assert.Nil(t, plain.Misalignment)
}

// The text health monitoring sees must not carry the vendor message, which
// ReportHealthStatus scans for "429", "401", "403" and similar substrings.
func TestContentPolicyBlockError(t *testing.T) {
	inner := streamErrorEvent(`{"code":"cyber_policy","message":"rate limit 429 403 unauthorized"}`)
	err := &ContentPolicyBlockError{Code: "cyber_policy", Err: inner}
	assert.Equal(t, "upstream blocked the request by content policy (code=cyber_policy)", err.Error())
	for _, s := range []string{"429", "401", "403", "rate limit", "unauthorized", "forbidden"} {
		assert.NotContains(t, err.Error(), s)
	}
	var se *ssestream.StreamError
	require.True(t, errors.As(err, &se), "the original error stays reachable for the server log")
	assert.Contains(t, UpstreamLogDetail(err), "rate limit 429")
}

func TestUpstreamLogDetail_StreamErrorEvent(t *testing.T) {
	detail := UpstreamLogDetail(fmt.Errorf("stream error: %w", codeError("cyber_policy")))
	assert.Contains(t, detail, `event="error"`)
	assert.Contains(t, detail, `"sequence_number":9`, "the whole event payload, not only the error object")
	assert.Contains(t, detail, `"code":"cyber_policy"`)
	assert.False(t, strings.HasSuffix(detail, "\n"))
}
