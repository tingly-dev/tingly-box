package quota

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	typ "github.com/tingly-dev/tingly-box/ai"
)

func TestBackoffDelayDoublesToTheCap(t *testing.T) {
	t.Parallel()
	const base, limit = 5 * time.Minute, 6 * time.Hour
	want := []time.Duration{
		10 * time.Minute, 20 * time.Minute, 40 * time.Minute, 80 * time.Minute,
		160 * time.Minute, 320 * time.Minute, limit, limit,
	}
	for i, w := range want {
		if got := backoffDelay(base, limit, i+1, 0); got != w {
			t.Errorf("after failure %d: delay = %v, want %v", i+1, got, w)
		}
	}
	if got := backoffDelay(base, limit, 1000, 0.99); got != limit {
		t.Errorf("many failures: delay = %v, want the %v cap", got, limit)
	}
	if got := backoffDelay(base, limit, 0, 0); got != 0 {
		t.Errorf("no failures: delay = %v, want 0", got)
	}
}

func TestBackoffDelayJitterOnlyDelays(t *testing.T) {
	t.Parallel()
	const base, limit = 5 * time.Minute, 6 * time.Hour
	plain := backoffDelay(base, limit, 3, 0)
	jittered := backoffDelay(base, limit, 3, 0.999)
	if jittered <= plain || jittered > plain+plain/10 {
		t.Errorf("jittered delay = %v, want in (%v, %v]", jittered, plain, plain+plain/10)
	}
	// A cap is a cap: jitter cannot push past it.
	if got := backoffDelay(base, 12*time.Minute, 1, 0.999); got <= 10*time.Minute || got > 11*time.Minute {
		t.Errorf("jitter below the cap = %v, want in (10m, 11m]", got)
	}
	if got := backoffDelay(base, 10*time.Minute+30*time.Second, 1, 0.999); got != 10*time.Minute+30*time.Second {
		t.Errorf("jitter past the cap = %v, want the 10m30s cap", got)
	}
}

// A failure lands some seconds after a tick; the next tick one doubled window
// later is those seconds short of it. Slack keeps that tick from being skipped.
func TestBackoffDueLeavesSlackForTheTicker(t *testing.T) {
	t.Parallel()
	const tick = 5 * time.Minute
	clock := time.Date(2026, 11, 13, 10, 0, 7, 0, time.UTC) // a failure 7s after the tick
	b := newBackoffBook()
	b.now = func() time.Time { return clock }
	b.rand = func() float64 { return 0 }

	if _, ok := b.due("p", tick/4); !ok {
		t.Fatal("a provider with no failures must be due")
	}
	b.failed("p", true, tick, 6*time.Hour) // window: 10m
	clock = clock.Add(tick - 7*time.Second)
	if _, ok := b.due("p", tick/4); ok {
		t.Fatal("due after one tick of a two-tick window")
	}
	clock = clock.Add(tick) // exactly two ticks after the failed tick, 7s before next
	if _, ok := b.due("p", tick/4); !ok {
		t.Fatal("the tick two intervals later was skipped: every doubling would cost an extra tick")
	}
}

func TestBackoffManualFailureLeavesTheWindow(t *testing.T) {
	t.Parallel()
	const tick = 5 * time.Minute
	b := newBackoffBook()
	b.rand = func() float64 { return 0 }

	b.failed("p", false, tick, 6*time.Hour)
	if _, ok := b.due("p", 0); !ok {
		t.Fatal("a manual failure must not open a window")
	}
	b.failed("p", true, tick, 6*time.Hour)
	b.failed("p", true, tick, 6*time.Hour) // 20m
	until, _ := b.due("p", 0)
	b.failed("p", false, tick, 6*time.Hour)
	if again, _ := b.due("p", 0); !again.Equal(until) {
		t.Fatalf("a manual failure moved the window: %v -> %v", until, again)
	}
	b.succeeded("p")
	if _, ok := b.due("p", 0); !ok {
		t.Fatal("a success must clear the window")
	}
}

type scriptedFetcher struct {
	mu    sync.Mutex
	calls int
	fail  bool
	used  float64
}

func (*scriptedFetcher) Name() string                 { return "scripted" }
func (*scriptedFetcher) ProviderType() ProviderType   { return ProviderTypeAnthropic }
func (*scriptedFetcher) Validate(*typ.Provider) error { return nil }
func (*scriptedFetcher) RequiresAuth() typ.AuthType   { return "" }
func (f *scriptedFetcher) Fetch(_ context.Context, p *typ.Provider) (*ProviderUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail {
		return nil, errors.New("unexpected status code: 403")
	}
	now := time.Now()
	u := &ProviderUsage{ProviderUUID: p.UUID, ProviderName: p.Name, ProviderType: ProviderTypeAnthropic, FetchedAt: now, ExpiresAt: now.Add(time.Hour)}
	u.AddWindow("session", &UsageWindow{Used: f.used, Limit: 100, Label: "Session"})
	return u, nil
}

func (f *scriptedFetcher) set(fail bool, used float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail, f.used = fail, used
}

