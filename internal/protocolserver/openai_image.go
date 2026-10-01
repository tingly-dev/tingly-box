package protocolserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/vision/imagestore"
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
		stream.SendForwardingError(c, err)
		return
	}

	usage := protocol.NewTokenUsageWithCache(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), 0)
	ph.trackUsageWithTokenUsage(c, usage, nil)

	// Persist generated images under the config image directory (best-effort).
	setImageIDsHeader(c, ph.persistImageGeneration(&req, resp))

	c.JSON(http.StatusOK, resp)
}

// persistImageGeneration saves generated images and their prompts into the
// image archive (configDir/image/YYYYMMDD/, see internal/vision/imagestore).
// It is best-effort: any failure is logged but never blocks the response to
// the caller. Returns the archive id of each image in resp.Data order ("" for
// one that was not saved).
//
// This used to live inside the Codex client and wrote to .tingly-image/ in the
// process working directory. It now belongs to the server layer so persistence
// is uniform across providers and rooted at the application config directory.
func (ph *ProtocolHandler) persistImageGeneration(req *openai.ImageGenerateParams, resp *openai.ImagesResponse) []string {
	var meta imagestore.Meta
	if req != nil {
		meta = imagestore.Meta{
			Prompt:  req.Prompt,
			Model:   string(req.Model),
			Size:    string(req.Size),
			Quality: string(req.Quality),
			Format:  string(req.ResponseFormat),
			Style:   string(req.Style),
		}
	}
	return ph.persistImages(resp, meta)
}

// persistImageEdit is the edit-surface counterpart of persistImageGeneration:
// same directory layout and best-effort semantics, with edit-shaped metadata.
func (ph *ProtocolHandler) persistImageEdit(req *openai.ImageEditParams, resp *openai.ImagesResponse) []string {
	meta := imagestore.Meta{Operation: "edit"}
	if req != nil {
		meta.Prompt = req.Prompt
		meta.Model = string(req.Model)
		meta.Size = string(req.Size)
		meta.Quality = string(req.Quality)
	}
	return ph.persistImages(resp, meta)
}

// imageStore is the archive this handler persists into, rooted at the
// configured config directory (the same one the control-plane imagegen
// module reads from).
func (ph *ProtocolHandler) imageStore() *imagestore.Store {
	baseDir := ""
	if ph.deps.Config != nil {
		baseDir = ph.deps.Config.ConfigDir
	}
	if baseDir == "" {
		baseDir = constant.GetTinglyConfDir()
	}
	return imagestore.New(constant.GetImageDir(baseDir))
}

// persistImages writes each base64 image in resp into the archive. Shared by
// the generation and edit surfaces.
func (ph *ProtocolHandler) persistImages(resp *openai.ImagesResponse, meta imagestore.Meta) []string {
	if resp == nil || len(resp.Data) == 0 {
		return nil
	}
	store := ph.imageStore()
	ids := make([]string, len(resp.Data))
	for i, img := range resp.Data {
		// Only base64-encoded images can be persisted locally; URL-based
		// responses (e.g. some DashScope/MiniMax modes) are skipped.
		if img.B64JSON == "" {
			continue
		}
		imageData, err := base64.StdEncoding.DecodeString(img.B64JSON)
		if err != nil {
			logrus.Errorf("[ImageGen] Failed to decode base64 image data: %v", err)
			continue
		}
		id, err := store.Save(imageData, meta)
		if err != nil {
			logrus.Errorf("[ImageGen] Failed to persist image: %v", err)
		}
		if id != "" {
			ids[i] = id
			logrus.Infof("[ImageGen] Saved image %s under %s", id, store.Root())
		}
	}
	return ids
}

// imageIDsHeader carries the archive id of each returned image, in
// response-data order, comma-separated ("" for one that was not saved). It
// is how the Playground learns which archived file a result is, without the
// gateway's response body departing from the OpenAI shape. Other clients
// simply ignore it.
const imageIDsHeader = "X-Tingly-Image-Ids"

func setImageIDsHeader(c *gin.Context, ids []string) {
	for _, id := range ids {
		if id != "" {
			c.Header(imageIDsHeader, strings.Join(ids, ","))
			c.Header("Access-Control-Expose-Headers", imageIDsHeader)
			return
		}
	}
}
