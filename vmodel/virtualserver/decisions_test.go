package virtualserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3"
	openaiopt "github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Decisions over the official OpenAI SDK + in-memory listener: the exact path a
// vmodel provider takes in the gateway.
func TestServe_Decisions(t *testing.T) {
	client := openAISDK(t)

	var raw []byte
	err := client.Post(context.Background(), "decisions", nil, &raw, openaiopt.WithRequestBody("application/json",
		[]byte(`{"model":"decision-first","state":"hi","questions":{"q":{"type":"choice","criteria":{"a":"","b":""}}}}`)))
	require.NoError(t, err)
	assert.Equal(t, "a", gjson.GetBytes(raw, "answers.q.choice").String())

	// Unknown model → 404, no options → 400, both with real statuses.
	for body, want := range map[string]int{
		`{"model":"nope","state":"x","questions":{"q":{"type":"noul"}}}`: http.StatusNotFound,
		`{"model":"decision-first","state":"x"}`:                         http.StatusBadRequest,
		`{"state":"x","questions":{"q":{"type":"noul"}}}`:                http.StatusBadRequest,
	} {
		err := client.Post(context.Background(), "decisions", nil, &raw, openaiopt.WithRequestBody("application/json", []byte(body)))
		var apiErr *openai.Error
		require.ErrorAs(t, err, &apiErr, body)
		assert.Equal(t, want, apiErr.StatusCode, body)
	}
}

// Decision models are discoverable through the OpenAI-style model list and
// seeded into the builtin OpenAI provider, so users can select them in rules.
func TestDecisionModelsAreListedAndSeeded(t *testing.T) {
	client := openAISDK(t)
	page, err := client.Models.List(context.Background())
	require.NoError(t, err)
	var ids []string
	for _, m := range page.Data {
		ids = append(ids, m.ID)
	}
	assert.Contains(t, ids, "decision-first")
	assert.Contains(t, ids, "decision-stable")

	var seeded []string
	for _, p := range NewService().BuildBuiltinProviders() {
		if p.UUID == BuiltinOpenAIUUID {
			seeded = p.VModelDetail.Models
		}
	}
	assert.Contains(t, seeded, "decision-first")
}
