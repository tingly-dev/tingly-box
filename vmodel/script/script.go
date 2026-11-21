// Package script is the scripted-conversation behaviour of vmodel: a scene of
// predefined replies that real agent clients (Claude Code, the Projects
// coordinator and its threads, their subagents) run against, many at once.
//
// A script is a set of tracks, one per conversation. A request is routed to
// the track whose earlier reply it carries (the tool_use ids and texts we
// emitted come back verbatim in the client's history), so concurrent
// conversations never steal each other's steps; a new conversation claims a
// track by a declared selector. Each step answers one request and may assert
// what that request and the results of its tool calls contain. Anything no
// track or background rule expects fails the run. See
// .design/vmodel-script.md.
package script

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// Script is one scene.
type Script struct {
	// Strict fails requests nothing expects (the default). Off, they get the
	// Unmatched reply and a warning, for writing a script against a client.
	Strict *bool `json:"strict,omitempty"`
	// HoldTimeout bounds how long a step waits for its after conditions.
	HoldTimeout Duration `json:"hold_timeout,omitempty"`
	// CountTokens answers /v1/messages/count_tokens (default 100).
	CountTokens int `json:"count_tokens,omitempty"`
	// Unmatched is what a non-strict script answers to an unexpected request.
	Unmatched *Reply `json:"unmatched,omitempty"`
	// Background answers requests that belong to no track (titles,
	// classifiers) without using a step.
	Background []Background      `json:"background,omitempty"`
	Tracks     map[string]*Track `json:"tracks"`
}

// Track is one conversation.
type Track struct {
	// Bind claims a new conversation (a request with none of our replies in
	// it). A track some call spawns needs none: it binds to the request that
	// carries the call's title/prompt/description.
	Bind *Match `json:"bind,omitempty"`
	// Capture reads values from the binding request for {{var}} templates.
	Capture map[string]Extract `json:"capture,omitempty"`
	Steps   []*Step            `json:"steps"`
}

// Step answers one request of its track.
type Step struct {
	ID string `json:"id,omitempty"`
	// When guards the step: a request that does not match skips an optional
	// step and is a mismatch for any other.
	When     *Match `json:"when,omitempty"`
	Optional bool   `json:"optional,omitempty"`
	// After holds the reply until other steps were served ("track.step") or
	// the driver raised signals ("signal:name").
	After []string `json:"after,omitempty"`
	// Expect is checked on the request; a failure is reported and the step
	// still answers, so the run goes on and the trace stays whole.
	Expect  *Match             `json:"expect,omitempty"`
	Capture map[string]Extract `json:"capture,omitempty"`
	Reply   Reply              `json:"reply"`
}

// Reply is what a step answers. Field names follow the vmodel mock block.
type Reply struct {
	Content      string   `json:"content,omitempty"`
	ToolCalls    []*Call  `json:"tool_calls,omitempty"`
	StreamChunks []string `json:"stream_chunks,omitempty"`
	Delay        Duration `json:"delay,omitempty"`
	StopReason   string   `json:"stop_reason,omitempty"`
	Usage        *Usage   `json:"usage,omitempty"`
	// Error answers with an API error before any content instead.
	Error *vmodel.ErrorInjection `json:"error,omitempty"`
}

// Usage is the token usage a reply reports.
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// Call is one tool_use block of a reply.
type Call struct {
	// ID names the call for references (tool_result:step.call); the wire id
	// is derived from the script, track, step and call, so it is stable.
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	// Spawns is the track of the conversation this call starts (a thread,
	// a subagent).
	Spawns string `json:"spawns,omitempty"`
	// ExpectResult is checked on the request that brings the call's result.
	ExpectResult *ResultMatch `json:"expect_result,omitempty"`
}

// Background is a reply for requests outside every track.
type Background struct {
	ID    string `json:"id"`
	When  *Match `json:"when"`
	Reply Reply  `json:"reply"`
	Min   int    `json:"min,omitempty"`
	Max   int    `json:"max,omitempty"`
}

// Match is a predicate on a request. Every set field must hold.
type Match struct {
	Class             []string          `json:"class,omitempty"`
	Model             string            `json:"model,omitempty"`
	SystemContains    string            `json:"system_contains,omitempty"`
	SystemRegex       string            `json:"system_regex,omitempty"`
	FirstUserContains string            `json:"first_user_contains,omitempty"`
	LastUserContains  string            `json:"last_user_contains,omitempty"`
	LastUserRegex     string            `json:"last_user_regex,omitempty"`
	LastTextContains  string            `json:"last_text_contains,omitempty"`
	ToolsInclude      []string          `json:"tools_include,omitempty"`
	ToolsExclude      []string          `json:"tools_exclude,omitempty"`
	HasToolResult     string            `json:"has_tool_result,omitempty"`
	Header            map[string]string `json:"header,omitempty"`
	All               []*Match          `json:"all,omitempty"`
	Any               []*Match          `json:"any,omitempty"`
	Not               *Match            `json:"not,omitempty"`
}

