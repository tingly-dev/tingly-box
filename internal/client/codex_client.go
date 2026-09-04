package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// guard
var _ OpenAIClientInterface = (*CodexClient)(nil)

// CodexClient wraps OpenAIClient with Codex-specific behaviors.
// It embeds OpenAIClient to inherit standard OpenAI API functionality,
// while overriding methods that require special handling for ChatGPT backend API.
//
// Codex (ChatGPT OAuth) limitations:
//   - Does NOT support standard Chat Completions API
//   - Does NOT support /models endpoint
//   - Does NOT support the public /images/generations or /images/edits contracts;
//     both operations use Codex-native JSON image endpoints (see codex_images.go)
//   - Chat surfaces ONLY work through the Responses API with special parameters
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
	if accountID := provider.OAuthDetail.GetExtraFieldString("account_id"); accountID != "" {
		options = append(options, option.WithHeader("X-ChatGPT-Account-ID", accountID))
	}

	// Use createSessionBoundTransport which applies OAuth hooks and uses shared transport
	transport := &codexRoundTripper{
		RoundTripper: createSessionBoundTransport(provider, sessionID),
	}
	httpClient := &http.Client{
		Transport: wrapWithLogging(transport, provider),
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

// ImagesGenerate serves an OpenAI /images/generations request against the
// Codex-native JSON endpoint and preserves every returned data element.
func (c *CodexClient) ImagesGenerate(ctx context.Context, req openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	logrus.WithContext(ctx).Debugf("[Codex] Using native images/generations endpoint, model: %s", req.Model)

	var resp openai.ImagesResponse
	opts := []option.RequestOption{
		option.WithHeader("x-codex-image-turn-id", uuid.NewString()),
	}
	if err := c.Client().Post(ctx, "images/generations", buildCodexImageGenerationRequest(&req), &resp, opts...); err != nil {
		return nil, fmt.Errorf("codex image generation failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("codex image generation returned no image data")
	}

	logrus.WithContext(ctx).Infof("[Codex] Image generation succeeded, images: %d", len(resp.Data))
	return &resp, nil
}

// fastModelSuffix marks a virtual Codex catalog model id (e.g. "gpt-5.6-sol:fast")
// that requests OpenAI's priority service tier at higher cost. It is stripped
// before the request reaches the ChatGPT backend API, which knows nothing about it.
const fastModelSuffix = ":fast"

// applyCodexDefaultsToParams applies Codex-specific defaults to a ResponseNewParams struct.
func applyCodexDefaultsToParams(req *responses.ResponseNewParams) {
	// Resolve the ":fast" virtual model suffix into the real model id + priority service tier.
	if strings.HasSuffix(req.Model, fastModelSuffix) {
		req.Model = strings.TrimSuffix(req.Model, fastModelSuffix)
		req.ServiceTier = responses.ResponseNewParamsServiceTierPriority
	}

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
			logrus.Debugf("[Codex] Dropping reasoning input item with invalid id: %q", item.OfReasoning.ID)
			return false
		}
	}
	if item.OfFileSearchCall != nil && !isValidCodexIDStrict(item.OfFileSearchCall.ID) {
		logrus.Debugf("[Codex] Dropping file_search_call input item with invalid id: %q", item.OfFileSearchCall.ID)
		return false
	}
	if item.OfComputerCall != nil && !isValidCodexIDStrict(item.OfComputerCall.ID) {
		logrus.Debugf("[Codex] Dropping computer_call input item with invalid id: %q", item.OfComputerCall.ID)
		return false
	}
	if item.OfWebSearchCall != nil && !isValidCodexIDStrict(item.OfWebSearchCall.ID) {
		logrus.Debugf("[Codex] Dropping web_search_call input item with invalid id: %q", item.OfWebSearchCall.ID)
		return false
	}
	if item.OfImageGenerationCall != nil && !isValidCodexIDStrict(item.OfImageGenerationCall.ID) {
		logrus.Debugf("[Codex] Dropping image_generation_call input item with invalid id: %q", item.OfImageGenerationCall.ID)
		return false
	}
	if item.OfCodeInterpreterCall != nil && !isValidCodexIDStrict(item.OfCodeInterpreterCall.ID) {
		logrus.Debugf("[Codex] Dropping code_interpreter_call input item with invalid id: %q", item.OfCodeInterpreterCall.ID)
		return false
	}
	if item.OfLocalShellCall != nil && !isValidCodexIDStrict(item.OfLocalShellCall.ID) {
		logrus.Debugf("[Codex] Dropping local_shell_call input item with invalid id: %q", item.OfLocalShellCall.ID)
		return false
	}
	if item.OfLocalShellCallOutput != nil && !isValidCodexIDStrict(item.OfLocalShellCallOutput.ID) {
		logrus.Debugf("[Codex] Dropping local_shell_call_output input item with invalid id: %q", item.OfLocalShellCallOutput.ID)
		return false
	}
	if item.OfMcpListTools != nil && !isValidCodexIDStrict(item.OfMcpListTools.ID) {
		logrus.Debugf("[Codex] Dropping mcp_list_tools input item with invalid id: %q", item.OfMcpListTools.ID)
		return false
	}
	if item.OfMcpApprovalRequest != nil && !isValidCodexIDStrict(item.OfMcpApprovalRequest.ID) {
		logrus.Debugf("[Codex] Dropping mcp_approval_request input item with invalid id: %q", item.OfMcpApprovalRequest.ID)
		return false
	}
	if item.OfMcpCall != nil && !isValidCodexIDStrict(item.OfMcpCall.ID) {
		logrus.Debugf("[Codex] Dropping mcp_call input item with invalid id: %q", item.OfMcpCall.ID)
		return false
	}
	if item.OfItemReference != nil && !isValidCodexIDStrict(item.OfItemReference.ID) {
		logrus.Debugf("[Codex] Dropping item_reference input item with invalid id: %q", item.OfItemReference.ID)
		return false
	}

	// Drop message items with empty string content.
	// Codex rejects "content": "" as a missing required parameter.
	if item.OfMessage != nil {
		msg := item.OfMessage
		if msg.Content.OfString.Valid() && msg.Content.OfString.Value == "" &&
			len(msg.Content.OfInputItemContentList) == 0 {
			logrus.Warnf("[Codex] Dropping message item (role=%s) with empty string content", msg.Role)
			return false
		}
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
func (c *CodexClient) ListModels(ctx context.Context) (*ModelListResult, error) {
	return nil, &ErrModelsEndpointNotSupported{
		Provider: c.provider.Name,
		Reason:   "ChatGPT OAuth token cannot access /models endpoint",
	}
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

// Client returns the underlying OpenAI SDK client.
// For CodexClient, we delegate to the embedded OpenAIClient.
func (c *CodexClient) Client() *openai.Client {
	return c.OpenAIClient.Client()
}
