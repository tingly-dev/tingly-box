package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
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
	cmd.Stderr = f.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
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
	go func() {
		h.waitErr = cmd.Wait()
		close(h.done)
	}()
	return h, nil
}

type osHandle struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	done    chan struct{}
	waitErr error

	killOnce sync.Once
}

func (h *osHandle) Stdin() io.WriteCloser { return h.stdin }
func (h *osHandle) Stdout() io.ReadCloser { return h.stdout }
func (h *osHandle) Done() <-chan struct{} { return h.done }

func (h *osHandle) Wait() error {
	<-h.done
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
