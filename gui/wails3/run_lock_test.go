package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/lock"
)

func newTestAppManager(t *testing.T, configDir string) *app.AppManager {
	t.Helper()
	return app.NewLazyAppManager(configDir, func(err error) { t.Fatalf("unexpected config init error: %v", err) })
}

// The first launch of the desktop app: no config directory exists yet, because
// the lock is taken before AppConfig is built. It must start, not fail with a
// missing-directory error (which used to surface as "Port Unavailable", or,
// after the takeover prompt landed, as a bogus "Tingly Box Is Already Running").
func TestAcquireSingleInstanceLock_FirstLaunchWithoutConfigDir(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "home", ".tingly-box")

	fileLock, err := acquireSingleInstanceLock(newTestAppManager(t, configDir))
	if err != nil {
		t.Fatalf("first launch failed: %v", err)
	}
	defer fileLock.Unlock()

	if info, statErr := os.Stat(configDir); statErr != nil || !info.IsDir() {
		t.Fatalf("config dir was not created: %v", statErr)
	}
}

// A lost race is reported as lock.ErrLocked: that is the signal to focus the
// running GUI or offer to take over a CLI server.
func TestAcquireSingleInstanceLock_HeldLockIsErrLocked(t *testing.T) {
	configDir := t.TempDir()
	first, err := acquireSingleInstanceLock(newTestAppManager(t, configDir))
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer first.Unlock()

	_, err = acquireSingleInstanceLock(newTestAppManager(t, configDir))
	if !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("second acquire error = %v; want lock.ErrLocked", err)
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("message should say an instance is already running, got: %v", err)
	}
}

// Anything else (here: the config dir cannot be created) is not a running
// instance. It must not look like one, or the user is offered to stop a process
// that does not exist.
func TestAcquireSingleInstanceLock_UnusableConfigDirIsNotErrLocked(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := acquireSingleInstanceLock(newTestAppManager(t, filepath.Join(blocker, ".tingly-box")))
	if err == nil {
		t.Fatal("expected an error when the config dir cannot be created")
	}
	if errors.Is(err, lock.ErrLocked) {
		t.Fatalf("an unusable config dir must not be reported as a running instance: %v", err)
	}
}
