package mock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Step is one entry in a mock-agent script. Implementations describe how to
// emit events and (optionally) interact with the configured handler.
type Step interface {
	play(ctx context.Context, st *runState) error
}

// runState carries shared state across script playback.
type runState struct {
	agentType  agentboot.AgentType
	sessionID  string
	prompt     string
	handler    agentboot.MessageHandler
	opts       agentboot.ExecutionOptions
	cfg        Config
	events     *[]agentboot.Event
	stepIdx    int
	totalSteps int

	// mismatches collects expectation failures (PermissionStep / AskStep
	// asserts).
	mismatches []string

	// halt stops further script playback once set.
	halt bool
}

func (s *runState) emit(msg agentboot.AgentMessage) error {
	*s.events = append(*s.events, msg.ToEvent())
	if s.handler != nil {
		if err := s.handler.OnMessage(msg); err != nil {
			s.handler.OnError(err)
			return err
		}
	}
	return nil
}

func (s *runState) emitEvent(ev agentboot.Event) {
	*s.events = append(*s.events, ev)
	if s.handler != nil {
		if msg := agentboot.MessageFromEvent(ev, s.agentType); msg != nil {
			_ = s.handler.OnMessage(msg)
		}
	}
}

func (s *runState) recordMismatch(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.mismatches = append(s.mismatches, msg)
	if s.handler != nil {
		s.handler.OnError(fmt.Errorf("mockagent: %s", msg))
	}
}

// AssistantStep emits an assistant text message.
type AssistantStep struct {
	Text string
}

func (s *AssistantStep) play(_ context.Context, st *runState) error {
	return st.emit(agentboot.NewAssistantMessage(st.agentType, st.sessionID, s.Text))
}

// StreamDeltaStep emits a stream_delta event. Note: in the production
// streaming handler, deltas are silently logged and not surfaced to chat.
type StreamDeltaStep struct {
	Delta string
}

func (s *StreamDeltaStep) play(_ context.Context, st *runState) error {
	return st.emit(agentboot.NewStreamDeltaMessage(st.agentType, st.sessionID, s.Delta))
}

// ToolUseStep emits a tool_use ContentBlock attached to an assistant message.
type ToolUseStep struct {
	ToolID   string
	ToolName string
	Input    map[string]any
}

func (s *ToolUseStep) play(_ context.Context, st *runState) error {
	if s.ToolID == "" {
		s.ToolID = "tool_" + uuid.NewString()[:6]
	}
	msg := agentboot.NewAssistantMessage(st.agentType, st.sessionID, "")
	msg.Content = []agentboot.ContentBlock{{
		Type:     "tool_use",
		ToolID:   s.ToolID,
		ToolName: s.ToolName,
		Input:    s.Input,
	}}
	return st.emit(msg)
}

// ToolResultStep emits a tool_result event referencing a prior ToolUseStep.
type ToolResultStep struct {
	ToolID  string
	Output  any
	IsError bool
}

func (s *ToolResultStep) play(_ context.Context, st *runState) error {
	ev := agentboot.Event{
		Type:      agentboot.EventTypeToolResult,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"agent_type":  string(st.agentType),
			"session_id":  st.sessionID,
			"tool_use_id": s.ToolID,
			"is_error":    s.IsError,
		},
	}
	if s.Output != nil {
		ev.Data["content"] = s.Output
	}
	st.emitEvent(ev)
	return nil
}

// PermissionStep performs a permission round-trip with the handler.
//
// If ExpectApproved is non-nil and the handler returns a different value, the
// mismatch is recorded (surfaced via OnError and Result.Error).
//
// If the request is denied and OnDenyTerminate is true, the script halts and
// a "permission_denied" ResultMessage is emitted.
type PermissionStep struct {
	ToolName        string
	Input           map[string]any
	Reason          string
	RequestID       string // optional override; auto-generated otherwise
	ExpectApproved  *bool
	OnDenyTerminate bool
}

func (s *PermissionStep) play(ctx context.Context, st *runState) error {
	if s.RequestID == "" {
		s.RequestID = uuid.NewString()[:8]
	}

	permReq := agentboot.NewPermissionRequestMessage(
		st.agentType, st.sessionID, s.RequestID, s.ToolName, s.Input, s.Reason,
	)
	permReq.Step = st.stepIdx
	permReq.Total = st.totalSteps
	if err := st.emit(permReq); err != nil {
		return err
	}

	approved, result := s.invoke(ctx, st)

	reasonText := result.Reason
	if reasonText == "" {
		if approved {
			reasonText = "Approved"
		} else {
			reasonText = "Denied"
		}
	}
	if err := st.emit(agentboot.NewPermissionResultMessage(
		st.agentType, st.sessionID, s.RequestID, approved, reasonText,
	)); err != nil {
		return err
	}

	if s.ExpectApproved != nil && *s.ExpectApproved != approved {
		st.recordMismatch("step %d: permission expected approved=%v got %v",
			st.stepIdx, *s.ExpectApproved, approved)
	}

	if !approved && s.OnDenyTerminate {
		st.halt = true
		return st.emit(agentboot.NewResultMessage(
			st.agentType, st.sessionID, "permission_denied",
			fmt.Sprintf("Permission denied at step %d", st.stepIdx),
		))
	}
	return nil
}

