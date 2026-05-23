package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// CodexClient wraps OpenAIClient with Codex-specific behaviors.
// It embeds OpenAIClient to inherit standard OpenAI API functionality,
// while overriding methods that require special handling for ChatGPT backend API.
//
// Codex (ChatGPT OAuth) limitations:
// - Does NOT support standard Chat Completions API
// - Does NOT support /models endpoint
// - Does NOT support /images/generations endpoint
// - ONLY supports Responses API with special parameters
type CodexClient struct {
	*OpenAIClient
}

// NewCodexClient creates a new Codex client wrapper.
// The base OpenAIClient is configured with codexRoundTripper for path/header transformation.
func NewCodexClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*CodexClient, error) {
	if provider.OAuthDetail == nil && provider.APIBase != protocol.CodexAPIBase {
		logrus.Fatalf("Codex client not configured with Codex provider")
		panic("Codex client not configured with Codex provider")
	}

	if provider.OAuthDetail.Issuer != ai.IssuerCodex {
		logrus.Fatalf("Codex client can only work for codex provider")
		panic("Codex client can only work for codex provider")
	}

	// Add X-ChatGPT-Account-ID header if available from OAuth metadata
	// The codexHook will transform this to ChatGPT-Account-ID and add other required headers
	// Reference: https://github.com/SamSaffron/term-llm/blob/main/internal/llm/chatgpt.go
	var options = []option.RequestOption{}
	if accountID, ok := provider.OAuthDetail.ExtraFields["account_id"].(string); ok && accountID != "" {
		options = append(options, option.WithHeader("X-ChatGPT-Account-ID", accountID))
	}

	// Use createSessionBoundTransport which applies OAuth hooks and uses shared transport
	transport := &codexRoundTripper{
		RoundTripper: createSessionBoundTransport(provider, sessionID),
	}
	httpClient := &http.Client{
		Transport: transport,
	}
	options = append(options, option.WithHTTPClient(httpClient))

	base, err := NewOpenAIClient(provider, model, sessionID, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create base OpenAI client: %w", err)
	}

	return &CodexClient{
		OpenAIClient: base,
	}, nil
}

// ChatCompletionsNew creates a new chat completion request.
// For Codex, this returns an error as ChatGPT backend API does not support standard Chat Completions.
// Use Responses API instead.
func (c *CodexClient) ChatCompletionsNew(ctx context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	return nil, &ErrCodexNotSupported{
		Operation: "Chat Completions",
		Reason:    "ChatGPT backend API does not support standard /v1/chat/completions endpoint. Use Responses API instead.",
	}
}

