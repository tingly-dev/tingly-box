package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

var testImage = []byte("\x89PNG\r\n\x1a\nfake-image")

// openaiMask builds a 4x1 OpenAI-convention mask: the left half transparent
// (edit), the right half opaque (keep).
func openaiMask(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	for x := 0; x < 4; x++ {
		a := uint8(0)
		if x >= 2 {
			a = 0xff
		}
		img.SetNRGBA(x, 0, color.NRGBA{A: a})
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func decodeDataURLImage(t *testing.T, url string) image.Image {
	t.Helper()
	_, payload, ok := strings.Cut(url, ";base64,")
	require.True(t, ok, "not a base64 data URL: %.40s", url)
	raw, err := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	return img
}

func TestWhiteEditMask_InvertsTheConvention(t *testing.T) {
	out, err := whiteEditMask(openaiMask(t))
	require.NoError(t, err)
	img, err := png.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 4, 1), img.Bounds(), "mask keeps the image's pixel size")
	for x := 0; x < 4; x++ {
		r, _, _, _ := img.At(x, 0).RGBA()
		if x < 2 {
			assert.EqualValues(t, 0xffff, r, "transparent (edit) pixel %d must be white", x)
		} else {
			assert.EqualValues(t, 0, r, "opaque (keep) pixel %d must be black", x)
		}
	}
}

func TestWhiteEditMask_RejectsNonPNG(t *testing.T) {
	_, err := whiteEditMask([]byte("not a png"))
	assert.Error(t, err)
}

func TestDetectVendor_EditVendors(t *testing.T) {
	for base, want := range map[string]Vendor{
		"https://api.x.ai/v1/":             VendorXAI,
		"https://qianfan.baidubce.com/v2":  VendorQianfan,
		"https://api.openai.com/v1":        VendorOpenAICompat,
		"https://dashscope.aliyuncs.com/x": VendorDashScope,
	} {
		p := &typ.Provider{APIBase: base, APIStyle: protocol.APIStyleOpenAI}
		assert.Equal(t, want, DetectVendor(p), base)
	}
}

func TestEditRequestFromOpenAI(t *testing.T) {
	p := &openai.ImageEditParams{Prompt: "p", Model: "m", N: param.NewOpt(int64(2)), Size: "1024x1536"}
	p.Image.OfFileArray = []io.Reader{bytes.NewReader(testImage), bytes.NewReader(testImage)}
	p.Mask = bytes.NewReader([]byte("mask"))
	req, err := EditRequestFromOpenAI(p)
	require.NoError(t, err)
	assert.Len(t, req.Images, 2)
	assert.Equal(t, []byte("mask"), req.Mask)
	assert.Equal(t, 2, req.N)
	assert.False(t, req.WantsURL(), "unset response_format means base64")

	_, err = EditRequestFromOpenAI(&openai.ImageEditParams{Prompt: "p"})
	assert.Error(t, err, "no image")
}

// ---- xAI

func TestBuildXAIEditBody(t *testing.T) {
	body, err := buildXAIEditBody(&EditRequest{Model: "grok-imagine-image-2.0", Prompt: "p", Images: [][]byte{testImage}, Size: "1024x1536"})
	require.NoError(t, err)
	require.NotNil(t, body.Image)
	assert.True(t, strings.HasPrefix(body.Image.URL, "data:image/png;base64,"))
	assert.Empty(t, body.Images)
	assert.Equal(t, "2:3", body.AspectRatio)
	assert.Equal(t, "b64_json", body.ResponseFormat)

	body, err = buildXAIEditBody(&EditRequest{Prompt: "p", Images: [][]byte{testImage, testImage}, Size: "1000x999", ResponseFormat: "url"})
	require.NoError(t, err)
	assert.Nil(t, body.Image)
	assert.Len(t, body.Images, 2, "several references go in images[]")
	assert.Empty(t, body.AspectRatio, "a ratio xAI does not know is left to its default")
	assert.Equal(t, "url", body.ResponseFormat)

	_, err = buildXAIEditBody(&EditRequest{Prompt: "p", Images: [][]byte{testImage}, Mask: []byte("m")})
	assert.ErrorContains(t, err, "mask")
}

