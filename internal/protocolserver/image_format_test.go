package protocolserver

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
)

func b64Of(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func persistedFiles(t *testing.T, configDir string) map[string][]byte {
	t.Helper()
	root := constant.GetImageDir(configDir)
	out := map[string][]byte{}
	dirs, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return out
	}
	require.NoError(t, err)
	for _, d := range dirs {
		entries, err := os.ReadDir(filepath.Join(root, d.Name()))
		require.NoError(t, err)
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(root, d.Name(), e.Name()))
			require.NoError(t, err)
			out[e.Name()] = data
		}
	}
	return out
}

func TestPersistImagesUsesSniffedExtension(t *testing.T) {
	gif := "GIF89a\x01\x00\x01\x00animated-bytes"
	jpg := "\xff\xd8\xff\xe0jpeg-bytes"
	webp := "RIFF\x10\x00\x00\x00WEBPVP8 data"

	tmp := t.TempDir()
	h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}
	resp := &openai.ImagesResponse{Data: []openai.Image{{B64JSON: b64Of(gif)}, {B64JSON: b64Of(jpg)}, {B64JSON: b64Of(webp)}}}
	h.persistImageGeneration(&openai.ImageGenerateParams{Prompt: "dancing cat"}, resp)

	files := persistedFiles(t, tmp)
	var gifs, jpgs, webps, txts, pngs int
	for name, data := range files {
		switch filepath.Ext(name) {
		case ".gif":
			gifs++
			assert.Equal(t, gif, string(data))
		case ".jpg":
			jpgs++
			assert.Equal(t, jpg, string(data))
		case ".webp":
			webps++
		case ".png":
			pngs++
		case ".txt":
			txts++
			assert.Contains(t, string(data), "dancing cat")
		}
	}
	assert.Equal(t, 1, gifs)
	assert.Equal(t, 1, jpgs)
	assert.Equal(t, 1, webps)
	assert.Equal(t, 0, pngs, "a GIF must never be saved as .png")
	assert.Equal(t, 3, txts, "every image keeps its sidecar, whatever its extension")

	// Sidecar shares the image stem.
	for name := range files {
		if filepath.Ext(name) == ".txt" {
			stem := strings.TrimSuffix(name, ".txt")
			found := false
			for other := range files {
				if other != name && strings.TrimSuffix(other, filepath.Ext(other)) == stem {
					found = true
				}
			}
			assert.True(t, found, "sidecar %s has no image with the same stem", name)
		}
	}
}

func TestPersistImagesSkipsNonRaster(t *testing.T) {
	for name, payload := range map[string]string{
		"svg":   `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"html":  "<!doctype html><script>x</script>",
		"plain": "not an image",
	} {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			h := &ProtocolHandler{deps: ProtocolHandlerDeps{Config: &config.Config{ConfigDir: tmp}}}
			h.persistImageGeneration(&openai.ImageGenerateParams{Prompt: "x"},
				&openai.ImagesResponse{Data: []openai.Image{{B64JSON: b64Of(payload)}}})
			assert.Empty(t, persistedFiles(t, tmp), "non-raster bytes must not be written")
		})
	}
}

func TestAnnotateImageFormat(t *testing.T) {
	t.Run("sniffed format wins over a stale one", func(t *testing.T) {
		resp := &openai.ImagesResponse{
			OutputFormat: "jpeg",
			Data:         []openai.Image{{B64JSON: b64Of("\x89PNG\r\n\x1a\nxx")}},
		}
		annotateImageFormat(resp)
		assert.Equal(t, openai.ImagesResponseOutputFormat("png"), resp.OutputFormat)
	})
	t.Run("empty format is filled from the bytes", func(t *testing.T) {
		resp := &openai.ImagesResponse{Data: []openai.Image{{B64JSON: b64Of("GIF89a\x01\x00\x01\x00")}}}
		annotateImageFormat(resp)
		assert.Equal(t, openai.ImagesResponseOutputFormat("gif"), resp.OutputFormat)
	})
	t.Run("unknown bytes leave the field alone", func(t *testing.T) {
		resp := &openai.ImagesResponse{OutputFormat: "webp", Data: []openai.Image{{B64JSON: b64Of("???")}}}
		annotateImageFormat(resp)
		assert.Equal(t, openai.ImagesResponseOutputFormat("webp"), resp.OutputFormat)
	})
	t.Run("no data and nil are safe", func(t *testing.T) {
		annotateImageFormat(nil)
		annotateImageFormat(&openai.ImagesResponse{})
	})
}
