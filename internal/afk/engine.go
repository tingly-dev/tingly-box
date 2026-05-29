// Package afk (Agent Framework Kit / Away From Keyboard) provides a small,
// reusable ReAct agent loop built directly on the official Anthropic SDK
// (github.com/anthropics/anthropic-sdk-go).
//
// It is anthropic-first by design: messages are the SDK's native
// anthropic.BetaMessageParam, there is no provider-compat layer, and tool
// calls are dispatched through a simple Tool interface. The loop streams
// assistant text to a StreamSink as it is produced, executes tool_use blocks,
// feeds the results back, and repeats until the model stops requesting tools
// or the iteration budget is exhausted.
//
// This package deliberately lives in the root module (not agentboot) because it
// relies on Anthropic SDK v1.45 APIs that are wired in via the root go.mod
// replace directive; agentboot pins an older SDK without that replace.
package afk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

// Tool is a single callable tool exposed to the model.
//
// Param describes the tool to the model (name, description, input schema) as a
// native BetaToolParam — no conversion layer. Call executes it; the raw JSON
// input from the model is passed through unmodified so each tool owns its own
// argument decoding.
type Tool interface {
	// Param returns the full BetaToolParam sent to the model. Name is also used
	// as the dispatch key when routing tool_use blocks back to this tool.
	Param() anthropic.BetaToolParam
	// Call executes the tool with the raw JSON arguments produced by the model
	// and returns the textual result. A non-nil error is reported back to the
	// model as an error tool_result (the loop itself does not abort).
	Call(ctx context.Context, rawInput json.RawMessage) (string, error)
}

// StreamSink receives incremental updates as the loop runs. All methods are
// optional in spirit — a nil StreamSink disables streaming entirely, and the
// engine never assumes any method has side effects it depends on.
//
// Whether OnText is called per-fragment or once per turn with the aggregated
// text is controlled by Config.StreamText (default: aggregated). See that field.
type StreamSink interface {
	// OnText is called with assistant text. By default (aggregated mode) it is
	// called once per assistant turn with the full text; in streaming mode it
	// is called many times per turn with partial fragments.
	OnText(delta string)
	// OnToolCall is called when the model invokes a tool, before execution.
	OnToolCall(name string, input json.RawMessage)
	// OnToolResult is called after a tool finishes, with the textual result and
	// whether it was an error.
	OnToolResult(name string, result string, isErr bool)
}

// Engine runs the ReAct loop against a configured model and toolset.
type Engine struct {
	client        anthropic.Client
	model         string
	system        string
	maxTokens     int64
	temperature   *float64
	maxIterations int
	streamText    bool
	tools         []Tool
	toolByName    map[string]Tool
	toolParams    []anthropic.BetaToolUnionParam
}

// Config configures an Engine.
type Config struct {
	// BaseURL and APIKey point the SDK at the tingly-box gateway.
	BaseURL string
	APIKey  string
	// Model is the model identifier (for tingly-box this is a bot-UUID rule).
	Model string
	// System is the system prompt.
	System string
	// MaxTokens caps a single response; defaults to 4096 when zero.
	MaxTokens int64
	// Temperature is optional; nil leaves it unset.
	Temperature *float64
	// MaxIterations caps tool-use rounds; defaults to 20 when zero.
	MaxIterations int
	// StreamText controls how assistant text reaches the StreamSink.
	//
	// Default (false): aggregated — the engine buffers each assistant turn's
	// text and calls StreamSink.OnText once, with the complete turn text. This
	// is the safe default while consumers don't yet handle incremental output.
	//
	// true: streaming — OnText is called per text fragment as it arrives. The
	// engine always consumes the model's HTTP stream either way; this flag only
	// changes the granularity of the OnText fan-out to the sink.
	StreamText bool
	// Tools are the callable tools exposed to the model.
	Tools []Tool
}

