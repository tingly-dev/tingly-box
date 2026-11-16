// Package limits implements the state behind routing limits: live in-flight
// counters per upstream account/service, sliding usage windows, and the
// all-or-nothing permits that tie both to a request's lifetime. It knows
// nothing about routing, gin or the database; see .design/routing-limits.md.
package limits

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/clock"
)

// State classifies how loaded a candidate is.
type State int

const (
	StateOK      State = iota // below every limit and every soft threshold
	StateHot                  // at/above a soft threshold, below every hard limit
	StateBlocked              // at/above a hard limit
)

func (s State) String() string {
	switch s {
	case StateHot:
		return "hot"
	case StateBlocked:
		return "blocked"
	default:
		return "ok"
	}
}

// Reason is a bit set of the limits that blocked a candidate.
type Reason uint8

const (
	ReasonConcurrency Reason = 1 << iota
	ReasonUsage
	ReasonQuota
)

func (r Reason) String() string {
	return joinFlags(uint8(r), []string{"concurrency", "usage", "quota"})
}

// joinFlags names the set bits of a flag value (bit i = names[i]), joined by "+".
func joinFlags(bits uint8, names []string) string {
	var out string
	for i, n := range names {
		if bits&(1<<i) == 0 {
			continue
		}
		if out != "" {
			out += "+"
		}
		out += n
	}
	return out
}

// Subject is one counted entity (an account or one rule's share of it) and the
// limits that apply to it.
type Subject struct {
	Key    string
	Limits ai.Limits
	// Quota is the provider UUID whose upstream quota Limits.QuotaPercent is
	// checked against. Empty = no quota check.
	Quota string
}

// Verdict is the result of assessing a set of subjects.
type Verdict struct {
	State   State
	Reasons Reason
	// Pressure is the largest used/limit ratio across all dimensions
	// (1.0 = at the hard limit; 0 when nothing is limited).
	Pressure float64
	// RetryAfter is a hint for how long until a blocked usage budget frees up
	// (or a short default for pure concurrency).
	RetryAfter time.Duration
}

// ConcurrencyOnly reports a blocked verdict that waiting could resolve within
// seconds: only in-flight caps are hit.
func (v Verdict) ConcurrencyOnly() bool {
	return v.State == StateBlocked && v.Reasons == ReasonConcurrency
}

// QuotaFunc returns the upstream quota used percentage (0-100) for a provider;
// ok=false means unknown, which never blocks.
type QuotaFunc func(providerUUID string) (pct float64, ok bool)

const (
	defaultMaxHold      = 2 * time.Hour
	sweepInterval       = 30 * time.Second
	defaultRetryAfter   = time.Second
	quotaCacheTTL       = 10 * time.Second
	maxWindowBuckets    = 60
	minWindowBucketSize = time.Second
)

type counter struct {
	inflight int
	win      *window
}

// Registry holds all counters. The zero value is not usable; use NewRegistry.
type Registry struct {
	mu       sync.Mutex
	counters map[string]*counter
	permits  map[*Permit]struct{}
	notify   chan struct{}
	maxHold  time.Duration

	quotaMu    sync.Mutex
	quota      QuotaFunc
	quotaCache map[string]quotaEntry
}

type quotaEntry struct {
	pct float64
	ok  bool
	at  time.Time
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   map[string]*counter{},
		permits:    map[*Permit]struct{}{},
		notify:     make(chan struct{}),
		maxHold:    defaultMaxHold,
		quotaCache: map[string]quotaEntry{},
	}
}

// SetQuotaFunc wires upstream quota lookups (optional). Lookups are cached for
// a few seconds so the request path never pays a store read per candidate.
func (r *Registry) SetQuotaFunc(fn QuotaFunc) {
	r.quotaMu.Lock()
	r.quota = fn
	r.quotaMu.Unlock()
}

// SetMaxHold overrides the leak-guard hold time (tests).
func (r *Registry) SetMaxHold(d time.Duration) {
	r.mu.Lock()
	r.maxHold = d
	r.mu.Unlock()
}

func (r *Registry) quotaPct(provider string) (float64, bool) {
	r.quotaMu.Lock()
	fn := r.quota
	if fn == nil {
		r.quotaMu.Unlock()
		return 0, false
	}
	now := clock.Now()
	if e, ok := r.quotaCache[provider]; ok && now.Sub(e.at) < quotaCacheTTL {
		r.quotaMu.Unlock()
		return e.pct, e.ok
	}
	r.quotaMu.Unlock()

	pct, ok := fn(provider) // outside any lock: may read a store
	r.quotaMu.Lock()
	r.quotaCache[provider] = quotaEntry{pct: pct, ok: ok, at: now}
	r.quotaMu.Unlock()
	return pct, ok
}

