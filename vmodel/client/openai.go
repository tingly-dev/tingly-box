// Package vmodelclient provides in-process implementations of the client
// interfaces backed by vmodel registries. They let virtual providers traverse
// the exact same dispatch path as real upstream providers — no short-circuits,
// no HTTP overhead.
package vmodelclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/typ"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
)

// OpenAIClient implements client.OpenAIClientInterface using the in-process
// vmodel OpenAI registry. No HTTP, no auth — just direct function calls.
type OpenAIClient struct {
	reg      *openaivm.Registry
	provider *typ.Provider
}

// NewOpenAIClient creates an in-process OpenAI vmodel client backed by reg.
func NewOpenAIClient(reg *openaivm.Registry, provider *typ.Provider) *OpenAIClient {
	return &OpenAIClient{reg: reg, provider: provider}
}

func (c *OpenAIClient) GetProvider() *typ.Provider          { return c.provider }
func (c *OpenAIClient) APIStyle() protocol.APIStyle         { return protocol.APIStyleOpenAI }
func (c *OpenAIClient) SetRecordSink(_ *obs.Sink)           {}
func (c *OpenAIClient) Client() *openai.Client              { return nil }
func (c *OpenAIClient) Close() error                        { return nil }

func (c *OpenAIClient) ListModels(_ context.Context) ([]string, error) {
	models := c.reg.ListModels()
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	return ids, nil
}

