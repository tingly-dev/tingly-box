package process

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestOSExecFactory_PerLaunchStderr(t *testing.T) {
	f := NewOSExecFactory()
	var buf bytes.Buffer
	h, err := f.Start(context.Background(), LaunchSpec{
		Command: []string{"sh", "-c", "echo boom >&2; exit 3"},
		Stderr:  &buf,
	})
	if err != nil {
		t.Skipf("sh not available: %v", err)
	}
	_, _ = io.ReadAll(h.Stdout())
	if err := h.Wait(); err == nil {
		t.Fatal("expected a non-zero exit")
	}
	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("stderr not captured: %q", buf.String())
	}
}
