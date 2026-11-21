package script

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Request classes: the main loop of a conversation, a subagent, or an
// auxiliary request of the client (titles, classifiers: no tools).
const (
	ClassMain      = "main"
	ClassSubagent  = "subagent"
	ClassAuxiliary = "auxiliary"
)

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type message struct {
	Role   string
	Blocks []block
}

// text joins the message's text blocks.
func (m *message) text() string {
	if m == nil {
		return ""
	}
	var parts []string
	for _, b := range m.Blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// lastText is the message's last text block.
func (m *message) lastText() string {
	if m == nil {
		return ""
	}
	for i := len(m.Blocks) - 1; i >= 0; i-- {
		if m.Blocks[i].Type == "text" {
			return m.Blocks[i].Text
		}
	}
	return ""
}

// request is what routing and matching read of a Messages request.
type request struct {
	Model    string
	Stream   bool
	System   string
	Tools    []string
	Messages []message // role "system" entries left out
	// Inserted are the role "system" entries after the last assistant
	// message: Claude Code 2.1.259 sends a message the user wrote mid-turn
	// there ("The user sent a new message while you were working: ...").
	Inserted []string
	Header   http.Header
	Session  string
}

func parseRequest(body []byte, h http.Header) (*request, error) {
	var raw struct {
		Model    string                  `json:"model"`
		Stream   bool                    `json:"stream"`
		System   json.RawMessage         `json:"system"`
		Tools    []struct{ Name string } `json:"tools"`
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("request must be a JSON object: %w", err)
	}
	r := &request{Model: raw.Model, Stream: raw.Stream, Header: h}
	sys, err := blocksOf(raw.System)
	if err != nil {
		return nil, fmt.Errorf("system: %w", err)
	}
	r.System = (&message{Blocks: sys}).text()
	for _, t := range raw.Tools {
		r.Tools = append(r.Tools, t.Name)
	}
	for i, m := range raw.Messages {
		bs, err := blocksOf(m.Content)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", i, err)
		}
		switch m.Role {
		case "system":
			// Claude Code 2.1.259 interleaves these: skills, budgets and
			// messages the user wrote mid-turn.
			r.Inserted = append(r.Inserted, (&message{Blocks: bs}).text())
		case "assistant":
			r.Inserted = nil
			fallthrough
		default:
			r.Messages = append(r.Messages, message{Role: m.Role, Blocks: bs})
		}
	}
	r.Session = h.Get("X-Claude-Code-Session-Id")
	if r.Session == "" && raw.Metadata.UserID != "" {
		var uid struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal([]byte(raw.Metadata.UserID), &uid) == nil {
			r.Session = uid.SessionID
		}
	}
	return r, nil
}

// blocksOf reads a string or a list of content blocks.
func blocksOf(raw json.RawMessage) ([]block, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []block{{Type: "text", Text: s}}, nil
	}
	var bs []block
	if err := json.Unmarshal(raw, &bs); err != nil {
		return nil, err
	}
	return bs, nil
}

// resultText is the text of a tool_result's content.
func (b block) resultText() string {
	bs, err := blocksOf(b.Content)
	if err != nil {
		return string(b.Content)
	}
	return (&message{Blocks: bs}).text()
}

func (r *request) class() string {
	switch c := r.Header.Get("X-Claude-Code-Request-Class"); c {
	case ClassMain, ClassSubagent:
		return c
	case "":
	default:
		return ClassAuxiliary
	}
	if strings.Contains(r.System, "cc_is_subagent=true") {
		return ClassSubagent
	}
	if len(r.Tools) == 0 {
		return ClassAuxiliary
	}
	return ClassMain
}

// fingerprint tells a repeated first request of a conversation from the
// first request of another one: same session, system, tools and message.
func (r *request) fingerprint() string {
	return wireID("", r.Session, r.System, strings.Join(r.Tools, ","), r.first("user").text())
}

func (r *request) first(role string) *message {
	for i := range r.Messages {
		if r.Messages[i].Role == role {
			return &r.Messages[i]
		}
	}
	return nil
}

func (r *request) last(role string) *message {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == role {
			return &r.Messages[i]
		}
	}
	return nil
}

// toolResults are every tool_result block of the request, by tool_use id.
func (r *request) toolResults() map[string]block {
	out := map[string]block{}
	for _, m := range r.Messages {
		for _, b := range m.Blocks {
			if b.Type == "tool_result" {
				out[b.ToolUseID] = b
			}
		}
	}
	return out
}

