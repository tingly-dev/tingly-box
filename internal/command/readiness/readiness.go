// Package readiness answers "can the server I just launched actually be
// reached yet?" for every entry point that starts one (start, restart, open,
// the TUI). A bound lock file or a forked process proves nothing: the URL a
// launcher prints is only honest once the HTTP health endpoint answers.
package readiness

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// HealthPath is the unauthenticated endpoint a ready server answers 200 on.
const HealthPath = "/api/v1/info/health"

// pollInterval is how often the health endpoint is probed.
const pollInterval = 100 * time.Millisecond

// ProbeURL returns the health URL for a server bound to host:port. A wildcard
// or empty host (the CLI binds every interface by default) is probed through
// localhost, since the wildcard address is not dialable on every platform.
func ProbeURL(host string, port int) string {
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + HealthPath
}

// TimeoutError means the server never answered the health endpoint.
type TimeoutError struct {
	URL     string
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("server did not answer %s within %s", e.URL, e.Timeout)
}

// ExitedError means the server process ended before it became ready. Err is
// the process's exit status or the error its Start returned (may be nil).
type ExitedError struct {
	Err error
}

func (e *ExitedError) Error() string {
	if e.Err == nil {
		return "server exited before it became ready"
	}
	return fmt.Sprintf("server exited before it became ready: %v", e.Err)
}

func (e *ExitedError) Unwrap() error { return e.Err }

// Wait polls url until it answers 200 and returns nil. It gives up with a
// *TimeoutError after timeout, with an *ExitedError as soon as exited yields
// (pass nil if there is no process to watch), or with ctx's error.
func Wait(ctx context.Context, url string, exited <-chan error, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

	for {
		if probe(ctx, client, url) {
			return nil
		}
		select {
		case err := <-exited:
			return &ExitedError{Err: err}
		case <-deadline.C:
			return &TimeoutError{URL: url, Timeout: timeout}
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func probe(ctx context.Context, client *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
