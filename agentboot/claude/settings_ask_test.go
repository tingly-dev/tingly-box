package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
)

func decodeSettings(t *testing.T, s string) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &doc))
	return doc
}

func askListOf(t *testing.T, doc map[string]any) []any {
	t.Helper()
	perms, ok := doc["permissions"].(map[string]any)
	require.True(t, ok, "permissions must be an object")
	list, ok := perms["ask"].([]any)
	require.True(t, ok, "permissions.ask must be a list")
	return list
}

func TestEnsureInteractiveAskRules_EmptyBase(t *testing.T) {
	out := ensureInteractiveAskRules("")
	doc := decodeSettings(t, out)
	assert.Equal(t, []any{"AskUserQuestion"}, askListOf(t, doc))
}

func TestEnsureInteractiveAskRules_MergesFilePreservingKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := `{
		"defaultMode": "acceptEdits",
		"env": {"FOO": "bar"},
		"permissions": {"allow": ["Read"], "ask": ["WebSearch"]}
	}`
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	out := ensureInteractiveAskRules(path)
	require.NotEqual(t, path, out, "must return merged inline JSON")

	doc := decodeSettings(t, out)
	assert.Equal(t, "acceptEdits", doc["defaultMode"])
	assert.Equal(t, map[string]any{"FOO": "bar"}, doc["env"])
	perms := doc["permissions"].(map[string]any)
	assert.Equal(t, []any{"Read"}, perms["allow"])
	assert.Equal(t, []any{"WebSearch", "AskUserQuestion"}, perms["ask"])

	// The user's file itself is never modified.
	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, original, string(onDisk))
}

func TestEnsureInteractiveAskRules_InlineJSONBase(t *testing.T) {
	out := ensureInteractiveAskRules(`{"defaultMode":"plan"}`)
	doc := decodeSettings(t, out)
	assert.Equal(t, "plan", doc["defaultMode"])
	assert.Equal(t, []any{"AskUserQuestion"}, askListOf(t, doc))
}

func TestEnsureInteractiveAskRules_AlreadyPresent_Unchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"permissions":{"ask":["AskUserQuestion"]}}`), 0o644))

	assert.Equal(t, path, ensureInteractiveAskRules(path))
}

func TestEnsureInteractiveAskRules_FailuresPassThrough(t *testing.T) {
	// Missing file.
	missing := filepath.Join(t.TempDir(), "nope.json")
	assert.Equal(t, missing, ensureInteractiveAskRules(missing))

	// Malformed JSON.
	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{not json"), 0o644))
	assert.Equal(t, bad, ensureInteractiveAskRules(bad))

	// Unexpected shapes.
	assert.Equal(t, `{"permissions":[]}`, ensureInteractiveAskRules(`{"permissions":[]}`))
	assert.Equal(t, `{"permissions":{"ask":"x"}}`, ensureInteractiveAskRules(`{"permissions":{"ask":"x"}}`))
}

// --- driver integration -----------------------------------------------------

func settingsArgValues(args []string) []string {
	var vals []string
	for i, a := range args {
		if a == "--settings" && i+1 < len(args) {
			vals = append(vals, args[i+1])
		}
	}
	return vals
}

func TestBuildArgs_StreamJSON_InjectsAskRule(t *testing.T) {
	d := NewDriver(DefaultConfig())

	args, err := d.buildArgs(agentboot.OutputFormatStreamJSON, "hi",
		agentboot.ExecutionOptions{}, d.config, false)
	require.NoError(t, err)

	vals := settingsArgValues(args)
	require.Len(t, vals, 1)
	doc := decodeSettings(t, vals[0])
	assert.Equal(t, []any{"AskUserQuestion"}, askListOf(t, doc))
}

func TestBuildArgs_StreamJSON_MergesProfileSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"defaultMode":"acceptEdits"}`), 0o644))

	d := NewDriver(DefaultConfig())
	args, err := d.buildArgs(agentboot.OutputFormatStreamJSON, "hi",
		agentboot.ExecutionOptions{SettingsPath: path}, d.config, false)
	require.NoError(t, err)

	vals := settingsArgValues(args)
	require.Len(t, vals, 1, "merged document must replace the profile path")
	doc := decodeSettings(t, vals[0])
	assert.Equal(t, "acceptEdits", doc["defaultMode"])
	assert.Equal(t, []any{"AskUserQuestion"}, askListOf(t, doc))
}

func TestBuildArgs_TextFormat_NoInjection(t *testing.T) {
	d := NewDriver(DefaultConfig())
	args, err := d.buildArgs(agentboot.OutputFormatText, "hi",
		agentboot.ExecutionOptions{}, d.config, false)
	require.NoError(t, err)
	assert.Empty(t, settingsArgValues(args))
}
