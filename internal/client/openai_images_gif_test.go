package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/vision/imagefmt"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

func TestOpenAICompatImagesGenerate_GIFFieldsReachTheUpstream(t *testing.T) {
	var path string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"` + gifB64 + `"}]}`))
	}))
	defer srv.Close()

	provider := &typ.Provider{Name: "compat", APIBase: srv.URL + "/v1", APIStyle: protocol.APIStyleOpenAI}
	c := &OpenAIClient{provider: provider, client: openai.NewClient(option.WithBaseURL(srv.URL+"/v1/"), option.WithAPIKey("k"))}

	t.Run("gif carries the frame controls", func(t *testing.T) {
		ctx := imagegen.WithOutputOptions(context.Background(), imagegen.OutputOptions{Format: imagefmt.GIF, FrameCount: 4, FrameDurationMS: 90})
		req := openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2", OutputFormat: "gif"}
		resp, err := c.ImagesGenerate(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, gifB64, resp.Data[0].B64JSON)
		assert.Equal(t, "/v1/images/generations", path)
		assert.Equal(t, "gif", gjson.GetBytes(body, "output_format").String())
		assert.EqualValues(t, 4, gjson.GetBytes(body, "frame_count").Int())
		assert.EqualValues(t, 90, gjson.GetBytes(body, "frame_duration_ms").Int())
	})

	t.Run("png request is untouched", func(t *testing.T) {
		req := openai.ImageGenerateParams{Prompt: "x", Model: "gpt-image-2"}
		_, err := c.ImagesGenerate(context.Background(), req)
		require.NoError(t, err)
		assert.False(t, gjson.GetBytes(body, "frame_count").Exists())
		assert.False(t, gjson.GetBytes(body, "output_format").Exists())
	})
}
