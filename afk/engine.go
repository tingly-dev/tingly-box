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

	"github.com/anthropics/anthropic-sdk-go/v2"
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
	// OnThinking is called once per assistant turn that produced reasoning, with
	// that turn's thinking text. It is deliberately separate from OnText:
	// reasoning is not an answer, and a sink is free to render it differently or
	// drop it entirely. Newer models return thinking blocks with empty text by
	// default, in which case this is not called at all.
	OnThinking(text string)
	// OnTurnEnd is called once per assistant turn, after the model stream
	// closes, with that turn's token accounting and stop reason. Cache fields
	// on the usage report how much of the prompt was served from the prompt
	// cache, which is the only way to tell whether cache breakpoints are
	// actually landing.
	OnTurnEnd(usage anthropic.BetaUsage, stopReason anthropic.BetaStopReason)
}

// Usage accumulates token counts across the turns of a single run. The engine
// keeps one per Run for its run-level log line; callers that need a per-run
// total for their own reporting can accumulate the OnTurnEnd values the same
// way instead of re-deriving the arithmetic.
type Usage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheReadInputTokens     int64
	CacheCreationInputTokens int64
}

// Add folds one turn's usage into the accumulator.
func (u *Usage) Add(t anthropic.BetaUsage) {
	u.InputTokens += t.InputTokens
	u.OutputTokens += t.OutputTokens
	u.CacheReadInputTokens += t.CacheReadInputTokens
	u.CacheCreationInputTokens += t.CacheCreationInputTokens
}

// LogFields renders the accumulator for structured logging.
func (u Usage) LogFields() logrus.Fields {
	return logrus.Fields{
		"input_tokens":   u.InputTokens,
		"output_tokens":  u.OutputTokens,
		"cache_read":     u.CacheReadInputTokens,
		"cache_creation": u.CacheCreationInputTokens,
	}
}

// ThinkingMode selects how the model reasons, and whether that reasoning comes
// back to us.
//
// One field rather than an on/off flag plus a visibility flag, because the two
// are not independent in the API: reasoning you cannot see and no reasoning at
// all are different requests, and "unset" is a third thing again — on current
// models omitting the parameter already means adaptive thinking, while on
// slightly older ones it means none.
//
//	                      | thinking parameter sent
//	ThinkingModelDefault  | (none — the model's own default applies)
//	ThinkingVisible       | {type: adaptive, display: summarized}
//	ThinkingHidden        | {type: adaptive, display: omitted}
//	ThinkingOff           | {type: disabled}
type ThinkingMode string

const (
	// ThinkingModelDefault sends nothing and lets the model decide. This is the
	// zero value, and the right default for a gateway that routes to whatever
	// model the user configured: forcing a mode a model does not accept is a
	// hard failure, while its own default is by definition supported.
	ThinkingModelDefault ThinkingMode = ""

	// ThinkingVisible asks for adaptive thinking and for the reasoning to be
	// returned. This is the mode to pick if anything renders StreamSink
	// .OnThinking — display defaults to omitted on current models, which means
	// thinking blocks arrive with empty text and OnThinking never fires with
	// anything in it.
	ThinkingVisible ThinkingMode = "visible"

	// ThinkingHidden asks for adaptive thinking but not for its content. The
	// model still reasons and still returns a signature for multi-turn
	// continuity; we simply do not pay to carry the text around. Distinct from
	// ThinkingModelDefault on models where omitting the parameter means no
	// thinking at all.
	ThinkingHidden ThinkingMode = "hidden"

	// ThinkingOff turns thinking off, for latency- or cost-sensitive callers.
	ThinkingOff ThinkingMode = "off"
)

