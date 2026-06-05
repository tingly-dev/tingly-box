package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// probeEchoInstruction is the system/instruction prompt used by SDK probes to
// keep the upstream response minimal.
const probeEchoInstruction = "work as `echo` if possible"

// The SDK probe helpers below dispatch a minimal request through each client's
// real-traffic methods (ChatCompletionsNew, ResponsesNew, MessagesNew,
// GenerateContent). Routing probes through the same methods as production
// traffic means provider-specific quirks — Kimi model-name normalization,
// Codex Responses handling — apply identically and cannot drift from the real
// path. The client package therefore no longer owns any probe-specific code.

// probeOpenAIChat builds and dispatches a minimal Chat Completions probe.
func probeOpenAIChat(ctx context.Context, oc client.OpenAIClientInterface, model, message string, mode client.ProbeMode) (*client.ProbeResult, error) {
	start := time.Now()
	params := openai.ChatCompletionNewParams{
		Model: model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(probeEchoInstruction),
			openai.UserMessage(message),
		},
	}
	if mode == client.ProbeModeTool {
		params.Tools = client.GetProbeToolsOpenAI()
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.Opt("auto")}
	}

	url := oc.GetProvider().APIBase + "/chat/completions"
	if mode == client.ProbeModeSimple {
		resp, err := oc.ChatCompletionsNew(ctx, params)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, false), nil
	}

	stream := oc.ChatCompletionsNewStreaming(ctx, params)
	if stream == nil {
		return nil, fmt.Errorf("chat streaming not supported by provider")
	}
	defer stream.Close()
	var chunks []interface{}
	for stream.Next() {
		chunks = append(chunks, stream.Current())
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(chunks)
	return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, true), nil
}

// probeOpenAIResponses builds and dispatches a minimal Responses API probe.
func probeOpenAIResponses(ctx context.Context, oc client.OpenAIClientInterface, model, message string, mode client.ProbeMode) (*client.ProbeResult, error) {
	start := time.Now()
	params := responses.ResponseNewParams{
		Model:        model,
		Instructions: param.NewOpt(probeEchoInstruction),
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
	if mode == client.ProbeModeTool {
		params.Tools = client.GetProbeToolsResponses()
		params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
			OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
		}
	}

	url := oc.GetProvider().APIBase + "/responses"
	if mode == client.ProbeModeSimple {
		resp, err := oc.ResponsesNew(ctx, params)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, false), nil
	}

	stream := oc.ResponsesNewStreaming(ctx, params)
	if stream == nil {
		return nil, fmt.Errorf("responses streaming not supported by provider")
	}
	defer stream.Close()
	var chunks []interface{}
	for stream.Next() {
		chunks = append(chunks, stream.Current())
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(chunks)
	return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, true), nil
}

// probeAnthropicMessages builds and dispatches a minimal Messages probe.
func probeAnthropicMessages(ctx context.Context, ac client.AnthropicClientInterface, model, message string, mode client.ProbeMode) (*client.ProbeResult, error) {
	start := time.Now()
	provider := ac.GetProvider()

	system := []anthropic.TextBlockParam{{Text: probeEchoInstruction}}
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil &&
		provider.OAuthDetail.GetIssuer() == ai.IssuerClaudeCode {
		system = append([]anthropic.TextBlockParam{{Text: client.ClaudeCodeSystemHeader}}, system...)
	}

	params := &anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 1024,
		System:    system,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(message)),
		},
	}
	if mode == client.ProbeModeTool {
		params.Tools = client.GetProbeToolsAnthropic()
		params.ToolChoice = client.GetProbeToolChoiceAutoAnthropic()
	}

	url := provider.APIBase + "/v1/messages"
	if mode == client.ProbeModeSimple {
		resp, err := ac.MessagesNew(ctx, params)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, false), nil
	}

	stream := ac.MessagesNewStreaming(ctx, params)
	if stream == nil {
		return nil, fmt.Errorf("messages streaming not supported by provider")
	}
	defer stream.Close()
	var chunks []interface{}
	for stream.Next() {
		chunks = append(chunks, stream.Current())
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(chunks)
	return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, true), nil
}

// probeGoogleGenerate builds and dispatches a minimal GenerateContent probe.
func probeGoogleGenerate(ctx context.Context, gc *client.GoogleClient, model, message string, mode client.ProbeMode) (*client.ProbeResult, error) {
	start := time.Now()
	contents := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: message}}},
	}
	config := &genai.GenerateContentConfig{MaxOutputTokens: 1024}
	url := gc.GetProvider().APIBase

	if mode == client.ProbeModeSimple {
		resp, err := gc.GenerateContent(ctx, model, contents, config)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(resp)
		return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, false), nil
	}

	var chunks []interface{}
	for resp, err := range gc.GenerateContentStream(ctx, model, contents, config) {
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, resp)
	}
	b, _ := json.Marshal(chunks)
	return client.ToProbeResult(string(b), time.Since(start).Milliseconds(), url, true), nil
}

// probeOptions issues a bare OPTIONS request to the provider base URL with the
// auth headers appropriate for its API style. Used by the lightweight probe;
// results are advisory.
func probeOptions(ctx context.Context, provider *typ.Provider) client.ProbeResult {
	start := time.Now()

	url := provider.APIBase
	header := http.Header{}
	switch provider.APIStyle {
	case protocol.APIStyleAnthropic:
		apiBase := strings.TrimSuffix(provider.APIBase, "/")
		if !strings.Contains(apiBase, "/v1") {
			apiBase += "/v1"
		}
		url = apiBase
		header.Set("x-api-key", provider.GetAccessToken())
		header.Set("anthropic-version", "2023-06-01")
	case protocol.APIStyleGoogle:
		if !strings.HasSuffix(url, "/") {
			url += "/"
		}
		header.Set("x-goog-api-key", provider.GetAccessToken())
	default:
		header.Set("Authorization", "Bearer "+provider.GetAccessToken())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, url, nil)
	if err != nil {
		return client.ProbeResult{Success: false, ErrorMessage: fmt.Sprintf("Failed to create OPTIONS request: %v", err)}
	}
	req.Header = header

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return client.ProbeResult{Success: false, ErrorMessage: fmt.Sprintf("OPTIONS request failed: %v", err), LatencyMs: latencyMs}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return client.ProbeResult{Success: true, Message: "OPTIONS request successful", LatencyMs: latencyMs}
	}
	return client.ProbeResult{Success: false, ErrorMessage: fmt.Sprintf("OPTIONS request failed with status: %d", resp.StatusCode), LatencyMs: latencyMs}
}

// isCodexOAuth reports whether the provider is a Codex OAuth provider, which
// only speaks the Responses API.
func isCodexOAuth(provider *typ.Provider) bool {
	return provider.AuthType == typ.AuthTypeOAuth &&
		provider.OAuthDetail != nil &&
		provider.OAuthDetail.GetIssuer() == ai.IssuerCodex
}
