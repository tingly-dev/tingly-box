// Package readiness answers "can the server I just launched actually be
// reached yet?" for every entry point that starts one (start, restart, open,
// the TUI). A bound lock file or a forked process proves nothing: the URL a
// launcher prints is only honest once the HTTP health endpoint answers.
//
// The wait is a small state machine:
//
//	starting --(health 200 from *this* instance)--> ready
//	starting --(the process ended)---------------> failed
//	starting --(total timeout, process alive)----> still starting
//
// Every observation is classified so a timeout can say *why* it did not
// become ready: the port never accepted a connection, the port accepted but
// the health endpoint was not 200, or the port was answered by a different
// instance than the one being started.
package readiness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// HealthPath is the unauthenticated endpoint a ready server answers 200 on.
const HealthPath = "/api/v1/info/health"

const (
	// DefaultPollInterval is how often the health endpoint is probed.
	DefaultPollInterval = 100 * time.Millisecond
	// DefaultRequestTimeout bounds a single health request. It is short so a
	// single slow/hung probe cannot hide an early process exit for long.
	DefaultRequestTimeout = time.Second
	// dialTimeout bounds the side-check that classifies a failed probe as
	// "port unreachable" versus "port reachable but not ready".
	dialTimeout = 500 * time.Millisecond
)

// State is the outcome of a readiness wait.
type State string

const (
	// StateReady means the health endpoint answered 200 for this instance.
	StateReady State = "ready"
	// StateFailed means the process ended before it became ready.
	StateFailed State = "failed"
	// StateStillStarting means the total timeout elapsed while the process was
	// still alive (it may yet become ready).
	StateStillStarting State = "still_starting"
	// StateCanceled means the caller's context ended first.
	StateCanceled State = "canceled"
)

// PortState classifies the last probe's view of the port.
type PortState string

const (
	// PortUntested means no probe completed.
	PortUntested PortState = "untested"
	// PortUnreachable means nothing accepted a TCP connection on the port.
	PortUnreachable PortState = "unreachable"
	// PortReachable means the port accepted a connection, but the health
	// endpoint did not answer 200 for this instance.
	PortReachable PortState = "reachable"
)

// Options configures Wait.
type Options struct {
	// URL is the health endpoint URL (see ProbeURL).
	URL string
	// Timeout bounds the whole wait. Zero or negative means "no total
	// timeout": wait until ready, the process exits, or ctx is cancelled.
	Timeout time.Duration
	// PollInterval defaults to DefaultPollInterval.
	PollInterval time.Duration
	// RequestTimeout bounds a single request, defaulting to
	// DefaultRequestTimeout.
	RequestTimeout time.Duration
	// Exited, when non-nil, yields the process result when the process being
	// waited for ends. A closed channel is treated as "exited, reason
	// unknown". Pass nil when there is no process to watch (e.g. probing an
	// already-running server).
	Exited <-chan error
	// InstanceToken, when set, is the value the server must echo in the
	// constant.HealthInstanceHeader response header. It lets a restart
	// distinguish the instance it just started from an older server that is
	// still shutting down and answering on the same port. Empty accepts any
	// tingly-box health response.
	InstanceToken string
}

// Outcome is what Wait observed. Err renders it as an error.
type Outcome struct {
	State   State
	URL     string
	Elapsed time.Duration
	Timeout time.Duration
	// Port is the last observed port state.
	Port PortState
	// StatusCode is the last HTTP status seen (0 when no response).
	StatusCode int
	// LastErr is the last probe error (nil when the last probe got HTTP).
	LastErr error
	// ExitErr is the process result when State is StateFailed.
	ExitErr error
	// OtherInstance reports that the port answered 200 but was not the
	// instance being waited for (a restart race with the old server).
	OtherInstance bool
	// CtxErr is set when State is StateCanceled.
	CtxErr error
}

// Err returns nil for StateReady, otherwise a typed error describing the
// outcome. Timeouts wrap *TimeoutError, process exits wrap *ExitedError.
func (o Outcome) Err() error {
	switch o.State {
	case StateReady:
		return nil
	case StateFailed:
		return &ExitedError{Err: o.ExitErr, Elapsed: o.Elapsed}
	case StateCanceled:
		if o.CtxErr != nil {
			return o.CtxErr
		}
		return context.Canceled
	default:
		return &TimeoutError{
			URL:           o.URL,
			Timeout:       o.Timeout,
			Elapsed:       o.Elapsed,
			Port:          o.Port,
			StatusCode:    o.StatusCode,
			LastErr:       o.LastErr,
			OtherInstance: o.OtherInstance,
		}
	}
}

