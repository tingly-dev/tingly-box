package vmodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DecodeConfig reads one JSON or YAML document into an existing configuration
// type. JSON tags define the shared field names. Unknown and duplicate fields,
// non-JSON YAML values, and additional documents are rejected.
func DecodeConfig(data []byte, dst any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var value any
	if err := dec.Decode(&value); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	if value == nil {
		return fmt.Errorf("config must not be null or empty")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("config must contain exactly one document")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("config must use JSON-compatible values: %w", err)
	}
	jd := json.NewDecoder(bytes.NewReader(raw))
	jd.DisallowUnknownFields()
	jd.UseNumber()
	if err := jd.Decode(dst); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

// LoadMockSpec loads a single static/tool/error model without registering it in
// production defaults. Pass the result directly to either protocol's NewMockModel.
// Deprecated: use vmodel/spec.Load for the common model and test file contract.
func LoadMockSpec(path string) (*SharedMockSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	spec, err := DecodeMockSpec(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return spec, nil
}

// DecodeMockSpec accepts the same fields in JSON and YAML. delay accepts a Go
// duration string ("50ms") or integer nanoseconds, matching time.Duration's JSON
// representation. No other units are inferred.
func DecodeMockSpec(data []byte) (*SharedMockSpec, error) {
	type plain SharedMockSpec
	wire := struct {
		*plain
		Delay json.RawMessage `json:"delay"`
	}{plain: new(plain)}
	if err := DecodeConfig(data, &wire); err != nil {
		return nil, err
	}
	spec := (*SharedMockSpec)(wire.plain)
	if len(wire.Delay) > 0 {
		if bytes.Equal(wire.Delay, []byte("null")) {
			return nil, fmt.Errorf("delay must be a duration string or integer nanoseconds")
		}
		if wire.Delay[0] == '"' {
			var value string
			if err := json.Unmarshal(wire.Delay, &value); err != nil {
				return nil, err
			}
			delay, err := time.ParseDuration(value)
			if err != nil {
				return nil, fmt.Errorf("delay: %w", err)
			}
			spec.Delay = delay
		} else if err := json.Unmarshal(wire.Delay, &spec.Delay); err != nil {
			return nil, fmt.Errorf("delay: %w", err)
		}
	}
	if strings.TrimSpace(spec.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	if spec.Delay < 0 {
		return nil, fmt.Errorf("delay must not be negative")
	}
	if spec.ToolCall != nil && len(spec.ToolCalls) > 0 {
		return nil, fmt.Errorf("tool_call and tool_calls cannot both be set")
	}
	for i, call := range spec.Calls() {
		if strings.TrimSpace(call.Name) == "" {
			return nil, fmt.Errorf("tool_calls[%d].name is required", i)
		}
		if call.RawArguments != nil && (!json.Valid(call.RawArguments) || call.Arguments != nil) {
			return nil, fmt.Errorf("tool_calls[%d]: raw_arguments must be JSON and cannot accompany arguments", i)
		}
	}
	if e := spec.Error; e != nil {
		if e.Stage != ErrorStagePreContent && e.Stage != ErrorStageMidStream {
			return nil, fmt.Errorf("error.stage must be 0 (pre-content) or 1 (mid-stream)")
		}
		if e.Status != 0 && (e.Status < 400 || e.Status > 599) {
			return nil, fmt.Errorf("error.status must be 400..599 or 0 for the default")
		}
		if e.MidStreamMode < MidStreamModeConnectionClose || e.MidStreamMode > MidStreamModeCleanEOF {
			return nil, fmt.Errorf("error.mid_stream_mode must be 0, 1, or 2")
		}
	}
	if spec.Name == "" {
		spec.Name = spec.ID
	}
	return spec, nil
}
