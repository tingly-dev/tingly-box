package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/command/readiness"
)

const (
	// defaultStartReadyTimeout bounds how long a launcher waits for a server
	// it just started to answer its health endpoint. Generous on purpose:
	// first start runs database migrations and brings up remote bots, and a
	// slow-but-healthy start must not be reported as a failure. Override with
	// --start-timeout or TINGLY_BOX_START_TIMEOUT.
	defaultStartReadyTimeout = 60 * time.Second
	// startTimeoutEnv lets scripted/CI launches widen the wait without a flag.
	startTimeoutEnv = "TINGLY_BOX_START_TIMEOUT"
	// logTailLines is how many recent log lines a failed/still-starting launch
	// reports, so the exit code and the reason are in one place.
	logTailLines = 20
)

// resolveStartTimeout returns flag when > 0, else TINGLY_BOX_START_TIMEOUT,
// else defaultStartReadyTimeout.
func resolveStartTimeout(flag time.Duration) time.Duration {
	if flag > 0 {
		return flag
	}
	if v := strings.TrimSpace(os.Getenv(startTimeoutEnv)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultStartReadyTimeout
}

// startWait describes one launch's readiness wait.
type startWait struct {
	// ProbeURL is the health URL of the instance just launched.
	ProbeURL string
	// Token, when set, must be echoed by the health response. It keeps a
	// restart from accepting a still-dying older server on the same port.
	Token string
	// Exited yields the launched process's result (nil when there is none to
	// watch, e.g. probing an already-running server).
	Exited <-chan error
	// Timeout bounds the wait; zero resolves to the configured default.
	Timeout time.Duration
	// KeepWaiting, when true, keeps waiting past Timeout while the process is
	// still alive instead of reporting "still starting".
	KeepWaiting bool
	// LogPath is where the server logs its startup, quoted in failures.
	LogPath string
	// PID is the launched child's process id, quoted in messages (0 unknown).
	PID int
}

// confirm waits until the server behind ProbeURL answers its health endpoint,
// and only then calls announce — the one place a launcher is allowed to print
// the URL, token and "running" hints, so the user is never handed a link they
// cannot open yet.
//
// It fails fast when the process exits, reports the exit code and recent log
// lines, and distinguishes "still starting" from a real failure on timeout.
func (w startWait) confirm(announce func()) error {
	timeout := resolveStartTimeout(w.Timeout)
	out := readiness.Wait(context.Background(), readiness.Options{
		URL:           w.ProbeURL,
		Timeout:       timeout,
		Exited:        w.Exited,
		InstanceToken: w.Token,
	})
	if out.State == readiness.StateReady {
		announce()
		return nil
	}
	if out.State == readiness.StateFailed {
		return w.failure(out)
	}
	if out.State == readiness.StateCanceled {
		return out.Err()
	}

	// Still starting: the process is alive. With --wait, keep waiting without
	// a total timeout (the process exiting or the user interrupting still
	// ends it); otherwise report a clear, actionable state.
	if w.KeepWaiting {
		fmt.Fprintf(os.Stderr,
			"Server is still starting after %s (%s); --wait given, continuing to wait (press Ctrl+C to stop)...\n",
			humanDuration(out.Elapsed), out.Reason())
		out = readiness.Wait(context.Background(), readiness.Options{
			URL:           w.ProbeURL,
			Exited:        w.Exited,
			InstanceToken: w.Token,
		})
		if out.State == readiness.StateReady {
			announce()
			return nil
		}
		if out.State == readiness.StateFailed {
			return w.failure(out)
		}
		if out.State == readiness.StateCanceled {
			return out.Err()
		}
	}
	return w.notReady(out)
}

// failure renders a process that ended before it became ready, including the
// exit code (inside readiness.ExitedError) and the recent log.
func (w startWait) failure(out readiness.Outcome) error {
	err := out.Err()
	if tail := logTail(w.LogPath, logTailLines); len(tail) > 0 {
		return fmt.Errorf("%w — last lines of %s:\n%s", err, w.LogPath, strings.Join(tail, "\n"))
	}
	return fmt.Errorf("%w — check the log: %s", err, w.LogPath)
}

// notReady renders a timeout that is not a proven failure: the process is
// still alive but the health endpoint has not answered for this instance.
func (w startWait) notReady(out readiness.Outcome) error {
	err := out.Err()
	alive := ""
	if w.PID > 0 {
		alive = fmt.Sprintf(" (background process %d is still running)", w.PID)
	}
	hint := fmt.Sprintf("; the server is still starting%s. Check progress with 'tingly-box log' or the log file: %s — re-run with --wait to keep waiting, or 'tingly-box stop' to abort", alive, w.LogPath)
	tail := logTail(w.LogPath, logTailLines)
	if len(tail) == 0 {
		return fmt.Errorf("%w%s", err, hint)
	}
	return fmt.Errorf("%w%s\nlast lines of %s:\n%s", err, hint, w.LogPath, strings.Join(tail, "\n"))
}

// humanDuration rounds a duration for messages: sub-second waits keep
// millisecond precision (a 50ms timeout must not read "after 0s"), longer ones
// round to a second.
func humanDuration(d time.Duration) time.Duration {
	if d < time.Second {
		return d.Round(10 * time.Millisecond)
	}
	return d.Round(time.Second)
}

// logTail returns up to n trailing lines of the file at path. It reads only a
// bounded window from the end, so a large log does not get slurped. Any error
// means "no tail" — a missing log must not turn into a second failure.
func logTail(path string, n int) []string {
	if path == "" || n <= 0 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return nil
	}
	const window = int64(16 * 1024)
	size := st.Size()
	start := size - window
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return nil
	}

	text := strings.TrimRight(string(buf), "\r\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if start > 0 && len(lines) > 1 {
		lines = lines[1:] // drop a partial first line
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