// Reason explains the outcome's last observation in user terms.
func (o Outcome) Reason() string {
	switch o.State {
	case StateReady:
		return "ready"
	case StateFailed:
		return o.Err().Error()
	case StateCanceled:
		return "cancelled"
	}
	return (&TimeoutError{
		URL:           o.URL,
		Timeout:       o.Timeout,
		Elapsed:       o.Elapsed,
		Port:          o.Port,
		StatusCode:    o.StatusCode,
		LastErr:       o.LastErr,
		OtherInstance: o.OtherInstance,
	}).reason()
}

// TimeoutError means the server never answered the health endpoint with 200
// for this instance before the total timeout.
type TimeoutError struct {
	URL           string
	Timeout       time.Duration
	Elapsed       time.Duration
	Port          PortState
	StatusCode    int
	LastErr       error
	OtherInstance bool
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("server did not become ready at %s within %s (%s)", e.URL, e.Timeout, e.reason())
}

// reason explains the last observed state in user terms.
func (e *TimeoutError) reason() string {
	switch e.Port {
	case PortUnreachable:
		return "the port is not accepting connections"
	case PortReachable:
		switch {
		case e.OtherInstance:
			return "the port answers, but not from the instance that was just started — an older server may still be shutting down or another process owns the port"
		case e.StatusCode == http.StatusOK && e.LastErr != nil:
			return fmt.Sprintf("the port answered 200, but not with a Tingly Box health response: %v", e.LastErr)
		case e.StatusCode != 0:
			return fmt.Sprintf("the port is reachable but the health endpoint returned HTTP %d — something else (a proxy, an old build, or another service) may be answering", e.StatusCode)
		case e.LastErr != nil:
			return fmt.Sprintf("the port is reachable but the health request failed: %v", e.LastErr)
		}
		return "the port is reachable but the health endpoint is not answering 200"
	default:
		return "the server never accepted a connection"
	}
}

func (e *TimeoutError) Unwrap() error { return e.LastErr }

// NewInstanceToken returns a random token a launcher hands to the server it
// starts and then requires back on the health response.
func NewInstanceToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is not recoverable, but a token is only a
		// same-process handshake; fall back to a value with the same shape.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

// ExitedError means the server process ended before it became ready. Err is
// the process's exit status or the error its Start returned (may be nil).
type ExitedError struct {
	Err     error
	Elapsed time.Duration
}

// ExitCode returns the process exit code when Err is an *exec.ExitError.
func (e *ExitedError) ExitCode() (int, bool) {
	var ee *exec.ExitError
	if errors.As(e.Err, &ee) {
		return ee.ExitCode(), true
	}
	return 0, false
}

func (e *ExitedError) Error() string {
	if code, ok := e.ExitCode(); ok {
		return fmt.Sprintf("server exited before it became ready (exit code %d)", code)
	}
	if e.Err == nil {
		return "server exited before it became ready"
	}
	return fmt.Sprintf("server exited before it became ready: %v", e.Err)
}

func (e *ExitedError) Unwrap() error { return e.Err }

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