// resolveQuota pre-computes quota readings for the subjects that need them.
func (r *Registry) resolveQuota(subjects []Subject) []quotaEntry {
	var out []quotaEntry
	for i, s := range subjects {
		if s.Limits.QuotaPercent <= 0 || s.Quota == "" {
			continue
		}
		if out == nil {
			out = make([]quotaEntry, len(subjects))
		}
		pct, ok := r.quotaPct(s.Quota)
		out[i] = quotaEntry{pct: pct, ok: ok}
	}
	return out
}

func (r *Registry) counterLocked(key string) *counter {
	c := r.counters[key]
	if c == nil {
		c = &counter{}
		r.counters[key] = c
	}
	return c
}

// windowLocked returns the counter's window, (re)building it when the
// configured length changed.
func (c *counter) windowFor(l *ai.Limits) *window {
	want := l.Window()
	if want <= 0 {
		return nil
	}
	if c.win == nil || c.win.length != want {
		c.win = newWindow(want)
	}
	return c.win
}

// Assess classifies the subjects without consuming anything.
func (r *Registry) Assess(subjects []Subject) Verdict {
	q := r.resolveQuota(subjects)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.assessLocked(subjects, q, clock.Now())
}

func (r *Registry) assessLocked(subjects []Subject, quota []quotaEntry, now time.Time) Verdict {
	var v Verdict
	for i := range subjects {
		s := &subjects[i]
		l := &s.Limits
		ld := r.loadLocked(s, now)
		ratio := maxf(ld.conc, ld.usage)

		if ld.hasConc && ld.conc >= 1 {
			v.State, v.Reasons = StateBlocked, v.Reasons|ReasonConcurrency
			if v.RetryAfter < defaultRetryAfter {
				v.RetryAfter = defaultRetryAfter
			}
		}
		if ld.hasUsage && ld.usage >= 1 {
			v.State, v.Reasons = StateBlocked, v.Reasons|ReasonUsage
			if ld.win != nil {
				if ra := ld.win.freeIn(now); ra > v.RetryAfter {
					v.RetryAfter = ra
				}
			}
		}
		if quota != nil && quota[i].ok && l.QuotaPercent > 0 {
			rt := quota[i].pct / float64(l.QuotaPercent)
			if rt >= 1 {
				v.State, v.Reasons = StateBlocked, v.Reasons|ReasonQuota
				if v.RetryAfter < time.Minute {
					v.RetryAfter = time.Minute
				}
			}
			ratio = maxf(ratio, rt)
		}

		if l.SoftPercent > 0 && ratio*100 >= float64(l.SoftPercent) && v.State == StateOK {
			v.State = StateHot
		}
		v.Pressure = maxf(v.Pressure, ratio)
	}
	return v
}

// load is one subject's use of each limited dimension as a fraction of its hard
// limit (1 = at the limit). has* is false when the subject does not limit that
// dimension, so "unlimited" stays distinguishable from "idle".
type load struct {
	conc, usage       float64
	hasConc, hasUsage bool
	win               *window // the live window behind usage; nil when none yet
}

func (r *Registry) loadLocked(s *Subject, now time.Time) load {
	var ld load
	l := &s.Limits
	c := r.counters[s.Key]
	if l.MaxConcurrent > 0 {
		ld.hasConc = true
		if c != nil {
			ld.conc = float64(c.inflight) / float64(l.MaxConcurrent)
		}
	}
	if l.HasBudget() {
		ld.hasUsage = true
		if c != nil && c.win != nil && c.win.length == l.Window() {
			ld.win = c.win
			reqs, toks := ld.win.sums(now)
			if l.MaxRequests > 0 {
				ld.usage = float64(reqs) / float64(l.MaxRequests)
			}
			if l.MaxTokens > 0 {
				ld.usage = maxf(ld.usage, float64(toks)/float64(l.MaxTokens))
			}
		}
	}
	return ld
}

