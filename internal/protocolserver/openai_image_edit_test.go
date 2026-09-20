package protocolserver

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

var editTestPNG = []byte("\x89PNG\r\n\x1a\nfake-image-data")

// editTestPNGBase64 is the base64 form of editTestPNG, shared by the tests
// that inline it into JSON request bodies or mock upstream responses.
var editTestPNGBase64 = base64.StdEncoding.EncodeToString(editTestPNG)

func newEditTestContext(t *testing.T, method, contentType string, body io.Reader) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/tingly/imagegen/v1/images/edits", body)
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return c
}

func buildMultipartBody(t *testing.T, imageField string, imageCount int, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for i := 0; i < imageCount; i++ {
		fw, err := w.CreateFormFile(imageField, "input.png")
		require.NoError(t, err)
		_, err = fw.Write(editTestPNG)
		require.NoError(t, err)
	}
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	require.NoError(t, w.Close())
	return buf, w.FormDataContentType()
}

func TestParseImageEditMultipart_SingleImage(t *testing.T) {
	body, contentType := buildMultipartBody(t, "image", 1, map[string]string{
		"prompt":  "add a red hat",
		"model":   "gpt-image-2",
		"size":    "1024x1024",
		"quality": "high",
		"n":       "2",
	})
	c := newEditTestContext(t, "POST", contentType, body)

	req, err := parseImageEditRequest(c)
	require.NoError(t, err)

	assert.Equal(t, "add a red hat", req.Prompt)
	assert.Equal(t, "gpt-image-2", string(req.Model))
	assert.Equal(t, "1024x1024", string(req.Size))
	assert.Equal(t, "high", string(req.Quality))
	require.True(t, req.N.Valid())
	assert.Equal(t, int64(2), req.N.Value)

	require.NotNil(t, req.Image.OfFile)
	data, err := io.ReadAll(req.Image.OfFile)
	require.NoError(t, err)
	assert.Equal(t, editTestPNG, data)
}

func TestParseImageEditMultipart_ImageArrayField(t *testing.T) {
	body, contentType := buildMultipartBody(t, "image[]", 2, map[string]string{
		"prompt": "merge",
		"model":  "gpt-image-2",
	})
	c := newEditTestContext(t, "POST", contentType, body)

	req, err := parseImageEditRequest(c)
	require.NoError(t, err)
	assert.Nil(t, req.Image.OfFile)
	assert.Len(t, req.Image.OfFileArray, 2)
}

