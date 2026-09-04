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

func TestTransport_NotConnectedFailsFast(t *testing.T) {
	vmodelclient.Connect(nil)
	req, _ := http.NewRequest(http.MethodGet, "http://vmodel.internal/openai/v1/models", nil)
	_, err := vmodelclient.Transport().RoundTrip(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no virtual-model server connected")
}

// The OpenAI SDK pointed at HTTPBase over Transport must reach the server and
// see injected statuses as real HTTP errors.
func TestTransport_SDKRoundTrip(t *testing.T) {
	srv := virtualserver.Serve(virtualserver.NewService())
	defer srv.Close()
	vmodelclient.Connect(srv.DialContext)
	defer vmodelclient.Connect(nil)

	apiBase := vmodelclient.APIBase(protocol.APIStyleOpenAI)
	client := openai.NewClient(
		openaiopt.WithBaseURL(vmodelclient.HTTPBase(apiBase, protocol.APIStyleOpenAI)),
		openaiopt.WithAPIKey("EMPTY"),
		openaiopt.WithHTTPClient(&http.Client{Transport: vmodelclient.Transport()}),
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