// NewEngine builds an Engine from cfg. BaseURL, APIKey and Model are required.
func NewEngine(cfg Config) (*Engine, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic engine: BaseURL and APIKey are required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("anthropic engine: Model is required")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	maxIter := cfg.MaxIterations
	if maxIter == 0 {
		maxIter = 20
	}

	client := newClient(cfg.BaseURL, cfg.APIKey)

	e := &Engine{
		client:        client,
		model:         cfg.Model,
		system:        cfg.System,
		maxTokens:     maxTokens,
		temperature:   cfg.Temperature,
		maxIterations: maxIter,
		streamText:    cfg.StreamText,
		toolByName:    make(map[string]Tool, len(cfg.Tools)),
	}
	for _, t := range cfg.Tools {
		e.registerTool(t)
	}
	return e, nil
}

// registerTool adds a tool to the engine's dispatch table and param list.
func (e *Engine) registerTool(t Tool) {
	p := t.Param()
	e.tools = append(e.tools, t)
	e.toolByName[p.Name] = t
	e.toolParams = append(e.toolParams, anthropic.BetaToolUnionParam{OfTool: &p})
}

// Run executes the ReAct loop. It appends the user prompt to history, then
// streams/executes until the model produces a final answer (no tool_use) or the
// iteration budget is reached.
//
// history is the prior conversation as native SDK beta message params (may be
// empty). It returns the full updated message slice (history + this exchange)
// so the caller can persist it, plus the final assistant text.
func (e *Engine) Run(
	ctx context.Context,
	history []anthropic.BetaMessageParam,
	userText string,
	sink StreamSink,
) ([]anthropic.BetaMessageParam, string, error) {
	messages := append([]anthropic.BetaMessageParam(nil), history...)
	messages = append(messages, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(userText)))

	var finalText string

	logrus.WithFields(logrus.Fields{
		"model":         e.model,
		"history_msgs":  len(history),
		"prompt_len":    len(userText),
		"tools":         len(e.tools),
		"maxIterations": e.maxIterations,
		"stream_text":   e.streamText,
	}).Debug("afk engine: run start")

	for i := 0; i < e.maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			logrus.WithError(err).WithField("iteration", i).Debug("afk engine: context cancelled")
			return messages, finalText, err
		}

		msg, turnText, err := e.streamTurn(ctx, messages, sink)
		if err != nil {
			return messages, finalText, err
		}
		messages = append(messages, msg.ToParam())
		if turnText != "" {
			finalText = turnText
		}

		// Collect tool_use blocks from the SDK-native content slice.
		var toolUses []anthropic.BetaContentBlockUnion
		for _, block := range msg.Content {
			if block.Type == "tool_use" {
				toolUses = append(toolUses, block)
			}
		}

		logrus.WithFields(logrus.Fields{
			"iteration": i,
			"turn_text": len(turnText),
			"tool_uses": len(toolUses),
		}).Debug("afk engine: iteration result")

		if len(toolUses) == 0 {
			// No tools requested — this is the final answer.
			logrus.WithFields(logrus.Fields{
				"iterations": i + 1,
				"final_len":  len(finalText),
			}).Debug("afk engine: run complete (final answer)")
			return messages, finalText, nil
		}

		results := e.dispatchTools(ctx, toolUses, sink)
		messages = append(messages, anthropic.NewBetaUserMessage(results...))
	}

	// Loop exhausted while still requesting tools. If no text was ever produced
	// the caller (and user) would otherwise see nothing despite many tool calls,
	// so log loudly with enough context to debug.
	logrus.WithFields(logrus.Fields{
		"model":         e.model,
		"maxIterations": e.maxIterations,
		"final_len":     len(finalText),
		"had_text":      finalText != "",
	}).Warn("afk engine: hit max iterations without a tool-free final answer")
	return messages, finalText, nil
}