func TestParseImageEditMultipart_MissingImage(t *testing.T) {
	body, contentType := buildMultipartBody(t, "image", 0, map[string]string{
		"prompt": "x",
		"model":  "m",
	})
	c := newEditTestContext(t, "POST", contentType, body)

	_, err := parseImageEditRequest(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image")
}

func TestParseImageEditJSON_DataURL(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(editTestPNG)
	body := `{
		"image": "data:image/png;base64,` + b64 + `",
		"prompt": "add a red hat",
		"model": "gpt-image-2",
		"quality": "medium"
	}`
	c := newEditTestContext(t, "POST", "application/json", strings.NewReader(body))

	req, err := parseImageEditRequest(c)
	require.NoError(t, err)

	assert.Equal(t, "add a red hat", req.Prompt)
	assert.Equal(t, "medium", string(req.Quality))
	require.NotNil(t, req.Image.OfFile)
	data, err := io.ReadAll(req.Image.OfFile)
	require.NoError(t, err)
	assert.Equal(t, editTestPNG, data)
}

func TestParseImageEditJSON_Mask(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(editTestPNG)
	body := `{
		"image": "data:image/png;base64,` + b64 + `",
		"mask": "data:image/png;base64,` + b64 + `",
		"prompt": "replace the sofa",
		"model": "gpt-image-2"
	}`
	c := newEditTestContext(t, "POST", "application/json", strings.NewReader(body))

	req, err := parseImageEditRequest(c)
	require.NoError(t, err)
	require.NotNil(t, req.Mask)
	data, err := io.ReadAll(req.Mask)
	require.NoError(t, err)
	assert.Equal(t, editTestPNG, data)
}

func TestParseImageEditJSON_InvalidMask(t *testing.T) {
	body := `{"image": "` + base64.StdEncoding.EncodeToString(editTestPNG) + `", "mask": "https://example.com/m.png", "prompt": "p", "model": "m"}`
	c := newEditTestContext(t, "POST", "application/json", strings.NewReader(body))

	_, err := parseImageEditRequest(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mask")
}

func TestParseImageEditJSON_BareBase64Array(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(editTestPNG)
	body := `{"image": ["` + b64 + `", "` + b64 + `"], "prompt": "p", "model": "m"}`
	c := newEditTestContext(t, "POST", "application/json", strings.NewReader(body))

	req, err := parseImageEditRequest(c)
	require.NoError(t, err)
	assert.Nil(t, req.Image.OfFile)
	assert.Len(t, req.Image.OfFileArray, 2)
}

func TestParseImageEditJSON_RejectsRemoteURL(t *testing.T) {
	body := `{"image": "https://example.com/x.png", "prompt": "p", "model": "m"}`
	c := newEditTestContext(t, "POST", "application/json", strings.NewReader(body))

	_, err := parseImageEditRequest(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote image URLs are not supported")
}

func TestParseImageEditJSON_RequiredFields(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(editTestPNG)

	tests := []struct {
		name string
		body string
		want string
	}{
		{"missing model", `{"image": "` + b64 + `", "prompt": "p"}`, "Model is required"},
		{"missing prompt", `{"image": "` + b64 + `", "model": "m"}`, "Prompt is required"},
		{"missing image", `{"prompt": "p", "model": "m"}`, "input image is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newEditTestContext(t, "POST", "application/json", strings.NewReader(tt.body))
			_, err := parseImageEditRequest(c)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDecodeInlineImage(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(editTestPNG)

	t.Run("data URL with declared type", func(t *testing.T) {
		data, contentType, err := decodeInlineImage("data:image/png;base64," + b64)
		require.NoError(t, err)
		assert.Equal(t, editTestPNG, data)
		assert.Equal(t, "image/png", contentType)
	})

	t.Run("bare base64 sniffs content type", func(t *testing.T) {
		data, contentType, err := decodeInlineImage(b64)
		require.NoError(t, err)
		assert.Equal(t, editTestPNG, data)
		assert.Equal(t, "image/png", contentType)
	})

	t.Run("non-base64 data URL rejected", func(t *testing.T) {
		_, _, err := decodeInlineImage("data:image/png,rawdata")
		assert.Error(t, err)
	})

	t.Run("invalid base64 rejected", func(t *testing.T) {
		_, _, err := decodeInlineImage("!!not-base64!!")
		assert.Error(t, err)
	})
}

func TestPersistImageEdit(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(editTestPNG)
	tmp := t.TempDir()
	h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}

	req := &openai.ImageEditParams{
		Prompt: "add a red hat",
		Model:  "gpt-image-2",
	}
	resp := &openai.ImagesResponse{Data: []openai.Image{{B64JSON: b64}}}

	h.persistImageEdit(req, resp)

	dirs, err := os.ReadDir(constant.GetImageDir(tmp))
	require.NoError(t, err)
	require.Len(t, dirs, 1)

	dateDir := filepath.Join(constant.GetImageDir(tmp), dirs[0].Name())
	entries, err := os.ReadDir(dateDir)
	require.NoError(t, err)

	var pngFiles, txtFiles int
	for _, e := range entries {
		switch filepath.Ext(e.Name()) {
		case ".png":
			pngFiles++
		case ".txt":
			txtFiles++
			meta, readErr := os.ReadFile(filepath.Join(dateDir, e.Name()))
			require.NoError(t, readErr)
			assert.Contains(t, string(meta), "add a red hat")
			assert.Contains(t, string(meta), "Operation: edit")
		}
	}
	assert.Equal(t, 1, pngFiles)
	assert.Equal(t, 1, txtFiles)
}
