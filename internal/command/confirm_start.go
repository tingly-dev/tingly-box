package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/command/readiness"
)

// startReadyTimeout bounds how long a launcher waits for a server it just
// started to answer its health endpoint. Generous on purpose: first start runs
// database migrations and brings up the remote bots.
var startReadyTimeout = 30 * time.Second

// confirmStarted waits until the server behind probeURL answers its health
// endpoint, and only then calls announce — the one place a launcher is allowed
// to print the URL, token and "running" hints, so the user is never handed a
// link they cannot open yet.
//
// exited, when non-nil, reports the server process ending (exit status or the
// error its Start returned); the launch then fails at once instead of waiting
// out the timeout. Failures name logPath so the user knows where to look.
func confirmStarted(probeURL string, exited <-chan error, timeout time.Duration, logPath string, announce func()) error {
	err := readiness.Wait(context.Background(), probeURL, exited, timeout)
	if err == nil {
		announce()
		return nil
	}

	var timedOut *readiness.TimeoutError
	if errors.As(err, &timedOut) {
		return fmt.Errorf("%w; it may still be starting or be stuck — check the log: %s (run 'tingly-box stop' to clean up)", err, logPath)
	}
	return fmt.Errorf("%w — check the log: %s", err, logPath)
}
