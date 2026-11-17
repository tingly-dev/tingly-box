package server

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/limits"
	"github.com/tingly-dev/tingly-box/internal/routing"
)

func TestWarmStartPolicySeedsPersonTeamModelAndTemplateWindows(t *testing.T) {
	dir, err := os.MkdirTemp("", "warm-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	cfg, err := config.NewConfig(config.WithConfigDir(dir))
	require.NoError(t, err)
	usage, err := db.NewUsageStore(t.TempDir())
	require.NoError(t, err)
	store, err := db.NewLimitPolicyStore(t.TempDir())
	require.NoError(t, err)

	now := time.Now().UTC()
	rec := func(user, team, model string, ago time.Duration, tokens int) {
		require.NoError(t, usage.RecordUsage(&db.UsageRecord{ProviderUUID: "p", ProviderName: "p", Model: model, Scenario: "team",
			UserID: user, TeamID: team, Timestamp: now.Add(-ago), TotalTokens: tokens, InputTokens: tokens, Status: "success"}))
	}
	rec("alice", "t1", "opus", 2*time.Minute, 100)
	rec("alice", "t1", "opus", 3*time.Minute, 100)
	rec("bob", "t1", "haiku", 2*time.Minute, 50)
	rec("alice", "t1", "opus", 5*time.Hour, 9999) // outside the window

	reg := limits.NewRegistry()
	guard := routing.NewLimitsGuard(reg, cfg)
	guard.SetPolicies(store)
	win := ai.Limits{MaxTokens: 100000, WindowSeconds: 3600}

	person, _, err := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "alice", Limits: win})
	require.NoError(t, err)
	team, _, _ := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyTeam, Ref: "t1", Limits: win})
	model, _, _ := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyModel, Ref: "opus", Limits: win})
	tmpl, _, _ := store.Upsert(ai.LimitPolicy{Kind: ai.LimitPolicyTeamMember, Ref: "t1", Limits: win})

	for _, p := range []ai.LimitPolicy{person, team, model, tmpl} {
		warmStartPolicy(guard, usage, store, p)
	}
	require.Equal(t, int64(200), reg.Stat(routing.UserLimitKey("alice")).Tokens, "alice's own policy seeds her window")
	require.Equal(t, int64(250), reg.Stat(routing.TeamLimitKey("t1")).Tokens)
	require.Equal(t, int64(200), reg.Stat(routing.ModelLimitKey("opus")).Tokens)
	require.Equal(t, int64(50), reg.Stat(routing.UserLimitKey("bob")).Tokens, "the template seeds each member without a policy")
	require.Equal(t, int64(2), reg.Stat(routing.UserLimitKey("alice")).Requests, "the template did not re-seed (or double count) alice")

	// idempotent: saving a policy again does not double count
	warmStartPolicy(guard, usage, store, person)
	warmStartPolicy(guard, usage, store, team)
	require.Equal(t, int64(200), reg.Stat(routing.UserLimitKey("alice")).Tokens)
	require.Equal(t, int64(250), reg.Stat(routing.TeamLimitKey("t1")).Tokens)

	// a person warm-started over budget is blocked straight after a "restart"
	tight := ai.Limits{MaxTokens: 150, WindowSeconds: 3600}
	reg2 := limits.NewRegistry()
	guard2 := routing.NewLimitsGuard(reg2, cfg)
	guard2.SetPolicies(store)
	warmStartPolicy(guard2, usage, store, ai.LimitPolicy{Kind: ai.LimitPolicyUser, Ref: "alice", Limits: tight})
	v := reg2.Assess([]limits.Subject{{Key: routing.UserLimitKey("alice"), Scope: limits.ScopeUser, Limits: tight}})
	require.Equal(t, limits.StateBlocked, v.State)
	require.Equal(t, limits.ScopeUser, v.Scopes)
}
