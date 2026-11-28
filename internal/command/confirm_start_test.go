package command

import (
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/command/readiness"
	"github.com/tingly-dev/tingly-box/internal/constant"
)

// freePort returns a port nothing is listening on right now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// serveHealthAfter starts answering the health endpoint on port after delay
// and returns the instant the listener was bound.
func serveHealthAfter(t *testing.T, port int, delay time.Duration) <-chan time.Time {
	t.Helper()
	bound := make(chan time.Time, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == readiness.HealthPath {
			w.Header().Set(constant.HealthServiceHeader, constant.HealthServiceName)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})}
	t.Cleanup(func() { _ = srv.Close() })
	go func() {
		time.Sleep(delay)
		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			return
		}
		bound <- time.Now()
		_ = srv.Serve(l)
	}()
	return bound
}

// The access info (URL, token) must not be announced while the server is still
// starting: with a delayed listener, announce runs only after the listener is
// bound and answering.
func TestConfirmAnnouncesOnlyAfterReady(t *testing.T) {
	port := freePort(t)
	bound := serveHealthAfter(t, port, 400*time.Millisecond)

	var mu sync.Mutex
	var announcedAt time.Time
	err := startWait{
		ProbeURL: readiness.ProbeURL("127.0.0.1", port),
		Timeout:  5 * time.Second,
		LogPath:  filepath.Join(t.TempDir(), "x.log"),
	}.confirm(func() {
		mu.Lock()
		announcedAt = time.Now()
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if announcedAt.IsZero() {
		t.Fatal("announce was never called")
	}
	select {
	case at := <-bound:
		if announcedAt.Before(at) {
			t.Fatalf("announced at %v, before the server was listening at %v", announcedAt, at)
		}
	default:
		t.Fatal("announced before the server ever listened")
	}
}

// A server that never comes up must produce a "still starting" error carrying
// the log path, and must not announce anything.
func TestConfirmTimeoutReportsStillStarting(t *testing.T) {
	port := freePort(t)
	logPath := filepath.Join(t.TempDir(), "tb.log")
	if err := os.WriteFile(logPath, []byte("line one\nline two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	announced := false
	err := startWait{
		ProbeURL: readiness.ProbeURL("127.0.0.1", port),
		Timeout:  300 * time.Millisecond,
		LogPath:  logPath,
		PID:      4242,
	}.confirm(func() { announced = true })
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if announced {
		t.Fatal("announced although the server never became reachable")
	}
	var timeout *readiness.TimeoutError
	if !errors.As(err, &timeout) {
		t.Errorf("error %v does not wrap a TimeoutError", err)
	}
	msg := err.Error()
	for _, want := range []string{"still starting", logPath, "4242", "line two"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got: %v", want, msg)
		}
	}
}

// A process that dies during startup fails the launch right away (well before
// the timeout) and reports the exit code and recent log.
func TestConfirmEarlyExitFailsFastWithLogTail(t *testing.T) {
	port := freePort(t)
	cmd := exec.Command("sh", "-c", "exit 3")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitErr := cmd.Wait()
	exited := make(chan error, 1)
	exited <- waitErr

	logPath := filepath.Join(t.TempDir(), "tb.log")
	if err := os.WriteFile(logPath, []byte("boot: migrating\nboot: fatal: boom\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	announced := false
	start := time.Now()
	err := startWait{
		ProbeURL: readiness.ProbeURL("127.0.0.1", port),
		Exited:   exited,
		Timeout:  time.Minute,
		LogPath:  logPath,
	}.confirm(func() { announced = true })
	if err == nil {
		t.Fatal("expected an error for a server that exited early")
	}
	if announced {
		t.Fatal("announced although the server exited")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("early exit took %v to be noticed", time.Since(start))
	}
	var ex *readiness.ExitedError
	if !errors.As(err, &ex) {
		t.Errorf("error %v does not wrap an ExitedError", err)
	}
	msg := err.Error()
	for _, want := range []string{"exit code 3", logPath, "boot: fatal: boom"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got: %v", want, msg)
		}
	}
}

// With KeepWaiting, a slow start past the initial timeout keeps waiting until
// the server actually becomes ready instead of failing.
func TestConfirmKeepWaitingPastTimeout(t *testing.T) {
	port := freePort(t)
	ready := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-ready:
			w.Header().Set(constant.HealthServiceHeader, constant.HealthServiceName)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = srv.Close()
		_ = l.Close()
	})
	go func() { _ = srv.Serve(l) }()

	go func() {
		time.Sleep(400 * time.Millisecond)
		close(ready)
	}()

	announced := false
	err = startWait{
		ProbeURL:    readiness.ProbeURL("127.0.0.1", port),
		Timeout:     150 * time.Millisecond,
		KeepWaiting: true,
		LogPath:     filepath.Join(t.TempDir(), "tb.log"),
	}.confirm(func() { announced = true })
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !announced {
		t.Fatal("never announced after the server became ready")
	}
}

// The instance token comes from the environment for a daemon child; a restart
// passes a fresh one so an old instance cannot satisfy the probe.
func TestConfirmRejectsOldInstanceOnRestart(t *testing.T) {
	port := freePort(t)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(constant.HealthServiceHeader, constant.HealthServiceName)
		w.Header().Set(constant.HealthInstanceHeader, "old")
		w.WriteHeader(http.StatusOK)
	})}
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = srv.Close()
		_ = l.Close()
	})
	go func() { _ = srv.Serve(l) }()

	err = startWait{
		ProbeURL: readiness.ProbeURL("127.0.0.1", port),
		Token:    "new",
		Timeout:  300 * time.Millisecond,
		LogPath:  filepath.Join(t.TempDir(), "tb.log"),
	}.confirm(func() { t.Fatal("announced an old instance") })
	if err == nil {
		t.Fatal("expected the old instance to be rejected")
	}
	if !strings.Contains(err.Error(), "not from the instance") {
		t.Errorf("error should name the instance mismatch, got: %v", err)
	}
}

func TestResolveStartTimeout(t *testing.T) {
	t.Setenv(startTimeoutEnv, "")
	if got := resolveStartTimeout(0); got != defaultStartReadyTimeout {
		t.Errorf("default = %v, want %v", got, defaultStartReadyTimeout)
	}
	if got := resolveStartTimeout(7 * time.Second); got != 7*time.Second {
		t.Errorf("flag = %v, want 7s", got)
	}
	t.Setenv(startTimeoutEnv, "90s")
	if got := resolveStartTimeout(0); got != 90*time.Second {
		t.Errorf("env = %v, want 90s", got)
	}
	t.Setenv(startTimeoutEnv, "not-a-duration")
	if got := resolveStartTimeout(0); got != defaultStartReadyTimeout {
		t.Errorf("bad env = %v, want default", got)
	}
}

func TestLogTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tail := logTail(path, 3)
	want := []string{"c", "d", "e"}
	if strings.Join(tail, ",") != strings.Join(want, ",") {
		t.Errorf("tail = %v, want %v", tail, want)
	}
	if got := logTail(filepath.Join(dir, "missing.log"), 3); got != nil {
		t.Errorf("missing file tail = %v, want nil", got)
	}
}