// ChatCompletionsNewStreaming creates a new streaming chat completion request.
// For Codex, this returns nil as ChatGPT backend API does not support standard Chat Completions.
// Use Responses API instead.
func (c *CodexClient) ChatCompletionsNewStreaming(ctx context.Context, req openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {
	logrus.WithContext(ctx).Errorf("[Codex] Chat Completions Streaming not supported, use Responses API instead")
	return nil
}

// ResponsesNew creates a new Responses API request.
// For Codex, this internally uses streaming mode and assembles the result
// into a non-streaming Response, as required by the ChatGPT backend API.
func (c *CodexClient) ResponsesNew(ctx context.Context, req responses.ResponseNewParams) (*responses.Response, error) {
	// Apply Codex-specific defaults to the request
	applyCodexDefaultsToParams(&req)

	// Call streaming API
	stream := c.OpenAIClient.ResponsesNewStreaming(ctx, req)
	defer stream.Close()

	// Parse streaming response and assemble into non-streaming Response
	return c.parseResponsesStream(ctx, stream)
}

// ResponsesNewStreaming creates a new streaming Responses API request with Codex-specific defaults.
func (c *CodexClient) ResponsesNewStreaming(ctx context.Context, req responses.ResponseNewParams) *ssestream.Stream[responses.ResponseStreamEventUnion] {
	// Apply Codex-specific defaults to the request
	applyCodexDefaultsToParams(&req)
	// Call the base implementation
	return c.OpenAIClient.ResponsesNewStreaming(ctx, req)
}

// ImagesGenerate creates a new image generation request.
// For Codex, this transforms the request to use the Responses API with the image_generation tool,
// as ChatGPT backend API does not support the standard /images/generations endpoint.
// Persistence of generated images is handled by the server layer, not the client.
func (c *CodexClient) ImagesGenerate(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	logrus.WithContext(ctx).Debugf("[Codex] Using Responses API for image generation, model: %s", req.Model)

	// Build Responses API request
	responsesReq := c.buildImageGenerationResponsesRequest(req)

	// Call streaming Responses API
	stream := c.OpenAIClient.ResponsesNewStreaming(ctx, responsesReq)

	// Parse streaming response
	return c.parseImageGenerationStream(ctx, stream)
}

// applyCodexDefaultsToParams applies Codex-specific defaults to a ResponseNewParams struct.
func applyCodexDefaultsToParams(req *responses.ResponseNewParams) {
	// Set default instructions if not provided
	if !req.Instructions.Valid() {
		req.Instructions = param.NewOpt(defaultInstructions)
	}
	// Set store to false for Codex
	req.Store = param.NewOpt(false)
	// Insert defaults only if client did not provide them
	if len(req.Tools) == 0 {
		req.Tools = []responses.ToolUnionParam{}
	}
	if !req.ParallelToolCalls.Valid() {
		req.ParallelToolCalls = param.NewOpt(false)
	}

	// Remove unsupported parameters for Codex
	// ChatGPT backend API does NOT support: temperature, top_p, max_output_tokens
	// Set them to invalid/zero state so they won't be included in the request
	req.Temperature = param.Null[float64]()
	req.TopP = param.Null[float64]()
	req.MaxOutputTokens = param.Null[int64]()

	// Merge "reasoning.encrypted_content" into existing include array (preserve client-provided values)
	includes := req.Include
	hasMarker := false
	for _, v := range includes {
		if string(v) == reasoningMarker {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		includes = append(includes, responses.ResponseIncludable(reasoningMarker))
	}
	req.Include = includes

	// Get the current extra fields (call the method)
	extraFields := map[string]interface{}{}
	if len(req.ExtraFields()) > 0 {
		// Copy existing extra fields
		for k, v := range req.ExtraFields() {
			if k == "max_output_tokens" {
				continue
			}
			extraFields[k] = v
		}
	}

	extraFields["stream"] = true

	// ChatGPT Codex rejects empty/invalid item ids in input[].
	// These ids are optional for request items, so strip malformed values.
	sanitizeResponseInputIDs(req)

	// Set the modified extra fields back
	req.SetExtraFields(extraFields)
}

// sanitizeResponseInputIDs sanitizes item IDs in ResponseNewParams.Input for Codex.
// ChatGPT Codex rejects empty/invalid item ids, so we strip malformed values
// and drop reasoning items whose required plain-string ID cannot be omitted.
func sanitizeResponseInputIDs(req *responses.ResponseNewParams) {
	if req.Input.OfInputItemList == nil {
		return
	}

	inputItems := req.Input.OfInputItemList
	sanitized := inputItems[:0]
	for i := range inputItems {
		item := inputItems[i]
		if sanitizeInputItemID(&item) {
			sanitized = append(sanitized, item)
		}
	}

	req.Input.OfInputItemList = sanitized
}

// sanitizeInputItemID sanitizes the ID field in a ResponseInputItemUnionParam
// by clearing invalid IDs directly on the inner SDK struct fields.
// Returns false if the item must be dropped entirely (because its required
// id field is invalid and cannot be omitted).
func sanitizeInputItemID(item *responses.ResponseInputItemUnionParam) bool {
	// Optional Opt[string] ids: clear when invalid so the SDK omits the field.
	if item.OfFunctionCall != nil {
		sanitizeOptID(&item.OfFunctionCall.ID)
	}
	if item.OfFunctionCallOutput != nil {
		sanitizeOptID(&item.OfFunctionCallOutput.ID)
	}
	if item.OfComputerCallOutput != nil {
		sanitizeOptID(&item.OfComputerCallOutput.ID)
	}
	if item.OfCustomToolCall != nil {
		sanitizeOptID(&item.OfCustomToolCall.ID)
	}
	if item.OfCustomToolCallOutput != nil {
		sanitizeOptID(&item.OfCustomToolCallOutput.ID)
	}
	if item.OfShellCall != nil {
		sanitizeOptID(&item.OfShellCall.ID)
	}
	if item.OfShellCallOutput != nil {
		sanitizeOptID(&item.OfShellCallOutput.ID)
	}
	if item.OfApplyPatchCall != nil {
		sanitizeOptID(&item.OfApplyPatchCall.ID)
	}
	if item.OfApplyPatchCallOutput != nil {
		sanitizeOptID(&item.OfApplyPatchCallOutput.ID)
	}
	if item.OfMcpApprovalResponse != nil {
		sanitizeOptID(&item.OfMcpApprovalResponse.ID)
	}
	if item.OfCompaction != nil {
		sanitizeOptID(&item.OfCompaction.ID)
	}

	// Required plain-string ids: cannot be omitted, drop the item if invalid.
	if item.OfReasoning != nil {
		item.OfReasoning.ID = strings.TrimSpace(item.OfReasoning.ID)
		if item.OfReasoning.ID == "" || !isValidCodexID(item.OfReasoning.ID) {
			logrus.Warnf("[Codex] Dropping reasoning input item with invalid id: %q", item.OfReasoning.ID)
			return false
		}
	}
	if item.OfFileSearchCall != nil && !isValidCodexIDStrict(item.OfFileSearchCall.ID) {
		logrus.Warnf("[Codex] Dropping file_search_call input item with invalid id: %q", item.OfFileSearchCall.ID)
		return false
	}
	if item.OfComputerCall != nil && !isValidCodexIDStrict(item.OfComputerCall.ID) {
		logrus.Warnf("[Codex] Dropping computer_call input item with invalid id: %q", item.OfComputerCall.ID)
		return false
	}
	if item.OfWebSearchCall != nil && !isValidCodexIDStrict(item.OfWebSearchCall.ID) {
		logrus.Warnf("[Codex] Dropping web_search_call input item with invalid id: %q", item.OfWebSearchCall.ID)
		return false
	}
	if item.OfImageGenerationCall != nil && !isValidCodexIDStrict(item.OfImageGenerationCall.ID) {
		logrus.Warnf("[Codex] Dropping image_generation_call input item with invalid id: %q", item.OfImageGenerationCall.ID)
		return false
	}
	if item.OfCodeInterpreterCall != nil && !isValidCodexIDStrict(item.OfCodeInterpreterCall.ID) {
		logrus.Warnf("[Codex] Dropping code_interpreter_call input item with invalid id: %q", item.OfCodeInterpreterCall.ID)
		return false
	}
	if item.OfLocalShellCall != nil && !isValidCodexIDStrict(item.OfLocalShellCall.ID) {
		logrus.Warnf("[Codex] Dropping local_shell_call input item with invalid id: %q", item.OfLocalShellCall.ID)
		return false
	}
	if item.OfLocalShellCallOutput != nil && !isValidCodexIDStrict(item.OfLocalShellCallOutput.ID) {
		logrus.Warnf("[Codex] Dropping local_shell_call_output input item with invalid id: %q", item.OfLocalShellCallOutput.ID)
		return false
	}
	if item.OfMcpListTools != nil && !isValidCodexIDStrict(item.OfMcpListTools.ID) {
		logrus.Warnf("[Codex] Dropping mcp_list_tools input item with invalid id: %q", item.OfMcpListTools.ID)
		return false
	}
	if item.OfMcpApprovalRequest != nil && !isValidCodexIDStrict(item.OfMcpApprovalRequest.ID) {
		logrus.Warnf("[Codex] Dropping mcp_approval_request input item with invalid id: %q", item.OfMcpApprovalRequest.ID)
		return false
	}
	if item.OfMcpCall != nil && !isValidCodexIDStrict(item.OfMcpCall.ID) {
		logrus.Warnf("[Codex] Dropping mcp_call input item with invalid id: %q", item.OfMcpCall.ID)
		return false
	}
	if item.OfItemReference != nil && !isValidCodexIDStrict(item.OfItemReference.ID) {
		logrus.Warnf("[Codex] Dropping item_reference input item with invalid id: %q", item.OfItemReference.ID)
		return false
	}
	return true
}

// isValidCodexIDStrict returns true when id is non-empty (after trimming) and
// contains only characters accepted by the ChatGPT backend.
func isValidCodexIDStrict(id string) bool {
	trimmed := strings.TrimSpace(id)
	return trimmed != "" && isValidCodexID(trimmed)
}

func sanitizeOptID(id *param.Opt[string]) {
	if !id.Valid() {
		return
	}
	v := strings.TrimSpace(id.Value)
	if v == "" || !isValidCodexID(v) {
		*id = param.Opt[string]{}
	}
}

// isValidCodexID checks if a string is a valid Codex ID.
// Valid IDs contain only alphanumeric characters, underscores, and hyphens.
func isValidCodexID(id string) bool {
	if len(id) == 0 {
		return false
	}
	for _, c := range id {
		if !isAlnumunderscoreHyphen(c) {
			return false
		}
	}
	return true
}

// isAlnumunderscoreHyphen checks if a rune is alphanumeric, underscore, or hyphen.
func isAlnumunderscoreHyphen(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_' || c == '-'
}

// ListModels returns the list of available models.
// For Codex, this returns an error as ChatGPT OAuth tokens cannot access /models endpoint.
func (c *CodexClient) ListModels(ctx context.Context) ([]string, error) {
	return nil, &ErrModelsEndpointNotSupported{
		Provider: c.provider.Name,
		Reason:   "ChatGPT OAuth token cannot access /models endpoint",
	}
}

// buildImageGenerationResponsesRequest transforms ImageGenerateParams into
// a Responses API request with the image_generation tool.
func (c *CodexClient) buildImageGenerationResponsesRequest(req openai.ImageGenerateParams) responses.ResponseNewParams {
	// Build the Responses API request with Codex-specific defaults
	params := responses.ResponseNewParams{
		Model: req.Model,
	}

	// Set default values directly on the struct
	params.Store = param.NewOpt(false)
	params.Instructions = param.NewOpt(defaultInstructions)
	params.ParallelToolCalls = param.NewOpt(false)
	params.Include = []responses.ResponseIncludable{responses.ResponseIncludable(reasoningMarker)}

	// Build input content
	contentItem := responses.ResponseInputContentParamOfInputText(string(req.Prompt))
	contentItems := responses.ResponseInputMessageContentListParam{contentItem}

	// Build input message
	inputItem := responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Type:    responses.EasyInputMessageTypeMessage,
			Role:    responses.EasyInputMessageRoleUser,
			Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: contentItems},
		},
	}
	inputItems := responses.ResponseInputParam{inputItem}
	params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: inputItems}

	// Determine quality
	quality := "auto"
	if req.Quality != "" {
		qualityStr := string(req.Quality)
		if qualityStr == "standard" {
			quality = "medium"
		} else if qualityStr == "hd" {
			quality = "high"
		} else {
			quality = qualityStr
		}
	}

	// Determine output format
	outputFormat := "png"
	if req.ResponseFormat != "" {
		outputFormat = string(req.ResponseFormat)
	}

	// Build image_generation tool
	toolParam := &responses.ToolImageGenerationParam{
		Type:         "image_generation",
		Size:         string(req.Size),
		Quality:      quality,
		OutputFormat: outputFormat,
		//Action:       "auto",
		//Background:   "auto",
		//Moderation:   "auto",
	}

	params.Tools = []responses.ToolUnionParam{{OfImageGeneration: toolParam}}

	// Log warning for unsupported N parameter
	if req.N.Valid() {
		n := req.N.Value
		if n > 1 {
			logrus.Warnf("[Codex] Multiple images (N=%d) not supported, using N=1", n)
		}
	}

	// Log warning for unsupported style parameter
	if req.Style != "" {
		logrus.Warnf("[Codex] Style parameter not supported for image generation")
	}

	// Set stream=true via ExtraFields
	extraFields := map[string]interface{}{
		"stream": true,
	}
	params.SetExtraFields(extraFields)

	return params
}

