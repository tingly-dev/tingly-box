package protocolserver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/constant"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIImageGeneration serves OpenAI-compatible image generation requests
// against the upstream POST /v1/images/generations endpoint. The request is
// forwarded as-is; tingly-box does not probe whether the upstream prefers the
// dedicated images endpoint or the Responses API — the caller chooses the
// surface and the corresponding tingly-box route.
//
// Exposed via the mixin route group, but gated on TransportImageGen: only
// scenarios whose descriptor declares it can reach this endpoint. The
// canonical home is the dedicated `imagegen` scenario.
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

	provider, selectedService, err := ph.selectServiceForImageGeneration(c, scenarioType, rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Resolve dual endpoint: when the provider has an OpenAI-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = provider.ResolveStyle(protocol.APIStyleOpenAI)

	actualModel := selectedService.Model
	req.Model = openai.ImageModel(actualModel)

	sessionID := resolveSessionID(c, &req)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	SetTrackingContext(c, rule, provider, actualModel, responseModel, false)

	// Collects why individual images of an n > 1 request failed while the
	// request itself succeeded (writeImagesResponse).
	fwdCtx, imageFailures := client.WithImageCallFailures(c.Request.Context())
	fc := forwarding.NewForwardContext(fwdCtx, provider)

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
		stream.SendForwardingError(c, err)
		return
	}

	usage := protocol.NewTokenUsageWithCache(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), 0)
	ph.trackUsageWithTokenUsage(c, usage, nil)

	// Persist generated images under the config image directory (best-effort).
	ph.persistImageGeneration(&req, resp)

	writeImagesResponse(c, resp, imageFailures)
}

// imagePartialFailure is one image of a multi-image request that did not come
// back while the others did — the same status and client-safe message a
// request failing outright would have carried.
type imagePartialFailure struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// writeImagesResponse answers a successful images request. When some of the
// images behind it failed (a gateway fan-out such as Codex's n > 1), the body
// carries a `partial_failures` array next to `data`: without it the caller
// only sees fewer images than it asked for, with no way to tell a rate limit
// from a moderation block. It is an extension field, so OpenAI SDKs that do
// not know it ignore it. See .design/image-mask.md §9.1.
func writeImagesResponse(c *gin.Context, resp *openai.ImagesResponse, failures *client.ImageCallFailures) {
	errs := failures.Errors()
	if len(errs) == 0 {
		c.JSON(http.StatusOK, resp)
		return
	}

	reported := make([]imagePartialFailure, 0, len(errs))
	for _, err := range errs {
		failure := protocol.ClassifyUpstreamFailure(err, http.StatusBadGateway)
		message := failure.Message
		// The reason is read inside one image slot, next to its status: the
		// upstream's own message says it; the SDK's "POST <url>: 429 Too Many
		// Requests {raw json}" repeats the status around it.
		var oaiErr *openai.Error
		if errors.As(err, &oaiErr) && oaiErr.Message != "" {
			message = oaiErr.Message
		}
		reported = append(reported, imagePartialFailure{Status: failure.Status, Message: message})
	}

	body, err := json.Marshal(resp)
	if err == nil {
		body, err = sjson.SetBytes(body, "partial_failures", reported)
	}
	if err != nil {
		// The images are the payload; losing the annotation must not lose them.
		logrus.Errorf("[ImageGen] Failed to attach partial failures to response: %v", err)
		c.JSON(http.StatusOK, resp)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// persistImageGeneration saves generated images and their prompts under the
// configured image directory (configDir/image/YYYYMMDD/). It is best-effort:
// any failure is logged but never blocks the response to the caller.
//
// This used to live inside the Codex client and wrote to .tingly-image/ in the
// process working directory. It now belongs to the server layer so persistence
// is uniform across providers and rooted at the application config directory.
func (ph *ProtocolHandler) persistImageGeneration(req *openai.ImageGenerateParams, resp *openai.ImagesResponse) {
	var meta string
	if req != nil {
		meta = buildImagePersistMeta(imageMetaInfo{
			Prompt:  req.Prompt,
			Model:   string(req.Model),
			Size:    string(req.Size),
			Quality: string(req.Quality),
			Format:  string(req.ResponseFormat),
			Style:   string(req.Style),
		})
	}
	ph.persistImages(resp, meta)
}

// persistImageEdit is the edit-surface counterpart of persistImageGeneration:
// same directory layout and best-effort semantics, with edit-shaped metadata.
func (ph *ProtocolHandler) persistImageEdit(req *openai.ImageEditParams, resp *openai.ImagesResponse) {
	var meta string
	if req != nil {
		meta = buildImagePersistMeta(imageMetaInfo{
			Prompt:    req.Prompt,
			Operation: "edit",
			Model:     string(req.Model),
			Size:      string(req.Size),
			Quality:   string(req.Quality),
		})
	}
	ph.persistImages(resp, meta)
}

// imageMetaInfo carries the fields persisted alongside a generated or edited
// image. Generation- and edit-only fields (Format/Style vs Operation) are
// simply left zero by the caller that doesn't have them.
type imageMetaInfo struct {
	Prompt    string
	Operation string
	Model     string
	Size      string
	Quality   string
	Format    string
	Style     string
}

// buildImagePersistMeta renders the sidecar .txt content saved next to a
// persisted image. Shared by the generation and edit surfaces so the two
// near-identical formats don't drift independently.
func buildImagePersistMeta(info imageMetaInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Prompt: %s\n\n", info.Prompt)
	if info.Operation != "" {
		fmt.Fprintf(&b, "Operation: %s\n", info.Operation)
	}
	fmt.Fprintf(&b, "Model: %s\nSize: %s\nQuality: %s\n", info.Model, info.Size, info.Quality)
	if info.Format != "" {
		fmt.Fprintf(&b, "Format: %s\n", info.Format)
	}
	fmt.Fprintf(&b, "Timestamp: %s\n", time.Now().Format(time.RFC3339))
	if info.Style != "" {
		fmt.Fprintf(&b, "Style: %s\n", info.Style)
	}
	return b.String()
}

// persistImages writes each base64 image in resp (plus an optional metadata
// sidecar) under configDir/image/YYYYMMDD/. Shared by the generation and edit
// surfaces.
func (ph *ProtocolHandler) persistImages(resp *openai.ImagesResponse, promptMeta string) {
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

		if promptMeta == "" {
			continue
		}

		promptPath := filepath.Join(dateDir, strings.Replace(filename, ".png", ".txt", 1))
		if err := os.WriteFile(promptPath, []byte(promptMeta), 0600); err != nil {
			logrus.Errorf("[ImageGen] Failed to write prompt file: %v", err)
			continue
		}
	}
}
