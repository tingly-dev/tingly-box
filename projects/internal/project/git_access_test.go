package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func settingsPerms(t *testing.T, path string) (allow, ask []string) {
	t.Helper()
	perms, _ := readSettings(t, path)["permissions"].(map[string]any)
	return strs(t, perms["allow"]), strs(t, perms["ask"])
}

func TestAutoModeThreadMayPushItsOwnBranchAndAsksToPublish(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	f.project.ThreadPermissionMode = "auto"
	require.NoError(t, f.store.SaveProject(f.project))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Fix login", Task: "fix"})
	require.NoError(t, err)
	require.NotEmpty(t, th.Branch)
	f.report(t)

	allow, ask := settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.Contains(t, allow, "Bash(git push origin "+th.Branch+")")
	assert.Contains(t, allow, "Bash(git push -u origin "+th.Branch+")")
	assert.Contains(t, allow, "Bash(git push --set-upstream origin "+th.Branch+")")
	for _, a := range allow {
		if a != "" && len(a) > 8 && a[:8] == "Bash(git" {
			assert.Contains(t, a, th.Branch, "only the thread's own branch is pre-approved: %s", a)
		}
	}
	for _, want := range []string{"Bash(gh)", "Bash(gh *)", "Bash(git push --force *)", "Bash(git push origin main)"} {
		assert.Contains(t, ask, want)
	}
	assert.NotContains(t, ask, "Bash(git push *)", "a blanket ask would override the own-branch allow")
}

func TestOtherPermissionModesGetNoGitRules(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Fix login", Task: "fix"})
	require.NoError(t, err)
	f.report(t)

	allow, ask := settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	for _, r := range append(allow, ask...) {
		assert.NotContains(t, r, "git push")
		assert.NotContains(t, r, "gh ")
	}
}

func TestSwitchingToAutoModeRefreshesThreadSettingsOnNextTurn(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Fix login", Task: "fix"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	_, ask := settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.ElementsMatch(t, []string{toolSendMessage}, ask, "only the native SendMessage asks: the GitHub tools ask on their own cards")

	_, err = f.threads.SetPermissionMode(th.ID, "auto")
	require.NoError(t, err)
	_, err = f.threads.Send(context.Background(), th.ID, "go on", Source{}, nil)
	require.NoError(t, err)
	f.report(t)

	allow, ask := settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.Contains(t, allow, "Bash(git push origin "+th.Branch+")")
	assert.Contains(t, ask, "Bash(gh *)")
}

func TestGrantAutoGitKeepsTheHostsRulesAndSkipsUnsafeBranches(t *testing.T) {
	p := &db.Project{ThreadPermissionMode: "auto"}
	doc := map[string]any{"permissions": map[string]any{
		"allow": []any{"Bash(make test)"},
		"ask":   []any{"AskUserQuestion", "Bash(gh *)"},
	}}
	grantAutoGit(doc, p, &db.Thread{Branch: "tp/a b)", BaseBranch: "main"})
	perms := doc["permissions"].(map[string]any)
	assert.Equal(t, []any{"Bash(make test)"}, perms["allow"], "no allow rule for a branch name that cannot sit in a rule")
	ask := perms["ask"].([]any)
	assert.Equal(t, "AskUserQuestion", ask[0])
	count := 0
	for _, a := range ask {
		if a == "Bash(gh *)" {
			count++
		}
	}
	assert.Equal(t, 1, count, "no duplicates")
}

func TestWriteSettingsLeavesAnUnchangedFileAlone(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Fix login", Task: "fix"})
	require.NoError(t, err)
	f.report(t)
	path := f.threads.layout.SettingsFile(f.project.ID, th.ID)
	before, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, f.threads.writeSettings(f.project, th))
	same, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, same), "identical content is not rewritten")

	th.PermissionMode = "auto"
	require.NoError(t, f.threads.writeSettings(f.project, th))
	changed, err := os.Stat(path)
	require.NoError(t, err)
	assert.False(t, os.SameFile(before, changed), "new content replaces the file in one rename")
	_, ask := settingsPerms(t, path)
	assert.Contains(t, ask, "Bash(gh *)")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "no temp file is left behind")
	}
}
