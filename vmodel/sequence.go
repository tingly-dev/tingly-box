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

	// ErrorMessage and ErrorType override the error envelope for an error
	// step. When empty they are derived from Status (see defaultErrorMeta).
	// Ignored for success steps.
	ErrorMessage string `json:"error_message,omitempty" yaml:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty" yaml:"error_type,omitempty"`

	// Repeat serves this step Repeat consecutive times before advancing to
	// the next one. Values <= 0 are treated as 1. Useful for compact configs
	// like "succeed 5×, then fail once".
	Repeat int `json:"repeat,omitempty" yaml:"repeat,omitempty"`
}

// FallbackSequenceContent is the module-level fallback body for a success step
// that sets neither its own Content nor SequenceConfig.DefaultContent. It keeps
// a bare Step(200) useful out of the box. Distinct from SequenceConfig's
// DefaultContent — that one is per-model, this one is the last resort when a
// model configures no default at all.
const FallbackSequenceContent = "Sequenced virtual response."

// StepOption customizes a SequenceStep built by Step. Only Status is required;
// these options override the otherwise-defaulted fields for the uncommon cases.
type StepOption func(*SequenceStep)

// WithContent sets a success step's response body (overrides the config/module
// default content).
func WithContent(content string) StepOption {
	return func(s *SequenceStep) { s.Content = content }
}

// WithErrorMessage overrides an error step's message (otherwise derived from
// Status).
func WithErrorMessage(message string) StepOption {
	return func(s *SequenceStep) { s.ErrorMessage = message }
}

// WithErrorType overrides an error step's type (otherwise derived from Status).
func WithErrorType(typ string) StepOption {
	return func(s *SequenceStep) { s.ErrorType = typ }
}

// WithRepeat serves the step n consecutive times before advancing.
func WithRepeat(n int) StepOption {
	return func(s *SequenceStep) { s.Repeat = n }
}

