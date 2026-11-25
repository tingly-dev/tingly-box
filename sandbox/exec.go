package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ExecCommand starts cmd as a host process and returns it as a Proc: the building
// block of runtimes that wrap a command (bwrap, runsc, krun, a container CLI).
// It behaves as agentboot's OSExecFactory does (stdout stays readable until
// Wait, and is closed a grace period after the process exits).
func ExecCommand(ctx context.Context, cmd Cmd) (Proc, error) {
	if len(cmd.Args) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	c := exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	c.Dir = cmd.Dir
	c.Env = cmd.Env
	if c.Env == nil {
		c.Env = []string{}
	}
	c.Stderr = cmd.Stderr
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	// Our own pipe, not c.StdoutPipe: Wait closes a StdoutPipe as soon as the
	// process exits, which would race the consumer's last reads.
	stdout, stdoutW, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	c.Stdout = stdoutW
	err = c.Start()
	_ = stdoutW.Close()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start %q: %w", cmd.Args[0], err)
	}
	p := &execProc{cmd: c, stdin: stdin, stdout: stdout, done: make(chan struct{})}
	go func() {
		p.waitErr = c.Wait()
		close(p.done)
		// A grandchild holding stdout may keep EOF from coming: let the
		// consumer drain, then close so it cannot block forever.
		t := time.NewTimer(drainGrace)
		defer t.Stop()
		<-t.C
		p.closeStdout()
	}()
	return p, nil
}

// drainGrace is how long stdout stays readable after the process exits when
// nobody has called Wait yet. A variable for tests.
var drainGrace = 5 * time.Second

type execProc struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *os.File
	done    chan struct{}
	waitErr error

	killOnce, stdoutOnce sync.Once
}

func (p *execProc) closeStdout()          { p.stdoutOnce.Do(func() { _ = p.stdout.Close() }) }
func (p *execProc) Stdin() io.WriteCloser { return p.stdin }
func (p *execProc) Stdout() io.ReadCloser { return p.stdout }
func (p *execProc) Done() <-chan struct{} { return p.done }

func (p *execProc) Wait() error {
	<-p.done
	p.closeStdout()
	return p.waitErr
}

func (p *execProc) Kill() error {
	var err error
	p.killOnce.Do(func() {
		select {
		case <-p.done:
			return
		default:
		}
		err = p.cmd.Process.Kill()
	})
	return err
}