// parseImageGenerationStream parses the streaming Responses API response
// and extracts the generated image data from the output array.
//
// The image data comes through two event types:
// 1. response.image_generation_call.partial_image - streaming partial image chunks
// 2. response.output_item.done - final status of the image generation call
func (c *CodexClient) parseImageGenerationStream(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion]) (*openai.ImagesResponse, error) {
	defer stream.Close()

	var b64JSON string
	var imageCallID string

	// Collect image data from stream events
	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "response.image_generation_call.partial_image":
			// Partial image chunks during generation
			partialEvent := event.AsResponseImageGenerationCallPartialImage()
			if partialEvent.PartialImageB64 != "" {
				b64JSON += partialEvent.PartialImageB64
				imageCallID = partialEvent.ItemID
				logrus.WithContext(ctx).Debugf("[Codex] Received partial image chunk, item_id: %s, total_size: %d",
					partialEvent.ItemID, len(b64JSON))
			}

		case "response.output_item.done":
			// Final status of output items
			doneEvent := event.AsResponseOutputItemDone()
			item := doneEvent.Item

			if item.Type == "image_generation_call" {
				imageCall := item.AsImageGenerationCall()
				// Check for image result in the done event
				// The status can be "generating", "completed", or other values
				// If we haven't received partial images, use the Result field
				if b64JSON == "" && imageCall.Result != "" {
					b64JSON = imageCall.Result
					imageCallID = imageCall.ID
					logrus.WithContext(ctx).Debugf("[Codex] Received image result in done event, id: %s, status: %s",
						imageCall.ID, imageCall.Status)
				}
				// Update imageCallID even if we already have data from partial events
				if imageCallID == "" {
					imageCallID = imageCall.ID
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	if b64JSON == "" {
		return nil, fmt.Errorf("no image data in response (image_call_id: %s)", imageCallID)
	}

	logrus.WithContext(ctx).Infof("[Codex] Successfully extracted image data, id: %s, size: %d bytes", imageCallID, len(b64JSON))

	// Build standard ImagesResponse from extracted data
	return &openai.ImagesResponse{
		Data: []openai.Image{
			{
				B64JSON: b64JSON,
			},
		},
	}, nil
}

// parseImageGenerationStreamNext parses the streaming Responses API response
// and extracts the generated image data using the ResponsesAssembler.
//
// The image data comes through two event types:
// 1. response.image_generation_call.partial_image - streaming partial image chunks
// 2. response.output_item.done - final status of the image generation call
//
// Supports multiple images via different output indices.
func (c *CodexClient) parseImageGenerationStreamNext(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion]) (*openai.ImagesResponse, error) {
	defer stream.Close()

	// Use assembler to accumulate streaming events
	asm := assembler.NewResponsesAssembler()

	for stream.Next() {
		event := stream.Current()
		asm.Accumulate(event)

		// Early exit on terminal states
		if asm.IsFinished() {
			break
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	// Get all images from assembler
	imagesMap := asm.Images()
	if len(imagesMap) == 0 {
		return nil, fmt.Errorf("no image data in response")
	}

	// Build ImagesResponse with all images
	// Sort by output index to maintain order
	imageCount := asm.ImageCount()
	data := make([]openai.Image, 0, imageCount)
	for idx := 0; idx < imageCount; idx++ {
		b64JSON := asm.ImageDataAt(idx)
		if b64JSON == "" {
			logrus.WithContext(ctx).Warnf("[Codex] Missing image data at index %d", idx)
			continue
		}
		data = append(data, openai.Image{
			B64JSON: b64JSON,
		})
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no valid image data in response")
	}

	logrus.WithContext(ctx).Infof("[Codex] Successfully extracted %d image(s) via assembler", len(data))
	return &openai.ImagesResponse{
		Data: data,
	}, nil
}

// parseResponsesStream parses the streaming Responses API response
// and assembles it into a complete non-streaming Response object using
// the ResponsesAssembler.
func (c *CodexClient) parseResponsesStream(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion]) (*responses.Response, error) {
	defer stream.Close()

	// Use assembler to accumulate streaming events
	asm := assembler.NewResponsesAssembler()

	for stream.Next() {
		event := stream.Current()
		asm.Accumulate(event)

		// Early exit on terminal states
		if asm.IsFinished() {
			break
		}
	}

	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream error: %w", err)
	}

	// Get the final response from assembler
	resp := asm.Finish()
	if resp == nil {
		return nil, fmt.Errorf("response assembly failed: status=%s", asm.Status())
	}

	logrus.WithContext(ctx).Debugf("[Codex] Response assembled via assembler, id: %s, status: %s", resp.ID, asm.Status())
	return resp, nil
}

// SetRecordSink sets the record sink for the client.
// For CodexClient, we delegate to the embedded OpenAIClient.
func (c *CodexClient) SetRecordSink(sink *obs.Sink) {
	c.OpenAIClient.SetRecordSink(sink)
}

// Client returns the underlying OpenAI SDK client.
// For CodexClient, we delegate to the embedded OpenAIClient.
func (c *CodexClient) Client() *openai.Client {
	return c.OpenAIClient.Client()
}

// ProbeChatEndpoint tests the chat endpoint.
// For Codex, this delegates to the embedded OpenAIClient's probeResponsesEndpoint.
func (c *CodexClient) Probe(ctx context.Context, model string) ProbeResult {
	startTime := time.Now()
	r, err := c.ProbeStream(ctx, model, "hi", ProbeModeTool)
	latencyMs := time.Since(startTime).Milliseconds()
	if err != nil {
		return ProbeResult{
			Success:      false,
			ErrorMessage: err.Error(),
			LatencyMs:    latencyMs,
		}
	}

	return ProbeResult{
		Success:          true,
		Message:          "Responses endpoint is accessible",
		Content:          r.Content,
		LatencyMs:        r.LatencyMs,
		PromptTokens:     r.PromptTokens,
		CompletionTokens: r.CompletionTokens,
		TotalTokens:      r.TotalTokens,
	}
}

// ProbeStream performs a streaming probe with configurable test mode (public interface)
// Codex uses the Responses API with proper beta headers
func (c *CodexClient) ProbeStream(ctx context.Context, model, message string, testMode ProbeMode) (*ProbeResult, error) {
	return c.probeChatStream(ctx, model, message, testMode)
}

// probeChatStream performs a streaming probe using Codex's Responses API
// Codex requires special handling:
// - Uses Responses API (not Chat Completions)
// - Applies beta headers for responses API
// - Proper session ID injection
func (c *CodexClient) probeChatStream(ctx context.Context, model, message string, testMode ProbeMode) (*ProbeResult, error) {
	return nil, fmt.Errorf("Codex do not support chat complement")
}

func (c *CodexClient) ProbeResponsesStream(ctx context.Context, model, message string, testMode ProbeMode) (*ProbeResult, error) {
	startTime := time.Now()

	// Build Responses API request
	params := responses.ResponseNewParams{
		Model:        model,
		Instructions: param.NewOpt("work as `echo` if possible"),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: []responses.ResponseInputItemUnionParam{
				responses.ResponseInputItemParamOfMessage(
					responses.ResponseInputMessageContentListParam{
						responses.ResponseInputContentParamOfInputText(message),
					},
					responses.EasyInputMessageRoleUser,
				),
			},
		},
	}

	// Add tools for tool mode
	if testMode == ProbeModeTool {
		params.Tools = GetProbeToolsResponses()
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	}

	// Use ResponsesNewStreaming with proper beta headers
	stream := c.ResponsesNewStreaming(ctx, params)
	defer stream.Close()

	var chunks []interface{}
	for stream.Next() {
		chunks = append(chunks, stream.Current())
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	chunksJSON, _ := json.Marshal(chunks)
	return ToProbeResult(string(chunksJSON), time.Since(startTime).Milliseconds(), c.provider.APIBase+"/responses", true), nil
}
