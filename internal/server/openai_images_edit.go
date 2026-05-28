package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIImageEdit serves OpenAI-compatible image editing requests
// (POST /images/edits). The upstream accepts multipart/form-data containing
// one or more source images, an optional inpainting mask, and the prompt
// fields. Vendor fragmentation is hidden inside the client wrapper:
//   - OpenAI-compatible providers → SDK Images.Edit
//   - Codex (ChatGPT OAuth) → Responses API with image_generation tool, action:"edit"
//   - DashScope / MiniMax → ErrEditUnsupported → 400
func (s *Server) HandleOpenAIImageEdit(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	if !isValidRuleScenario(scenarioType) {
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
				Message: fmt.Sprintf("scenario %s does not support image editing", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Parse multipart form. OpenAI specifies max 25 MB per file for GPT image
	// models; we allow up to 100 MB total form size for multi-image requests.
	if err := c.Request.ParseMultipartForm(100 << 20); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to parse multipart form: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	form := c.Request.MultipartForm

	// --- Required fields ---
	prompt := c.PostForm("prompt")
	if prompt == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{Message: "prompt is required", Type: "invalid_request_error"},
		})
		return
	}

	modelStr := c.PostForm("model")
	if modelStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{Message: "model is required", Type: "invalid_request_error"},
		})
		return
	}

	// --- Build ImageEditParams ---
	req := openai.ImageEditParams{
		Prompt: prompt,
		Model:  openai.ImageModel(modelStr),
	}

	// Scalar optional fields.
	if v := c.PostForm("size"); v != "" {
		req.Size = openai.ImageEditParamsSize(v)
	}
	if v := c.PostForm("quality"); v != "" {
		req.Quality = openai.ImageEditParamsQuality(v)
	}
	if v := c.PostForm("background"); v != "" {
		req.Background = openai.ImageEditParamsBackground(v)
	}
	if v := c.PostForm("output_format"); v != "" {
		req.OutputFormat = openai.ImageEditParamsOutputFormat(v)
	}
	if v := c.PostForm("response_format"); v != "" {
		req.ResponseFormat = openai.ImageEditParamsResponseFormat(v)
	}
	if v := c.PostForm("input_fidelity"); v != "" {
		req.InputFidelity = openai.ImageEditParamsInputFidelity(v)
	}

	// Source image(s). The field name is "image"; multiple files share the
	// same name. A single file sets OfFile; multiple files set OfFileArray.
	imageFiles := form.File["image"]
	if len(imageFiles) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{Message: "image is required", Type: "invalid_request_error"},
		})
		return
	}

	if len(imageFiles) == 1 {
		f, err := imageFiles[0].Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "failed to open image: " + err.Error(),
					Type:    "invalid_request_error",
				},
			})
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "failed to read image: " + err.Error(),
					Type:    "api_error",
				},
			})
			return
		}
		req.Image = openai.ImageEditParamsImageUnion{OfFile: bytes.NewReader(data)}
	} else {
		readers := make([]io.Reader, 0, len(imageFiles))
		for _, fh := range imageFiles {
			f, err := fh.Open()
			if err != nil {
				c.JSON(http.StatusBadRequest, ErrorResponse{
					Error: ErrorDetail{
						Message: "failed to open image: " + err.Error(),
						Type:    "invalid_request_error",
					},
				})
				return
			}
			defer f.Close()
			data, err := io.ReadAll(f)
			if err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error: ErrorDetail{
						Message: "failed to read image: " + err.Error(),
						Type:    "api_error",
					},
				})
				return
			}
			readers = append(readers, bytes.NewReader(data))
		}
		req.Image = openai.ImageEditParamsImageUnion{OfFileArray: readers}
	}

	// Optional mask file.
	if maskFiles := form.File["mask"]; len(maskFiles) > 0 {
		f, err := maskFiles[0].Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Message: "failed to open mask: " + err.Error(),
					Type:    "invalid_request_error",
				},
			})
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: ErrorDetail{
					Message: "failed to read mask: " + err.Error(),
					Type:    "api_error",
				},
			})
			return
		}
		req.Mask = bytes.NewReader(data)
	}

	requestModel := req.Model

	rule, err := s.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{Message: err.Error(), Type: "invalid_request_error"},
		})
		return
	}

	provider, selectedService, err := s.routingSelector.SelectServiceForImageGeneration(c, scenarioType, rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{Message: err.Error(), Type: "invalid_request_error"},
		})
		return
	}

	actualModel := selectedService.Model
	req.Model = openai.ImageModel(actualModel)

	sessionID := resolveSessionID(c, &req)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	SetTrackingContext(c, rule, provider, actualModel, string(requestModel), false)

	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	resp, cancel, err := forwarding.ForwardOpenAIImageEdit(fc, wrapper, &req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		s.trackUsageWithTokenUsage(c, usage, err)
		logrus.Errorf("Failed to forward image edit request: %v", err)
		statusCode := http.StatusInternalServerError
		if isUnsupportedEditError(err) {
			statusCode = http.StatusBadRequest
		}
		c.JSON(statusCode, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	usage := protocol.NewTokenUsageWithCache(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), 0)
	s.trackUsageWithTokenUsage(c, usage, nil)

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

// isUnsupportedEditError returns true when the error originates from a vendor
// that explicitly does not support image editing (ErrEditUnsupported).
func isUnsupportedEditError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not support image editing")
}