// streamTurn runs one model call via the Beta Messages API, streaming text to
// the sink and accumulating the full assistant BetaMessage. It returns the
// accumulated message and the concatenated text (or thinking, as fallback) of
// this turn.
func (e *Engine) streamTurn(
	ctx context.Context,
	messages []anthropic.BetaMessageParam,
	sink StreamSink,
) (anthropic.BetaMessage, string, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(e.model),
		MaxTokens: e.maxTokens,
		Messages:  messages,
	}
	if e.system != "" {
		params.System = []anthropic.BetaTextBlockParam{{Text: e.system}}
	}
	if e.temperature != nil {
		params.Temperature = anthropic.Float(*e.temperature)
	}
	if len(e.toolParams) > 0 {
		params.Tools = e.toolParams
	}

	stream := e.client.Beta.Messages.NewStreaming(ctx, params)
	msg := anthropic.BetaMessage{}

	for stream.Next() {
		event := stream.Current()
		// Let the SDK accumulate the canonical BetaMessage (text concatenated
		// into content blocks, tool_use inputs assembled). We never hand-roll
		// text aggregation — we read it back from the accumulated message below.
		if err := msg.Accumulate(event); err != nil {
			return msg, "", fmt.Errorf("accumulate stream event: %w", err)
		}
		// Streaming mode only: fan out each text fragment as it arrives. This is
		// a UI concern, independent of aggregation, so it reads the delta
		// directly rather than the accumulator.
		if sink != nil && e.streamText {
			if delta, ok := event.AsAny().(anthropic.BetaRawContentBlockDeltaEvent); ok && delta.Delta.Text != "" {
				sink.OnText(delta.Delta.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return msg, "", fmt.Errorf("model stream error: %w", err)
	}

	// Scan the SDK-accumulated content blocks once: collect text, thinking,
	// and per-type counts for the diagnostic log below.
	var textB, thinkB strings.Builder
	var nText, nThinking, nToolUse int
	for _, block := range msg.Content {
		switch block.Type {
		case "text":
			textB.WriteString(block.Text)
			nText++
		case "thinking":
			thinkB.WriteString(block.Thinking)
			nThinking++
		case "tool_use":
			nToolUse++
		}
	}

	// Real text wins over thinking. Fall back to thinking only when the turn
	// produced no visible text — e.g. extended-thinking-only turns where the
	// model writes its reasoning in a thinking block then calls a tool.
	turnText := textB.String()
	usedThinking := false
	if turnText == "" {
		if think := thinkB.String(); think != "" {
			turnText = think
			usedThinking = true
		}
	}

	logrus.WithFields(logrus.Fields{
		"model":           e.model,
		"stop_reason":     msg.StopReason,
		"text_len":        len(turnText),
		"text_blocks":     nText,
		"thinking_blocks": nThinking,
		"used_thinking":   usedThinking,
		"tool_uses":       nToolUse,
	}).Debug("afk engine: assistant turn complete")

	// Aggregated mode: emit the whole turn's text once, after the stream ends.
	if sink != nil && !e.streamText && turnText != "" {
		sink.OnText(turnText)
	}
	return msg, turnText, nil
}

// dispatchTools executes every tool_use block (SDK BetaContentBlockUnion) and
// returns the corresponding tool_result content blocks, in order.
func (e *Engine) dispatchTools(
	ctx context.Context,
	toolUses []anthropic.BetaContentBlockUnion,
	sink StreamSink,
) []anthropic.BetaContentBlockParamUnion {
	results := make([]anthropic.BetaContentBlockParamUnion, 0, len(toolUses))
	for _, tu := range toolUses {
		if sink != nil {
			sink.OnToolCall(tu.Name, tu.Input)
		}
		out, isErr := e.callTool(ctx, tu)
		if sink != nil {
			sink.OnToolResult(tu.Name, out, isErr)
		}
		results = append(results, anthropic.NewBetaToolResultBlock(tu.ID, out, isErr))
	}
	return results
}

// callTool resolves and invokes a single tool, converting a Go error or unknown
// tool name into an error result string (the loop continues either way).
func (e *Engine) callTool(ctx context.Context, tu anthropic.BetaContentBlockUnion) (string, bool) {
	tool, ok := e.toolByName[tu.Name]
	if !ok {
		logrus.WithField("tool", tu.Name).Warn("afk engine: unknown tool requested")
		return fmt.Sprintf("Error: unknown tool %q", tu.Name), true
	}
	logrus.WithFields(logrus.Fields{
		"tool":  tu.Name,
		"input": string(tu.Input),
	}).Debug("afk engine: tool call")
	out, err := tool.Call(ctx, tu.Input)
	if err != nil {
		logrus.WithError(err).WithField("tool", tu.Name).Warn("afk engine: tool call failed")
		if out == "" {
			out = fmt.Sprintf("Error: %v", err)
		}
		return out, true
	}
	logrus.WithFields(logrus.Fields{
		"tool":       tu.Name,
		"result_len": len(out),
	}).Debug("afk engine: tool result")
	return out, false
}
