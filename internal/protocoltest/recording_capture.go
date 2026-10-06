package protocoltest

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/recording/capture"
	"github.com/tingly-dev/tingly-box/internal/recording/tracestore"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// checkBoundaryRecording drives one request through a recording-enabled
// gateway and asserts the stored trace: the inbound body as sent, and at
// least one upstream exchange with request and response bodies, the
// response marked streaming when the client streamed and ended cleanly.
func checkBoundaryRecording(t flagTB, source, target protocol.APIType, streaming bool) {
	t.Helper()
	dir, err := os.MkdirTemp("", "flag-paths-capture-*")
	if err != nil {
		t.Fatalf("temp record dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	env, err := NewTestEnvForCLI(NewTestEnvOptionWithRecordDir(dir))
	if err != nil {
		t.Fatalf("create recording env: %v", err)
	}
	t.Cleanup(env.Close)

	model := flagPathsRoute(env, source, target, typ.RuleFlags{Recording: "client_request,upstream_request,upstream_response"})
	flagPathsSend(t, env, source, target, model, streaming, flagPathsMaterial{}, nil)
	env.FlushTraces(context.Background())

	traces := readTraces(t, dir)
	if len(traces) != 1 {
		t.Fatalf("traces = %d, want 1", len(traces))
	}
	tr := traces[0]
	if tr.Inbound == nil || !strings.Contains(string(tr.Inbound.Body), model) {
		t.Errorf("inbound request missing or not the client body")
	}
	if len(tr.Exchanges) == 0 {
		t.Fatal("trace has no upstream exchange")
	}
	ex := tr.Exchanges[len(tr.Exchanges)-1]
	if ex.Provider == "" {
		t.Error("exchange has no provider")
	}
	if ex.Request == nil || len(ex.Request.Body) == 0 {
		t.Error("exchange missing upstream request body")
	}
	if ex.Response == nil || ex.Response.Status != 200 || len(ex.Response.Body) == 0 {
		t.Fatalf("exchange missing upstream response: %+v", ex.Response)
	}
	if ex.Response.Stream != streaming {
		t.Errorf("response stream = %v, want %v", ex.Response.Stream, streaming)
	}
	// SDKs stop at the protocol terminator, so a stream may end "closed"
	// with every byte received; only a read error is a failure here.
	if ex.Response.End != capture.EndEOF && ex.Response.End != capture.EndClosed {
		t.Errorf("upstream response end = %q", ex.Response.End)
	}
}

// readTraces restores every boundary-recording trace under recordDir.
func readTraces(t flagTB, recordDir string) []*tracestore.Trace {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(recordDir, "traces", "*", "*", "*.jsonl.gz"))
	var out []*tracestore.Trace
	for _, f := range files {
		traces, err := tracestore.ReadPartition(f)
		if err != nil {
			t.Errorf("read %s: %v", f, err)
		}
		out = append(out, traces...)
	}
	return out
}
