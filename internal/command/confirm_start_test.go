package command

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/command/readiness"
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
func TestConfirmStartedAnnouncesOnlyAfterReady(t *testing.T) {
	port := freePort(t)
	bound := serveHealthAfter(t, port, 400*time.Millisecond)

	var mu sync.Mutex
	var announcedAt time.Time
	err := confirmStarted(readiness.ProbeURL("127.0.0.1", port), nil, 5*time.Second, "/tmp/x.log", func() {
		mu.Lock()
		announcedAt = time.Now()
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("confirmStarted: %v", err)
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

// A server that never comes up must produce an error carrying the log path and
// must not announce anything.
func TestConfirmStartedTimeoutDoesNotAnnounce(t *testing.T) {
	port := freePort(t)
	announced := false
	err := confirmStarted(readiness.ProbeURL("127.0.0.1", port), nil, 300*time.Millisecond, "/var/log/tb.log", func() { announced = true })
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
	if !strings.Contains(err.Error(), "/var/log/tb.log") {
		t.Errorf("error should point at the log file, got: %v", err)
	}
}

// A process that dies during startup fails the launch right away (well before
// the timeout) and reports why.
func TestConfirmStartedEarlyExitFailsFast(t *testing.T) {
	port := freePort(t)
	exited := make(chan error, 1)
	exited <- errors.New("exit status 1")

	announced := false
	start := time.Now()
	err := confirmStarted(readiness.ProbeURL("127.0.0.1", port), exited, time.Minute, "/var/log/tb.log", func() { announced = true })
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
	if msg := err.Error(); !strings.Contains(msg, "exit status 1") || !strings.Contains(msg, "/var/log/tb.log") {
		t.Errorf("error should carry the exit status and log path, got: %v", err)
	}
}

func TestProbeURL(t *testing.T) {
	for host, want := range map[string]string{
		"":          "http://localhost:12580/api/v1/info/health",
		"0.0.0.0":   "http://localhost:12580/api/v1/info/health",
		"localhost": "http://localhost:12580/api/v1/info/health",
		"10.1.2.3":  "http://10.1.2.3:12580/api/v1/info/health",
		"::1":       "http://[::1]:12580/api/v1/info/health",
	} {
		if got := readiness.ProbeURL(host, 12580); got != want {
			t.Errorf("ProbeURL(%q) = %q, want %q", host, got, want)
		}
	}
}
