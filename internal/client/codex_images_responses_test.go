package client

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResolveCodexImageEditRoute(t *testing.T) {
	t.Run("mask decides when unset", func(t *testing.T) {
		assert.Equal(t, codexImageEditRouteNative, resolveCodexImageEditRoute(false))
		assert.Equal(t, codexImageEditRouteResponses, resolveCodexImageEditRoute(true))
	})

	t.Run("env forces responses without a mask", func(t *testing.T) {
		t.Setenv(codexImageEditRouteEnv, "responses")
		assert.Equal(t, codexImageEditRouteResponses, resolveCodexImageEditRoute(false))
	})

	t.Run("env pins native even with a mask", func(t *testing.T) {
		t.Setenv(codexImageEditRouteEnv, "native")
		assert.Equal(t, codexImageEditRouteNative, resolveCodexImageEditRoute(true))
	})

	t.Run("unknown value falls back to the mask rule", func(t *testing.T) {
		t.Setenv(codexImageEditRouteEnv, "nonsense")
		assert.Equal(t, codexImageEditRouteResponses, resolveCodexImageEditRoute(true))
	})
}

func TestBuildImageEditResponsesRequest_ImageAndMask(t *testing.T) {
	maskBytes := []byte("\x89PNG\r\n\x1a\nfake-mask-data")
	req := &openai.ImageEditParams{
		Prompt:  "replace the sofa",
		Model:   "gpt-image-2",
		Quality: openai.ImageEditParamsQualityStandard,
		Size:    openai.ImageEditParamsSize1024x1536,
	}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")
	req.Mask = openai.File(bytes.NewReader(maskBytes), "mask.png", "image/png")

	params, err := buildImageEditResponsesRequest(req)
	require.NoError(t, err)

	body, err := json.Marshal(params)
	require.NoError(t, err)
	parsed := gjson.ParseBytes(body)

	// The prompt and the reference image ride in one user message.
	content := parsed.Get("input.0.content")
	require.True(t, content.IsArray())
	assert.Equal(t, "input_text", content.Array()[0].Get("type").String())
	assert.Equal(t, "replace the sofa", content.Array()[0].Get("text").String())
	assert.Equal(t, "input_image", content.Array()[1].Get("type").String())
	assert.True(t, strings.HasPrefix(content.Array()[1].Get("image_url").String(), "data:image/png;base64,"))

	// The mask rides on the tool, which must say this is an edit.
	tool := parsed.Get("tools.0")
	assert.Equal(t, "image_generation", tool.Get("type").String())
	assert.Equal(t, "edit", tool.Get("action").String())
	assert.Equal(t, "1024x1536", tool.Get("size").String())
	// "standard" is not in the Codex quality enum; it normalizes like every other path.
	assert.Equal(t, "medium", tool.Get("quality").String())
	assert.True(t, strings.HasPrefix(tool.Get("input_image_mask.image_url").String(), "data:image/png;base64,"))
}

func TestBuildImageEditResponsesRequest_MultipleImagesNoMask(t *testing.T) {
	req := &openai.ImageEditParams{Prompt: "merge these", Model: "gpt-image-2"}
	req.Image.OfFileArray = []io.Reader{
		bytes.NewReader(testPNGBytes),
		bytes.NewReader(testPNGBytes),
	}

	params, err := buildImageEditResponsesRequest(req)
	require.NoError(t, err)

	body, err := json.Marshal(params)
	require.NoError(t, err)
	parsed := gjson.ParseBytes(body)

	content := parsed.Get("input.0.content").Array()
	require.Len(t, content, 3, "one text item plus both reference images")
	assert.Equal(t, "input_image", content[1].Get("type").String())
	assert.Equal(t, "input_image", content[2].Get("type").String())
	assert.False(t, parsed.Get("tools.0.input_image_mask.image_url").Exists())
	assert.Equal(t, "edit", parsed.Get("tools.0.action").String())
}

func TestBuildImageEditResponsesRequest_NoImage(t *testing.T) {
	req := &openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	_, err := buildImageEditResponsesRequest(req)
	assert.Error(t, err)
}

func TestBuildCodexImageEditRequest_MaskIsAnError(t *testing.T) {
	req := &openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")
	req.Mask = openai.File(bytes.NewReader(testPNGBytes), "mask.png", "image/png")

	_, err := buildCodexImageEditRequest(req)
	require.Error(t, err, "the native protocol cannot express a mask, so dropping it would change the result silently")
	assert.Contains(t, err.Error(), "mask")
}

// Keeps the N-passthrough expectation explicit: the Responses tool returns one
// image, so n>1 is logged and dropped rather than silently promised.
func TestBuildImageEditResponsesRequest_NIgnored(t *testing.T) {
	req := &openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2", N: param.NewOpt(int64(3))}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")

	params, err := buildImageEditResponsesRequest(req)
	require.NoError(t, err)
	body, err := json.Marshal(params)
	require.NoError(t, err)
	assert.False(t, gjson.ParseBytes(body).Get("n").Exists())
}
