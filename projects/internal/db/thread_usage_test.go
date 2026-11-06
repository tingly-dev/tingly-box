package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAddUsageSumsTotalsAndKeepsLatestContext(t *testing.T) {
	th := &Thread{}
	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	th.AddUsage(Usage{Input: 100, Output: 20, CacheRead: 60, CacheWrite: 10, ContextTokens: 90, ContextWindow: 200000, DurationMS: 4000}, at)
	th.AddUsage(Usage{Input: 50, Output: 5, CacheRead: 40, ContextTokens: 120, DurationMS: 1500}, at.Add(time.Minute))

	assert.EqualValues(t, 150, th.InputTokens)
	assert.EqualValues(t, 25, th.OutputTokens)
	assert.EqualValues(t, 100, th.CacheReadTokens)
	assert.EqualValues(t, 10, th.CacheWriteTokens)
	assert.EqualValues(t, 120, th.ContextTokens, "context is the latest turn's, not a sum")
	assert.Equal(t, 200000, th.ContextWindow, "a turn that does not know the window keeps the known one")
	assert.EqualValues(t, 1500, th.LastTurnMS)
	if assert.NotNil(t, th.LastTurnAt) {
		assert.True(t, th.LastTurnAt.Equal(at.Add(time.Minute)))
	}
}

func TestAddUsageIgnoresATurnThatNeverReachedTheModel(t *testing.T) {
	th := &Thread{ContextTokens: 77, ContextWindow: 1000}
	th.AddUsage(Usage{}, time.Now())
	assert.EqualValues(t, 77, th.ContextTokens)
	assert.Equal(t, 1000, th.ContextWindow)
}

func TestUsageColumnsMigrateOntoAnOlderDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	store, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "P", Type: ProjectProject}
	require.NoError(t, store.CreateProject(p))
	th := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode, Status: ThreadIdle, InputTokens: 9}
	require.NoError(t, store.CreateThread(th))
	require.NoError(t, store.Close())

	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	for _, c := range []string{"cache_read_tokens", "cache_write_tokens", "context_tokens", "context_window", "last_turn_ms", "last_turn_at", "rate_limits"} {
		require.NoError(t, g.Exec("ALTER TABLE threads DROP COLUMN "+c).Error)
	}
	sqlDB, err := g.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	store, err = Open(path)
	require.NoError(t, err)
	defer store.Close()
	got, err := store.GetThread(th.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 9, got.InputTokens)
	assert.Zero(t, got.ContextTokens)
	assert.Nil(t, got.LastTurnAt)
}

func TestAddUsageKeepsTheLatestRateLimits(t *testing.T) {
	th := &Thread{}
	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	five := &RateLimitWindow{Utilization: 0.42, ResetsAt: at.Add(2 * time.Hour).Unix()}
	th.AddUsage(Usage{Input: 1, RateLimits: &RateLimits{Status: "allowed", FiveHour: five}}, at)
	if assert.NotNil(t, th.RateLimits) {
		assert.Equal(t, five, th.RateLimits.FiveHour)
		assert.True(t, th.RateLimits.UpdatedAt.Equal(at), "stamped with the turn's end so a reader can tell how old the reading is")
	}

	th.AddUsage(Usage{Input: 1}, at.Add(time.Hour))
	if assert.NotNil(t, th.RateLimits, "a turn that read no windows keeps the last reading") {
		assert.Equal(t, five, th.RateLimits.FiveHour)
	}
}

func TestRateLimitsRoundTripThroughSaveThreadUsage(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "projects.db"))
	require.NoError(t, err)
	defer store.Close()
	p := &Project{Name: "P", Type: ProjectProject}
	require.NoError(t, store.CreateProject(p))
	th := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode, Status: ThreadIdle}
	require.NoError(t, store.CreateThread(th))

	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	th.AddUsage(Usage{Input: 5, RateLimits: &RateLimits{Status: "allowed_warning",
		FiveHour: &RateLimitWindow{Utilization: 0.91, ResetsAt: 1793958000},
		SevenDay: &RateLimitWindow{Utilization: 0.17, ResetsAt: 1794354000}}}, at)
	require.NoError(t, store.SaveThreadUsage(th))

	got, err := store.GetThread(th.ID)
	require.NoError(t, err)
	require.NotNil(t, got.RateLimits)
	assert.Equal(t, "allowed_warning", got.RateLimits.Status)
	assert.Equal(t, 0.91, got.RateLimits.FiveHour.Utilization)
	assert.EqualValues(t, 1794354000, got.RateLimits.SevenDay.ResetsAt)
	assert.EqualValues(t, 5, got.InputTokens)
}

func TestThreadWithoutRateLimitsHasNone(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "projects.db"))
	require.NoError(t, err)
	defer store.Close()
	p := &Project{Name: "P", Type: ProjectProject}
	require.NoError(t, store.CreateProject(p))
	th := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode, Status: ThreadIdle}
	require.NoError(t, store.CreateThread(th))
	th.AddUsage(Usage{Input: 5}, time.Now())
	require.NoError(t, store.SaveThreadUsage(th))
	got, err := store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Nil(t, got.RateLimits)
}

func TestRateLimitsMergeWindowByWindow(t *testing.T) {
	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	th := &Thread{}
	th.AddUsage(Usage{RateLimits: &RateLimits{Status: "allowed",
		FiveHour: &RateLimitWindow{Utilization: 0.4, ResetsAt: 100},
		SevenDay: &RateLimitWindow{Utilization: 0.1, ResetsAt: 200}}}, at)
	// A later event may name only the window that is limiting.
	th.AddUsage(Usage{RateLimits: &RateLimits{Status: "allowed_warning",
		FiveHour: &RateLimitWindow{Utilization: 0.9, ResetsAt: 100}}}, at.Add(time.Minute))
	require.NotNil(t, th.RateLimits)
	assert.Equal(t, "allowed_warning", th.RateLimits.Status)
	assert.Equal(t, 0.9, th.RateLimits.FiveHour.Utilization)
	if assert.NotNil(t, th.RateLimits.SevenDay, "a window the new reading does not mention keeps its last reading") {
		assert.Equal(t, 0.1, th.RateLimits.SevenDay.Utilization)
	}
}
