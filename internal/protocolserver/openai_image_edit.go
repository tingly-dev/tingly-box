package protocolserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocolserver/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// maxImageEditFormMemory caps how much of a multipart image-edit upload is
// held in memory before net/http spills parts to temp files.
const maxImageEditFormMemory = 32 << 20 // 32 MiB

// HandleOpenAIImageEdit serves OpenAI-compatible image edit requests against
// the upstream POST /v1/images/edits surface. Two inbound encodings are
// accepted:
//
//   - multipart/form-data — the standard OpenAI wire format (image files +
//     form fields), as sent by the official SDKs and curl -F.
//   - application/json — a convenience mirror of the same fields where each
//     image is an inline base64 string or data URL, matching how the
//     Codex-native edit protocol references images. Useful for programmatic
//     callers that already hold base64 (and for chaining a previous
//     generation's b64_json straight back into an edit).
//
// The wrapper's ImagesEdit hides vendor fragmentation exactly like image
// generation does: OpenAI-compatible providers get the SDK's multipart
// upload, Codex (ChatGPT OAuth) gets its native JSON images/edits endpoint.
//
// Exposed via the mixin route group alongside /images/generations; the
// canonical home is the dedicated `imagegen` scenario.
func (ph *ProtocolHandler) HandleOpenAIImageEdit(c *gin.Context) {
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
				Message: fmt.Sprintf("scenario %s does not support image edit", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	req, err := parseImageEditRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
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

	sessionID := resolveSessionID(c, req)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	SetTrackingContext(c, rule, provider, actualModel, string(responseModel), false)

	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	resp, cancel, err := forwarding.ForwardOpenAIImageEdit(fc, wrapper, req)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		ph.trackUsageWithTokenUsage(c, usage, err)
		logrus.Errorf("Failed to forward image edit request: %v", err)
		c.JSON(protocol.UpstreamStatus(err, http.StatusInternalServerError), ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + protocol.UpstreamMessage(err),
				Type:    "api_error",
			},
		})
		return
	}

	usage := protocol.NewTokenUsageWithCache(int(resp.Usage.InputTokens), int(resp.Usage.OutputTokens), 0)
	ph.trackUsageWithTokenUsage(c, usage, nil)

	// Persist edited images under the config image directory (best-effort).
	ph.persistImageEdit(req, resp)

	c.JSON(http.StatusOK, resp)
}

// parseImageEditRequest decodes the inbound request into ImageEditParams,
// dispatching on Content-Type, and validates the required fields.
func parseImageEditRequest(c *gin.Context) (*openai.ImageEditParams, error) {
	var (
		req *openai.ImageEditParams
		err error
	)
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		req, err = parseImageEditMultipart(c)
	} else {
		req, err = parseImageEditJSON(c)
	}
	if err != nil {
		return nil, err
	}

	if string(req.Model) == "" {
		return nil, fmt.Errorf("Model is required")
	}
	if req.Prompt == "" {
		return nil, fmt.Errorf("Prompt is required")
	}
	if req.Image.OfFile == nil && len(req.Image.OfFileArray) == 0 {
		return nil, fmt.Errorf("At least one input image is required")
	}
	return req, nil
}

// parseImageEditMultipart parses the standard OpenAI multipart wire format.
// Image files may arrive under "image" (repeated) or "image[]" — both spellings
// are used in the wild across SDKs and curl examples.
func parseImageEditMultipart(c *gin.Context) (*openai.ImageEditParams, error) {
	if err := c.Request.ParseMultipartForm(maxImageEditFormMemory); err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %s", err.Error())
	}
	form := c.Request.MultipartForm
	if form == nil {
		return nil, fmt.Errorf("empty multipart form")
	}

	var fileHeaders []*multipart.FileHeader
	for _, key := range []string{"image", "image[]"} {
		fileHeaders = append(fileHeaders, form.File[key]...)
	}
	if len(fileHeaders) == 0 {
		return nil, fmt.Errorf("at least one image file is required (field \"image\" or \"image[]\")")
	}

	req := &openai.ImageEditParams{}
	for _, fh := range fileHeaders {
		data, err := readMultipartFile(fh)
		if err != nil {
			return nil, fmt.Errorf("failed to read image %q: %s", fh.Filename, err.Error())
		}
		contentType := fh.Header.Get("Content-Type")
		if contentType == "" || contentType == "application/octet-stream" {
			contentType = http.DetectContentType(data)
		}
		file := openai.File(bytes.NewReader(data), fh.Filename, contentType)
		if len(fileHeaders) == 1 {
			req.Image.OfFile = file
		} else {
			req.Image.OfFileArray = append(req.Image.OfFileArray, file)
		}
	}

	if masks := form.File["mask"]; len(masks) > 0 {
		data, err := readMultipartFile(masks[0])
		if err != nil {
			return nil, fmt.Errorf("failed to read mask: %s", err.Error())
		}
		req.Mask = openai.File(bytes.NewReader(data), masks[0].Filename, "image/png")
	}

	formValue := func(key string) string {
		if vs := form.Value[key]; len(vs) > 0 {
			return vs[0]
		}
		return ""
	}

	req.Prompt = formValue("prompt")
	req.Model = openai.ImageModel(formValue("model"))
	req.Size = openai.ImageEditParamsSize(formValue("size"))
	req.Quality = openai.ImageEditParamsQuality(formValue("quality"))
	req.Background = openai.ImageEditParamsBackground(formValue("background"))
	req.ResponseFormat = openai.ImageEditParamsResponseFormat(formValue("response_format"))
	req.OutputFormat = openai.ImageEditParamsOutputFormat(formValue("output_format"))
	req.InputFidelity = openai.ImageEditParamsInputFidelity(formValue("input_fidelity"))
	if v := formValue("user"); v != "" {
		req.User = param.NewOpt(v)
	}
	if v := formValue("n"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid n: %s", v)
		}
		req.N = param.NewOpt(n)
	}
	if v := formValue("output_compression"); v != "" {
		oc, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid output_compression: %s", v)
		}
		req.OutputCompression = param.NewOpt(oc)
	}

	return req, nil
}