func maxf(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

// TryAcquire atomically re-checks the subjects and, if none is blocked (or
// force is set), counts the request: it takes one in-flight slot on every
// subject and one request in every usage window. The returned Permit must be
// released exactly once per attempt (Release is idempotent).
func (r *Registry) TryAcquire(subjects []Subject, force bool) (*Permit, Verdict) {
	q := r.resolveQuota(subjects)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := clock.Now()
	v := r.assessLocked(subjects, q, now)
	if v.State == StateBlocked && !force {
		return nil, v
	}
	p := &Permit{r: r, acquiredAt: now, keys: make([]string, 0, len(subjects))}
	for _, s := range subjects {
		c := r.counterLocked(s.Key)
		c.inflight++
		p.keys = append(p.keys, s.Key)
		if w := c.windowFor(&s.Limits); w != nil {
			w.add(now, 1, 0)
		}
	}
	r.permits[p] = struct{}{}
	return p, v
}

// Record adds completed-request tokens to the subjects' usage windows.
func (r *Registry) Record(subjects []Subject, tokens int64) {
	if tokens <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := clock.Now()
	for _, s := range subjects {
		if !s.Limits.HasBudget() {
			continue
		}
		if w := r.counterLocked(s.Key).windowFor(&s.Limits); w != nil {
			w.add(now, 0, tokens)
		}
	}
}

// Seed adds historical usage (warm start after a restart) to a key's window.
func (r *Registry) Seed(key string, l ai.Limits, events []UsageEvent) {
	if !l.HasBudget() || len(events) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	w := r.counterLocked(key).windowFor(&l)
	for _, e := range events {
		w.add(e.At, e.Requests, e.Tokens)
	}
}

// UsageEvent is a historical usage sample used by Seed.
type UsageEvent struct {
	At       time.Time
	Requests int64
	Tokens   int64
}

// Changed returns a channel closed on the next release; waiters re-assess then.
func (r *Registry) Changed() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.notify
}

func (r *Registry) broadcastLocked() {
	close(r.notify)
	r.notify = make(chan struct{})
}

// Stat is the live state of one key, for status APIs.
type Stat struct {
	Inflight int
	Requests int64
	Tokens   int64
}

// Stat returns the live counters for a key (zero value when unknown).
func (r *Registry) Stat(key string) Stat {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.counters[key]
	if c == nil {
		return Stat{}
	}
	st := Stat{Inflight: c.inflight}
	if c.win != nil {
		st.Requests, st.Tokens = c.win.sums(clock.Now())
	}
	return st
}

// Reap force-releases permits held longer than the leak-guard hold time and
// drops idle counters. Returns the number of permits reaped.
func (r *Registry) Reap() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := clock.Now()
	n := 0
	for p := range r.permits {
		if now.Sub(p.acquiredAt) >= r.maxHold {
			logrus.Warnf("[limits] force-releasing permit held for %s (keys=%v): possible leak", now.Sub(p.acquiredAt).Round(time.Second), p.keys)
			r.releaseLocked(p)
			n++
		}
	}
	for k, c := range r.counters {
		if c.inflight == 0 && (c.win == nil || c.win.empty(now)) {
			delete(r.counters, k)
		}
	}
	return n
}

// StartSweeper runs Reap periodically until ctx is done.
func (r *Registry) StartSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(sweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.Reap()
			}
		}
	}()
}

func (r *Registry) releaseLocked(p *Permit) {
	if p.released {
		return
	}
	p.released = true
	for _, k := range p.keys {
		if c := r.counters[k]; c != nil && c.inflight > 0 {
			c.inflight--
		}
	}
	delete(r.permits, p)
	r.broadcastLocked()
}

// Permit is one attempt's hold on a set of subjects.
type Permit struct {
	r          *Registry
	acquiredAt time.Time
	keys       []string
	released   bool
}

// Release frees the in-flight slots. Safe to call more than once and on nil.
func (p *Permit) Release() {
	if p == nil {
		return
	}
	p.r.mu.Lock()
	defer p.r.mu.Unlock()
	p.r.releaseLocked(p)
}

// Utilization is the per-dimension load of a set of subjects in percent
// (not capped: 100 = at the hard limit). Has* is false when no subject
// configures that dimension, so callers can tell "unlimited" from "idle".
type Utilization struct {
	Concurrency    float64
	HasConcurrency bool
	Usage          float64
	HasUsage       bool
}

// Utilization reports the tightest (highest) load per dimension across subjects.
func (r *Registry) Utilization(subjects []Subject) Utilization {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := clock.Now()
	var u Utilization
	for i := range subjects {
		ld := r.loadLocked(&subjects[i], now)
		u.HasConcurrency = u.HasConcurrency || ld.hasConc
		u.HasUsage = u.HasUsage || ld.hasUsage
		u.Concurrency = maxf(u.Concurrency, 100*ld.conc)
		u.Usage = maxf(u.Usage, 100*ld.usage)
	}
	return u
}
