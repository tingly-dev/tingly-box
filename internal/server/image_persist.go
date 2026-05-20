package server

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// persistImageGeneration saves generated images and their prompts under the
// configured image directory (configDir/image/YYYYMMDD/). It is best-effort:
// any failure is logged but never blocks the response to the caller.
//
// This used to live inside the Codex client and wrote to .tingly-image/ in the
// process working directory. It now belongs to the server layer so persistence
// is uniform across providers and rooted at the application config directory.
func (s *Server) persistImageGeneration(req *openai.ImageGenerateParams, resp *openai.ImagesResponse) {
	if resp == nil || len(resp.Data) == 0 {
		return
	}

	baseDir := ""
	if s.config != nil {
		baseDir = s.config.ConfigDir
	}
	if baseDir == "" {
		baseDir = constant.GetTinglyConfDir()
	}

	now := time.Now()
	timestamp := now.Format("20060102-150405")
	dateDir := filepath.Join(constant.GetImageDir(baseDir), now.Format("20060102"))

	dirReady := false
	ensureDir := func() bool {
		if dirReady {
			return true
		}
		if err := os.MkdirAll(dateDir, 0700); err != nil {
			logrus.Errorf("[ImageGen] Failed to create image directory: %v", err)
			return false
		}
		dirReady = true
		return true
	}

	for i, img := range resp.Data {
		// Only base64-encoded images can be persisted locally; URL-based
		// responses (e.g. some DashScope/MiniMax modes) are skipped.
		if img.B64JSON == "" {
			continue
		}
		if !ensureDir() {
			return
		}

		var filename string
		if i == 0 {
			filename = fmt.Sprintf("%s.png", timestamp)
		} else {
			filename = fmt.Sprintf("%s-%d.png", timestamp, i)
		}
		imagePath := filepath.Join(dateDir, filename)

		imageData, err := base64.StdEncoding.DecodeString(img.B64JSON)
		if err != nil {
			logrus.Errorf("[ImageGen] Failed to decode base64 image data: %v", err)
			continue
		}

		if err := os.WriteFile(imagePath, imageData, 0600); err != nil {
			logrus.Errorf("[ImageGen] Failed to write image file: %v", err)
			continue
		}

		logrus.Infof("[ImageGen] Saved image to: %s", imagePath)

		if req == nil {
			continue
		}

		promptPath := filepath.Join(dateDir, strings.Replace(filename, ".png", ".txt", 1))
		promptContent := fmt.Sprintf("Prompt: %s\n\nModel: %s\nSize: %s\nQuality: %s\nFormat: %s\nTimestamp: %s\n",
			req.Prompt,
			req.Model,
			req.Size,
			req.Quality,
			req.ResponseFormat,
			now.Format(time.RFC3339),
		)
		if req.Style != "" {
			promptContent += fmt.Sprintf("Style: %s\n", req.Style)
		}

		if err := os.WriteFile(promptPath, []byte(promptContent), 0600); err != nil {
			logrus.Errorf("[ImageGen] Failed to write prompt file: %v", err)
			continue
		}
	}
}
