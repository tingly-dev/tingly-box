package anthropic

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// Snapshotter is implemented by virtual models whose served behaviour varies
// per request (e.g. SequenceModel). The virtualserver handler calls Snapshot
// exactly once per request to obtain the concrete, stateless model that
// serves it — after which all existing dispatch machinery (ErrorInjection,
// HandleAnthropic, the mid-stream gate) works unchanged.
type Snapshotter interface {
	Snapshot() VirtualModel
}

// Snapshot returns the concrete model to serve a single request with. If vm
// advances per request it is resolved to a stateless snapshot; otherwise vm
// is returned unchanged. Callers must invoke this once per request.
func Snapshot(vm VirtualModel) VirtualModel {
	if s, ok := vm.(Snapshotter); ok {
		return s.Snapshot()
	}
	return vm
}

// SequenceModel walks a configured program of per-request outcomes (e.g.
// 200, 200, 429) to simulate a flaky upstream. Each request atomically
// advances a shared cursor; the resolved step is materialised as a plain
// MockModel snapshot (a success returns content; an error step carries a
// pre-content ErrorInjection), so the handler needs no sequence-specific code.
type SequenceModel struct {
	vmodel.BaseMockModel
	seq *vmodel.Sequence
}

// Compile-time interface checks.
var (
	_ VirtualModel = (*SequenceModel)(nil)
	_ Snapshotter  = (*SequenceModel)(nil)
)

// NewSequenceModel constructs an Anthropic-protocol sequence model from cfg.
func NewSequenceModel(cfg *vmodel.SequenceConfig) *SequenceModel {
	description := cfg.Description
	if description == "" {
		description = vmodel.DefaultMockDescription
	}
	return &SequenceModel{
		BaseMockModel: vmodel.BaseMockModel{
			ID:          cfg.ID,
			Name:        cfg.Name,
			Description: description,
			Type:        vmodel.VirtualModelTypeSequence,
			Delay:       cfg.Delay,
		},
		seq: vmodel.NewSequence(*cfg),
	}
}

// NewStatusSequence is the shorthand for the common case: a sequence model
// driven purely by a list of status codes, e.g.
// NewStatusSequence("flaky", "Flaky", 200, 200, 429). Success content and error
// type/message come from module defaults; build a SequenceConfig and call
// NewSequenceModel when you need per-step content, repeats, or a non-default
// OnExhaust policy.
func NewStatusSequence(id, name string, statuses ...int) *SequenceModel {
	return NewSequenceModel(&vmodel.SequenceConfig{
		ID:    id,
		Name:  name,
		Steps: vmodel.Steps(statuses...),
	})
}

// Snapshot advances the sequence and returns the MockModel snapshot for this
// request. This is the single point at which the cursor advances.
func (m *SequenceModel) Snapshot() VirtualModel {
	step := m.seq.Next()
	return NewMockModel(&MockModelConfig{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Content:     step.Content,
		Delay:       m.Delay,
		Error:       step.Error,
	})
}

// HandleAnthropic delegates to a freshly resolved snapshot so direct (non-
// handler) consumers still get one step per call. The production handler
// resolves via Snapshot first and never reaches this path.
func (m *SequenceModel) HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error) {
	return m.Snapshot().HandleAnthropic(req)
}

// HandleAnthropicStream mirrors HandleAnthropic for the streaming path.
func (m *SequenceModel) HandleAnthropicStream(ctx context.Context, req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error {
	return m.Snapshot().HandleAnthropicStream(ctx, req, emit)
}