func TestXAIEdit_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/images/edits", r.URL.Path)
		assert.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		raw, _ := io.ReadAll(r.Body)
		assert.Equal(t, "b64_json", gjson.GetBytes(raw, "response_format").String())
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"AAAA"},{"b64_json":"BBBB"}],"usage":{"input_tokens":3}}`))
	}))
	defer srv.Close()

	c, err := newXAIClient(&typ.Provider{APIBase: srv.URL + "/v1", Token: "key"})
	require.NoError(t, err)
	resp, err := c.Edit(context.Background(), &EditRequest{Prompt: "p", Images: [][]byte{testImage}, N: 2})
	require.NoError(t, err)
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "AAAA", resp.Data[0].B64JSON)
	assert.EqualValues(t, 3, resp.Usage.InputTokens)
}

// ---- Qianfan

func TestBuildQianfanEditBody(t *testing.T) {
	t.Run("ernie with a mask repaints, mask inverted", func(t *testing.T) {
		body, err := buildQianfanEditBody(&EditRequest{Model: "ernie-irag-edit", Prompt: "p", Images: [][]byte{testImage}, Mask: openaiMask(t)})
		require.NoError(t, err)
		assert.Equal(t, "repaint", body.Feature)
		r, _, _, _ := decodeDataURLImage(t, body.Mask).At(0, 0).RGBA()
		assert.EqualValues(t, 0xffff, r, "edit area is white")
	})
	t.Run("ernie without a mask is a variation", func(t *testing.T) {
		body, err := buildQianfanEditBody(&EditRequest{Model: "ernie-irag-edit", Prompt: "p", Images: [][]byte{testImage}})
		require.NoError(t, err)
		assert.Equal(t, "variation", body.Feature)
		assert.Empty(t, body.Mask)
	})
	t.Run("qwen takes several images as an array", func(t *testing.T) {
		body, err := buildQianfanEditBody(&EditRequest{Model: "qwen-image-edit", Prompt: "p", Images: [][]byte{testImage, testImage}})
		require.NoError(t, err)
		assert.Empty(t, body.Feature)
		assert.Len(t, body.Image, 2)
	})
	t.Run("a mask on a model without one is refused", func(t *testing.T) {
		_, err := buildQianfanEditBody(&EditRequest{Model: "qwen-image-edit", Prompt: "p", Images: [][]byte{testImage}, Mask: openaiMask(t)})
		assert.ErrorContains(t, err, "ernie-irag-edit")
	})
}

func TestQianfanEdit_InlinesResultURLs(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/images/edits":
			raw, _ := io.ReadAll(r.Body)
			assert.True(t, strings.HasPrefix(gjson.GetBytes(raw, "image").String(), "data:image/"))
			_, _ = w.Write([]byte(`{"created":7,"data":[{"url":"` + srv.URL + `/result.png"}]}`))
		case "/result.png":
			_, _ = w.Write([]byte("PNGBYTES"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := newQianfanClient(&typ.Provider{APIBase: srv.URL + "/v2", Token: "key"})
	require.NoError(t, err)
	resp, err := c.Edit(context.Background(), &EditRequest{Model: "qwen-image-edit", Prompt: "p", Images: [][]byte{testImage}})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Empty(t, resp.Data[0].URL, "the expiring URL is replaced")
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("PNGBYTES")), resp.Data[0].B64JSON)

	resp, err = c.Edit(context.Background(), &EditRequest{Model: "qwen-image-edit", Prompt: "p", Images: [][]byte{testImage}, ResponseFormat: "url"})
	require.NoError(t, err)
	assert.Equal(t, srv.URL+"/result.png", resp.Data[0].URL, "an explicit url request keeps the URL")
}

// ---- DashScope

func TestBuildWanxEditBody(t *testing.T) {
	body, err := buildWanxEditBody(&EditRequest{Model: "wanx2.1-imageedit", Prompt: "p", Images: [][]byte{testImage}, Mask: openaiMask(t), N: 2})
	require.NoError(t, err)
	assert.Equal(t, "description_edit_with_mask", body.Input.Function)
	r, _, _, _ := decodeDataURLImage(t, body.Input.MaskImageURL).At(3, 0).RGBA()
	assert.EqualValues(t, 0, r, "keep area is black")
	assert.Equal(t, 2, body.Parameters["n"])

	body, err = buildWanxEditBody(&EditRequest{Model: "wanx2.1-imageedit", Prompt: "p", Images: [][]byte{testImage}})
	require.NoError(t, err)
	assert.Equal(t, "description_edit", body.Input.Function)
	assert.Empty(t, body.Input.MaskImageURL)

	_, err = buildWanxEditBody(&EditRequest{Model: "wanx2.1-imageedit", Images: [][]byte{testImage, testImage}})
	assert.Error(t, err)
}

func TestBuildQwenImageEditBody(t *testing.T) {
	body, err := buildQwenImageEditBody(&EditRequest{Model: "qwen-image-edit-plus", Prompt: "p", Images: [][]byte{testImage, testImage}, Size: "1024x1536"})
	require.NoError(t, err)
	raw, _ := json.Marshal(body)
	content := gjson.GetBytes(raw, "input.messages.0.content").Array()
	require.Len(t, content, 3)
	assert.True(t, content[0].Get("image").Exists())
	assert.Equal(t, "p", content[2].Get("text").String(), "the instruction comes after the images")
	assert.Equal(t, "1024*1536", gjson.GetBytes(raw, "parameters.size").String())

	_, err = buildQwenImageEditBody(&EditRequest{Model: "qwen-image-edit", Images: [][]byte{testImage}, Mask: []byte("m")})
	assert.ErrorContains(t, err, "wanx2.1-imageedit")
}

func TestDashScopeEdit_WanxPollsTheTask(t *testing.T) {
	var polls atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/image2image/image-synthesis":
			assert.Equal(t, "enable", r.Header.Get("X-DashScope-Async"))
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"PENDING"}}`))
		case "/api/v1/tasks/t1":
			if polls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"RUNNING"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"output":{"task_id":"t1","task_status":"SUCCEEDED","results":[{"url":"` + srv.URL + `/out.png"}]}}`))
		case "/out.png":
			_, _ = w.Write([]byte("WANX"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := newDashScopeClient(&typ.Provider{APIBase: srv.URL + "/compatible-mode/v1", Token: "key"})
	require.NoError(t, err)
	c.pollInterval = 1
	resp, err := c.Edit(context.Background(), &EditRequest{Model: "wanx2.1-imageedit", Prompt: "p", Images: [][]byte{testImage}, Mask: openaiMask(t)})
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("WANX")), resp.Data[0].B64JSON)
}

func TestDashScopeEdit_QwenImageIsSync(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/services/aigc/multimodal-generation/generation":
			assert.Empty(t, r.Header.Get("X-DashScope-Async"))
			_, _ = w.Write([]byte(`{"output":{"choices":[{"message":{"content":[{"image":"` + srv.URL + `/q.png"}]}}]}}`))
		case "/q.png":
			_, _ = w.Write([]byte("QWEN"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := newDashScopeClient(&typ.Provider{APIBase: srv.URL + "/compatible-mode/v1", Token: "key"})
	require.NoError(t, err)
	resp, err := c.Edit(context.Background(), &EditRequest{Model: "qwen-image-edit", Prompt: "p", Images: [][]byte{testImage}})
	require.NoError(t, err)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("QWEN")), resp.Data[0].B64JSON)
}

func TestNewEditor_UnsupportedVendor(t *testing.T) {
	_, err := NewEditor(&typ.Provider{APIBase: "https://api.openai.com/v1", APIStyle: protocol.APIStyleOpenAI})
	assert.ErrorIs(t, err, ErrEditUnsupported)
}