func (f *scriptedFetcher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newBackoffTestManager(t *testing.T) (*Manager, *scriptedFetcher, *time.Time) {
	t.Helper()
	store, err := NewGormStore(filepath.Join(t.TempDir(), "test.db"), logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	provider := &typ.Provider{UUID: "p1", Name: "Provider", APIBase: "https://api.anthropic.com/v1", Enabled: true}
	m := NewManager(DefaultConfig(), store, managerTestProviderManager{[]*typ.Provider{provider}}, logrus.New())
	f := &scriptedFetcher{used: 40}
	if err := m.RegisterFetcher(f); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	m.backoff.now = func() time.Time { return clock }
	m.backoff.rand = func() float64 { return 0 }
	return m, f, &clock
}

func TestRefreshDueBacksOffAndManualRefreshForces(t *testing.T) {
	m, f, clock := newBackoffTestManager(t)
	ctx := context.Background()
	tick := m.config.RefreshInterval

	if _, err := m.RefreshDue(ctx); err != nil || f.count() != 1 {
		t.Fatalf("first refresh: err=%v calls=%d, want 1 call", err, f.count())
	}

	f.set(true, 0)
	*clock = clock.Add(tick)
	m.RefreshDue(ctx) // fails: window 10m
	if f.count() != 2 {
		t.Fatalf("calls = %d, want 2", f.count())
	}
	*clock = clock.Add(tick)
	if got, _ := m.RefreshDue(ctx); len(got) != 0 || f.count() != 2 {
		t.Fatalf("one tick into a 10m window: results=%d calls=%d, want the provider skipped", len(got), f.count())
	}

	// A person asking is never made to wait, and failing leaves the window alone.
	if _, err := m.Refresh(ctx); err != nil || f.count() != 3 {
		t.Fatalf("manual refresh: err=%v calls=%d, want a real request", err, f.count())
	}
	*clock = clock.Add(tick)
	m.RefreshDue(ctx) // 10m since the failed tick: due, fails again, window 20m
	if f.count() != 4 {
		t.Fatalf("calls = %d, want 4: the manual failure must not have moved the window", f.count())
	}
	for range 3 {
		*clock = clock.Add(tick)
		m.RefreshDue(ctx)
	}
	if f.count() != 4 {
		t.Fatalf("calls = %d, want 4: a 20m window should skip three ticks", f.count())
	}
	*clock = clock.Add(tick)
	m.RefreshDue(ctx)
	if f.count() != 5 {
		t.Fatalf("calls = %d, want 5 once the 20m window ran out", f.count())
	}

	// A manual success clears the window; the next tick asks again.
	f.set(false, 50)
	if _, err := m.RefreshProvider(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(tick)
	m.RefreshDue(ctx)
	if f.count() != 7 {
		t.Fatalf("calls = %d, want 7: success must put the provider back on every tick", f.count())
	}
}

func TestFailedRefreshKeepsTheLastReading(t *testing.T) {
	m, f, _ := newBackoffTestManager(t)
	ctx := context.Background()

	good, err := m.RefreshProvider(ctx, "p1")
	if err != nil || good.LastError != "" {
		t.Fatalf("setup refresh: usage=%+v err=%v", good, err)
	}

	f.set(true, 0)
	failed, err := m.RefreshProvider(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]*ProviderUsage{"returned": failed, "stored": mustGet(t, m, "p1")} {
		if u.LastError == "" || u.LastErrorAt == nil {
			t.Errorf("%s: the failure was not recorded: %+v", name, u)
		}
		if len(u.Windows) != 1 || u.Windows[0].Used != 40 {
			t.Errorf("%s: the last good reading is gone: %+v", name, u.Windows)
		}
		if !u.FetchedAt.Equal(good.FetchedAt) {
			t.Errorf("%s: FetchedAt = %v, want the success's %v", name, u.FetchedAt, good.FetchedAt)
		}
		if !u.LastErrorAt.After(u.FetchedAt) {
			t.Errorf("%s: LastErrorAt %v is not after FetchedAt %v", name, u.LastErrorAt, u.FetchedAt)
		}
	}

	// History is a log of attempts: the failure is in it, without a reading.
	history, err := m.QuotaHistory(ctx, HistoryQuery{ProviderUUID: "p1"})
	if err != nil || len(history) != 2 {
		t.Fatalf("history = %d rows, err=%v, want 2", len(history), err)
	}
	if history[0].LastError == "" || len(history[0].Windows) != 0 {
		t.Errorf("newest history row should be the bare failure: %+v", history[0])
	}

	// Failing again keeps it too, and a success replaces it and drops the error.
	if _, err := m.RefreshProvider(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if u := mustGet(t, m, "p1"); len(u.Windows) != 1 || u.Windows[0].Used != 40 {
		t.Errorf("second failure lost the reading: %+v", u.Windows)
	}
	f.set(false, 70)
	if _, err := m.RefreshProvider(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if u := mustGet(t, m, "p1"); u.LastError != "" || len(u.Windows) != 1 || u.Windows[0].Used != 70 {
		t.Errorf("success did not replace the reading and clear the error: %+v", u)
	}
}

func TestFailureWithNoPriorReadingIsJustTheError(t *testing.T) {
	m, f, _ := newBackoffTestManager(t)
	f.set(true, 0)
	u, err := m.RefreshProvider(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if u.LastError == "" || len(u.Windows) != 0 {
		t.Fatalf("usage = %+v, want an error and no windows", u)
	}
}

func mustGet(t *testing.T, m *Manager, uuid string) *ProviderUsage {
	t.Helper()
	u, err := m.GetQuota(context.Background(), uuid)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