func readMultipartFile(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// imageEditJSONRequest is the JSON mirror of the multipart edit form. Image
// carries one or more inline images (data URL or bare base64).
type imageEditJSONRequest struct {
	Image             imageEditJSONImages `json:"image"`
	Prompt            string              `json:"prompt"`
	Model             string              `json:"model"`
	N                 *int64              `json:"n,omitempty"`
	Size              string              `json:"size,omitempty"`
	Quality           string              `json:"quality,omitempty"`
	Background        string              `json:"background,omitempty"`
	ResponseFormat    string              `json:"response_format,omitempty"`
	OutputFormat      string              `json:"output_format,omitempty"`
	OutputCompression *int64              `json:"output_compression,omitempty"`
	InputFidelity     string              `json:"input_fidelity,omitempty"`
	User              string              `json:"user,omitempty"`
}

// imageEditJSONImages accepts either a single string or an array of strings.
type imageEditJSONImages []string

func (i *imageEditJSONImages) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*i = []string{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("image must be a base64/data-URL string or an array of them")
	}
	*i = many
	return nil
}

// parseImageEditJSON parses the JSON convenience encoding. Remote http(s)
// image URLs are intentionally rejected: the gateway does not fetch arbitrary
// URLs on behalf of callers (SSRF), it only decodes inline content.
func parseImageEditJSON(c *gin.Context) (*openai.ImageEditParams, error) {
	bodyBytes, err := c.GetRawData()
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %s", err.Error())
	}

	var jsonReq imageEditJSONRequest
	if err := json.Unmarshal(bodyBytes, &jsonReq); err != nil {
		return nil, fmt.Errorf("invalid request body: %s", err.Error())
	}

	req := &openai.ImageEditParams{
		Prompt:         jsonReq.Prompt,
		Model:          openai.ImageModel(jsonReq.Model),
		Size:           openai.ImageEditParamsSize(jsonReq.Size),
		Quality:        openai.ImageEditParamsQuality(jsonReq.Quality),
		Background:     openai.ImageEditParamsBackground(jsonReq.Background),
		ResponseFormat: openai.ImageEditParamsResponseFormat(jsonReq.ResponseFormat),
		OutputFormat:   openai.ImageEditParamsOutputFormat(jsonReq.OutputFormat),
		InputFidelity:  openai.ImageEditParamsInputFidelity(jsonReq.InputFidelity),
	}
	if jsonReq.N != nil {
		req.N = param.NewOpt(*jsonReq.N)
	}
	if jsonReq.OutputCompression != nil {
		req.OutputCompression = param.NewOpt(*jsonReq.OutputCompression)
	}
	if jsonReq.User != "" {
		req.User = param.NewOpt(jsonReq.User)
	}

	for idx, s := range jsonReq.Image {
		data, contentType, err := decodeInlineImage(s)
		if err != nil {
			return nil, fmt.Errorf("invalid image[%d]: %s", idx, err.Error())
		}
		name := "image-" + strconv.Itoa(idx) + extensionForImageContentType(contentType)
		file := openai.File(bytes.NewReader(data), name, contentType)
		if len(jsonReq.Image) == 1 {
			req.Image.OfFile = file
		} else {
			req.Image.OfFileArray = append(req.Image.OfFileArray, file)
		}
	}

	return req, nil
}

// decodeInlineImage decodes a data URL or bare base64 string into image bytes
// plus a content type (sniffed when the data URL does not declare one). The
// data-URL split reuses request.ParseImageURLToAnthropicSource, the same
// helper the vision proxy and Google converters use for OpenAI image_url
// strings, rather than re-parsing the "data:<mime>;base64,<payload>" shape
// here.
func decodeInlineImage(s string) ([]byte, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "", fmt.Errorf("empty image content")
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return nil, "", fmt.Errorf("remote image URLs are not supported; inline the image as base64 or a data URL")
	}

	mediaType, payload, remoteURL := request.ParseImageURLToAnthropicSource(s)
	if remoteURL != "" {
		// Not a "data:...;base64,..." URL — treat the whole string as bare
		// base64 (or let the decode below reject it if it's malformed).
		payload = remoteURL
	}

	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("invalid base64 image content: %s", err.Error())
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty image content")
	}

	contentType := mediaType
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

// extensionForImageContentType maps common image media types to a file
// extension for the synthetic upload filename.
func extensionForImageContentType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}
