package vmodelclient_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// The OpenAI SDK pointed at HTTPBase over NewTransport must reach the server
// and see injected statuses as real HTTP errors.
func TestNewTransport_SDKRoundTrip(t *testing.T) {
	srv := virtualserver.Serve(virtualserver.NewService())
	defer srv.Close()

	apiBase := vmodelclient.APIBase(protocol.APIStyleOpenAI)
	client := openai.NewClient(
		openaiopt.WithBaseURL(vmodelclient.HTTPBase(apiBase, protocol.APIStyleOpenAI)),
		openaiopt.WithAPIKey("EMPTY"),
		openaiopt.WithHTTPClient(&http.Client{Transport: vmodelclient.NewTransport(srv.DialContext)}),
		openaiopt.WithMaxRetries(0),
	)
	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model:    "echo-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Choices[0].Message.Content, "Echo")

	_, err = client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model:    "virtual-fail-429",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("x")},
	})
	var apiErr *openai.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 429, apiErr.StatusCode)
}
