package protocolserver

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/vision/imagestore"
)

func TestPersistImageGeneration(t *testing.T) {
	pngBytes := []byte("\x89PNG\r\n\x1a\nfake-image-data")
	b64 := base64.StdEncoding.EncodeToString(pngBytes)

	t.Run("writes image and prompt under configDir/image", func(t *testing.T) {
		tmp := t.TempDir()
		h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}

		req := &openai.ImageGenerateParams{
			Prompt: "a red bicycle",
			Model:  "gpt-image-1",
			Size:   openai.ImageGenerateParamsSize1024x1024,
		}
		resp := &openai.ImagesResponse{Data: []openai.Image{{B64JSON: b64}}}

		h.persistImageGeneration(req, resp)

		imageRoot := constant.GetImageDir(tmp)
		dirs, err := os.ReadDir(imageRoot)
		require.NoError(t, err)
		require.Len(t, dirs, 1, "expected a single date directory")

		dateDir := filepath.Join(imageRoot, dirs[0].Name())
		entries, err := os.ReadDir(dateDir)
		require.NoError(t, err)

		var pngFiles, txtFiles int
		for _, e := range entries {
			switch filepath.Ext(e.Name()) {
			case ".png":
				pngFiles++
				data, readErr := os.ReadFile(filepath.Join(dateDir, e.Name()))
				require.NoError(t, readErr)
				assert.Equal(t, pngBytes, data, "decoded image bytes should match")
			case ".txt":
				txtFiles++
				meta, readErr := os.ReadFile(filepath.Join(dateDir, e.Name()))
				require.NoError(t, readErr)
				assert.Contains(t, string(meta), "a red bicycle")
			}
		}
		assert.Equal(t, 1, pngFiles)
		assert.Equal(t, 1, txtFiles)
	})

	t.Run("writes multiple images with indexed filenames", func(t *testing.T) {
		tmp := t.TempDir()
		h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}

		resp := &openai.ImagesResponse{Data: []openai.Image{{B64JSON: b64}, {B64JSON: b64}}}
		h.persistImageGeneration(&openai.ImageGenerateParams{Prompt: "x"}, resp)

		dirs, err := os.ReadDir(constant.GetImageDir(tmp))
		require.NoError(t, err)
		require.Len(t, dirs, 1)
		entries, err := os.ReadDir(filepath.Join(constant.GetImageDir(tmp), dirs[0].Name()))
		require.NoError(t, err)

		var pngFiles int
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".png" {
				pngFiles++
			}
		}
		assert.Equal(t, 2, pngFiles)
	})

	t.Run("skips images without base64 data", func(t *testing.T) {
		tmp := t.TempDir()
		h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}

		resp := &openai.ImagesResponse{Data: []openai.Image{{URL: "https://example.com/x.png"}}}
		h.persistImageGeneration(&openai.ImageGenerateParams{Prompt: "x"}, resp)

		_, err := os.ReadDir(constant.GetImageDir(tmp))
		assert.True(t, os.IsNotExist(err), "no image directory should be created")
	})

	t.Run("no-op for empty response", func(t *testing.T) {
		tmp := t.TempDir()
		h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}

		h.persistImageGeneration(&openai.ImageGenerateParams{}, &openai.ImagesResponse{})

		_, err := os.ReadDir(constant.GetImageDir(tmp))
		assert.True(t, os.IsNotExist(err))
	})
}

func TestPersistImagesReturnsArchiveIDs(t *testing.T) {
	pngBytes := []byte("\x89PNG\r\n\x1a\nfake-image-data")
	b64 := base64.StdEncoding.EncodeToString(pngBytes)
	tmp := t.TempDir()
	h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}

	// Two images in one response, plus a URL-only one that is not saved: the
	// ids line up with resp.Data so the Playground can match them to results.
	resp := &openai.ImagesResponse{Data: []openai.Image{{B64JSON: b64}, {URL: "https://example.com/x.png"}, {B64JSON: b64}}}
	ids := h.persistImageGeneration(&openai.ImageGenerateParams{Prompt: "x"}, resp)
	require.Len(t, ids, 3)
	assert.NotEmpty(t, ids[0])
	assert.Empty(t, ids[1])
	assert.NotEmpty(t, ids[2])
	assert.NotEqual(t, ids[0], ids[2], "same-second images must not share a name")

	store := imagestore.New(constant.GetImageDir(tmp))
	img, err := store.Get(ids[2])
	require.NoError(t, err)
	assert.Equal(t, "x", img.Meta.Prompt)
}
