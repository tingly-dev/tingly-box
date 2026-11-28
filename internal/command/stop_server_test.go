package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/lock"
)

// holdLock simulates a running server: the test process takes the
// single-instance lock, but the PID recorded in it (what stop signals) is a
// separate helper process. when that helper dies, the lock is released, which
// is what the OS does for a real server.
//
// The helper is always a process this test started, never a real Tingly Box.
func holdLock(t *testing.T, helper *exec.Cmd, lockDir string) (*lock.FileLock, *exec.Cmd) {
	t.Helper()
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() {
		if helper.Process != nil {
			_ = helper.Process.Kill()
		}
	})

	fl := lock.NewFileLock(lockDir)
	if err := fl.TryLock(); err != nil {
		t.Fatalf("take lock: %v", err)
	}
	// TryLock writes the test's own PID; point it at the helper so stop
	// signals the helper, not the test.
	lockPath := filepath.Join(lockDir, "tingly-server.lock")
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(helper.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	// Release the lock when the helper really exits, as a dying server would.
	done := make(chan struct{})
	go func() {
		_ = helper.Wait()
		_ = fl.Unlock()
		close(done)
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = fl.Unlock()
		}
	})
	return fl, helper
}

func shortenStopTimeouts(t *testing.T) {
	t.Helper()
	grace, force := stopGracePeriod, stopForcePeriod
	stopGracePeriod, stopForcePeriod = 500*time.Millisecond, 2*time.Second
	t.Cleanup(func() { stopGracePeriod, stopForcePeriod = grace, force })
}

// A graceful server (exits on SIGTERM) is stopped and the call returns once
// the lock is released — no force kill.
func TestStopServerGraceful(t *testing.T) {
	shortenStopTimeouts(t)
	dir := t.TempDir()
	fl, _ := holdLock(t, exec.Command("sleep", "60"), dir)

	start := time.Now()
	if err := stopServerWithFileLock(fl); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed > stopForcePeriod {
		t.Errorf("graceful stop took %v; it should not have waited for the force phase", elapsed)
	}
	if fl.IsLocked() {
		t.Error("lock still held after stop")
	}
}

// A server that ignores SIGTERM is force-killed; stop still returns success
// once the lock clears, and says it force-killed.
func TestStopServerForceKill(t *testing.T) {
	shortenStopTimeouts(t)
	dir := t.TempDir()
	// Ignore SIGTERM, so only SIGKILL ends it. The marker file tells us the
	// trap is installed before stop sends its first signal.
	readyFile := filepath.Join(t.TempDir(), "ready")
	helper := exec.Command("sh", "-c", `trap "" TERM; : > "$1"; while :; do sleep 1; done`, "sh", readyFile)
	fl, _ := holdLock(t, helper, dir)
	waitForFile(t, readyFile, 2*time.Second)

	start := time.Now()
	if err := stopServerWithFileLock(fl); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed < stopGracePeriod {
		t.Errorf("force path returned after %v, before the grace period %v", elapsed, stopGracePeriod)
	}
	if fl.IsLocked() {
		t.Error("lock still held after force stop")
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s not created within %v", path, timeout)
}

// Stopping when nothing holds the lock is a no-op, not an error.
func TestStopServerNotRunning(t *testing.T) {
	fl := lock.NewFileLock(t.TempDir())
	if err := stopServerWithFileLock(fl); err != nil {
		t.Fatalf("stop of a non-running server: %v", err)
	}
}
