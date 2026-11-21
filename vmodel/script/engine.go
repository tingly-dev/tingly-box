package script

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Engine runs one script: it routes requests to tracks and keeps what
// happened. One engine is one run; build a new one to run again.
type Engine struct {
	id string
	s  *Script

	mu       sync.Mutex
	changed  chan struct{} // closed and replaced on every state change
	tracks   map[string]*trackState
	emitted  map[string]emission   // wire tool_use id → the reply that carried it
	texts    map[string][]emission // reply text → the replies that carried it
	checked  map[string]bool       // tool_use ids whose expect_result ran
	signals  map[string]bool
	served   map[string]bool // "track.step" written to a client
	bgCount  map[string]int
	failures []Issue
	warnings []Issue
	trace    []Entry
	seq      int
	finished bool
}

type trackState struct {
	name    string
	t       *Track
	bound   bool
	class   string
	session string
	firstFP string // the binding request's first user message, to spot its retry
	vars    map[string]string
	cursor  int // index of the next step
	// responses are the replies served on this track, in order.
	responses []*response
	// A spawned track binds only after its parent call went out, to a request
	// carrying one of the call's hints.
	spawnedBy  string
	spawnReady bool
	spawnHints []string
}

type response struct {
	step  int // -1 for none
	reply *rendered
}

type emission struct {
	track string
	resp  int
}

// New builds an engine for a validated script; id names it in wire ids
// and reports.
func New(id string, s *Script) (*Engine, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	e := &Engine{
		id: id, s: s, changed: make(chan struct{}),
		tracks: map[string]*trackState{}, emitted: map[string]emission{}, texts: map[string][]emission{},
		checked: map[string]bool{}, signals: map[string]bool{}, served: map[string]bool{}, bgCount: map[string]int{},
	}
	for _, name := range s.TrackNames() {
		e.tracks[name] = &trackState{name: name, t: s.Tracks[name], vars: map[string]string{}}
	}
	for _, name := range s.TrackNames() {
		for _, st := range s.Tracks[name].Steps {
			for _, c := range st.Reply.ToolCalls {
				if c.Spawns != "" {
					e.tracks[c.Spawns].spawnedBy = name + "." + st.ID
				}
			}
		}
	}
	return e, nil
}

// ID is the script's id.
func (e *Engine) ID() string { return e.id }

// notify wakes waiters; call with mu held.
func (e *Engine) notify() {
	close(e.changed)
	e.changed = make(chan struct{})
}

func (e *Engine) fail(i Issue) { e.failures = append(e.failures, i) }
func (e *Engine) warn(i Issue) { e.warnings = append(e.warnings, i) }

// Signal raises a signal for steps held with after: signal:<name>.
func (e *Engine) Signal(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.signals[name] = true
	e.notify()
}

// WaitServed blocks until the step "track.step" was answered.
func (e *Engine) WaitServed(ctx context.Context, ref string) error {
	if _, ok := e.s.step(ref); !ok {
		return fmt.Errorf("no step %s", ref)
	}
	for {
		e.mu.Lock()
		done, ch := e.served[ref], e.changed
		e.mu.Unlock()
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("step %s not served: %w", ref, ctx.Err())
		case <-ch:
		}
	}
}

// waitAfter blocks until every after condition holds or the hold times out;
// it returns the conditions still missing.
func (e *Engine) waitAfter(ctx context.Context, after []string) []string {
	timeout := time.Duration(e.s.HoldTimeout)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		e.mu.Lock()
		var missing []string
		for _, a := range after {
			if len(a) > 7 && a[:7] == "signal:" {
				if !e.signals[a[7:]] {
					missing = append(missing, a)
				}
			} else if !e.served[a] {
				missing = append(missing, a)
			}
		}
		ch := e.changed
		e.mu.Unlock()
		if len(missing) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return missing
		case <-ch:
		}
	}
}

// Finish ends the run: later requests fail, steps never served and
// background rules outside their bounds become failures.
func (e *Engine) Finish() Report {
	e.mu.Lock()
	if !e.finished {
		e.finished = true
		for _, name := range e.s.TrackNames() {
			ts := e.tracks[name]
			for _, st := range ts.t.Steps[ts.cursor:] {
				if !st.Optional {
					e.fail(Issue{Kind: IssueUnconsumed, Track: name, Step: st.ID, Detail: e.stalledAt(ts)})
				}
			}
			for i, st := range ts.t.Steps[:ts.cursor] {
				for j, c := range st.Reply.ToolCalls {
					id := e.callID(ts, i, j)
					if c.ExpectResult != nil && id != "" && !e.checked[id] {
						e.fail(Issue{Kind: IssueResultMissed, Track: name, Step: st.ID, Detail: "no request brought the result of call " + c.ID})
					}
				}
			}
		}
		for _, b := range e.s.Background {
			n := e.bgCount[b.ID]
			if n < b.Min || (b.Max > 0 && n > b.Max) {
				e.fail(Issue{Kind: IssueBackground, Detail: fmt.Sprintf("%s ran %d times, want %d..%d", b.ID, n, b.Min, b.Max)})
			}
		}
		e.notify()
	}
	e.mu.Unlock()
	return e.Report()
}

func (e *Engine) stalledAt(ts *trackState) string {
	if !ts.bound {
		return "the track never bound to a conversation"
	}
	if ts.cursor == 0 {
		return "the conversation bound but no step was served"
	}
	return "the conversation stopped after step " + ts.t.Steps[ts.cursor-1].ID
}

// callID is the wire id the call got the last time its step was served.
func (e *Engine) callID(ts *trackState, step, call int) string {
	for i := len(ts.responses) - 1; i >= 0; i-- {
		r := ts.responses[i]
		if r.step == step && call < len(r.reply.Calls) {
			return r.reply.Calls[call].ID
		}
	}
	return ""
}

// Report is the run so far.
func (e *Engine) Report() Report {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := Report{Script: e.id, Finished: e.finished, Requests: e.seq,
		Failures: append([]Issue{}, e.failures...), Warnings: append([]Issue{}, e.warnings...), Background: map[string]int{}}
	for k, v := range e.bgCount {
		r.Background[k] = v
	}
	sortIssues(r.Failures)
	sortIssues(r.Warnings)
	for _, name := range e.s.TrackNames() {
		ts := e.tracks[name]
		tr := TrackReport{Name: name, Bound: ts.bound, Session: ts.session, Served: []string{}, Remaining: []string{}}
		for _, resp := range ts.responses {
			if resp.step >= 0 {
				tr.Served = append(tr.Served, ts.t.Steps[resp.step].ID)
			}
		}
		for _, st := range ts.t.Steps[ts.cursor:] {
			tr.Remaining = append(tr.Remaining, st.ID)
		}
		r.Tracks = append(r.Tracks, tr)
	}
	r.OK = len(r.Failures) == 0
	return r
}

// Trace is every request so far, in arrival order.
func (e *Engine) Trace() []Entry {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Entry{}, e.trace...)
}

// sortIssues orders by track and step, not arrival: arrival across tracks is
// not deterministic and the report should be.
func sortIssues(is []Issue) {
	sort.SliceStable(is, func(a, b int) bool {
		if is[a].Track != is[b].Track {
			return is[a].Track < is[b].Track
		}
		return is[a].Step < is[b].Step
	})
}
