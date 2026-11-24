package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wbBase = "http://localhost:12580/tingly/workbuddy"
const wbDisplay = "~/.workbuddy/models.json"

func wbReadJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func wbReadArray(t *testing.T, path string) []interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var out []interface{}
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func wbIDs(entries []interface{}) []string {
	var ids []string
	for _, e := range entries {
		ids = append(ids, e.(map[string]interface{})["id"].(string))
	}
	return ids
}

func TestApplyWorkBuddyConfig_CreatesArrayFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".workbuddy", "models.json")
	res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "tok", []string{"claude-sonnet-5"})
	require.True(t, res.Success, res.Message)
	assert.True(t, res.Created)

	models := wbReadArray(t, path)
	require.Len(t, models, 1)
	m := models[0].(map[string]interface{})
	assert.Equal(t, "claude-sonnet-5", m["id"])
	assert.Equal(t, wbBase+"/v1/chat/completions", m["url"])
	assert.Equal(t, "tok", m["apiKey"])
	assert.Equal(t, true, m["supportsToolCall"])
	assert.Equal(t, false, m["supportsImages"])

	if os.PathSeparator == '/' {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

// The desktop app stores models.json as a top-level array.
func TestApplyWorkBuddyConfig_MergesIntoArrayFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte(`[
  {"id": "mine", "vendor": "Custom", "url": "http://localhost:12580/tingly/openai", "apiKey": "k", "useCustomProtocol": false},
  {"id": "old-tingly", "url": "http://localhost:12580/tingly/workbuddy/v1/chat/completions", "apiKey": "old"}
]`), 0o600))

	res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "new", []string{"fresh"})
	require.True(t, res.Success, res.Message)
	assert.True(t, res.Updated)
	assert.NotEmpty(t, res.BackupPath)

	models := wbReadArray(t, path)
	assert.Equal(t, []string{"mine", "fresh"}, wbIDs(models), "user entry kept, stale managed entry replaced")
	assert.Equal(t, false, models[0].(map[string]interface{})["useCustomProtocol"], "user fields preserved")

	applied := readWorkBuddyAppliedStateAt(path)
	assert.True(t, applied.Managed)
	assert.Equal(t, []string{"fresh"}, applied.Models)
}

func TestApplyWorkBuddyConfig_PreservesObjectFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "custom": {"keep": true},
  "models": [
    {"id": "mine", "url": "https://api.example.com/v1/chat/completions", "apiKey": "k"},
    {"id": "old-tingly", "url": "http://localhost:12580/tingly/workbuddy/v1/chat/completions", "apiKey": "old"}
  ]
}`), 0o640))
	// WriteFile's creation mode is filtered by the process umask. Set the
	// intended initial permissions explicitly before testing preservation.
	require.NoError(t, os.Chmod(path, 0o640))

	res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "new", []string{"fresh"})
	require.True(t, res.Success, res.Message)
	assert.True(t, res.Updated)
	assert.NotEmpty(t, res.BackupPath)

	cfg := wbReadJSON(t, path)
	assert.Equal(t, map[string]interface{}{"keep": true}, cfg["custom"])
	assert.Equal(t, []string{"mine", "fresh"}, wbIDs(cfg["models"].([]interface{})), "user entry kept, stale managed entry replaced")

	if os.PathSeparator == '/' {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "existing permissions preserved")
	}
}

func TestApplyWorkBuddyConfig_SkipsIDOwnedByUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte(`[{"id":"gpt-x","url":"https://api.example.com/v1/chat/completions"}]`), 0o600))

	res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "tok", []string{"gpt-x", "other"})
	require.True(t, res.Success, res.Message)
	assert.Equal(t, []string{"gpt-x"}, res.Skipped)
	assert.Contains(t, res.Message, "gpt-x")

	models := wbReadArray(t, path)
	require.Len(t, models, 2)
	assert.Equal(t, "https://api.example.com/v1/chat/completions", models[0].(map[string]interface{})["url"], "user entry untouched")
}

func TestApplyWorkBuddyConfig_KeepsAvailableModelsWhitelistConsistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
  "models":[{"id":"stale","url":"http://h/tingly/workbuddy/v1/chat/completions"}],
  "availableModels":["mine","stale"]
}`), 0o600))

	res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "tok", []string{"fresh"})
	require.True(t, res.Success, res.Message)
	cfg := wbReadJSON(t, path)
	assert.Equal(t, []interface{}{"mine", "fresh"}, cfg["availableModels"])
}

func TestApplyWorkBuddyConfig_LeavesInvalidFileUntouched(t *testing.T) {
	for _, orig := range []string{`"just a string"`, `{"models": [`} {
		path := filepath.Join(t.TempDir(), "models.json")
		require.NoError(t, os.WriteFile(path, []byte(orig), 0o600))

		res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "tok", []string{"m"})
		assert.False(t, res.Success)
		assert.Contains(t, res.Message, wbDisplay)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, orig, string(data))
	}
}

func TestWorkBuddyStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	assert.Equal(t, WorkBuddyAppliedState{}, readWorkBuddyAppliedStateAt(path), "missing file reads as not applied")
	state, _ := CompareWorkBuddyConfig(WorkBuddyAppliedState{}, []string{"a"})
	assert.Equal(t, ClientConfigNotApplied, state)

	require.True(t, applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "tok", []string{"a", "b"}).Success)
	applied := readWorkBuddyAppliedStateAt(path)
	assert.True(t, applied.Managed)

	state, diffs := CompareWorkBuddyConfig(applied, []string{"b", "a"})
	assert.Equal(t, ClientConfigApplied, state)
	assert.Empty(t, diffs)

	state, diffs = CompareWorkBuddyConfig(applied, []string{"a", "c"})
	assert.Equal(t, ClientConfigOutdated, state)
	require.Len(t, diffs, 1)
	assert.Equal(t, "models", diffs[0].Key)
}

func TestApplyWorkBuddyConfig_RefusesWhenEveryIDIsTaken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	orig := `{"models":[{"id":"gpt-x","url":"https://api.example.com/v1/chat/completions"},{"id":"old","url":"http://h/tingly/workbuddy/v1/chat/completions"}]}`
	require.NoError(t, os.WriteFile(path, []byte(orig), 0o600))

	res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "tok", []string{"gpt-x"})
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "gpt-x")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, orig, string(data), "nothing written, stale managed entry kept")
	assert.Empty(t, res.BackupPath)
}

func TestApplyWorkBuddyConfig_AcceptsUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	require.NoError(t, os.WriteFile(path, []byte("\xef\xbb\xbf"+`[{"id":"mine","url":"https://api.example.com/v1/chat/completions"}]`), 0o600))

	res := applyWorkBuddyConfigAt(path, wbDisplay, wbBase, "tok", []string{"m"})
	require.True(t, res.Success, res.Message)
	require.Len(t, wbReadArray(t, path), 2)
	assert.True(t, readWorkBuddyAppliedStateAt(path).Managed)
}

func wbSetHome(t *testing.T, dirs ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(home, d), 0o755))
	}
	return home
}

func TestWorkBuddyTargets_DetectsInstalledEditions(t *testing.T) {
	wbSetHome(t, ".workbuddy-ai")
	installed, all, err := WorkBuddyTargets()
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Len(t, installed, 1)
	assert.Equal(t, "intl", installed[0].Edition)
	assert.Equal(t, "~/.workbuddy-ai/models.json", installed[0].DisplayPath)
}

func TestApplyWorkBuddyConfig_WritesEveryInstalledEdition(t *testing.T) {
	home := wbSetHome(t, ".workbuddy", ".workbuddy-ai")
	intl := filepath.Join(home, ".workbuddy-ai", "models.json")
	require.NoError(t, os.WriteFile(intl, []byte(`[{"id":"mine","url":"https://x/v1/chat/completions"}]`), 0o600))

	res, err := ApplyWorkBuddyConfig(wbBase, "tok", []string{"a"})
	require.NoError(t, err)
	require.True(t, res.Success, res.Message)
	require.Len(t, res.Files, 2)
	assert.Equal(t, "cn", res.Files[0].Edition)
	assert.True(t, res.Files[0].Created)
	assert.Equal(t, "intl", res.Files[1].Edition)
	assert.True(t, res.Files[1].Updated)

	assert.Equal(t, []string{"a"}, wbIDs(wbReadArray(t, filepath.Join(home, ".workbuddy", "models.json"))))
	assert.Equal(t, []string{"mine", "a"}, wbIDs(wbReadArray(t, intl)))

	installed, _, err := WorkBuddyTargets()
	require.NoError(t, err)
	state, diffs := CompareWorkBuddyTargets(installed, []string{"a"})
	assert.Equal(t, ClientConfigApplied, state)
	assert.Empty(t, diffs)

	state, diffs = CompareWorkBuddyTargets(installed, []string{"a", "b"})
	assert.Equal(t, ClientConfigOutdated, state)
	require.Len(t, diffs, 2)
	assert.Contains(t, diffs[1].Key, "~/.workbuddy-ai/models.json")
}

func TestApplyWorkBuddyConfig_NoEditionInstalled(t *testing.T) {
	home := wbSetHome(t)
	res, err := ApplyWorkBuddyConfig(wbBase, "tok", []string{"a"})
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "WorkBuddy not found")
	_, statErr := os.Stat(filepath.Join(home, ".workbuddy"))
	assert.True(t, os.IsNotExist(statErr), "no config folder is invented")
}

func TestApplyWorkBuddyConfig_OneBrokenEditionDoesNotBlockTheOther(t *testing.T) {
	home := wbSetHome(t, ".workbuddy", ".workbuddy-ai")
	cn := filepath.Join(home, ".workbuddy", "models.json")
	require.NoError(t, os.WriteFile(cn, []byte(`not json`), 0o600))

	res, err := ApplyWorkBuddyConfig(wbBase, "tok", []string{"a"})
	require.NoError(t, err)
	assert.False(t, res.Success, "overall fails when any edition fails")
	require.Len(t, res.Files, 2)
	assert.False(t, res.Files[0].Success)
	assert.True(t, res.Files[1].Success)
	assert.Equal(t, []string{"a"}, wbIDs(wbReadArray(t, filepath.Join(home, ".workbuddy-ai", "models.json"))))
}
