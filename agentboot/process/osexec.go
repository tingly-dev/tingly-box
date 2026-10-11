package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// OSExecFactory is the production Factory backed by os/exec.
type OSExecFactory struct {
	// Stderr, if non-nil, receives the child process's stderr stream.
	// Defaults to os.Stderr.
	Stderr io.Writer
}

// NewOSExecFactory returns a Factory wired to os.Stderr.
func NewOSExecFactory() *OSExecFactory {
	return &OSExecFactory{Stderr: os.Stderr}
}

func (f *OSExecFactory) Start(ctx context.Context, spec LaunchSpec) (Handle, error) {
	if len(spec.Command) == 0 {
		return nil, fmt.Errorf("empty launch command")
	}
	cmd := spec.BuildCmd(ctx)
	cmd.Stderr = spec.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = f.Stderr
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	// Stdout is our own pipe, not cmd.StdoutPipe: exec.Cmd.Wait closes a
	// StdoutPipe's read end as soon as the process exits, and the goroutine
	// below calls Wait right after Start. A child that writes its last line
	// and exits (Claude Code after its result) would then race the
	// consumer's final reads and fail with "read |0: file already closed".
	// Wait never touches a pipe we created, so the read end stays open until
	// Handle.Wait, as the Handle contract promises; the consumer reads to the
	// EOF the child's exit produces.
	stdout, stdoutW, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stdout = stdoutW

	err = cmd.Start()
	// The child holds its own copy of the write end now.
	_ = stdoutW.Close()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start %q: %w", spec.Command[0], err)
	}

	h := &osHandle{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		done:   make(chan struct{}),
	}
	grace := stdoutDrainGrace
	go func() {
		h.waitErr = cmd.Wait()
		close(h.done)
		// A grandchild that inherited stdout can keep the write end open
		// after the child is gone, so EOF may never come. Give the consumer
		// time to drain what the child wrote, then close the read end so it
		// cannot block forever.
		timer := time.NewTimer(grace)
		defer timer.Stop()
		<-timer.C
		h.closeStdout()
	}()
	return h, nil
}

// stdoutDrainGrace is how long stdout stays readable after the process
// exits when the consumer has not called Wait yet. A variable for tests.
var stdoutDrainGrace = 5 * time.Second

type osHandle struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *os.File

	done    chan struct{}
	waitErr error

	killOnce   sync.Once
	stdoutOnce sync.Once
}

func (h *osHandle) closeStdout() {
	h.stdoutOnce.Do(func() { _ = h.stdout.Close() })
}

func (h *osHandle) Stdin() io.WriteCloser { return h.stdin }
func (h *osHandle) Stdout() io.ReadCloser { return h.stdout }
func (h *osHandle) Done() <-chan struct{} { return h.done }

func (h *osHandle) Wait() error {
	<-h.done
	h.closeStdout()
	return h.waitErr
}

func (h *osHandle) Kill() error {
	var err error
	h.killOnce.Do(func() {
		if h.cmd.Process == nil {
			return
		}
		select {
		case <-h.done:
			return
		default:
		}
		err = h.cmd.Process.Kill()
	})
	return err
}
