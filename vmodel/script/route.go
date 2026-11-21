package script

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ServeHTTP answers Anthropic Messages requests (and count_tokens) from the
// script; HEAD probes get 200, anything else 404.
func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimSuffix(req.URL.Path, "/")
	switch {
	case req.Method == http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/v1/messages/count_tokens"):
		n := e.s.CountTokens
		if n == 0 {
			n = 100
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int{"input_tokens": n})
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/v1/messages"):
		e.serveMessages(w, req)
	default:
		writeError(w, errorFor(404, "", "vmodel script: no route "+req.Method+" "+req.URL.Path))
	}
}

// decision is where routing put a request.
type decision struct {
	via    string
	ts     *trackState
	step   int       // a claimed step, or -1
	replay *rendered // a retry: answer what was answered before
	bg     *Background
	err    *apiError
}

func (e *Engine) serveMessages(w http.ResponseWriter, hr *http.Request) {
	body, err := io.ReadAll(hr.Body)
	if err != nil {
		writeError(w, errorFor(400, "", "read body: "+err.Error()))
		return
	}
	r, err := parseRequest(body, hr.Header)
	if err != nil {
		writeError(w, errorFor(400, "", err.Error()))
		return
	}

	e.mu.Lock()
	e.seq++
	entry := Entry{Seq: e.seq, Class: r.class(), Model: r.Model, Session: r.Session}
	var d decision
	if e.finished {
		e.fail(Issue{Kind: IssueFinished, Seq: entry.Seq, Detail: "request after the script finished"})
		d = decision{via: "unmatched", step: -1, err: errorFor(400, "", "vmodel script: the script has finished")}
	} else {
		d = e.route(r, entry.Seq)
	}
	if d.ts != nil {
		entry.Track = d.ts.name
		if d.step >= 0 && d.err == nil {
			e.reached[d.ts.name+"."+d.ts.t.Steps[d.step].ID] = true
			e.notify()
		}
	}
	entry.Via = d.via
	e.mu.Unlock()

	var reply *rendered
	switch {
	case d.err != nil:
		reply = &rendered{Error: d.err}
	case d.replay != nil:
		reply = d.replay
	case d.bg != nil:
		e.mu.Lock()
		reply = e.renderBackground(d.bg)
		e.mu.Unlock()
	default:
		st := d.ts.t.Steps[d.step]
		entry.Step = st.ID
		start := time.Now()
		if missing := e.waitAfter(hr.Context(), st.After); len(missing) > 0 {
			e.mu.Lock()
			e.fail(Issue{Kind: IssueAfterTimeout, Track: d.ts.name, Step: st.ID, Seq: entry.Seq, Detail: "still waiting for " + strings.Join(missing, ", ")})
			e.mu.Unlock()
			reply = &rendered{Error: errorFor(529, "", "vmodel script: step "+d.ts.name+"."+st.ID+" waited too long for "+strings.Join(missing, ", "))}
			break
		}
		entry.HeldMS = time.Since(start).Milliseconds()
		e.mu.Lock()
		entry.Checks = e.checkStep(d.ts, st, r, entry.Seq)
		reply = e.renderStep(d.ts, d.step, entry.Seq)
		e.mu.Unlock()
		if d := time.Duration(st.Reply.Delay); d > 0 {
			time.Sleep(d)
		}
	}
	reply.write(w, r.Model, r.Stream)

	e.mu.Lock()
	defer e.mu.Unlock()
	if reply.Error != nil {
		entry.Status, entry.Error = reply.Error.Status, reply.Error.Message
	} else {
		entry.Status = 200
	}
	entry.Text = clip(reply.Text, 120)
	for _, c := range reply.Calls {
		entry.Tools = append(entry.Tools, c.Name)
	}
	if d.ts != nil && d.step >= 0 && d.err == nil {
		e.served[d.ts.name+"."+d.ts.t.Steps[d.step].ID] = true
	}
	e.trace = append(e.trace, entry)
	e.notify()
}

