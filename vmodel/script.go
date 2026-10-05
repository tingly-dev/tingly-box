package vmodel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// A script is a YAML (or JSON) document that describes one SequenceConfig:
// the ordered program of per-request outcomes a scripted virtual model walks.
// It is the config-driven way to say "on request 1 call Read, on request 2
// call Edit, on request 3 fail with 529, then answer" — see
// .design/vmodel-script.md for the schema and rationale.
//
//	id: refactor-flow          # defaults to the file name
//	on_exhaust: clamp          # loop (default) | clamp | fail
//	steps:
//	  - say: "Let me look at it."
//	    tool: {name: Read, arguments: {file_path: /tmp/a.go}}
//	  - 529                    # bare number == {status: 529}
//	  - say: "Done."
//	    usage: {input: 1200, output: 40, cache_read: 1000}

var scriptIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ParseScript decodes and validates one script. fallbackID (typically the file
// name without extension) is used when the script sets no id. Unknown fields
// are rejected so a typo fails loudly instead of silently becoming a default.
func ParseScript(data []byte, fallbackID string) (SequenceConfig, error) {
	var cfg SequenceConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return cfg, errors.New("script is empty")
		}
		return cfg, err
	}
	if cfg.ID == "" {
		cfg.ID = fallbackID
	}
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks a config for the mistakes a hand-written script can make and
// normalises OnExhaust ("loop" → the zero value). Programmatically built
// configs (Steps(...), NewStatusSequence) need not call it.
func (c *SequenceConfig) Validate() error {
	if !scriptIDPattern.MatchString(c.ID) {
		return fmt.Errorf("invalid id %q: use letters, digits, '.', '_' or '-'", c.ID)
	}
	switch c.OnExhaust {
	case "loop":
		c.OnExhaust = ExhaustLoop
	case ExhaustLoop, ExhaustClamp, ExhaustFail:
	default:
		return fmt.Errorf("unknown on_exhaust %q (want loop, clamp or fail)", c.OnExhaust)
	}
	if len(c.Steps) == 0 {
		return errors.New("script has no steps")
	}
	for i, s := range c.Steps {
		if err := s.validate(); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

func (s SequenceStep) validate() error {
	if s.Repeat < 0 {
		return errors.New("repeat must not be negative")
	}
	if s.Tool != nil && strings.TrimSpace(s.Tool.Name) == "" {
		return errors.New("tool needs a name")
	}
	if s.MidStream != nil {
		if _, err := s.MidStream.injection(); err != nil {
			return err
		}
		if s.MidStream.AfterEvents < 0 {
			return errors.New("midstream.after_events must not be negative")
		}
	}
	if s.Status == 0 || s.Status == 200 {
		return nil
	}
	if s.Status < 400 || s.Status > 599 {
		return fmt.Errorf("status %d: use 200 or an error status (400-599)", s.Status)
	}
	if s.Content != "" || s.Tool != nil || s.Usage != nil || s.StopReason != "" || s.MidStream != nil {
		return fmt.Errorf("status %d is an error step: say/tool/usage/stop_reason/midstream only apply to a success step", s.Status)
	}
	return nil
}
