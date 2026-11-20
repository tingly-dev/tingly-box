package server

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type noProviders struct{}

func (noProviders) GetProviderByUUID(string) (*typ.Provider, error) { return nil, nil }
func (noProviders) ListProviders() []*typ.Provider                  { return nil }

func passiveQuotaFixture(t *testing.T) (*passiveClaudeQuota, quota.Store, *time.Time) {
	t.Helper()
	store, err := quota.NewGormStore(filepath.Join(t.TempDir(), "quota.db"), logrus.New())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	clock := time.Unix(1786000000, 0)
	p := &passiveClaudeQuota{
		mgr:  quota.NewManager(nil, store, noProviders{}, logrus.New()),
		now:  func() time.Time { return clock },
		last: map[string]passiveClaudeState{},
	}
	return p, store, &clock
}

func claudeHeaders(status, five string) http.Header {
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", status)
	h.Set("anthropic-ratelimit-unified-5h-utilization", five)
	h.Set("anthropic-ratelimit-unified-5h-reset", "1786147200")
	return h
}

func seedClaudeQuota(t *testing.T, store quota.Store, fiveHour float64) {
	t.Helper()
	u := &quota.ProviderUsage{ProviderUUID: "claude-1", ProviderName: "Claude", ProviderType: quota.ProviderTypeAnthropic,
		FetchedAt: time.Unix(1785990000, 0), ExpiresAt: time.Unix(1786090000, 0)}
	u.AddWindow("five_hour", &quota.UsageWindow{Type: quota.WindowTypeSession, Unit: quota.UsageUnitPercent, Kind: quota.WindowKindLimit,
		Used: fiveHour, Limit: 100, UsedPercent: fiveHour, WindowMinutes: 300, Label: "5-Hour Window"})
	require.NoError(t, store.Save(context.Background(), u))
}

func waitForFiveHour(t *testing.T, store quota.Store, want float64) {
	t.Helper()
	require.Eventually(t, func() bool {
		u, err := store.Get(context.Background(), "claude-1")
		if err != nil {
			return false
		}
		for _, w := range u.Windows {
			if w.Key == "five_hour" && w.UsedPercent > want-0.01 && w.UsedPercent < want+0.01 {
				return true
			}
		}
		return false
	}, 3*time.Second, 20*time.Millisecond)
}

func TestPassiveClaudeQuota_UpdatesAnExistingSnapshotAndThrottles(t *testing.T) {
	p, store, clock := passiveQuotaFixture(t)
	provider := &typ.Provider{UUID: "claude-1"}
	seedClaudeQuota(t, store, 10)

	p.observe(provider, claudeHeaders("allowed", "0.40"))
	waitForFiveHour(t, store, 40)

	// Within the interval a moved percentage is not stored again…
	*clock = clock.Add(30 * time.Second)
	p.observe(provider, claudeHeaders("allowed", "0.45"))
	time.Sleep(100 * time.Millisecond)
	waitForFiveHour(t, store, 40)

	// …after it, it is.
	*clock = clock.Add(passiveClaudeQuotaInterval)
	p.observe(provider, claudeHeaders("allowed", "0.50"))
	waitForFiveHour(t, store, 50)
}

func TestPassiveClaudeQuota_RejectionIsStoredAtOnce(t *testing.T) {
	p, store, clock := passiveQuotaFixture(t)
	provider := &typ.Provider{UUID: "claude-1"}
	seedClaudeQuota(t, store, 10)

	p.observe(provider, claudeHeaders("allowed", "0.40"))
	waitForFiveHour(t, store, 40)

	*clock = clock.Add(5 * time.Second) // well inside the interval
	p.observe(provider, claudeHeaders("rejected", "1"))
	waitForFiveHour(t, store, 100)
	u, err := store.Get(context.Background(), "claude-1")
	require.NoError(t, err)
	var reached bool
	for _, w := range u.Windows {
		if w.Key == "five_hour" && w.LimitReached != nil {
			reached = *w.LimitReached
		}
	}
	assert.True(t, reached, "the window must read limit reached")
}

func TestPassiveClaudeQuota_NeverCreatesASnapshot(t *testing.T) {
	p, store, _ := passiveQuotaFixture(t)
	p.observe(&typ.Provider{UUID: "claude-1"}, claudeHeaders("allowed", "0.40"))
	time.Sleep(150 * time.Millisecond)
	_, err := store.Get(context.Background(), "claude-1")
	assert.Error(t, err, "a provider the poll has not read yet must not get a header-only record")
}

func TestPassiveClaudeQuota_IgnoresResponsesWithoutWindows(t *testing.T) {
	p, store, _ := passiveQuotaFixture(t)
	seedClaudeQuota(t, store, 10)
	h := http.Header{}
	h.Set("anthropic-ratelimit-unified-status", "allowed")
	p.observe(&typ.Provider{UUID: "claude-1"}, h)
	p.observe(nil, claudeHeaders("allowed", "0.4"))
	time.Sleep(100 * time.Millisecond)
	waitForFiveHour(t, store, 10)
}