// thinkingParam renders the mode as the request parameter, and reports whether
// there is one to send.
func (m ThinkingMode) thinkingParam() (anthropic.BetaThinkingConfigParamUnion, bool) {
	switch m {
	case ThinkingVisible:
		return anthropic.BetaThinkingConfigParamUnion{
			OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{
				Display: anthropic.BetaThinkingConfigAdaptiveDisplaySummarized,
			},
		}, true
	case ThinkingHidden:
		return anthropic.BetaThinkingConfigParamUnion{
			OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{
				Display: anthropic.BetaThinkingConfigAdaptiveDisplayOmitted,
			},
		}, true
	case ThinkingOff:
		return anthropic.BetaThinkingConfigParamUnion{
			OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{},
		}, true
	default:
		return anthropic.BetaThinkingConfigParamUnion{}, false
	}
}

// EffortLevel is how hard the model works on a turn — the API's primary
// intelligence/latency/cost control.
//
// Orthogonal to ThinkingMode and kept as its own field for that reason: thinking
// is *whether* the model reasons and whether we see it, effort is *how much* of
// everything it spends. They interact only at the edges — on some models
// disabling thinking is rejected above `high` — which is a model-specific rule
// this package cannot check, because it does not know which model the gateway
// routed to.
type EffortLevel string

const (
	// EffortModelDefault sends nothing, letting the model's own default apply.
	// Same reasoning as ThinkingModelDefault: the default is the only setting
	// guaranteed to be accepted by an unknown model.
	EffortModelDefault EffortLevel = ""

	EffortLow    EffortLevel = "low"
	EffortMedium EffortLevel = "medium"
	EffortHigh   EffortLevel = "high"
	EffortXHigh  EffortLevel = "xhigh"
	EffortMax    EffortLevel = "max"
)

// outputConfig renders the level as the request parameter, and reports whether
// there is one to send. An unrecognized value sends nothing rather than being
// forwarded: it arrives as a plain string from config, and a typo must not
// become a rejected request.
func (e EffortLevel) outputConfig() (anthropic.BetaOutputConfigParam, bool) {
	switch e {
	case EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax:
		return anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffort(e),
		}, true
	default:
		return anthropic.BetaOutputConfigParam{}, false
	}
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
	thinking      ThinkingMode
	effort        EffortLevel
	serverCompact bool
	serverTrigger int64
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
	// Thinking selects the model's reasoning mode. Zero value leaves the
	// parameter unset; see ThinkingMode.
	Thinking ThinkingMode
	// Effort is how hard the model works on a turn. Zero value leaves the
	// parameter unset; see EffortLevel.
	Effort EffortLevel
	// DisableServerCompaction turns off server-side compaction, which is
	// otherwise on by default. With it off, nothing bounds the prompt: a long
	// enough conversation eventually exceeds the context window and fails.
	DisableServerCompaction bool
	// ServerCompactTrigger overrides the input-token count at which the server
	// compacts. Zero uses DefaultServerCompactTrigger.
	ServerCompactTrigger int64
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
		thinking:      cfg.Thinking,
		effort:        cfg.Effort,
		serverCompact: !cfg.DisableServerCompaction,
		serverTrigger: cfg.ServerCompactTrigger,
		toolByName:    make(map[string]Tool, len(cfg.Tools)),
	}
	for _, t := range cfg.Tools {
		e.registerTool(t)
	}
	// Static-prefix cache breakpoint. The request renders as tools -> system ->
	// messages, and a breakpoint caches everything up to and including its own
	// block, so one breakpoint on the system prompt covers the tools too. Only
	// when there is no system prompt do we fall back to marking the last tool,
	// which is otherwise redundant with it.
	if cfg.System == "" && len(e.toolParams) > 0 {
		if last := e.toolParams[len(e.toolParams)-1].OfTool; last != nil {
			last.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
		}
	}
	return e, nil
}

