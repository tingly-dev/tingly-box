package vmodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

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

// Validate checks the common runtime mock configuration. File decoding belongs
// to vmodel/spec, which owns the sole model definition contract.
func (spec *SharedMockSpec) Validate() error {
	if strings.TrimSpace(spec.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if spec.Delay < 0 {
		return fmt.Errorf("delay must not be negative")
	}
	for i, call := range spec.ToolCalls {
		if strings.TrimSpace(call.Name) == "" {
			return fmt.Errorf("tool_calls[%d].name is required", i)
		}
		if call.RawArguments != nil && (!json.Valid(call.RawArguments) || call.Arguments != nil) {
			return fmt.Errorf("tool_calls[%d]: raw_arguments must be JSON and cannot accompany arguments", i)
		}
	}
	if e := spec.Error; e != nil {
		if e.Stage != ErrorStagePreContent && e.Stage != ErrorStageMidStream {
			return fmt.Errorf("error.stage must be 0 (pre-content) or 1 (mid-stream)")
		}
		if e.Status != 0 && (e.Status < 400 || e.Status > 599) {
			return fmt.Errorf("error.status must be 400..599 or 0 for the default")
		}
		if e.MidStreamMode < MidStreamModeConnectionClose || e.MidStreamMode > MidStreamModeCleanEOF {
			return fmt.Errorf("error.mid_stream_mode must be 0, 1, or 2")
		}
	}
	if spec.Name == "" {
		spec.Name = spec.ID
	}
	return nil
}
