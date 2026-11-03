package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
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

// The Responses tool returns one image per call, so n never reaches the wire:
// n > 1 is served by fanning the same request out (codex_images_fanout.go).
func TestBuildImageEditResponsesRequest_NIgnored(t *testing.T) {
	req := &openai.ImageEditParams{Prompt: "x", Model: "gpt-image-2", N: param.NewOpt(int64(3))}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")

	params, err := buildImageEditResponsesRequest(req)
	require.NoError(t, err)
	body, err := json.Marshal(params)
	require.NoError(t, err)
	assert.False(t, gjson.ParseBytes(body).Get("n").Exists())
}

// Masked edits use the same terminal response handling as image generation.
func TestCodexMaskedEditReturnsOnCompleted(t *testing.T) {
	t.Setenv(codexImageEditRouteEnv, "")
	reader, writer := io.Pipe()
	defer writer.Close()
	requests := make(chan []byte, 1)
	transport := rtPtr(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		requests <- body
		go func() {
			_, _ = io.WriteString(writer, "data: "+doneFinal+"\n\ndata: {\"type\":\"response.completed\",\"sequence_number\":4}\n\n")
		}()
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader, Request: req}, nil
	})
	c := &CodexClient{OpenAIClient: &OpenAIClient{client: openai.NewClient(
		option.WithBaseURL("https://codex.test/backend-api/"), option.WithAPIKey("test"), option.WithMaxRetries(0),
		option.WithHTTPClient(&http.Client{Transport: transport}),
	)}}
	req := openai.ImageEditParams{Prompt: "edit", Model: "gpt-image-2"}
	req.Image.OfFile = openai.File(bytes.NewReader(testPNGBytes), "input.png", "image/png")
	req.Mask = openai.File(bytes.NewReader(testPNGBytes), "mask.png", "image/png")
	type outcome struct {
		response *openai.ImagesResponse
		err      error
	}
	done := make(chan outcome, 1)
	go func() { response, err := c.ImagesEdit(context.Background(), req); done <- outcome{response, err} }()
	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.Len(t, result.response.Data, 1)
		assert.Equal(t, "FINAL", result.response.Data[0].B64JSON)
		body := <-requests
		assert.Equal(t, "edit", gjson.GetBytes(body, "tools.0.action").String())
		assert.True(t, gjson.GetBytes(body, "tools.0.input_image_mask.image_url").Exists())
	case <-time.After(time.Second):
		t.Fatal("masked edit waited for the transport after response.completed")
	}
}