// withConversationCacheBreakpoint returns messages with an ephemeral cache
// breakpoint on the final content block, so the whole conversation so far is
// cached and the next request reads it back.
//
// The breakpoint rolls forward every request rather than being pinned once.
// That is not just for coverage: a breakpoint only searches backwards a bounded
// number of content blocks for an existing cache entry, and a single tool-heavy
// turn can append more blocks than that window — a pinned breakpoint would go
// silently cold exactly on the runs that cost the most.
//
// It copies the tail rather than writing in place. The blocks are pointers into
// the caller's history, which Smart Guide persists verbatim; an in-place write
// would leak breakpoints into the stored session, and a reloaded conversation
// carrying more than the per-request breakpoint limit is rejected outright.
func withConversationCacheBreakpoint(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	if len(messages) == 0 {
		return messages
	}
	last := messages[len(messages)-1]
	if len(last.Content) == 0 {
		return messages
	}

	blocks := append([]anthropic.BetaContentBlockParamUnion(nil), last.Content...)
	tail := blocks[len(blocks)-1]
	switch {
	case tail.OfText != nil:
		b := *tail.OfText
		b.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
		blocks[len(blocks)-1] = anthropic.BetaContentBlockParamUnion{OfText: &b}
	case tail.OfToolResult != nil:
		b := *tail.OfToolResult
		b.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
		blocks[len(blocks)-1] = anthropic.BetaContentBlockParamUnion{OfToolResult: &b}
	default:
		// Some other block type ends the turn. Skipping it costs a cache write,
		// not correctness — the static prefix breakpoint still applies.
		return messages
	}

	out := append([]anthropic.BetaMessageParam(nil), messages...)
	lastCopy := last
	lastCopy.Content = blocks
	out[len(out)-1] = lastCopy
	return out
}

// registerTool adds a tool to the engine's dispatch table and param list.
func (e *Engine) registerTool(t Tool) {
	p := t.Param()
	e.toolByName[p.Name] = t
	e.toolParams = append(e.toolParams, anthropic.BetaToolUnionParam{OfTool: &p})
}

// StepResult is everything one step produced.
//
// A step is one model call plus the tool batch that call requested. It is the
// unit the loop above it advances in, and the unit persistence and steering
// hang off — which is why it reports its own output rather than mutating a
// shared conversation slice.
type StepResult struct {
	// Messages are what this step appends to the conversation: the assistant
	// turn, followed by a user turn carrying tool results when tools ran.
	Messages []anthropic.BetaMessageParam
	// Text is this step's assistant text, empty for a tool-only step.
	Text string
	// Usage and StopReason are the model's accounting for this step.
	Usage      anthropic.BetaUsage
	StopReason anthropic.BetaStopReason
	// ToolCalls is how many tools this step invoked. It is also what says the
	// run is not finished: their results are already in Messages and the model
	// has to be shown them, no matter what else this step produced.
	ToolCalls int
	// ServerCompacted reports that the response carried a compaction block, so
	// the API compacted this request's history on its own. It is a fact, not an
	// estimate, which is why the harness trusts it over any token arithmetic.
	ServerCompacted bool
}

// Step runs one model call against the given conversation and executes whatever
// tools it requests. It does not mutate messages; the caller decides what to do
// with the result.
func (e *Engine) Step(
	ctx context.Context,
	messages []anthropic.BetaMessageParam,
	sink StreamSink,
) (StepResult, error) {
	msg, turnText, err := e.streamTurn(ctx, messages, sink)
	if err != nil {
		return StepResult{}, err
	}

	res := StepResult{
		Messages:   []anthropic.BetaMessageParam{msg.ToParam()},
		Text:       turnText,
		Usage:      msg.Usage,
		StopReason: msg.StopReason,
	}

	// Walk the SDK-native content slice once: collect tool_use blocks, and note
	// whether the API compacted this request's history. A compaction block is
	// the API telling us it did; it rides along in the assistant message and is
	// carried forward in history, which is how the next request knows what was
	// replaced.
	var toolUses []anthropic.BetaContentBlockUnion
	for _, block := range msg.Content {
		switch block.Type {
		case "tool_use":
			toolUses = append(toolUses, block)
		case "compaction":
			res.ServerCompacted = true
		}
	}
	if len(toolUses) == 0 {
		return res, nil
	}

	results := e.dispatchTools(ctx, toolUses, sink)
	res.Messages = append(res.Messages, anthropic.NewBetaUserMessage(results...))
	res.ToolCalls = len(toolUses)
	return res, nil
}

