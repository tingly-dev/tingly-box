// Package spec is the shared file contract and execution adapter for virtual
// models. CLI services and tests load the same Definition values.
package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"gopkg.in/yaml.v3"
)

type Kind string

const (
	Mock      Kind = "mock"
	Recording Kind = "recording"
	Sequence  Kind = "sequence"
	Decision  Kind = "decision"
	Transform Kind = "transform"
	Builtin   Kind = "builtin"
)

// Definition keeps identity, protocol selection and assertions identical across
// every behavior. Exactly one behavior block is selected by kind.
type Definition struct {
	ID             string                                        `json:"id"`
	Name           string                                        `json:"name,omitempty"`
	Description    string                                        `json:"description,omitempty"`
	Kind           Kind                                          `json:"kind,omitempty"`
	Protocols      []scenario.ResponseFormat                     `json:"protocols,omitempty"`
	Mock           *vmodel.SharedMockSpec                        `json:"mock,omitempty"`
	Responses      map[scenario.ResponseFormat]scenario.Response `json:"responses,omitempty"`
	Sequence       *SequenceSpec                                 `json:"sequence,omitempty"`
	Decision       *DecisionSpec                                 `json:"decision,omitempty"`
	Transform      *TransformSpec                                `json:"transform,omitempty"`
	Builtin        string                                        `json:"builtin,omitempty"`
	Tags           []string                                      `json:"tags,omitempty"`
	Expect         *scenario.Expect                              `json:"expect,omitempty"`
	Structural     *scenario.Expect                              `json:"structural,omitempty"`
	SkipTransitive bool                                          `json:"skip_transitive,omitempty"`
}