// Wait polls the health endpoint and returns the outcome. It returns as soon
// as the endpoint answers 200 for this instance, as soon as the watched
// process exits, at the total timeout, or when ctx ends — whichever first.
func Wait(ctx context.Context, opts Options) Outcome {
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = DefaultRequestTimeout
	}

	start := time.Now()
	p := prober{
		url:            opts.URL,
		token:          opts.InstanceToken,
		requestTimeout: opts.RequestTimeout,
		client:         newProbeClient(opts.RequestTimeout),
	}
	last := attempt{port: PortUntested}
	var deadline time.Time
	if opts.Timeout > 0 {
		deadline = start.Add(opts.Timeout)
	}

	for {
		remaining := time.Duration(0)
		if !deadline.IsZero() {
			remaining = time.Until(deadline)
			if remaining <= 0 {
				return outcome(opts, last, start, StateStillStarting, time.Now())
			}
		}
		last = p.probe(ctx, remaining)

		if last.ready {
			return outcome(opts, last, start, StateReady, time.Now())
		}
		if err := ctx.Err(); err != nil {
			out := outcome(opts, last, start, StateCanceled, time.Now())
			out.CtxErr = err
			return out
		}

		wait := opts.PollInterval
		if !deadline.IsZero() {
			if rem := time.Until(deadline); rem < wait {
				wait = rem
			}
		}
		if wait <= 0 {
			return outcome(opts, last, start, StateStillStarting, time.Now())
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			out := outcome(opts, last, start, StateCanceled, time.Now())
			out.CtxErr = ctx.Err()
			return out
		case err := <-opts.Exited:
			timer.Stop()
			out := outcome(opts, last, start, StateFailed, time.Now())
			out.ExitErr = err
			return out
		case <-timer.C:
		}
	}
}

func outcome(opts Options, a attempt, start time.Time, state State, now time.Time) Outcome {
	return Outcome{
		State:         state,
		URL:           opts.URL,
		Elapsed:       now.Sub(start),
		Timeout:       opts.Timeout,
		Port:          a.port,
		StatusCode:    a.statusCode,
		LastErr:       a.err,
		OtherInstance: a.otherInstance,
	}
}

type attempt struct {
	ready         bool
	port          PortState
	statusCode    int
	err           error
	otherInstance bool
}

// prober holds the per-wait client. The transport deliberately ignores
// HTTP_PROXY/HTTPS_PROXY: a local readiness probe must never be routed through
// a proxy, which would make a healthy server look dead (or a proxy error look
// like a crash). Keep-alives are off so an attempt never reuses a connection
// to an instance that has just been replaced.
type prober struct {
	url            string
	token          string
	requestTimeout time.Duration
	client         *http.Client
}

var probeTransport = &http.Transport{
	Proxy:             nil,
	DisableKeepAlives: true,
	DialContext: (&net.Dialer{
		Timeout: dialTimeout,
	}).DialContext,
}

func newProbeClient(requestTimeout time.Duration) *http.Client {
	return &http.Client{Timeout: requestTimeout, Transport: probeTransport}
}

func (p *prober) probe(ctx context.Context, remaining time.Duration) attempt {
	timeout := p.requestTimeout
	if remaining > 0 && remaining < timeout {
		timeout = remaining
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The client timeout covers the request; reqCtx additionally lets the
	// deadline cancel an in-flight probe instead of waiting it out.
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, p.url, nil)
	if err != nil {
		return attempt{port: PortUntested, err: err}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return attempt{port: classifyPort(p.url), err: err}
	}
	defer resp.Body.Close()

	a := attempt{port: PortReachable, statusCode: resp.StatusCode}
	if resp.StatusCode != http.StatusOK {
		a.err = fmt.Errorf("health endpoint returned HTTP %d", resp.StatusCode)
		return a
	}

	service := resp.Header.Get(constant.HealthServiceHeader)
	instance := resp.Header.Get(constant.HealthInstanceHeader)
	if p.token != "" {
		if instance != p.token {
			a.otherInstance = true
			a.err = fmt.Errorf("health endpoint answered by another instance")
			return a
		}
		a.ready = true
		return a
	}
	// No expected token (probing an already-running server): reject an
	// explicit non-tingly-box service, but accept a bare 200 from a build that
	// predates the header.
	if service != "" && service != constant.HealthServiceName {
		a.err = fmt.Errorf("port answered as service %q, not tingly-box", service)
		return a
	}
	a.ready = true
	return a
}

// PortIsListening reports whether host:port in rawURL accepts a TCP
// connection right now. Used to tell "port taken by another process" apart
// from "port not yet released".
func PortIsListening(rawURL string) bool { return classifyPort(rawURL) == PortReachable }

// classifyPort side-checks TCP reachability when the HTTP probe failed, so a
// timeout can distinguish "nothing is listening" from "listening but not
// ready". The check gets its own bounded context so an overall deadline does
// not make a reachable port look unreachable.
func classifyPort(rawURL string) PortState {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return PortUntested
	}
	d := net.Dialer{Timeout: dialTimeout}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return PortUnreachable
	}
	_ = conn.Close()
	return PortReachable
}
