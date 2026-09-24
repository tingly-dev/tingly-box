package protocolserver

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/client"
)

func TestWithImageFailures_AddsFailuresToResponse(t *testing.T) {
	resp := &openai.ImagesResponse{Created: 1, Data: []openai.Image{{B64JSON: "a"}}}

	plain, err := json.Marshal(withImageFailures(resp, nil))
	require.NoError(t, err)
	assert.NotContains(t, string(plain), "tingly_image_failures")
	assert.Contains(t, string(plain), `"b64_json":"a"`)

	_, failures := client.WithImageFailures(t.Context())
	failures.Add(client.ImageFailure{Index: 2, Requested: 2, Err: errors.New("moderation_blocked")})

	out, err := json.Marshal(withImageFailures(resp, failures))
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(out, &decoded))
	assert.Equal(t, []any{"image 2/2: moderation_blocked"}, decoded["tingly_image_failures"])
	assert.NotEmpty(t, decoded["data"])
}
