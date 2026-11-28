package readiness

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// healthServer serves the health path with the given headers/status.
func healthServer(t *testing.T, status int, headers map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != HealthPath {
			http.NotFound(w, r)
			return
		}
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// slowHealthServer answers health after delay.
func slowHealthServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != HealthPath {
			http.NotFound(w, r)
			return
		}
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// unusedPort returns a 127.0.0.1 port nothing is listening on right now.
func unusedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestWaitSlowStartWithinTimeout(t *testing.T) {
	srv := slowHealthServer(t, 300*time.Millisecond)

	start := time.Now()
	out := Wait(context.Background(), Options{URL: srv.URL + HealthPath, Timeout: 3 * time.Second})
	if out.State != StateReady {
		t.Fatalf("state = %s (%v), want ready", out.State, out.Err())
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("returned before the server answered: %v", elapsed)
	}
}

func TestWaitReachableButNotReady(t *testing.T) {
	srv := healthServer(t, http.StatusServiceUnavailable, nil)

	out := Wait(context.Background(), Options{URL: srv.URL + HealthPath, Timeout: 350 * time.Millisecond})
	if out.State != StateStillStarting {
		t.Fatalf("state = %s, want still_starting", out.State)
	}
	if out.Port != PortReachable {
		t.Errorf("port = %s, want reachable", out.Port)
	}
	if out.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", out.StatusCode)
	}
	var timeout *TimeoutError
	if !errors.As(out.Err(), &timeout) {
		t.Fatalf("err %v does not wrap TimeoutError", out.Err())
	}
	if !strings.Contains(out.Err().Error(), "reachable") || !strings.Contains(out.Err().Error(), "503") {
		t.Errorf("message should distinguish reachable-not-ready, got: %v", out.Err())
	}
}

func TestWaitPortUnreachable(t *testing.T) {
	url := "http://127.0.0.1:" + strconv.Itoa(unusedPort(t)) + HealthPath
	out := Wait(context.Background(), Options{URL: url, Timeout: 300 * time.Millisecond})
	if out.State != StateStillStarting {
		t.Fatalf("state = %s, want still_starting", out.State)
	}
	if out.Port != PortUnreachable {
		t.Errorf("port = %s, want unreachable", out.Port)
	}
	if !strings.Contains(out.Err().Error(), "not accepting connections") {
		t.Errorf("message should say the port is unreachable, got: %v", out.Err())
	}
}

func TestWaitEarlyExitFailsFastWithExitCode(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 3")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitErr := cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("setup: expected exit 3, got %v", waitErr)
	}

	exited := make(chan error, 1)
	exited <- waitErr

	url := "http://127.0.0.1:" + strconv.Itoa(unusedPort(t)) + HealthPath
	start := time.Now()
	out := Wait(context.Background(), Options{URL: url, Exited: exited, Timeout: time.Minute})
	if out.State != StateFailed {
		t.Fatalf("state = %s, want failed", out.State)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("early exit took %v to notice", time.Since(start))
	}
	var ex *ExitedError
	if !errors.As(out.Err(), &ex) {
		t.Fatalf("err %v does not wrap ExitedError", out.Err())
	}
	if code, ok := ex.ExitCode(); !ok || code != 3 {
		t.Errorf("exit code = %d, %v; want 3", code, ok)
	}
	if !strings.Contains(out.Err().Error(), "exit code 3") {
		t.Errorf("message should carry the exit code, got: %v", out.Err())
	}
}

// An older server still answering on the port must not be accepted as the
// instance the launcher just started.
func TestWaitRejectsOtherInstance(t *testing.T) {
	old := healthServer(t, http.StatusOK, map[string]string{constant.HealthInstanceHeader: "old"})

	out := Wait(context.Background(), Options{
		URL:           old.URL + HealthPath,
		Timeout:       350 * time.Millisecond,
		InstanceToken: "new",
	})
	if out.State == StateReady {
		t.Fatal("accepted an old instance as ready (the restart race)")
	}
	if !out.OtherInstance {
		t.Errorf("OtherInstance = false, want true")
	}
	if !strings.Contains(out.Err().Error(), "not from the instance") {
		t.Errorf("message should name the instance mismatch, got: %v", out.Err())
	}
}