func nameMatches(pattern, name string) bool {
	switch {
	case strings.HasPrefix(pattern, "__"):
		return strings.HasSuffix(name, pattern)
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == name
}

func (r *request) hasTool(pattern string) bool {
	for _, t := range r.Tools {
		if nameMatches(pattern, t) {
			return true
		}
	}
	return false
}

// eval reports whether the request matches, and if not, the first reason.
// resolve maps a call reference to the wire id we gave it.
func (m *Match) eval(r *request, resolve func(string) string) (bool, string) {
	if m == nil {
		return true, ""
	}
	if len(m.Class) > 0 {
		c := r.class()
		ok := false
		for _, want := range m.Class {
			ok = ok || want == c
		}
		if !ok {
			return false, fmt.Sprintf("class is %s, want %v", c, m.Class)
		}
	}
	if m.Model != "" && !re(m.Model).MatchString(r.Model) {
		return false, fmt.Sprintf("model %q does not match %q", r.Model, m.Model)
	}
	if m.SystemContains != "" && !strings.Contains(r.System, m.SystemContains) {
		return false, fmt.Sprintf("system lacks %q", m.SystemContains)
	}
	if m.SystemRegex != "" && !re(m.SystemRegex).MatchString(r.System) {
		return false, fmt.Sprintf("system does not match %q", m.SystemRegex)
	}
	if m.FirstUserContains != "" && !strings.Contains(r.first("user").text(), m.FirstUserContains) {
		return false, fmt.Sprintf("first user message lacks %q", m.FirstUserContains)
	}
	if m.LastUserContains != "" && !strings.Contains(r.last("user").text(), m.LastUserContains) {
		return false, fmt.Sprintf("last user message lacks %q", m.LastUserContains)
	}
	if m.LastUserRegex != "" && !re(m.LastUserRegex).MatchString(r.last("user").text()) {
		return false, fmt.Sprintf("last user message does not match %q", m.LastUserRegex)
	}
	if m.LastTextContains != "" && !strings.Contains(r.last("user").lastText(), m.LastTextContains) {
		return false, fmt.Sprintf("last text block lacks %q", m.LastTextContains)
	}
	if m.InsertedContains != "" && !strings.Contains(strings.Join(r.Inserted, "\n"), m.InsertedContains) {
		return false, fmt.Sprintf("no message inserted since the last reply contains %q", m.InsertedContains)
	}
	for _, t := range m.ToolsInclude {
		if !r.hasTool(t) {
			return false, fmt.Sprintf("tools lack %s", t)
		}
	}
	for _, t := range m.ToolsExclude {
		if r.hasTool(t) {
			return false, fmt.Sprintf("tools include %s", t)
		}
	}
	if m.HasToolResult != "" {
		id := resolve(m.HasToolResult)
		if _, ok := r.toolResults()[id]; id == "" || !ok {
			return false, fmt.Sprintf("no result of %s", m.HasToolResult)
		}
	}
	for name, expr := range m.Header {
		if !re(expr).MatchString(r.Header.Get(name)) {
			return false, fmt.Sprintf("header %s %q does not match %q", name, r.Header.Get(name), expr)
		}
	}
	for _, sub := range m.All {
		if ok, why := sub.eval(r, resolve); !ok {
			return false, why
		}
	}
	if len(m.Any) > 0 {
		var whys []string
		ok := false
		for _, sub := range m.Any {
			if o, why := sub.eval(r, resolve); o {
				ok = true
				break
			} else {
				whys = append(whys, why)
			}
		}
		if !ok {
			return false, "none of any: " + strings.Join(whys, "; ")
		}
	}
	if m.Not != nil {
		if ok, _ := m.Not.eval(r, resolve); ok {
			return false, "not matched"
		}
	}
	return true, ""
}

func (m *ResultMatch) eval(b block) (bool, string) {
	text := b.resultText()
	if m.Contains != "" && !strings.Contains(text, m.Contains) {
		return false, fmt.Sprintf("result lacks %q: %s", m.Contains, clip(text, 200))
	}
	if m.Regex != "" && !re(m.Regex).MatchString(text) {
		return false, fmt.Sprintf("result does not match %q: %s", m.Regex, clip(text, 200))
	}
	if m.IsError != nil && b.IsError != *m.IsError {
		return false, fmt.Sprintf("is_error is %v: %s", b.IsError, clip(text, 200))
	}
	return true, ""
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
