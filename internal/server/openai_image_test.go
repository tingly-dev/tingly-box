package server

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/server/config"
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