// ChatCompletionsNew calls the vmodel synchronously and returns an
// *openai.ChatCompletion built via JSON round-trip.
func (c *OpenAIClient) ChatCompletionsNew(_ context.Context, req openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	vm := c.reg.Get(req.Model)
	if vm == nil {
		return nil, fmt.Errorf("vmodel not found: %s", req.Model)
	}
	if err := injectedPreContentError(vm); err != nil {
		return nil, err
	}

	vmReq := &protocol.OpenAIChatCompletionRequest{
		ChatCompletionNewParams: req,
	}

	resp, err := vm.HandleOpenAIChat(vmReq)
	if err != nil {
		return nil, err
	}

	promptTokens := int64(token.EstimateMessagesTokens(req.Messages))
	completionTokens := int64(token.EstimateTokensString(resp.Content))

	// Build tool calls in the non-streaming format.
	var toolCalls []map[string]interface{}
	for i, tc := range resp.ToolCalls {
		toolCalls = append(toolCalls, map[string]interface{}{
			"id":   tc.ID,
			"type": "function",
			"index": i,
			"function": map[string]interface{}{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		})
	}

	msg := map[string]interface{}{
		"role":    "assistant",
		"content": resp.Content,
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	payload := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   req.Model,
		"choices": []interface{}{
			map[string]interface{}{
				"index":         0,
				"message":       msg,
				"finish_reason": resp.FinishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("vmodel marshal: %w", err)
	}
	var out openai.ChatCompletion
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("vmodel unmarshal: %w", err)
	}
	return &out, nil
}

// ChatCompletionsNewStreaming wraps vm.HandleOpenAIChatStream with a
// channel-based decoder so the caller gets a standard ssestream.Stream.
func (c *OpenAIClient) ChatCompletionsNewStreaming(_ context.Context, req openai.ChatCompletionNewParams) *openaistream.Stream[openai.ChatCompletionChunk] {
	vm := c.reg.Get(req.Model)
	if vm == nil {
		return openaistream.NewStream[openai.ChatCompletionChunk](errDecoder{fmt.Errorf("vmodel not found: %s", req.Model)}, nil)
	}
	if err := injectedPreContentError(vm); err != nil {
		return openaistream.NewStream[openai.ChatCompletionChunk](errDecoder{err}, nil)
	}

	vmReq := &protocol.OpenAIChatCompletionRequest{
		ChatCompletionNewParams: req,
	}

	dec := newOpenAIVModelDecoder(vm, vmReq)
	return openaistream.NewStream[openai.ChatCompletionChunk](dec, nil)
}

// Unsupported methods — vmodel only covers chat completions.

func (c *OpenAIClient) ImagesGenerate(_ context.Context, _ openai.ImageGenerateParams) (*openai.ImagesResponse, error) {
	return nil, fmt.Errorf("images not supported by vmodel")
}

func (c *OpenAIClient) ResponsesNew(_ context.Context, _ responses.ResponseNewParams) (*responses.Response, error) {
	return nil, fmt.Errorf("responses API not supported by vmodel")
}

func (c *OpenAIClient) ResponsesNewStreaming(_ context.Context, _ responses.ResponseNewParams) *openaistream.Stream[responses.ResponseStreamEventUnion] {
	return openaistream.NewStream[responses.ResponseStreamEventUnion](errDecoder{fmt.Errorf("responses API not supported by vmodel")}, nil)
}

func (c *OpenAIClient) EmbeddingsNew(_ context.Context, _ openai.EmbeddingNewParams) (*openai.CreateEmbeddingResponse, error) {
	return nil, fmt.Errorf("embeddings not supported by vmodel")
}

// ── streaming decoder ─────────────────────────────────────────────────────────

type openAIVModelDecoder struct {
	ch      <-chan []byte
	current []byte
	err     error
}

func newOpenAIVModelDecoder(vm openaivm.VirtualModel, req *protocol.OpenAIChatCompletionRequest) *openAIVModelDecoder {
	ch := make(chan []byte, 64)
	d := &openAIVModelDecoder{ch: ch}

	msgID := fmt.Sprintf("chatcmpl-virtual-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	model := req.Model

	go func() {
		defer close(ch)

		var completionText string
		var explicitUsage *openAIUsageCapture

		chunkIndex := 0

		emitJSON := func(v interface{}) {
			b, err := json.Marshal(v)
			if err != nil {
				return
			}
			ch <- b
		}

		streamErr := vm.HandleOpenAIChatStream(req, func(ev any) {
			switch e := ev.(type) {
			case openaivm.DeltaEvent:
				chunkIndex = e.Index + 1
				completionText += e.Content
				emitJSON(map[string]interface{}{
					"id":      msgID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": []interface{}{
						map[string]interface{}{
							"index":         e.Index,
							"delta":         map[string]interface{}{"content": e.Content},
							"finish_reason": nil,
						},
					},
				})

			case openaivm.ToolEvent:
				tc := e.ToolCall
				emitJSON(map[string]interface{}{
					"id":      msgID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": []interface{}{
						map[string]interface{}{
							"index": 0,
							"delta": map[string]interface{}{
								"tool_calls": []interface{}{
									map[string]interface{}{
										"index": e.Index,
										"id":    tc.ID,
										"type":  "function",
										"function": map[string]interface{}{
											"name":      tc.Name,
											"arguments": tc.Arguments,
										},
									},
								},
							},
							"finish_reason": nil,
						},
					},
				})

			case openaivm.UsageEvent:
				u := e.Usage
				explicitUsage = &openAIUsageCapture{
					prompt:     u.PromptTokens,
					completion: u.CompletionTokens,
					cached:     u.CachedInputTokens,
					reasoning:  u.ReasoningTokens,
				}

			case openaivm.DoneEvent:
				emitJSON(map[string]interface{}{
					"id":      msgID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": []interface{}{
						map[string]interface{}{
							"index":         chunkIndex,
							"delta":         map[string]interface{}{},
							"finish_reason": e.FinishReason,
						},
					},
				})

				// Trailing usage chunk — mirrors real OpenAI stream_options.include_usage.
				usageMap := map[string]interface{}{}
				if explicitUsage != nil {
					usageMap["prompt_tokens"] = explicitUsage.prompt
					usageMap["completion_tokens"] = explicitUsage.completion
					usageMap["total_tokens"] = explicitUsage.prompt + explicitUsage.completion
					if explicitUsage.cached > 0 {
						usageMap["prompt_tokens_details"] = map[string]interface{}{
							"cached_tokens": explicitUsage.cached,
						}
					}
					if explicitUsage.reasoning > 0 {
						usageMap["completion_tokens_details"] = map[string]interface{}{
							"reasoning_tokens": explicitUsage.reasoning,
						}
					}
				} else {
					p := int64(token.EstimateMessagesTokens(req.Messages))
					comp := int64(token.EstimateTokensString(completionText))
					usageMap["prompt_tokens"] = p
					usageMap["completion_tokens"] = comp
					usageMap["total_tokens"] = p + comp
				}
				emitJSON(map[string]interface{}{
					"id":      msgID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": []interface{}{},
					"usage":   usageMap,
				})
			}
		})

		if streamErr != nil {
			d.err = streamErr
		}
	}()

	return d
}

type openAIUsageCapture struct {
	prompt, completion, cached, reasoning int64
}

func (d *openAIVModelDecoder) Next() bool {
	b, ok := <-d.ch
	if !ok {
		return false
	}
	d.current = b
	return true
}

func (d *openAIVModelDecoder) Event() openaistream.Event {
	return openaistream.Event{Data: d.current}
}

func (d *openAIVModelDecoder) Close() error { return nil }
func (d *openAIVModelDecoder) Err() error   { return d.err }

// errDecoder is a no-op decoder that immediately returns an error from Err().
type errDecoder struct{ err error }

func (e errDecoder) Next() bool               { return false }
func (e errDecoder) Event() openaistream.Event { return openaistream.Event{} }
func (e errDecoder) Close() error             { return nil }
func (e errDecoder) Err() error               { return e.err }