type SequenceSpec struct {
	Steps          []vmodel.SequenceStep `json:"steps"`
	DefaultContent string                `json:"default_content,omitempty"`
	OnExhaust      vmodel.ExhaustPolicy  `json:"on_exhaust,omitempty"`
	Delay          time.Duration         `json:"delay,omitempty"`
}
type DecisionSpec struct {
	Strategy   string  `json:"strategy"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Transform names are registered transforms, not executable expressions.
type TransformSpec struct {
	Builtin string `json:"builtin"`
}

func (d Definition) MarshalYAML() (any, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	return node.Content[0], nil
}

// Load accepts one definition or a list. Legacy flat mock, SequenceConfig and
// name/mock_responses fixture documents are normalized before validation.
func Load(path string) ([]Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	defs, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return defs, nil
}
func Decode(data []byte) ([]Definition, error) {
	var value any
	if err := vmodel.DecodeConfig(data, &value); err != nil {
		return nil, err
	}
	items, ok := value.([]any)
	if !ok {
		items = []any{value}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("definitions must not be empty")
	}
	var out []Definition
	seen := map[string]bool{}
	for i, item := range items {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		d, err := decodeOne(raw)
		if err != nil {
			return nil, fmt.Errorf("definition[%d]: %w", i, err)
		}
		if seen[d.ID] {
			return nil, fmt.Errorf("duplicate definition id %q", d.ID)
		}
		seen[d.ID] = true
		out = append(out, d)
	}
	return out, nil
}

func decodeOne(raw []byte) (Definition, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return Definition{}, fmt.Errorf("definition must be an object")
	}
	if _, ok := fields["id"]; !ok {
		if name, ok := fields["name"]; ok {
			fields["id"] = name
		}
	}
	if legacy, ok := fields["mock_responses"]; ok {
		if _, exists := fields["responses"]; exists {
			return Definition{}, fmt.Errorf("responses and mock_responses cannot both be set")
		}
		fields["responses"] = legacy
		delete(fields, "mock_responses")
	}
	// Preserve existing flat mock/sequence file syntax as an import adapter.
	var mock map[string]json.RawMessage
	for _, key := range []string{"content", "tool_call", "tool_calls", "stream_chunks", "finish_reason", "stop_reason", "usage", "error", "error_category", "is_retryable", "severity"} {
		if v, ok := fields[key]; ok {
			if mock == nil {
				mock = map[string]json.RawMessage{}
			}
			mock[key] = v
			delete(fields, key)
		}
	}
	if mock != nil {
		if _, ok := fields["mock"]; ok {
			return Definition{}, fmt.Errorf("mock block conflicts with flat mock fields")
		}
		fields["mock"], _ = json.Marshal(mock)
	}
	if _, ok := fields["steps"]; ok {
		if _, exists := fields["sequence"]; exists {
			return Definition{}, fmt.Errorf("sequence block conflicts with flat sequence fields")
		}
		seq := map[string]json.RawMessage{}
		for _, key := range []string{"steps", "default_content", "on_exhaust"} {
			if v, ok := fields[key]; ok {
				seq[key] = v
				delete(fields, key)
			}
		}
		fields["sequence"], _ = json.Marshal(seq)
	}
	if delay, ok := fields["delay"]; ok {
		target := "mock"
		if _, ok := fields["sequence"]; ok {
			target = "sequence"
		}
		block := map[string]json.RawMessage{}
		if b, ok := fields[target]; ok {
			if err := json.Unmarshal(b, &block); err != nil {
				return Definition{}, err
			}
		}
		if _, ok := block["delay"]; ok {
			return Definition{}, fmt.Errorf("delay specified twice")
		}
		block["delay"] = delay
		fields[target], _ = json.Marshal(block)
		delete(fields, "delay")
	}
	for _, key := range []string{"mock", "sequence"} {
		if b, ok := fields[key]; ok {
			var block map[string]json.RawMessage
			if err := json.Unmarshal(b, &block); err != nil || block == nil {
				return Definition{}, fmt.Errorf("%s must be an object", key)
			}
			if v, ok := block["delay"]; ok {
				var duration time.Duration
				if len(v) > 0 && v[0] == '"' {
					var s string
					_ = json.Unmarshal(v, &s)
					parsed, err := time.ParseDuration(s)
					if err != nil {
						return Definition{}, fmt.Errorf("%s.delay: %w", key, err)
					}
					duration = parsed
				} else {
					if bytes.Equal(v, []byte("null")) {
						return Definition{}, fmt.Errorf("%s.delay cannot be null", key)
					}
					if err := json.Unmarshal(v, &duration); err != nil {
						return Definition{}, err
					}
				}
				if duration < 0 {
					return Definition{}, fmt.Errorf("%s.delay must not be negative", key)
				}
				block["delay"], _ = json.Marshal(duration)
			}
			fields[key], _ = json.Marshal(block)
		}
	}
	if _, ok := fields["kind"]; !ok {
		kinds := map[string]Kind{"mock": Mock, "responses": Recording, "sequence": Sequence, "decision": Decision, "transform": Transform, "builtin": Builtin}
		for key, kind := range kinds {
			if _, ok := fields[key]; ok {
				fields["kind"], _ = json.Marshal(kind)
				break
			}
		}
		if _, ok := fields["kind"]; !ok {
			fields["kind"], _ = json.Marshal(Mock)
			fields["mock"] = json.RawMessage(`{}`)
		}
	}
	raw, _ = json.Marshal(fields)
	var d Definition
	if err := vmodel.DecodeConfig(raw, &d); err != nil {
		return d, err
	}
	if err := d.Normalize(); err != nil {
		return d, err
	}
	return d, nil
}

// Normalize validates Go-built and file-built definitions with the same rules.
func (d *Definition) Normalize() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if d.Mock != nil {
		if d.Name == "" && d.Mock.Name != "" {
			d.Name = d.Mock.Name
		}
		if d.Description == "" && d.Mock.Description != "" {
			d.Description = d.Mock.Description
		}
	}
	if d.Name == "" {
		d.Name = d.ID
	}
	blocks := 0
	if d.Mock != nil {
		blocks++
	}
	if d.Responses != nil {
		blocks++
	}
	if d.Sequence != nil {
		blocks++
	}
	if d.Decision != nil {
		blocks++
	}
	if d.Transform != nil {
		blocks++
	}
	if d.Builtin != "" {
		blocks++
	}
	if blocks != 1 {
		return fmt.Errorf("exactly one behavior block is required")
	}
	switch d.Kind {
	case Mock:
		if d.Mock == nil {
			return fmt.Errorf("kind mock requires mock")
		}
		if d.Mock.ID != "" && d.Mock.ID != d.ID {
			return fmt.Errorf("mock.id conflicts with id")
		}
		d.Mock.ID = d.ID
		d.Mock.Name = d.Name
		d.Mock.Description = d.Description
		// The singular tool_call spelling is an import-only adapter.
		if d.Mock.ToolCall != nil {
			if len(d.Mock.ToolCalls) > 0 {
				return fmt.Errorf("tool_call and tool_calls cannot both be set")
			}
			// Legacy single-tool mocks used Content for streaming chunks but
			// ignored it in non-stream replies. Preserve that distinction explicitly.
			if d.Mock.Content != "" && len(d.Mock.StreamChunks) == 0 {
				d.Mock.StreamChunks = token.SplitIntoChunks(d.Mock.Content)
			}
			d.Mock.ToolCalls = d.Mock.Calls()
			d.Mock.ToolCall = nil
			d.Mock.Content = ""
		}
		raw, err := json.Marshal(d.Mock)
		if err != nil {
			return err
		}
		cfg, err := vmodel.DecodeMockSpec(raw)
		if err != nil {
			return err
		}
		d.Mock = cfg
	case Recording:
		if d.Responses == nil {
			return fmt.Errorf("kind recording requires responses")
		}
		if _, err := (scenario.Fixture{Name: d.ID, MockResponses: d.Responses, Expect: d.Expect, Structural: d.Structural}).Build(); err != nil {
			return err
		}
	case Sequence:
		if d.Sequence == nil || len(d.Sequence.Steps) == 0 {
			return fmt.Errorf("sequence requires nonempty steps")
		}
		s := d.Sequence
		if s.Delay < 0 {
			return fmt.Errorf("sequence.delay must not be negative")
		}
		if s.OnExhaust != vmodel.ExhaustLoop && s.OnExhaust != vmodel.ExhaustClamp && s.OnExhaust != vmodel.ExhaustFail {
			return fmt.Errorf("invalid sequence.on_exhaust %q", s.OnExhaust)
		}
		for _, step := range s.Steps {
			if step.Status != 0 && step.Status != 200 && (step.Status < 400 || step.Status > 599) {
				return fmt.Errorf("sequence status must be 0, 200 or 400..599")
			}
			if step.Repeat < 0 {
				return fmt.Errorf("sequence repeat must not be negative")
			}
		}
	case Decision:
		if d.Decision == nil {
			return fmt.Errorf("kind decision requires decision")
		}
		if d.Decision.Strategy != "first" && d.Decision.Strategy != "stable" {
			return fmt.Errorf("unknown decision.strategy %q", d.Decision.Strategy)
		}
		if d.Decision.Confidence < 0 || d.Decision.Confidence > 1 {
			return fmt.Errorf("decision.confidence must be 0..1")
		}
	case Transform:
		if d.Transform == nil || d.Transform.Builtin == "" {
			return fmt.Errorf("transform.builtin is required")
		}
	case Builtin:
		if d.Builtin == "" {
			return fmt.Errorf("builtin is required")
		}
	default:
		return fmt.Errorf("unknown kind %q", d.Kind)
	}
	seen := map[scenario.ResponseFormat]bool{}
	for _, p := range d.Protocols {
		if seen[p] {
			return fmt.Errorf("duplicate protocol %q", p)
		}
		seen[p] = true
		switch p {
		case scenario.FormatOpenAIChat, scenario.FormatOpenAIResponses, scenario.FormatAnthropic, scenario.FormatGoogle, FormatDecisions:
		default:
			return fmt.Errorf("unknown protocol %q", p)
		}
	}
	return nil
}

const FormatDecisions scenario.ResponseFormat = "decisions"

func FromMock(s vmodel.SharedMockSpec) Definition {
	d := Definition{ID: s.ID, Name: s.Name, Description: s.Description, Kind: Mock, Mock: &s}
	return d
}
func FromSequence(s vmodel.SequenceConfig) Definition {
	return Definition{ID: s.ID, Name: s.Name, Description: s.Description, Kind: Sequence, Sequence: &SequenceSpec{Steps: s.Steps, DefaultContent: s.DefaultContent, OnExhaust: s.OnExhaust, Delay: s.Delay}}
}
func FromFixture(f scenario.Fixture) Definition {
	return Definition{ID: f.Name, Name: f.Name, Description: f.Description, Kind: Recording, Responses: f.MockResponses, Expect: f.Expect, Structural: f.Structural, Tags: f.Tags, SkipTransitive: f.SkipTransitive}
}

func marshalYAML(defs []Definition) ([]byte, error) { return yaml.Marshal(defs) }

// MarshalJSON writes identity once, with readable durations, regardless of the
// legacy input shape. Runtime-only config identity is restored by Normalize.
func (d Definition) MarshalJSON() ([]byte, error) {
	type plain Definition
	raw, err := json.Marshal(plain(d))
	if err != nil {
		return nil, err
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	for _, key := range []string{"mock", "sequence"} {
		if b, ok := value[key]; ok {
			var block map[string]json.RawMessage
			if err := json.Unmarshal(b, &block); err != nil {
				return nil, err
			}
			if key == "mock" {
				delete(block, "id")
				delete(block, "name")
				delete(block, "description")
			}
			if delay, ok := block["delay"]; ok {
				var n time.Duration
				if err := json.Unmarshal(delay, &n); err != nil {
					return nil, err
				}
				block["delay"], _ = json.Marshal(n.String())
			}
			value[key], err = json.Marshal(block)
			if err != nil {
				return nil, err
			}
		}
	}
	return json.Marshal(value)
}