// Run executes a whole user turn and returns the updated conversation plus the
// final assistant text.
//
// It is a thin wrapper over a Harness with no log — the loop itself lives
// there, because everything that needs to act between steps (persistence,
// steering, compaction) acts at that layer, not inside a model call.
func (e *Engine) Run(
	ctx context.Context,
	history []anthropic.BetaMessageParam,
	userText string,
	sink StreamSink,
) ([]anthropic.BetaMessageParam, string, error) {
	res, err := NewHarness(e, nil).Run(ctx, history, userText, sink)
	return res.Messages, res.FinalText, err
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
		Messages:  withConversationCacheBreakpoint(messages),
	}
	if e.system != "" {
		params.System = []anthropic.BetaTextBlockParam{{
			Text:         e.system,
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}}
	}
	if e.temperature != nil {
		params.Temperature = anthropic.Float(*e.temperature)
	}
	if think, ok := e.thinking.thinkingParam(); ok {
		params.Thinking = think
	}
	if out, ok := e.effort.outputConfig(); ok {
		params.OutputConfig = out
	}
	if len(e.toolParams) > 0 {
		params.Tools = e.toolParams
	}
	if e.serverCompact {
		// Server-side compaction, on by default: the API summarizes the older
		// part of the conversation itself once the prompt crosses the trigger,
		// and returns a compaction block we carry forward in history. It
		// produces better summaries than we can and costs us no extra model
		// call. The in-process path stays as the backstop for upstreams that
		// drop this.
		params.Betas = append(params.Betas, betaCompaction)
		params.ContextManagement = serverCompactionEdit(e.serverCompactTrigger())
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

	// Only real text blocks are the turn's answer. Reasoning used to be
	// substituted in when a turn produced no text, which meant a thinking-then-
	// tool_use turn published the model's private deliberation to the chat as
	// though it were the reply. It goes to OnThinking instead, and the sink
	// decides what a "thinking" event is worth showing.
	turnText := textB.String()
	thinkText := thinkB.String()

	turnUsage := Usage{}
	turnUsage.Add(msg.Usage)
	logrus.WithFields(turnUsage.LogFields()).WithFields(logrus.Fields{
		"model":           e.model,
		"stop_reason":     msg.StopReason,
		"text_len":        len(turnText),
		"text_blocks":     nText,
		"thinking_blocks": nThinking,
		"thinking_len":    len(thinkText),
		"tool_uses":       nToolUse,
	}).Debug("afk engine: assistant turn complete")

	if sink != nil {
		// Reasoning first: it is what led to the text and the tool calls that
		// follow, so emitting it after them would read backwards.
		if thinkText != "" {
			sink.OnThinking(thinkText)
		}
		// Aggregated mode: emit the whole turn's text once, after the stream ends.
		if !e.streamText && turnText != "" {
			sink.OnText(turnText)
		}
		sink.OnTurnEnd(msg.Usage, msg.StopReason)
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
	// Backstop the context cap here rather than trusting every tool to remember
	// it. Tools that truncate themselves — to keep the tail, or to renumber
	// lines against an offset — come in already under the limit and pass through
	// untouched, so this only catches the ones that would otherwise be unbounded.
	if t := Truncate(out, TruncateOptions{}); t.Truncated {
		logrus.WithFields(logrus.Fields{
			"tool": tu.Name, "total_bytes": t.TotalBytes, "kept_bytes": t.KeptBytes,
		}).Warn("afk engine: tool result capped by the engine backstop")
		out = t.String()
	}
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