func TestWaitAcceptsMatchingInstance(t *testing.T) {
	srv := healthServer(t, http.StatusOK, map[string]string{constant.HealthInstanceHeader: "new"})
	out := Wait(context.Background(), Options{URL: srv.URL + HealthPath, Timeout: time.Second, InstanceToken: "new"})
	if out.State != StateReady {
		t.Fatalf("state = %s (%v), want ready", out.State, out.Err())
	}
}

// The restart race in full: the old instance answers on the port first, then
// is replaced by the new one. The waiter must ignore the old instance and go
// ready only once the new token answers.
func TestWaitSurvivesRestartRace(t *testing.T) {
	var mu sync.Mutex
	instance := "old"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cur := instance
		mu.Unlock()
		w.Header().Set(constant.HealthServiceHeader, constant.HealthServiceName)
		w.Header().Set(constant.HealthInstanceHeader, cur)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	go func() {
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		instance = "new"
		mu.Unlock()
	}()

	start := time.Now()
	out := Wait(context.Background(), Options{URL: srv.URL + HealthPath, Timeout: 3 * time.Second, InstanceToken: "new"})
	if out.State != StateReady {
		t.Fatalf("state = %s (%v), want ready once the new instance answered", out.State, out.Err())
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("went ready at %v, before the new instance took over", elapsed)
	}
}

// Without an expected token (probing an already-running server) a bare 200 is
// accepted for backward compatibility, but an explicit foreign service name
// is rejected.
func TestWaitToleratesOldBuildButRejectsForeignService(t *testing.T) {
	bare := healthServer(t, http.StatusOK, nil)
	if out := Wait(context.Background(), Options{URL: bare.URL + HealthPath, Timeout: time.Second}); out.State != StateReady {
		t.Fatalf("bare 200: state = %s (%v), want ready", out.State, out.Err())
	}

	foreign := healthServer(t, http.StatusOK, map[string]string{constant.HealthServiceHeader: "nginx"})
	out := Wait(context.Background(), Options{URL: foreign.URL + HealthPath, Timeout: 250 * time.Millisecond})
	if out.State == StateReady {
		t.Fatal("accepted a foreign service as tingly-box")
	}
	if !strings.Contains(out.Err().Error(), "nginx") {
		t.Errorf("message should name the foreign service, got: %v", out.Err())
	}
}

func TestWaitContextCanceled(t *testing.T) {
	url := "http://127.0.0.1:" + strconv.Itoa(unusedPort(t)) + HealthPath
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	out := Wait(ctx, Options{URL: url, Timeout: time.Minute})
	if out.State != StateCanceled {
		t.Fatalf("state = %s, want canceled", out.State)
	}
	if !errors.Is(out.Err(), context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", out.Err())
	}
}

// A local readiness probe must never be routed through a proxy: an inherited
// HTTP_PROXY would otherwise make a healthy localhost/LAN server look dead.
func TestProbeTransportIgnoresProxy(t *testing.T) {
	if probeTransport.Proxy != nil {
		t.Fatal("probe transport honours HTTP_PROXY; local probes must bypass proxies")
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
		if got := ProbeURL(host, 12580); got != want {
			t.Errorf("ProbeURL(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestPortIsListening(t *testing.T) {
	srv := healthServer(t, http.StatusOK, nil)
	if !PortIsListening(srv.URL) {
		t.Errorf("PortIsListening(%s) = false, want true", srv.URL)
	}
	url := "http://127.0.0.1:" + strconv.Itoa(unusedPort(t)) + HealthPath
	if PortIsListening(url) {
		t.Errorf("PortIsListening(%s) = true, want false", url)
	}
}