// route decides which track, step or rule answers r; call with mu held.
func (e *Engine) route(r *request, seq int) decision {
	class := r.class()
	if a := r.last("assistant"); a != nil {
		if em, ok := e.lookup(a, r.Session, class); ok {
			ts := e.tracks[em.track]
			if ts.class == class {
				if em.resp < len(ts.responses)-1 {
					next := ts.responses[em.resp+1]
					e.warn(Issue{Kind: IssueRetry, Track: ts.name, Seq: seq, Detail: "the request repeats one already answered; answered the same again"})
					return decision{via: "retry", ts: ts, step: -1, replay: next.reply}
				}
				return e.next(ts, r, "continuation", seq)
			}
		}
	} else {
		fp := r.fingerprint()
		for _, name := range e.s.TrackNames() {
			ts := e.tracks[name]
			if ts.bound && ts.class == class && ts.firstFP == fp && len(ts.responses) > 0 {
				e.warn(Issue{Kind: IssueRetry, Track: ts.name, Seq: seq, Detail: "the first request of the conversation came again; answered the same again"})
				return decision{via: "retry", ts: ts, step: -1, replay: ts.responses[0].reply}
			}
		}
	}

	var candidates []*trackState
	var nearest []string
	for _, name := range e.s.TrackNames() {
		ts := e.tracks[name]
		if ok, why := e.binds(ts, r); ok {
			candidates = append(candidates, ts)
		} else if !ts.bound && why != "" {
			nearest = append(nearest, name+" ("+why+")")
		}
	}
	switch len(candidates) {
	case 1:
		ts := candidates[0]
		e.bind(ts, r, seq)
		return e.next(ts, r, "bind", seq)
	case 0:
	default:
		var names []string
		for _, ts := range candidates {
			names = append(names, ts.name)
		}
		detail := "a new conversation matches tracks " + strings.Join(names, ", ")
		e.fail(Issue{Kind: IssueAmbiguous, Seq: seq, Detail: detail})
		return decision{via: "unmatched", step: -1, err: errorFor(400, "", "vmodel script: "+detail)}
	}

	for i := range e.s.Background {
		b := &e.s.Background[i]
		if ok, _ := b.When.eval(r, e.resolver(nil)); ok {
			e.bgCount[b.ID]++
			return decision{via: "background", step: -1, bg: b}
		}
	}

	detail := fmt.Sprintf("no track expects this %s request", class)
	if len(nearest) > 0 {
		detail += "; unbound: " + strings.Join(nearest, "; ")
	}
	if last := r.last("user"); last != nil {
		detail += "; last user text: " + clip(last.lastText(), 160)
	}
	if !e.s.strict() {
		e.warn(Issue{Kind: IssueUnmatched, Seq: seq, Detail: detail})
		u := e.s.Unmatched
		if u == nil {
			u = &Reply{Content: "ok"}
		}
		return decision{via: "unmatched", step: -1, bg: &Background{ID: "unmatched", Reply: *u}}
	}
	e.fail(Issue{Kind: IssueUnmatched, Seq: seq, Detail: detail})
	return decision{via: "unmatched", step: -1, err: errorFor(400, "", "vmodel script: "+detail)}
}

// lookup finds the reply an assistant message repeats: by tool_use id, else
// by text (several replies with the same text: the one of this session and
// class; a subagent shares its parent's session).
func (e *Engine) lookup(a *message, session, class string) (emission, bool) {
	for _, b := range a.Blocks {
		if b.Type == "tool_use" {
			if em, ok := e.emitted[b.ID]; ok {
				return em, true
			}
		}
	}
	ems := e.texts[a.text()]
	if len(ems) == 0 {
		return emission{}, false
	}
	for _, em := range ems {
		if ts := e.tracks[em.track]; ts.session == session && ts.class == class {
			return em, true
		}
	}
	return ems[len(ems)-1], true
}

// binds reports whether ts can claim r as its new conversation.
func (e *Engine) binds(ts *trackState, r *request) (bool, string) {
	if ts.bound {
		return false, ""
	}
	if ts.spawnedBy != "" {
		if !ts.spawnReady {
			return false, "waiting for " + ts.spawnedBy + " to start it"
		}
		hay := r.System + "\n" + r.first("user").text()
		found := len(ts.spawnHints) == 0
		for _, h := range ts.spawnHints {
			found = found || strings.Contains(hay, h)
		}
		if !found {
			return false, "the request carries none of " + strings.Join(ts.spawnHints, " / ")
		}
	}
	return ts.t.Bind.eval(r, e.resolver(ts))
}

func (e *Engine) bind(ts *trackState, r *request, seq int) {
	ts.bound, ts.class, ts.session = true, r.class(), r.Session
	ts.firstFP = r.fingerprint()
	e.capture(ts, "", ts.t.Capture, r, seq)
}

// next claims the track's next step for r.
func (e *Engine) next(ts *trackState, r *request, via string, seq int) decision {
	for ts.cursor < len(ts.t.Steps) {
		st := ts.t.Steps[ts.cursor]
		if ok, why := st.When.eval(r, e.resolver(ts)); !ok {
			if st.Optional {
				ts.cursor++
				continue
			}
			e.fail(Issue{Kind: IssueMismatch, Track: ts.name, Step: st.ID, Seq: seq, Detail: why})
			return decision{via: via, ts: ts, step: -1, err: errorFor(400, "", "vmodel script: "+ts.name+"."+st.ID+": "+why)}
		}
		ts.cursor++
		return decision{via: via, ts: ts, step: ts.cursor - 1}
	}
	detail := "no step left"
	if n := len(ts.t.Steps); n > 0 {
		detail += " after " + ts.t.Steps[n-1].ID
	}
	if last := r.last("user"); last != nil {
		detail += "; last user text: " + clip(last.lastText(), 160)
	}
	e.fail(Issue{Kind: IssueExhausted, Track: ts.name, Seq: seq, Detail: detail})
	return decision{via: via, ts: ts, step: -1, err: errorFor(400, "", "vmodel script: track "+ts.name+": "+detail)}
}

// resolver maps "step.call" (in ts) or "track.step.call" to a wire id.
func (e *Engine) resolver(ts *trackState) func(string) string {
	return func(ref string) string {
		parts := strings.Split(ref, ".")
		t := ts
		if len(parts) == 3 {
			t = e.tracks[parts[0]]
			parts = parts[1:]
		}
		if t == nil || len(parts) != 2 {
			return ""
		}
		for i, st := range t.t.Steps {
			if st.ID != parts[0] {
				continue
			}
			for j, c := range st.Reply.ToolCalls {
				if c.ID == parts[1] {
					return e.callID(t, i, j)
				}
			}
		}
		return ""
	}
}
