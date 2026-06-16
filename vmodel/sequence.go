package vmodel

import (
	"fmt"
	"sync/atomic"
	"time"
)

// SequenceStep is one entry in a SequenceModel's response program. A step is
// either a success (Status 0 or 200 → a normal content response) or a
// pre-content failure (any other status → the configured HTTP error envelope).
//
// This lets a single virtual model reproduce the real-world behaviour of a
// flaky upstream — e.g. "200, 200, 429, 200" — so failover / retry / backoff
// logic can be exercised deterministically without standing up a real or
// ad-hoc test provider.
type SequenceStep struct {
	// Status is the HTTP status this step serves. 0 and 200 both mean
	// "success" (return Content); anything else is served as a pre-content
	// error with that status code.
	Status int `json:"status" yaml:"status"`

	// Content is the response body for a success step. When empty the
	// SequenceConfig.DefaultContent is used. Ignored for error steps.
	Content string `json:"content,omitempty" yaml:"content,omitempty"`

	// Message and Type override the error envelope for an error step. When
	// empty they are derived from Status (see defaultErrorMeta). Ignored for
	// success steps.
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
	Type    string `json:"type,omitempty" yaml:"type,omitempty"`

	// Repeat serves this step Repeat consecutive times before advancing to
	// the next one. Values <= 0 are treated as 1. Useful for compact configs
	// like "succeed 5×, then fail once".
	Repeat int `json:"repeat,omitempty" yaml:"repeat,omitempty"`
}

// SequenceConfig describes a SequenceModel: an ordered program of steps that
// is walked one step per request. By default the program loops (wraps back to
// the first step) so the model is reusable across an unbounded number of
// requests.
type SequenceConfig struct {
	ID          string
	Name        string
	Description string
	Delay       time.Duration

	// DefaultContent backs any success step that does not set its own Content.
	DefaultContent string

	// Steps is the response program. Each step is expanded by its Repeat
	// count at construction time.
	Steps []SequenceStep

	// NoLoop, when true, clamps to the last step after the program is
	// exhausted instead of wrapping back to the start. Default (false) loops.
	NoLoop bool
}

// ResolvedStep is the concrete outcome of advancing a Sequence: either a
// success (Error == nil, use Content) or a pre-content failure (Error set).
type ResolvedStep struct {
	Status  int
	Content string
	Error   *ErrorInjection // nil for success steps
}

// Sequence is the protocol-neutral engine behind the per-protocol
// SequenceModel wrappers. It owns the expanded step program and an atomic
// cursor so concurrent requests each grab a distinct, monotonically advancing
// step without locking.
type Sequence struct {
	flat           []ResolvedStep
	defaultContent string
	noLoop         bool
	cursor         atomic.Uint64
}

// NewSequence flattens cfg.Steps (expanding Repeat) and pre-resolves each step
// so Next() is allocation-free on the hot path. A config with no steps yields
// a single success step backed by DefaultContent, so the model is always
// usable.
func NewSequence(cfg SequenceConfig) *Sequence {
	s := &Sequence{
		defaultContent: cfg.DefaultContent,
		noLoop:         cfg.NoLoop,
	}
	for _, step := range cfg.Steps {
		repeat := step.Repeat
		if repeat <= 0 {
			repeat = 1
		}
		resolved := s.resolve(step)
		for i := 0; i < repeat; i++ {
			s.flat = append(s.flat, resolved)
		}
	}
	if len(s.flat) == 0 {
		s.flat = []ResolvedStep{{Status: 200, Content: cfg.DefaultContent}}
	}
	return s
}

// Next atomically advances the cursor and returns the step for this request.
// It is safe for concurrent use; each caller observes a distinct cursor value.
func (s *Sequence) Next() ResolvedStep {
	n := s.cursor.Add(1) - 1
	idx := int(n % uint64(len(s.flat)))
	if s.noLoop && n >= uint64(len(s.flat)) {
		idx = len(s.flat) - 1
	}
	return s.flat[idx]
}

// Len reports the number of (post-expansion) steps in the program.
func (s *Sequence) Len() int { return len(s.flat) }

func (s *Sequence) resolve(step SequenceStep) ResolvedStep {
	if step.Status == 0 || step.Status == 200 {
		content := step.Content
		if content == "" {
			content = s.defaultContent
		}
		return ResolvedStep{Status: 200, Content: content}
	}
	typ, msg := step.Type, step.Message
	dtyp, dmsg := defaultErrorMeta(step.Status)
	if typ == "" {
		typ = dtyp
	}
	if msg == "" {
		msg = dmsg
	}
	return ResolvedStep{
		Status: step.Status,
		Error: &ErrorInjection{
			Stage:   ErrorStagePreContent,
			Status:  step.Status,
			Message: msg,
			Type:    typ,
		},
	}
}

// defaultErrorMeta maps an HTTP status to the protocol-conventional error type
// and a human-readable default message, mirroring the envelopes used by the
// always-fail error mocks (see defaults_shared.go / ExtendedErrorSpecs).
func defaultErrorMeta(status int) (typ, msg string) {
	switch status {
	case 400:
		return "invalid_request_error", "invalid request"
	case 401:
		return "authentication_error", "authentication failed"
	case 403:
		return "permission_error", "permission denied"
	case 404:
		return "not_found_error", "model not found"
	case 408:
		return "timeout_error", "request timeout"
	case 429:
		return "rate_limit_error", "rate limit exceeded"
	case 500:
		return "api_error", "internal server error"
	case 502:
		return "api_error", "bad gateway"
	case 503:
		return "overloaded_error", "service unavailable"
	case 504:
		return "timeout_error", "gateway timeout"
	case 529:
		return "overloaded_error", "overloaded"
	default:
		return "api_error", fmt.Sprintf("simulated error %d", status)
	}
}

// DefaultSequenceConfigs returns the user-facing demo sequence(s) registered
// into BOTH default registries by each protocol's RegisterDefaults. A demo
// sequence is genuinely useful for onboarding / dry-runs: it lets users see
// how the gateway reacts to an intermittently rate-limited upstream without
// configuring a real provider.
func DefaultSequenceConfigs() []SequenceConfig {
	return []SequenceConfig{
		{
			ID:          "virtual-sequence-429",
			Name:        "Virtual Sequence (200, 200, 429)",
			Description: "Cycles through HTTP 200, 200, 429 on successive requests — simulates a provider that intermittently rate-limits. Useful for exercising failover/retry/backoff without a real upstream.",
			Delay:       50 * time.Millisecond,
			DefaultContent: "Sequenced virtual response: this request succeeded. " +
				"Every third request returns HTTP 429 instead.",
			Steps: []SequenceStep{
				{Status: 200},
				{Status: 200},
				{Status: 429},
			},
		},
	}
}
