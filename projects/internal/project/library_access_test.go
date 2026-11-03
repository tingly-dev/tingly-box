package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	return doc
}

func strs(t *testing.T, v any) []string {
	t.Helper()
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, ok := e.(string)
		require.True(t, ok)
		out = append(out, s)
	}
	return out
}

// requireLibraryGranted checks a settings document lets the session use the
// library without asking, and tells the auto-mode classifier about it.
func requireLibraryGranted(t *testing.T, doc map[string]any, library string) {
	t.Helper()
	perms := doc["permissions"].(map[string]any)
	assert.Contains(t, strs(t, perms["additionalDirectories"]), library)
	assert.Contains(t, strs(t, perms["allow"]), "Edit(/"+library+"/**)")
	assert.Contains(t, strs(t, perms["allow"]), "Read(/"+library+"/**)")
	auto := doc["autoMode"].(map[string]any)
	for _, key := range []string{"environment", "allow"} {
		rules := strs(t, auto[key])
		require.NotEmpty(t, rules)
		assert.Equal(t, "$defaults", rules[0], "autoMode.%s must keep the built-in rules", key)
		assert.Contains(t, rules[len(rules)-1], library)
	}
}

func TestProjectThreadSettingsGrantTheLibrary(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	thread := &db.Thread{ProjectID: f.project.ID, Cwd: t.TempDir(), Title: "Shots"}
	require.NoError(t, f.store.CreateThread(thread))
	library := f.threads.layout.LibraryDir(f.project.ID)

	require.NoError(t, f.threads.writeSettings(f.project, thread))
	requireLibraryGranted(t, readSettings(t, f.threads.layout.SettingsFile(f.project.ID, thread.ID)), library)

	// A launch profile's settings keep their own entries and gain the grant.
	host := filepath.Join(t.TempDir(), "profile.json")
	require.NoError(t, os.WriteFile(host, []byte(`{"permissions":{"additionalDirectories":["/srv/x"],"allow":["Bash(ls)"]},"autoMode":{"environment":["mine"]}}`), 0o600))
	path, err := f.threads.workerSettings(f.project, thread, host)
	require.NoError(t, err)
	doc := readSettings(t, path)
	assert.Contains(t, strs(t, doc["permissions"].(map[string]any)["additionalDirectories"]), library)
	assert.Contains(t, strs(t, doc["permissions"].(map[string]any)["allow"]), "Edit(/"+library+"/**)")
	perms := doc["permissions"].(map[string]any)
	assert.Contains(t, strs(t, perms["additionalDirectories"]), "/srv/x")
	assert.Contains(t, strs(t, perms["allow"]), "Bash(ls)")
	env := strs(t, doc["autoMode"].(map[string]any)["environment"])
	assert.Equal(t, "mine", env[0])
	assert.Contains(t, env[len(env)-1], library)
}

func TestFolderThreadSettingsGrantTheLibrary(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	layout := Layout{Root: t.TempDir()}
	folder := t.TempDir()
	p, err := store.OpenFolder(folder, "")
	require.NoError(t, err)
	thread := &db.Thread{ProjectID: p.ID, Cwd: folder, Title: "Shots"}
	require.NoError(t, store.CreateThread(thread))
	m := &Threads{layout: layout}

	path, err := m.sessionSettings(p, thread, "")
	require.NoError(t, err)
	requireLibraryGranted(t, readSettings(t, path), layout.LibraryDir(p.ID))
}

