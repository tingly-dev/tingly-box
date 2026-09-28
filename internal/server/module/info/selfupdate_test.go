package info

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUpdater returns an npm install that is writable and unsupervised; tests
// override single hooks from there.
func fakeUpdater(t *testing.T) (*updater, *[][]string, *[]string) {
	t.Helper()
	var cmds [][]string
	var spawned []string
	u := &updater{
		launchSource: "npm",
		args:         []string{"--source=npm", "start", "--host", "0.0.0.0", "--port", "12580"},
		getenv:       func(string) string { return "" },
		fileExists:   func(string) bool { return false },
		runCmd: func(_ context.Context, name string, args ...string) ([]byte, error) {
			cmds = append(cmds, append([]string{name}, args...))
			switch strings.Join(args, " ") {
			case "prefix -g":
				return []byte("/opt/npm\n"), nil
			case "root -g":
				return []byte("/opt/npm/lib/node_modules\n"), nil
			}
			return []byte("changed 2 packages"), nil
		},
		writable: func(string) bool { return true },
		spawn: func(name string, args ...string) error {
			spawned = append([]string{name}, args...)
			return nil
		},
		after: func(_ time.Duration, f func()) { f() },
	}
	return u, &cmds, &spawned
}

func TestUpdaterSupport(t *testing.T) {
	ctx := context.Background()

	for _, source := range []string{"npx", "npx-bundle", "npm-bundle", "binary", ""} {
		u, _, _ := fakeUpdater(t)
		u.launchSource = source
		sup, _ := u.support(ctx)
		assert.False(t, sup.Supported, source)
		assert.NotEmpty(t, sup.Reason, source)
	}

	t.Run("container", func(t *testing.T) {
		u, _, _ := fakeUpdater(t)
		u.fileExists = func(path string) bool { return path == "/.dockerenv" }
		sup, _ := u.support(ctx)
		assert.False(t, sup.Supported)
		assert.Contains(t, sup.Reason, "container")
	})

	t.Run("npm missing", func(t *testing.T) {
		u, _, _ := fakeUpdater(t)
		u.runCmd = func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("not found") }
		sup, _ := u.support(ctx)
		assert.False(t, sup.Supported)
	})

	t.Run("global dir not writable", func(t *testing.T) {
		u, _, _ := fakeUpdater(t)
		u.writable = func(dir string) bool { return !strings.HasSuffix(dir, "node_modules") }
		sup, _ := u.support(ctx)
		assert.False(t, sup.Supported)
		assert.Contains(t, sup.Reason, "node_modules")
	})

	t.Run("writable npm install", func(t *testing.T) {
		u, _, _ := fakeUpdater(t)
		sup, prefix := u.support(ctx)
		assert.True(t, sup.Supported)
		assert.False(t, sup.Supervised)
		assert.Equal(t, "/opt/npm", prefix)
	})

	for _, env := range []string{"INVOCATION_ID", "pm_id"} {
		t.Run("supervised by "+env, func(t *testing.T) {
			u, _, _ := fakeUpdater(t)
			u.getenv = func(key string) string {
				if key == env {
					return "1"
				}
				return ""
			}
			sup, _ := u.support(ctx)
			assert.True(t, sup.Supported)
			assert.True(t, sup.Supervised)
		})
	}
}

func TestUpdaterUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("unsupervised installs and restarts through the new launcher", func(t *testing.T) {
		u, cmds, spawned := fakeUpdater(t)
		result, out, err := u.update(ctx, "0.261001.1")
		require.NoError(t, err)
		assert.Equal(t, "changed 2 packages", out)
		assert.Equal(t, SelfUpdateResult{Version: "0.261001.1", Restarting: true}, result)
		assert.Contains(t, *cmds, []string{npmExecutable(), "install", "-g", "tingly-box@0.261001.1"})
		require.NotEmpty(t, *spawned)
		assert.Equal(t, npmLauncher("/opt/npm"), (*spawned)[0])
		assert.Equal(t, []string{"--source=npm", "restart", "--host", "0.0.0.0", "--port", "12580"}, (*spawned)[1:])
	})

	t.Run("supervised installs but leaves the restart to the service manager", func(t *testing.T) {
		u, _, spawned := fakeUpdater(t)
		u.getenv = func(key string) string {
			if key == "INVOCATION_ID" {
				return "abc"
			}
			return ""
		}
		result, _, err := u.update(ctx, "0.261001.1")
		require.NoError(t, err)
		assert.Equal(t, SelfUpdateResult{Version: "0.261001.1", RestartRequired: true}, result)
		assert.Empty(t, *spawned)
	})

	t.Run("npm failure returns its output and does not restart", func(t *testing.T) {
		u, _, spawned := fakeUpdater(t)
		base := u.runCmd
		u.runCmd = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "install" {
				return []byte("npm error EACCES"), errors.New("exit status 243")
			}
			return base(ctx, name, args...)
		}
		_, out, err := u.update(ctx, "0.261001.1")
		require.Error(t, err)
		assert.Equal(t, "npm error EACCES", out)
		assert.Empty(t, *spawned)
	})

	t.Run("unsupported install is refused", func(t *testing.T) {
		u, cmds, _ := fakeUpdater(t)
		u.launchSource = "npx"
		_, _, err := u.update(ctx, "0.261001.1")
		var refused *refusedError
		require.ErrorAs(t, err, &refused)
		assert.Empty(t, *cmds, "nothing may be installed")
	})

	t.Run("concurrent update is refused", func(t *testing.T) {
		u, _, _ := fakeUpdater(t)
		u.running = true
		_, _, err := u.update(ctx, "0.261001.1")
		var refused *refusedError
		require.ErrorAs(t, err, &refused)
	})
}

func TestDirWritable(t *testing.T) {
	dir := t.TempDir()
	assert.True(t, dirWritable(dir))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the probe file must be cleaned up")
	assert.False(t, dirWritable(filepath.Join(dir, "missing")))
}

func TestRestartArgsFrom(t *testing.T) {
	assert.Equal(t, []string{"--source=npm", "restart", "--no-daemon"}, restartArgsFrom([]string{"--source=npm", "start", "--no-daemon"}))
	assert.Equal(t, []string{"--verbose", "restart"}, restartArgsFrom([]string{"--verbose"}))
}

func TestApplyUpdateHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	serve := func(h *Handler, method, path string, handle gin.HandlerFunc) (int, map[string]any) {
		r := gin.New()
		r.Handle(method, path, handle)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		return w.Code, body
	}
	newHandler := func(current, latest string) *Handler {
		u, _, _ := fakeUpdater(t)
		return &Handler{
			version:      current,
			launchSource: "npm",
			latest:       func() (string, string, error) { return latest, GithubReleases, nil },
			updater:      u,
		}
	}

	t.Run("already latest", func(t *testing.T) {
		h := newHandler("0.261001.1", "0.261001.1")
		code, _ := serve(h, http.MethodPost, "/update", h.ApplyUpdate)
		assert.Equal(t, http.StatusConflict, code)
	})

	t.Run("unsupported install", func(t *testing.T) {
		h := newHandler("0.260924.1", "0.261001.1")
		h.updater.launchSource = "npx"
		code, body := serve(h, http.MethodPost, "/update", h.ApplyUpdate)
		assert.Equal(t, http.StatusConflict, code)
		assert.Contains(t, body["error"], "npx")
	})

	t.Run("applies the update", func(t *testing.T) {
		h := newHandler("0.260924.1", "0.261001.1")
		code, body := serve(h, http.MethodPost, "/update", h.ApplyUpdate)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, true, body["success"])
		assert.Equal(t, map[string]any{"version": "0.261001.1", "restarting": true, "restart_required": false}, body["data"])
	})

	t.Run("version check reports self-update support only with an update", func(t *testing.T) {
		h := newHandler("0.260924.1", "0.261001.1")
		_, body := serve(h, http.MethodGet, "/check", h.GetLatestVersion)
		data := body["data"].(map[string]any)
		assert.Equal(t, map[string]any{"supported": true, "supervised": false}, data["self_update"])

		h = newHandler("0.261001.1", "0.261001.1")
		_, body = serve(h, http.MethodGet, "/check", h.GetLatestVersion)
		assert.NotContains(t, body["data"].(map[string]any), "self_update")
	})
}
