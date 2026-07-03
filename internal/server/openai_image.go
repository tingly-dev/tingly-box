package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIImageGeneration serves OpenAI-compatible image generation requests
// against the upstream POST /v1/images/generations endpoint. The request is
// forwarded as-is; tingly-box does not probe whether the upstream prefers the
// dedicated images endpoint or the Responses API — the caller chooses the
// surface and the corresponding tingly-box route.
//
// Exposed via the mixin route group, so any scenario whose descriptor declares
// TransportImageGen (or TransportOpenAI as a mixin) can reach it. The canonical
// home is the dedicated `imagegen` scenario.
func (ph *ProtocolHandler) HandleOpenAIImageGeneration(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	if !IsValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportImageGen) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("scenario %s does not support image generation", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	bodyBytes, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to read request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	var req openai.ImageGenerateParams
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if string(req.Model) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if req.Prompt == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Prompt is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	requestModel := req.Model
	responseModel := requestModel

	rule, err := ph.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	provider, selectedService, err := ph.deps.RoutingSelector.SelectServiceForImageGeneration(c, scenarioType, rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	actualModel := selectedService.Model
	req.Model = openai.ImageModel(actualModel)

	sessionID := resolveSessionID(c, &req)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	SetTrackingContext(c, rule, provider, actualModel, responseModel, false)

	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	// The OpenAI client wrapper handles vendor fragmentation internally:
	// OpenAI-compatible providers go straight through the SDK, DashScope and
	// MiniMax are dispatched to their native imagegen adapters, and Codex
	// (ChatGPT OAuth) rides the Responses API. The handler stays uniform.
	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	resp, cancel, err := forwarding.ForwardOpenAIImageGeneration(fc, wrapper, &req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		ph.trackUsageWithTokenUsage(c, usage, err)
		logrus.Errorf("Failed to forward image generation request: %v", err)
		c.JSON(protocol.UpstreamStatus(err, http.StatusInternalServerError), ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	usage := protocol.NewTokenUsageWithCache(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), 0)
	ph.trackUsageWithTokenUsage(c, usage, nil)

	// Persist generated images under the config image directory (best-effort).
	ph.persistImageGeneration(&req, resp)

	responseJSON, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to process response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, responseMap)
}

// persistImageGeneration saves generated images and their prompts under the
// configured image directory (configDir/image/YYYYMMDD/). It is best-effort:
// any failure is logged but never blocks the response to the caller.
//
// This used to live inside the Codex client and wrote to .tingly-image/ in the
// process working directory. It now belongs to the server layer so persistence
// is uniform across providers and rooted at the application config directory.
func (ph *ProtocolHandler) persistImageGeneration(req *openai.ImageGenerateParams, resp *openai.ImagesResponse) {
	if resp == nil || len(resp.Data) == 0 {
		return
	}

	baseDir := ""
	if ph.deps.Config != nil {
		baseDir = ph.deps.Config.ConfigDir
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