func TestSendFileAcceptsTheLibraryAndSaysWhereToSave(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	layout := Layout{Root: t.TempDir()}
	folder := t.TempDir()
	p, err := store.OpenFolder(folder, "")
	require.NoError(t, err)
	thread := &db.Thread{ProjectID: p.ID, Cwd: folder, Title: "Shots"}
	require.NoError(t, store.CreateThread(thread))
	svc := &Service{Store: store, Layout: layout, Hub: NewHub()}

	shot := filepath.Join(threadOutputDir(layout, p, thread), "page.png")
	require.NoError(t, os.MkdirAll(filepath.Dir(shot), 0o755))
	require.NoError(t, os.WriteFile(shot, []byte("png"), 0o600))
	_, err = svc.PublishFile(ctx, thread.ID, shot, "shot")
	require.NoError(t, err)

	tmp := filepath.Join(t.TempDir(), "shots.png")
	require.NoError(t, os.WriteFile(tmp, []byte("png"), 0o600))
	_, err = svc.PublishFile(ctx, thread.ID, tmp, "")
	require.ErrorContains(t, err, "outside")
	require.ErrorContains(t, err, threadOutputDir(layout, p, thread))

	server := &sessionToolsMCP{service: svc}
	t.Cleanup(func() { _ = server.close(ctx) })
	configPath, err := server.config(thread.ID)
	require.NoError(t, err)
	var config struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &config))
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: config.MCPServers["tingly-projects"].URL}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 1)
	assert.Contains(t, tools.Tools[0].Description, threadOutputDir(layout, p, thread))
	assert.DirExists(t, threadOutputDir(layout, p, thread))
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "send_file", Arguments: map[string]any{"path": shot}})
	require.NoError(t, err)
	require.False(t, result.IsError)
}

func requireTmpEnv(t *testing.T, env []string, dir string) {
	t.Helper()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		assert.Contains(t, env, k+"="+dir)
	}
	assert.DirExists(t, dir)
}

func TestThreadsGetATempDirInTheLibraryInEveryMode(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	work := filepath.Join(t.TempDir(), "work.json")
	require.NoError(t, os.WriteFile(work, []byte(`{"env":{"X":"profile"}}`), 0o600))
	f.threads.env = profiles{settings: work}
	library := f.threads.layout.LibraryDir(f.project.ID)

	// Project thread.
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "plain"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	dir := f.threads.layout.TmpDir(f.project.ID, th.ID)
	assert.Equal(t, filepath.Join(library, ".tmp", th.ID), dir)
	requireTmpEnv(t, f.agent.Launches()[0].Env, dir)

	// Project thread on a launch profile: the profile's env is kept.
	f.project.ThreadProfile = "work"
	require.NoError(t, f.store.SaveProject(f.project))
	th2, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "profile"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th2.ID, db.ThreadIdle)
	requireTmpEnv(t, f.agent.Launches()[1].Env, f.threads.layout.TmpDir(f.project.ID, th2.ID))

	// Folder thread.
	s, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "folder"})
	require.NoError(t, err)
	waitStatus(t, f.store, s.ID, db.ThreadIdle)
	requireTmpEnv(t, f.agent.Launches()[2].Env, f.threads.layout.TmpDir(s.ProjectID, s.ID))

	// Scratch files are not listed in the Library tab, send_file still reads them,
	// and resolving the thread removes the directory.
	shot := filepath.Join(dir, "shot.png")
	require.NoError(t, os.WriteFile(shot, []byte("png"), 0o600))
	files, err := f.threads.layout.ListLibrary(f.project.ID)
	require.NoError(t, err)
	for _, lf := range files {
		assert.NotContains(t, lf.Path, ".tmp")
	}
	svc := &Service{Store: f.store, Layout: f.threads.layout, Hub: NewHub()}
	_, err = svc.PublishFile(context.Background(), th.ID, shot, "")
	require.NoError(t, err)
	_, err = f.threads.Resolve(th.ID)
	require.NoError(t, err)
	assert.NoDirExists(t, dir)
	assert.DirExists(t, f.threads.layout.TmpDir(f.project.ID, th2.ID), "other threads keep theirs")
	uploads, err := f.threads.layout.ListLibrary(f.project.ID)
	require.NoError(t, err)
	var kept bool
	for _, u := range uploads {
		kept = kept || u.Origin == "upload" && filepath.Ext(u.Path) == ".png"
	}
	assert.True(t, kept, "a file sent from the temp dir was copied into uploads/ and survives cleanup")
}

func TestSetEnvReplacesExistingKeys(t *testing.T) {
	in := []string{"A=1", "TMPDIR=/tmp", "TMP=/tmp"}
	out := setEnv(in, "TMPDIR", "/x", "TMP", "/x", "TEMP", "/x")
	assert.Equal(t, []string{"A=1", "TMPDIR=/x", "TMP=/x", "TEMP=/x"}, out)
	assert.Equal(t, []string{"A=1", "TMPDIR=/tmp", "TMP=/tmp"}, in)
}
