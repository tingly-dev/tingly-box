package script

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// checkStep runs the step's expect and the expect_result of every call
// whose result r brings for the first time, then its captures; mu held.
func (e *Engine) checkStep(ts *trackState, st *Step, r *request, seq int) []Check {
	var checks []Check
	results := r.toolResults()
	for id, b := range results {
		em, ok := e.emitted[id]
		if !ok || e.checked[id] || em.track != ts.name {
			continue
		}
		e.checked[id] = true
		resp := ts.responses[em.resp]
		if resp.step < 0 {
			continue
		}
		for j, c := range resp.reply.Calls {
			if c.ID != id {
				continue
			}
			call := ts.t.Steps[resp.step].Reply.ToolCalls[j]
			if call.ExpectResult == nil {
				continue
			}
			ok, why := call.ExpectResult.eval(b)
			on := "expect_result " + c.Ref
			checks = append(checks, Check{On: on, OK: ok, Detail: why})
			if !ok {
				e.fail(Issue{Kind: IssueExpectResult, Track: ts.name, Step: ts.t.Steps[resp.step].ID, Seq: seq, Detail: "call " + call.ID + ": " + why})
			}
		}
	}
	if st.Expect != nil {
		ok, why := st.Expect.eval(r, e.resolver(ts))
		checks = append(checks, Check{On: "expect", OK: ok, Detail: why})
		if !ok {
			e.fail(Issue{Kind: IssueExpect, Track: ts.name, Step: st.ID, Seq: seq, Detail: why})
		}
	}
	e.capture(ts, st.ID, st.Capture, r, seq)
	return checks
}

func (e *Engine) capture(ts *trackState, step string, ex map[string]Extract, r *request, seq int) {
	for name, x := range ex {
		var src string
		switch {
		case x.From == "system":
			src = r.System
		case x.From == "first_user":
			src = r.first("user").text()
		case x.From == "last_user":
			src = r.last("user").text()
		case x.From == "last_text":
			src = r.last("user").lastText()
		case strings.HasPrefix(x.From, "header:"):
			src = r.Header.Get(strings.TrimPrefix(x.From, "header:"))
		case strings.HasPrefix(x.From, "tool_result:"):
			id := e.resolver(ts)(strings.TrimPrefix(x.From, "tool_result:"))
			if b, ok := r.toolResults()[id]; ok && id != "" {
				src = b.resultText()
			}
		}
		m := re(x.Regex).FindStringSubmatch(src)
		if m == nil {
			e.fail(Issue{Kind: IssueCapture, Track: ts.name, Step: step, Seq: seq, Detail: fmt.Sprintf("%s: %q found nothing in %s", name, x.Regex, x.From)})
			continue
		}
		v := m[0]
		if len(m) > 1 {
			v = m[1]
		}
		ts.vars[name] = v
	}
}

var templateRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)?)\s*\}\}`)

// fill replaces {{var}} and {{track.var}}; a missing value is a failure and
// the placeholder stays, so the reply shows what was missing.
func (e *Engine) fill(ts *trackState, s, step string, seq int) string {
	return templateRe.ReplaceAllStringFunc(s, func(m string) string {
		key := templateRe.FindStringSubmatch(m)[1]
		vars := ts.vars
		name := key
		if t, v, ok := strings.Cut(key, "."); ok {
			other := e.tracks[t]
			if other == nil {
				e.fail(Issue{Kind: IssueTemplate, Track: ts.name, Step: step, Seq: seq, Detail: "no track " + t + " for " + m})
				return m
			}
			vars, name = other.vars, v
		}
		v, ok := vars[name]
		if !ok {
			e.fail(Issue{Kind: IssueTemplate, Track: ts.name, Step: step, Seq: seq, Detail: "no value for " + m})
			return m
		}
		return v
	})
}

func (e *Engine) fillValue(ts *trackState, v any, step string, seq int) any {
	switch x := v.(type) {
	case string:
		return e.fill(ts, x, step, seq)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = e.fillValue(ts, val, step, seq)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = e.fillValue(ts, val, step, seq)
		}
		return out
	}
	return v
}

// renderStep turns the step's reply into wire form, indexes what it emits
// and readies the tracks it spawns; mu held.
func (e *Engine) renderStep(ts *trackState, step int, seq int) *rendered {
	st := ts.t.Steps[step]
	resp := len(ts.responses)
	out := e.render(&st.Reply, func(s string) string { return e.fill(ts, s, st.ID, seq) },
		func(v any) any { return e.fillValue(ts, v, st.ID, seq) }, st.ID, e.id, ts.name, fmt.Sprint(resp))
	ts.responses = append(ts.responses, &response{step: step, reply: out})
	em := emission{track: ts.name, resp: resp}
	for j, c := range out.Calls {
		e.emitted[c.ID] = em
		call := st.Reply.ToolCalls[j]
		if call.Spawns != "" {
			child := e.tracks[call.Spawns]
			child.spawnReady = true
			child.spawnHints = nil
			var args map[string]any
			_ = json.Unmarshal(c.Input, &args)
			for _, k := range []string{"title", "prompt", "description"} {
				if s, ok := args[k].(string); ok && s != "" {
					child.spawnHints = append(child.spawnHints, s)
				}
			}
		}
	}
	if out.Text != "" {
		e.texts[out.Text] = append(e.texts[out.Text], em)
	}
	return out
}

func (e *Engine) renderBackground(b *Background) *rendered {
	n := fmt.Sprint(e.bgCount[b.ID])
	id := func(s string) string { return s }
	val := func(v any) any { return v }
	return e.render(&b.Reply, id, val, b.ID, e.id, "background", b.ID, n)
}

func (e *Engine) render(r *Reply, text func(string) string, value func(any) any, stepID string, idParts ...string) *rendered {
	if r.Error != nil {
		return &rendered{Error: errorFor(r.Error.Status, r.Error.Type, r.Error.Message)}
	}
	out := &rendered{Text: text(r.Content), StopReason: r.StopReason, MessageID: wireID("msg_", idParts...)}
	if r.Usage != nil {
		out.Usage = *r.Usage
	}
	if len(r.StreamChunks) > 0 {
		for _, c := range r.StreamChunks {
			out.Chunks = append(out.Chunks, text(c))
		}
	}
	for j, c := range r.ToolCalls {
		args := map[string]any{}
		if c.Arguments != nil {
			args = value(c.Arguments).(map[string]any)
		}
		input, _ := json.Marshal(args)
		out.Calls = append(out.Calls, renderedCall{
			Ref: stepID + "." + c.ID, Name: c.Name, Input: input,
			ID: wireID("toolu_", append(append([]string{}, idParts...), fmt.Sprint(j))...),
		})
	}
	return out
}