// Step builds a SequenceStep from a status code plus optional overrides. Status
// is the only required input — content for a success step and the error
// type/message for a failure step are filled from defaults at build time
// (module FallbackSequenceContent / defaultErrorMeta), so Step(429) and
// Step(200) are both immediately usable.
func Step(status int, opts ...StepOption) SequenceStep {
	s := SequenceStep{Status: status}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

// Steps builds a program from a bare list of status codes — the common case,
// e.g. Steps(200, 200, 429). Each step takes default content / error metadata.
func Steps(statuses ...int) []SequenceStep {
	out := make([]SequenceStep, len(statuses))
	for i, status := range statuses {
		out[i] = SequenceStep{Status: status}
	}
	return out
}

// ExhaustPolicy selects what a sequence serves once every step has been
// consumed once. The zero value loops, which is the common default.
type ExhaustPolicy string

const (
	// ExhaustLoop wraps back to the first step and repeats the program
	// indefinitely. This is the zero value / default.
	ExhaustLoop ExhaustPolicy = ""

	// ExhaustClamp keeps serving the last step forever once the program is
	// exhausted (e.g. 200, 503 → 200, 503, 503, 503, …).
	ExhaustClamp ExhaustPolicy = "clamp"

	// ExhaustFail serves a terminal pre-content error (HTTP 410, type
	// "sequence_exhausted") for every request after the program is exhausted,
	// modelling an upstream whose scripted run is over.
	ExhaustFail ExhaustPolicy = "fail"
)

// SequenceConfig describes a SequenceModel: an ordered program of steps that
// is walked one step per request. By default the program loops (wraps back to
// the first step) so the model is reusable across an unbounded number of
// requests; set OnExhaust to change what happens once it is consumed.
//
// Tagged like SequenceStep even though nothing in the codebase currently
// (de)serializes it: both types describe the same potential external
// surface (a sequence loaded from a config file or management API), and
// tagging them together keeps that door open without committing to it yet.
type SequenceConfig struct {
	ID          string        `json:"id" yaml:"id"`
	Name        string        `json:"name" yaml:"name"`
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	Delay       time.Duration `json:"delay,omitempty" yaml:"delay,omitempty"`

	// DefaultContent backs any success step that does not set its own Content.
	DefaultContent string `json:"default_content,omitempty" yaml:"default_content,omitempty"`

	// Steps is the response program. Each step is expanded by its Repeat
	// count at construction time.
	Steps []SequenceStep `json:"steps,omitempty" yaml:"steps,omitempty"`

	// OnExhaust selects the behaviour after the program is consumed once:
	// ExhaustLoop (default) wraps around, ExhaustClamp repeats the last step,
	// ExhaustFail serves a terminal error.
	OnExhaust ExhaustPolicy `json:"on_exhaust,omitempty" yaml:"on_exhaust,omitempty"`
}

// ResolvedStep is the concrete outcome of advancing a Sequence: either a
// success (Error == nil, use Content) or a pre-content failure (Error set).
type ResolvedStep struct {
	Content string
	Error   *ErrorInjection // nil for success steps
}

// HTTPStatus reports the HTTP status this step serves: 200 for a success
// step, or the configured status for an error step. Derived from Error
// rather than stored separately, so there is exactly one source of truth for
// an error step's status.
func (r ResolvedStep) HTTPStatus() int {
	if r.Error != nil {
		return r.Error.Status
	}
	return 200
}

// Sequence is the protocol-neutral engine behind the per-protocol
// SequenceModel wrappers. It owns the expanded step program and an atomic
// cursor so concurrent requests each grab a distinct, monotonically advancing
// step without locking.
type Sequence struct {
	flat           []ResolvedStep
	defaultContent string
	onExhaust      ExhaustPolicy
	exhausted      ResolvedStep // served by Next when onExhaust == ExhaustFail
	cursor         atomic.Uint64
}

// NewSequence flattens cfg.Steps (expanding Repeat) and pre-resolves each step
// so Next() is allocation-free on the hot path. A config with no steps yields
// a single success step backed by DefaultContent, so the model is always
// usable.
func NewSequence(cfg SequenceConfig) *Sequence {
	s := &Sequence{
		defaultContent: cfg.DefaultContent,
		onExhaust:      cfg.OnExhaust,
		exhausted:      exhaustedStep(),
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
		s.flat = []ResolvedStep{{Content: cfg.DefaultContent}}
	}
	return s
}

// Next atomically advances the cursor and returns the step for this request.
// It is safe for concurrent use; each caller observes a distinct cursor value.
// Behaviour past the end of the program is governed by OnExhaust.
func (s *Sequence) Next() ResolvedStep {
	n := s.cursor.Add(1) - 1
	if n >= uint64(len(s.flat)) {
		switch s.onExhaust {
		case ExhaustClamp:
			return s.flat[len(s.flat)-1]
		case ExhaustFail:
			return s.exhausted
		}
		// ExhaustLoop (default): fall through to modulo wrap-around.
	}
	return s.flat[int(n%uint64(len(s.flat)))]
}

// exhaustedStep is the terminal error served once an ExhaustFail program is
// consumed: a non-retryable HTTP 410 with a dedicated type so callers can tell
// "the script is over" apart from a scripted in-band failure.
func exhaustedStep() ResolvedStep {
	return ResolvedStep{
		Error: &ErrorInjection{
			Stage:   ErrorStagePreContent,
			Status:  410,
			Message: "sequence exhausted",
			Type:    "sequence_exhausted",
		},
	}
}

// Len reports the number of (post-expansion) steps in the program.
func (s *Sequence) Len() int { return len(s.flat) }

func (s *Sequence) resolve(step SequenceStep) ResolvedStep {
	if step.Status == 0 || step.Status == 200 {
		content := step.Content
		if content == "" {
			content = s.defaultContent
		}
		if content == "" {
			content = FallbackSequenceContent
		}
		return ResolvedStep{Content: content}
	}
	typ, msg := step.ErrorType, step.ErrorMessage
	dtyp, dmsg := defaultErrorMeta(step.Status)
	if typ == "" {
		typ = dtyp
	}
	if msg == "" {
		msg = dmsg
	}
	return ResolvedStep{
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
			Steps:       Steps(200, 200, 429),
		},
	}
}
