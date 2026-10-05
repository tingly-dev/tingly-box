package virtualserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newScriptService returns a Service reading scripts from a fresh temp dir,
// served over HTTP the way production mounts it.
func newScriptService(t *testing.T) (svc *Service, dir string, baseURL string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir = t.TempDir()
	svc = NewService()
	svc.SetScriptDir(dir)
	engine := gin.New()
	svc.SetupRoutes(engine.Group("/v1"))
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return svc, dir, srv.URL
}

func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func postJSON(t *testing.T, url string, body any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func modelIDs(t *testing.T, baseURL string) map[string]bool {
	t.Helper()
	resp, err := http.Get(baseURL + "/v1/models")
	require.NoError(t, err)
	defer resp.Body.Close()
	var out struct {
		Data []struct{ ID string } `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	ids := map[string]bool{}
	for _, m := range out.Data {
		ids[m.ID] = true
	}
	return ids
}

const readEditFlow = `
steps:
  - say: "Let me look at the file."
    tool: {name: Read, arguments: {file_path: /tmp/a.go}}
  - tool: {name: Edit, arguments: {file_path: /tmp/a.go, old_string: foo, new_string: bar}}
  - say: "Done."
    usage: {input: 1200, output: 40}
`

// The headline flow: a script file written to the directory is picked up with
// no restart, and an Anthropic client walking it through the OFFICIAL SDK
// stream accumulator sees tool call → tool call → answer, each protocol-valid.
func TestScript_AnthropicAgentLoop(t *testing.T) {
	_, dir, baseURL := newScriptService(t)
	writeScript(t, dir, "read-edit.yaml", readEditFlow)
	client := sdk.NewClient(option.WithBaseURL(baseURL), option.WithAPIKey("k"))

	step := func(prompt string) sdk.Message {
		return accumulate(t, client, "read-edit", prompt)
	}

	m1 := step("fix foo")
	assert.Equal(t, sdk.StopReasonToolUse, m1.StopReason)
	var text1 string
	var tool1 sdk.ToolUseBlock
	for _, b := range m1.Content {
		switch v := b.AsAny().(type) {
		case sdk.TextBlock:
			text1 += v.Text
		case sdk.ToolUseBlock:
			tool1 = v
		}
	}
	assert.Equal(t, "Let me look at the file.", text1)
	assert.Equal(t, "Read", tool1.Name)
	assert.JSONEq(t, `{"file_path":"/tmp/a.go"}`, string(tool1.Input))

	m2 := step("(tool result)")
	assert.Equal(t, sdk.StopReasonToolUse, m2.StopReason)
	var tool2 sdk.ToolUseBlock
	for _, b := range m2.Content {
		if v, ok := b.AsAny().(sdk.ToolUseBlock); ok {
			tool2 = v
		}
	}
	assert.Equal(t, "Edit", tool2.Name)
	assert.Len(t, m2.Content, 1, "a tool step with no say is just the tool_use block, never an empty text block")
	assert.NotEqual(t, tool1.ID, tool2.ID, "tool ids must differ between steps")

	m3 := step("(tool result)")
	assert.Equal(t, sdk.StopReasonEndTurn, m3.StopReason)
	assert.EqualValues(t, 40, m3.Usage.OutputTokens)
}

// The same file answers the OpenAI Chat protocol too, with its own cursor.
func TestScript_OpenAIChat(t *testing.T) {
	_, dir, baseURL := newScriptService(t)
	writeScript(t, dir, "read-edit.yaml", readEditFlow)

	call := func() map[string]any {
		code, body := postJSON(t, baseURL+"/v1/chat/completions", map[string]any{
			"model":    "read-edit",
			"messages": []map[string]string{{"role": "user", "content": "go"}},
		})
		require.Equal(t, 200, code, string(body))
		var out map[string]any
		require.NoError(t, json.Unmarshal(body, &out))
		return out["choices"].([]any)[0].(map[string]any)
	}

	c1 := call()
	assert.Equal(t, "tool_calls", c1["finish_reason"])
	msg := c1["message"].(map[string]any)
	assert.Equal(t, "Let me look at the file.", msg["content"])
	tc := msg["tool_calls"].([]any)[0].(map[string]any)
	assert.Equal(t, "Read", tc["function"].(map[string]any)["name"])

	c2 := call()
	assert.Equal(t, "Edit", c2["message"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["name"])

	c3 := call()
	assert.Equal(t, "stop", c3["finish_reason"])
	assert.Equal(t, "Done.", c3["message"].(map[string]any)["content"])
}

func TestScript_ErrorAndMidStreamSteps(t *testing.T) {
	_, dir, baseURL := newScriptService(t)
	writeScript(t, dir, "flaky.yaml", `
steps:
  - 529
  - say: "this reply is cut off before it completes"
    midstream: {mode: eof, after_events: 1}
  - say: "recovered"
`)
	req := map[string]any{
		"model": "flaky", "max_tokens": 16, "stream": true,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}
	url := baseURL + "/v1/messages?beta=true"

	code, body := postJSON(t, url, req)
	assert.Equal(t, 529, code)
	assert.Contains(t, string(body), "overloaded_error")

	code, body = postJSON(t, url, req)
	assert.Equal(t, 200, code, "mid-stream failure arrives after a 200 status line")
	assert.NotContains(t, string(body), "message_stop", "the stream must be cut before completing")

	code, body = postJSON(t, url, req)
	assert.Equal(t, 200, code)
	assert.Contains(t, string(body), "recovered")
	assert.Contains(t, string(body), "message_stop")
}

func TestScript_DirectoryLifecycle(t *testing.T) {
	_, dir, baseURL := newScriptService(t)
	chat := func(model string) (int, string) {
		code, body := postJSON(t, baseURL+"/v1/chat/completions", map[string]any{
			"model":    model,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		return code, string(body)
	}

	// Not there yet.
	code, _ := chat("live")
	assert.Equal(t, 404, code)

	// Added: id comes from the file name; listed and callable immediately.
	writeScript(t, dir, "live.yaml", "steps:\n  - say: v1")
	assert.True(t, modelIDs(t, baseURL)["live"])
	code, body := chat("live")
	assert.Equal(t, 200, code)
	assert.Contains(t, body, "v1")

	// Edited: new content is served and the cursor restarts.
	time.Sleep(10 * time.Millisecond) // distinct mtime on coarse filesystems
	writeScript(t, dir, "live.yaml", "steps:\n  - say: v2-first\n  - say: v2-second")
	_, body = chat("live")
	assert.Contains(t, body, "v2-first")
	writeScript(t, dir, "live.yaml", "steps:\n  - say: v3-first\n  - say: v3-second ")
	_, body = chat("live")
	assert.Contains(t, body, "v3-first", "an edit restarts the program")

	// Broken edit: what is served always matches what is on disk, so the
	// model goes away and the reason is reported on the miss.
	time.Sleep(10 * time.Millisecond)
	writeScript(t, dir, "live.yaml", "steps:\n  - sya: oops")
	code, body = chat("live")
	assert.Equal(t, 404, code)
	assert.Contains(t, body, "live.yaml")
	assert.Contains(t, body, `unknown step field \"sya\"`)

	// Removed: gone from both the list and the endpoint.
	require.NoError(t, os.Remove(filepath.Join(dir, "live.yaml")))
	assert.False(t, modelIDs(t, baseURL)["live"])
	code, _ = chat("live")
	assert.Equal(t, 404, code)
}

func TestScript_DoesNotShadowBuiltinsOrEachOther(t *testing.T) {
	_, dir, baseURL := newScriptService(t)
	writeScript(t, dir, "echo-model.yaml", "steps:\n  - say: hijacked")
	writeScript(t, dir, "a.yaml", "id: shared\nsteps:\n  - say: from-a")
	writeScript(t, dir, "b.yaml", "id: shared\nsteps:\n  - say: from-b")

	code, body := postJSON(t, baseURL+"/v1/chat/completions", map[string]any{
		"model":    "echo-model",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	assert.Equal(t, 200, code)
	assert.NotContains(t, string(body), "hijacked", "a script must never replace a built-in model")

	code, body = postJSON(t, baseURL+"/v1/chat/completions", map[string]any{
		"model":    "shared",
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	assert.Equal(t, 200, code)
	assert.Contains(t, string(body), "from-a", "first file (by name) owns the id")
	_, body = postJSON(t, baseURL+"/v1/chat/completions", map[string]any{"model": "missing"})
	assert.Contains(t, string(body), "already used by a.yaml")
}

// A file that lost an id race is retried once the holder goes away, and two
// files can swap ids in one change.
func TestScript_FailedFilesAreRetried(t *testing.T) {
	_, dir, baseURL := newScriptService(t)
	say := func(model string) string {
		_, body := postJSON(t, baseURL+"/v1/chat/completions", map[string]any{
			"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
		return string(body)
	}

	writeScript(t, dir, "a.yaml", "id: x\nsteps:\n  - say: from-a")
	writeScript(t, dir, "b.yaml", "id: x\nsteps:\n  - say: from-b")
	assert.Contains(t, say("x"), "from-a")
	require.NoError(t, os.Remove(filepath.Join(dir, "a.yaml")))
	assert.Contains(t, say("x"), "from-b", "b.yaml is retried once a.yaml releases the id")

	time.Sleep(10 * time.Millisecond)
	writeScript(t, dir, "a.yaml", "id: p\nsteps:\n  - say: a-is-p")
	writeScript(t, dir, "b.yaml", "id: q\nsteps:\n  - say: b-is-q")
	assert.Contains(t, say("p"), "a-is-p")
	time.Sleep(10 * time.Millisecond)
	writeScript(t, dir, "a.yaml", "id: q\nsteps:\n  - say: a-is-q")
	writeScript(t, dir, "b.yaml", "id: p\nsteps:\n  - say: b-is-p")
	assert.Contains(t, say("q"), "a-is-q", "ids swapped between two files in one change")
	assert.Contains(t, say("p"), "b-is-p")
}

// RefreshScripts lets non-HTTP readers (the management UI listing) see files
// dropped since the last request.
func TestScript_RefreshScriptsForDirectRegistryReaders(t *testing.T) {
	svc, dir, _ := newScriptService(t)
	assert.False(t, svc.GetAnthropicRegistry().Has("fresh"))
	writeScript(t, dir, "fresh.yaml", "steps:\n  - say: hi")
	svc.RefreshScripts()
	assert.True(t, svc.GetAnthropicRegistry().Has("fresh"))
	assert.True(t, svc.GetOpenAIRegistry().Has("fresh"))
}

// A scripted stop_reason reaches the wire on tool steps too (it is the
// protocol's own word: Anthropic here).
func TestScript_StopReasonOnToolStep(t *testing.T) {
	_, dir, baseURL := newScriptService(t)
	writeScript(t, dir, "trunc.yaml", "steps:\n  - tool: {name: Read}\n    stop_reason: max_tokens")
	_, body := postJSON(t, baseURL+"/v1/messages?beta=true", map[string]any{
		"model": "trunc", "max_tokens": 16, "messages": []map[string]string{{"role": "user", "content": "hi"}}})
	assert.Contains(t, string(body), `"stop_reason":"max_tokens"`)
}
