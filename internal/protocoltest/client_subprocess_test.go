//go:build !windows

package protocoltest

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// testdataDriver returns the absolute path of a stub driver script under
// tests/clients/testdata, resolved relative to this source file so the test
// works from any working directory.
func testdataDriver(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "tests", "clients", "testdata", name)
}

// TestSubprocessClient_Contract verifies the JSON-over-stdin/stdout driver
// contract: the request is well-formed, and every field of the driver's
// response is mapped onto RoundTripResult. Uses a stub shell driver so the
// plumbing is covered on every PR without Python/Node installed.
func TestSubprocessClient_Contract(t *testing.T) {
	c := NewSubprocessClient("stub", "sh", testdataDriver(t, "stub_driver.sh"))

	spec := SendSpec{
		Source:       protocol.TypeAnthropicV1,
		Target:       protocol.TypeOpenAIChat,
		ScenarioName: "text",
		RequestModel: "pv-test-model",
		Streaming:    true,
		GatewayURL:   "http://127.0.0.1:1",
		APIKey:       "tb-test",
	}
	res, err := c.Send(nil, spec)
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if res.HTTPStatus != 200 {
		t.Errorf("http_status: got %d, want 200", res.HTTPStatus)
	}
	if res.Role != "assistant" {
		t.Errorf("role: got %q", res.Role)
	}
	if res.Content != "The capital of France is Paris." {
		t.Errorf("content: got %q", res.Content)
	}
	if res.Model != "stub-model" {
		t.Errorf("model: got %q", res.Model)
	}
	if res.FinishReason != "stop" {
		t.Errorf("finish_reason: got %q", res.FinishReason)
	}
	if res.ThinkingContent != "pondering" {
		t.Errorf("thinking: got %q", res.ThinkingContent)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "get_weather" {
		t.Errorf("tool_calls: got %+v", res.ToolCalls)
	}
	if res.Usage == nil || res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 8 {
		t.Errorf("usage: got %+v", res.Usage)
	}
	if len(res.StreamEvents) != 4 {
		t.Errorf("stream_event_count: got %d, want 4", len(res.StreamEvents))
	}
	if string(res.RawBody) != "stub raw body" {
		t.Errorf("raw_body: got %q", res.RawBody)
	}
	if res.SourceProtocol != spec.Source || !res.IsStreaming {
		t.Errorf("spec metadata not propagated: %+v", res)
	}
}

// TestCodexClient_SupportsResponsesOnly verifies the Codex driver is restricted
// to the openai_responses source (Codex speaks only the Responses API), so
// --client=codex visibly skips every other source instead of sending a
// non-Codex request. Cheap (no node) — runs on every PR.
func TestCodexClient_SupportsResponsesOnly(t *testing.T) {
	c := NewCodexClient(testdataDriver(t, ".."))
	if c.Name() != "codex" {
		t.Errorf("name: got %q, want codex", c.Name())
	}
	if !c.Supports(protocol.TypeOpenAIResponses) {
		t.Error("codex client must support openai_responses")
	}
	for _, src := range []protocol.APIType{
		protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat,
	} {
		if c.Supports(src) {
			t.Errorf("codex client must not support %s", src)
		}
	}
}

// TestSubprocessClient_DriverFailure verifies a broken driver (non-zero exit)
// surfaces as a Send error including stderr for debuggability.
func TestSubprocessClient_DriverFailure(t *testing.T) {
	c := NewSubprocessClient("broken", "sh", "-c", "echo boom >&2; exit 3")
	_, err := c.Send(nil, SendSpec{Source: protocol.TypeOpenAIChat})
	if err == nil {
		t.Fatal("expected error from broken driver")
	}
	if got := err.Error(); !strings.Contains(got, "boom") {
		t.Errorf("error should include driver stderr, got: %s", got)
	}
}