func (s *PermissionStep) invoke(ctx context.Context, st *runState) (bool, agentboot.PermissionResult) {
	if st.cfg.AutoApprove {
		return true, agentboot.PermissionResult{Approved: true, UpdatedInput: s.Input}
	}
	if st.handler == nil {
		return true, agentboot.PermissionResult{Approved: true, UpdatedInput: s.Input}
	}
	req := agentboot.PermissionRequest{
		RequestID: s.RequestID,
		AgentType: st.agentType,
		ToolName:  s.ToolName,
		Input:     s.Input,
		Reason:    s.Reason,
		Timestamp: time.Now(),
		SessionID: st.sessionID,
		BotUUID:   st.opts.BotUUID,
		ChatID:    st.opts.ChatID,
		Platform:  st.opts.Platform,
	}
	r, err := st.handler.OnApproval(ctx, req)
	if err != nil {
		return false, agentboot.PermissionResult{Approved: false, Reason: err.Error()}
	}
	return r.Approved, r
}

// AskQuestion is one question in an AskStep.
type AskQuestion struct {
	Question string
	Header   string
	Options  []AskOption
}

// AskOption is one selectable option for an AskQuestion.
type AskOption struct {
	Label       string
	Description string
}

// AskStep performs an AskUserQuestion round-trip.
//
// ExpectAnswers asserts that the handler returned a specific option index for
// each question (qIdx -> optIdx).
type AskStep struct {
	Questions       []AskQuestion
	RequestID       string
	ExpectApproved  *bool
	ExpectAnswers   map[int]int
	OnDenyTerminate bool
}

func (s *AskStep) play(ctx context.Context, st *runState) error {
	if s.RequestID == "" {
		s.RequestID = uuid.NewString()[:8]
	}

	input := s.buildInput()
	permReq := agentboot.NewPermissionRequestMessage(
		st.agentType, st.sessionID, s.RequestID, "AskUserQuestion", input, "Mock AskUserQuestion",
	)
	permReq.Step = st.stepIdx
	permReq.Total = st.totalSteps
	if err := st.emit(permReq); err != nil {
		return err
	}

	approved, result := s.invoke(ctx, st, input)

	if err := st.emit(agentboot.NewPermissionResultMessage(
		st.agentType, st.sessionID, s.RequestID, approved, result.Reason,
	)); err != nil {
		return err
	}

	if s.ExpectApproved != nil && *s.ExpectApproved != approved {
		st.recordMismatch("step %d: ask expected approved=%v got %v",
			st.stepIdx, *s.ExpectApproved, approved)
	}

	if approved {
		s.checkAnswers(st, result)
	}

	if !approved && s.OnDenyTerminate {
		st.halt = true
		return st.emit(agentboot.NewResultMessage(
			st.agentType, st.sessionID, "ask_denied",
			fmt.Sprintf("AskUserQuestion denied at step %d", st.stepIdx),
		))
	}
	return nil
}

func (s *AskStep) buildInput() map[string]interface{} {
	// IMPrompter consumes "questions" as []interface{}; serialize accordingly.
	qs := make([]interface{}, 0, len(s.Questions))
	for _, q := range s.Questions {
		opts := make([]interface{}, 0, len(q.Options))
		for _, o := range q.Options {
			opts = append(opts, map[string]interface{}{
				"label":       o.Label,
				"description": o.Description,
			})
		}
		qs = append(qs, map[string]interface{}{
			"question": q.Question,
			"header":   q.Header,
			"options":  opts,
		})
	}
	return map[string]interface{}{"questions": qs}
}

func (s *AskStep) invoke(ctx context.Context, st *runState, input map[string]interface{}) (bool, agentboot.AskResult) {
	if st.cfg.AutoApprove {
		return true, agentboot.AskResult{ID: s.RequestID, Approved: true, UpdatedInput: input}
	}
	if st.handler == nil {
		return true, agentboot.AskResult{ID: s.RequestID, Approved: true, UpdatedInput: input}
	}
	req := agentboot.AskRequest{
		ID:        s.RequestID,
		Type:      "tool_use",
		AgentType: st.agentType,
		Platform:  st.opts.Platform,
		ChatID:    st.opts.ChatID,
		BotUUID:   st.opts.BotUUID,
		SessionID: st.sessionID,
		ToolName:  "AskUserQuestion",
		Input:     input,
	}
	r, err := st.handler.OnAsk(ctx, req)
	if err != nil {
		return false, agentboot.AskResult{ID: s.RequestID, Approved: false, Reason: err.Error()}
	}
	return r.Approved, r
}

