package process

import (
	"context"
	"io"
	"testing"
	"time"
)

// A child that writes its last output and exits right away must not lose
// that output: Stdout stays readable until Wait returns, even when the
// consumer only gets to it after the process has exited. Regression test
// for "read |0: file already closed" at the end of a Claude Code turn.
func TestOSExecFactory_StdoutReadableAfterExit(t *testing.T) {
	f := NewOSExecFactory()
	h, err := f.Start(context.Background(), LaunchSpec{
		Command: []string{"sh", "-c", `printf 'line1\nline2\n'`},
	})
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	time.Sleep(50 * time.Millisecond) // the consumer is slow to read
	out, err := io.ReadAll(h.Stdout())
	if err != nil {
		t.Fatalf("read after exit: %v", err)
	}
	if string(out) != "line1\nline2\n" {
		t.Fatalf("stdout = %q", out)
	}
	if err := h.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

// A grandchild that inherits stdout and outlives the child must not keep
// the reader blocked forever after the child exits.
func TestOSExecFactory_LingeringGrandchildDoesNotBlockReader(t *testing.T) {
	old := stdoutDrainGrace
	stdoutDrainGrace = 200 * time.Millisecond
	defer func() { stdoutDrainGrace = old }()

	f := NewOSExecFactory()
	h, err := f.Start(context.Background(), LaunchSpec{
		Command: []string{"sh", "-c", `echo done; sleep 30 2>/dev/null &`},
	})
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	read := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(h.Stdout())
		read <- string(out)
	}()
	select {
	case out := <-read:
		if out != "done\n" {
			t.Fatalf("stdout = %q", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reader blocked by a lingering grandchild")
	}
	_ = h.Wait()
}