// ResultMatch is a predicate on a tool result.
type ResultMatch struct {
	Contains string `json:"contains,omitempty"`
	Regex    string `json:"regex,omitempty"`
	IsError  *bool  `json:"is_error,omitempty"`
}

// Extract reads one value: the first group of Regex (or the whole match)
// in From: system, first_user, last_user, last_text, header:<name> or
// tool_result:<step>.<call>.
type Extract struct {
	From  string `json:"from"`
	Regex string `json:"regex"`
}

// Duration accepts "30s" or integer nanoseconds.
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		v, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		*d = Duration(v)
		return nil
	}
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("duration must be a string like 30s or nanoseconds")
	}
	*d = Duration(n)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

// Decode reads a script from JSON or YAML, rejecting unknown fields.
func Decode(data []byte) (*Script, error) {
	var s Script
	if err := vmodel.DecodeConfig(data, &s); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Script) strict() bool { return s.Strict == nil || *s.Strict }

// TrackNames are the tracks in a stable order.
func (s *Script) TrackNames() []string {
	names := make([]string, 0, len(s.Tracks))
	for n := range s.Tracks {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Validate checks references and selectors and fills step ids.
func (s *Script) Validate() error {
	if len(s.Tracks) == 0 {
		return fmt.Errorf("script needs at least one track")
	}
	spawned := map[string]string{}
	for _, name := range s.TrackNames() {
		t := s.Tracks[name]
		if t == nil || len(t.Steps) == 0 {
			return fmt.Errorf("track %s: needs steps", name)
		}
		if strings.ContainsAny(name, ". :") {
			return fmt.Errorf("track %s: names cannot contain '.', ':' or spaces", name)
		}
		seen := map[string]bool{}
		for i, st := range t.Steps {
			if st == nil {
				return fmt.Errorf("track %s: step %d is empty", name, i)
			}
			if st.ID == "" {
				st.ID = fmt.Sprint(i)
			}
			if seen[st.ID] {
				return fmt.Errorf("track %s: duplicate step %s", name, st.ID)
			}
			seen[st.ID] = true
			calls := map[string]bool{}
			for j, c := range st.Reply.ToolCalls {
				if c == nil || c.Name == "" {
					return fmt.Errorf("track %s step %s: tool call %d needs a name", name, st.ID, j)
				}
				if c.ID == "" {
					c.ID = fmt.Sprint(j)
				}
				if calls[c.ID] {
					return fmt.Errorf("track %s step %s: duplicate call %s", name, st.ID, c.ID)
				}
				calls[c.ID] = true
				if c.Spawns != "" {
					if _, ok := s.Tracks[c.Spawns]; !ok {
						return fmt.Errorf("track %s step %s: spawns unknown track %s", name, st.ID, c.Spawns)
					}
					if prev, ok := spawned[c.Spawns]; ok {
						return fmt.Errorf("track %s is spawned twice (%s and %s.%s)", c.Spawns, prev, name, st.ID)
					}
					spawned[c.Spawns] = name + "." + st.ID
				}
				if err := c.ExpectResult.compile(); err != nil {
					return fmt.Errorf("track %s step %s call %s: %w", name, st.ID, c.ID, err)
				}
			}
			if err := st.Reply.validate(); err != nil {
				return fmt.Errorf("track %s step %s: %w", name, st.ID, err)
			}
			for _, m := range []*Match{st.When, st.Expect} {
				if err := m.compile(); err != nil {
					return fmt.Errorf("track %s step %s: %w", name, st.ID, err)
				}
			}
			if err := compileExtracts(st.Capture); err != nil {
				return fmt.Errorf("track %s step %s: %w", name, st.ID, err)
			}
		}
		if err := t.Bind.compile(); err != nil {
			return fmt.Errorf("track %s bind: %w", name, err)
		}
		if err := compileExtracts(t.Capture); err != nil {
			return fmt.Errorf("track %s: %w", name, err)
		}
	}
	for _, name := range s.TrackNames() {
		if s.Tracks[name].Bind == nil && spawned[name] == "" {
			return fmt.Errorf("track %s needs bind (no call spawns it)", name)
		}
	}
	for _, name := range s.TrackNames() {
		for _, st := range s.Tracks[name].Steps {
			for _, a := range st.After {
				if strings.HasPrefix(a, "signal:") {
					if strings.TrimPrefix(a, "signal:") == "" {
						return fmt.Errorf("track %s step %s: empty signal", name, st.ID)
					}
					continue
				}
				if _, ok := s.step(a); !ok {
					return fmt.Errorf("track %s step %s: after %q is no step (track.step or signal:name)", name, st.ID, a)
				}
			}
		}
	}
	bg := map[string]bool{}
	for i := range s.Background {
		b := &s.Background[i]
		if b.ID == "" || bg[b.ID] {
			return fmt.Errorf("background %d needs a unique id", i)
		}
		bg[b.ID] = true
		if b.When == nil {
			return fmt.Errorf("background %s needs when", b.ID)
		}
		if err := b.When.compile(); err != nil {
			return fmt.Errorf("background %s: %w", b.ID, err)
		}
		if err := b.Reply.validate(); err != nil {
			return fmt.Errorf("background %s: %w", b.ID, err)
		}
	}
	if s.Unmatched != nil {
		if err := s.Unmatched.validate(); err != nil {
			return fmt.Errorf("unmatched: %w", err)
		}
	}
	return nil
}

// step finds "track.step".
func (s *Script) step(ref string) (*Step, bool) {
	track, id, ok := strings.Cut(ref, ".")
	if !ok {
		return nil, false
	}
	t := s.Tracks[track]
	if t == nil {
		return nil, false
	}
	for _, st := range t.Steps {
		if st.ID == id {
			return st, true
		}
	}
	return nil, false
}

func (r *Reply) validate() error {
	if r.Error != nil {
		if r.Content != "" || len(r.ToolCalls) > 0 {
			return fmt.Errorf("an error reply has no content or tool calls")
		}
		if r.Error.Stage != vmodel.ErrorStagePreContent {
			return fmt.Errorf("only pre-content errors are supported")
		}
		return nil
	}
	if r.Content == "" && len(r.ToolCalls) == 0 {
		return fmt.Errorf("reply needs content, tool_calls or error")
	}
	if len(r.StreamChunks) > 0 && strings.Join(r.StreamChunks, "") != r.Content {
		return fmt.Errorf("stream_chunks must join to content")
	}
	return nil
}

var (
	regexMu    sync.Mutex
	regexCache = map[string]*regexp.Regexp{}
)

func compileRegex(expr string) error {
	if expr == "" {
		return nil
	}
	regexMu.Lock()
	defer regexMu.Unlock()
	if _, ok := regexCache[expr]; ok {
		return nil
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return err
	}
	regexCache[expr] = re
	return nil
}

// re returns a regex compiled by Validate.
func re(expr string) *regexp.Regexp {
	regexMu.Lock()
	defer regexMu.Unlock()
	if r, ok := regexCache[expr]; ok {
		return r
	}
	r := regexp.MustCompile(expr)
	regexCache[expr] = r
	return r
}

func (m *Match) compile() error {
	if m == nil {
		return nil
	}
	for _, c := range m.Class {
		switch c {
		case ClassMain, ClassSubagent, ClassAuxiliary:
		default:
			return fmt.Errorf("unknown class %q (main, subagent, auxiliary)", c)
		}
	}
	for _, e := range []string{m.Model, m.SystemRegex, m.LastUserRegex} {
		if err := compileRegex(e); err != nil {
			return err
		}
	}
	for _, e := range m.Header {
		if err := compileRegex(e); err != nil {
			return err
		}
	}
	for _, sub := range append(append([]*Match{}, m.All...), m.Any...) {
		if err := sub.compile(); err != nil {
			return err
		}
	}
	return m.Not.compile()
}

func (r *ResultMatch) compile() error {
	if r == nil {
		return nil
	}
	return compileRegex(r.Regex)
}

func compileExtracts(ex map[string]Extract) error {
	for name, e := range ex {
		if e.Regex == "" {
			return fmt.Errorf("capture %s needs regex", name)
		}
		switch {
		case e.From == "system", e.From == "first_user", e.From == "last_user", e.From == "last_text",
			strings.HasPrefix(e.From, "header:"), strings.HasPrefix(e.From, "tool_result:"):
		default:
			return fmt.Errorf("capture %s: unknown from %q", name, e.From)
		}
		if err := compileRegex(e.Regex); err != nil {
			return fmt.Errorf("capture %s: %w", name, err)
		}
	}
	return nil
}