func (s *AskStep) checkAnswers(st *runState, result agentboot.AskResult) {
	if len(s.ExpectAnswers) == 0 {
		return
	}
	answers := pickAnswers(result)
	for qIdx, optIdx := range s.ExpectAnswers {
		if qIdx < 0 || qIdx >= len(s.Questions) {
			continue
		}
		if optIdx < 0 || optIdx >= len(s.Questions[qIdx].Options) {
			continue
		}
		want := s.Questions[qIdx].Options[optIdx].Label
		got := lookupAnswer(answers, qIdx, s.Questions[qIdx].Question)
		if got != want {
			st.recordMismatch("step %d: ask q[%d] expected %q got %q",
				st.stepIdx, qIdx, want, got)
		}
	}
}

func pickAnswers(r agentboot.AskResult) map[string]interface{} {
	if r.UpdatedInput != nil {
		if m, ok := r.UpdatedInput["answers"].(map[string]interface{}); ok {
			return m
		}
	}
	return r.Selection
}

func lookupAnswer(m map[string]interface{}, qIdx int, qText string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[fmt.Sprintf("%d", qIdx)]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	if qText != "" {
		if v, ok := m[qText]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// ResultStep emits a final ResultMessage and halts further playback.
type ResultStep struct {
	Status  string
	Message string
	CostUSD float64
	Steps   int
}

func (s *ResultStep) play(_ context.Context, st *runState) error {
	status := s.Status
	if status == "" {
		status = "success"
	}
	msg := agentboot.NewResultMessage(st.agentType, st.sessionID, status, s.Message)
	msg.CostUSD = s.CostUSD
	msg.Steps = s.Steps
	st.halt = true
	return st.emit(msg)
}

// ErrorStep dispatches an error to the handler. Set Halt=true to stop the
// script after the error is delivered.
type ErrorStep struct {
	Err  error
	Halt bool
}

func (s *ErrorStep) play(_ context.Context, st *runState) error {
	if st.handler != nil && s.Err != nil {
		st.handler.OnError(s.Err)
	}
	if s.Halt {
		st.halt = true
	}
	return nil
}

// ExpectApprove is a convenience for ExpectApproved=true.
func ExpectApprove() *bool { v := true; return &v }

// ExpectDeny is a convenience for ExpectApproved=false.
func ExpectDeny() *bool { v := false; return &v }

// ScriptBuilder constructs a Step slice with a fluent API.
type ScriptBuilder struct {
	steps []Step
}

// NewScript returns a fresh ScriptBuilder.
func NewScript() *ScriptBuilder { return &ScriptBuilder{} }

// Build returns the accumulated step list.
func (b *ScriptBuilder) Build() []Step { return b.steps }

// Add appends arbitrary steps.
func (b *ScriptBuilder) Add(steps ...Step) *ScriptBuilder {
	b.steps = append(b.steps, steps...)
	return b
}

// Assistant appends an AssistantStep.
func (b *ScriptBuilder) Assistant(text string) *ScriptBuilder {
	return b.Add(&AssistantStep{Text: text})
}

// Stream appends a StreamDeltaStep.
func (b *ScriptBuilder) Stream(delta string) *ScriptBuilder {
	return b.Add(&StreamDeltaStep{Delta: delta})
}

// Tool appends a ToolUseStep.
func (b *ScriptBuilder) Tool(name string, input map[string]any) *ScriptBuilder {
	return b.Add(&ToolUseStep{ToolName: name, Input: input})
}

// ToolResult appends a ToolResultStep referencing the most recent ToolUseStep.
func (b *ScriptBuilder) ToolResult(output any) *ScriptBuilder {
	var toolID string
	for i := len(b.steps) - 1; i >= 0; i-- {
		if tu, ok := b.steps[i].(*ToolUseStep); ok {
			if tu.ToolID == "" {
				tu.ToolID = "tool_" + uuid.NewString()[:6]
			}
			toolID = tu.ToolID
			break
		}
	}
	return b.Add(&ToolResultStep{ToolID: toolID, Output: output})
}

// PermissionOpt is a functional option for ScriptBuilder.Permission.
type PermissionOpt func(*PermissionStep)

// WithReason sets the permission request reason.
func WithReason(r string) PermissionOpt {
	return func(p *PermissionStep) { p.Reason = r }
}

// WithExpectApproved asserts the user's approval value.
func WithExpectApproved(approved bool) PermissionOpt {
	v := approved
	return func(p *PermissionStep) { p.ExpectApproved = &v }
}

// WithDenyHalts overrides OnDenyTerminate (default: true).
func WithDenyHalts(halt bool) PermissionOpt {
	return func(p *PermissionStep) { p.OnDenyTerminate = halt }
}

// Permission appends a PermissionStep with OnDenyTerminate=true by default.
func (b *ScriptBuilder) Permission(toolName string, input map[string]any, opts ...PermissionOpt) *ScriptBuilder {
	p := &PermissionStep{
		ToolName:        toolName,
		Input:           input,
		OnDenyTerminate: true,
	}
	for _, o := range opts {
		o(p)
	}
	return b.Add(p)
}

// AskOpt is a functional option for ScriptBuilder.Ask.
type AskOpt func(*AskStep)

// WithAskExpectAnswers asserts the handler picked specific options per question.
func WithAskExpectAnswers(answers map[int]int) AskOpt {
	return func(a *AskStep) { a.ExpectAnswers = answers }
}

// WithAskExpectApproved asserts the handler approved or denied the ask.
func WithAskExpectApproved(approved bool) AskOpt {
	v := approved
	return func(a *AskStep) { a.ExpectApproved = &v }
}

// WithAskDenyHalts overrides OnDenyTerminate (default: true).
func WithAskDenyHalts(halt bool) AskOpt {
	return func(a *AskStep) { a.OnDenyTerminate = halt }
}

// Ask appends an AskStep with the given questions.
func (b *ScriptBuilder) Ask(questions []AskQuestion, opts ...AskOpt) *ScriptBuilder {
	a := &AskStep{Questions: questions, OnDenyTerminate: true}
	for _, o := range opts {
		o(a)
	}
	return b.Add(a)
}

// Success appends a success ResultStep.
func (b *ScriptBuilder) Success(message string) *ScriptBuilder {
	return b.Add(&ResultStep{Status: "success", Message: message})
}

// Result appends an arbitrary ResultStep.
func (b *ScriptBuilder) Result(status, message string) *ScriptBuilder {
	return b.Add(&ResultStep{Status: status, Message: message})
}

// Error appends an ErrorStep that does not halt.
func (b *ScriptBuilder) Error(err error) *ScriptBuilder {
	return b.Add(&ErrorStep{Err: err})
}

// FailWith appends an ErrorStep followed by an "error" ResultStep.
func (b *ScriptBuilder) FailWith(err error) *ScriptBuilder {
	return b.Add(&ErrorStep{Err: err}, &ResultStep{Status: "error", Message: err.Error()})
}

// defaultLinearScript replicates the legacy fixed-loop behavior for callers
// that don't supply a Script.
func defaultLinearScript(cfg Config) []Step {
	n := cfg.MaxIterations
	if n <= 0 {
		n = DefaultConfig().MaxIterations
	}
	out := make([]Step, 0, n*2+1)
	for step := 1; step <= n; step++ {
		if cfg.AskUserQuestionFrequency > 0 && step%cfg.AskUserQuestionFrequency == 0 {
			out = append(out, &AskStep{
				Questions: []AskQuestion{
					{
						Question: fmt.Sprintf("Mock question %d of %d", step, n),
						Header:   "Mock",
						Options: []AskOption{
							{Label: "Option A", Description: "First option"},
							{Label: "Option B", Description: "Second option"},
							{Label: "Option C", Description: "Third option"},
						},
					},
				},
				OnDenyTerminate: true,
			})
		} else {
			tool := mockToolNames[(step-1)%len(mockToolNames)]
			out = append(out, &PermissionStep{
				ToolName: tool,
				Input: map[string]interface{}{
					"step":  step,
					"total": n,
				},
				Reason:          fmt.Sprintf("Mock permission request %d of %d", step, n),
				OnDenyTerminate: true,
			})
		}
		out = append(out, &linearAssistantStep{tpl: cfg.ResponseTemplate, step: step, total: n})
	}
	out = append(out, &ResultStep{Status: "success", Message: "Mock agent completed all iterations"})
	return out
}

// linearAssistantStep formats and emits an assistant message using the legacy
// ResponseTemplate placeholders (used by defaultLinearScript only).
type linearAssistantStep struct {
	tpl   string
	step  int
	total int
}

func (s *linearAssistantStep) play(_ context.Context, st *runState) error {
	text := s.tpl
	text = strings.ReplaceAll(text, "{step}", fmt.Sprintf("%d", s.step))
	text = strings.ReplaceAll(text, "{total}", fmt.Sprintf("%d", s.total))
	text = strings.ReplaceAll(text, "{prompt}", truncatePrompt(st.prompt))
	return st.emit(agentboot.NewAssistantMessage(st.agentType, st.sessionID, text))
}
