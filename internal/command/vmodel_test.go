package command

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
)

func TestVModelCommandParsesWithoutSubcommand(t *testing.T) {
	var cli struct {
		VModel VModelCmdKong `kong:"cmd,name='vmodel'"`
	}
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"vmodel", "--config", "a.yaml", "--config", "b.json", "--port", "0", "--json"}); err != nil {
		t.Fatal(err)
	}
	if len(cli.VModel.Config) != 2 || cli.VModel.Port != 0 || cli.VModel.Host != "127.0.0.1" || !cli.VModel.JSON {
		t.Fatalf("flags: %+v", cli.VModel)
	}
}

func vmodelConfigFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.yaml")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVModelConfiguredHTTP(t *testing.T) {
	path := vmodelConfigFile(t, "id: configured\nkind: mock\nmock: {content: configured response}\n")
	cmd := VModelCmdKong{Config: []string{path}}
	handler, ids, err := cmd.handler()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "configured" {
		t.Fatalf("unexpected models: %v", ids)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, tc := range []struct{ path, body, want string }{
		{"/openai/v1/chat/completions", `{"model":"configured","messages":[{"role":"user","content":"hi"}]}`, "configured response"},
		{"/openai/v1/responses", `{"model":"configured","input":"hi"}`, "configured response"},
		{"/anthropic/v1/messages", `{"model":"configured","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`, "configured response"},
		{"/v1/chat/completions", `{"model":"configured","messages":[],"stream":true}`, "[DONE]"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, server.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 200 || !strings.Contains(string(body), tc.want) {
				t.Fatalf("response: %d %s", response.StatusCode, body)
			}
		})
	}
	for _, endpoint := range []string{"/openai/v1/models", "/anthropic/v1/models"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", endpoint, nil))
		var list struct{ Data []struct{ ID string } }
		if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if len(list.Data) != 1 || list.Data[0].ID != "configured" {
			t.Fatalf("list: %s", response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/openai/v1/chat/completions", strings.NewReader(`{"model":"echo-model","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unconfigured model: %d", response.Code)
	}
}

func TestVModelConfigValidationAndDefaults(t *testing.T) {
	path := vmodelConfigFile(t, "id: duplicate\nkind: mock\nmock: {content: hi}\n")
	if _, _, err := (&VModelCmdKong{Config: []string{path, path}}).handler(); err == nil {
		t.Fatal("accepted duplicate model")
	}
	invalid := vmodelConfigFile(t, "id: invalid\ncontnet: typo\n")
	if _, _, err := (&VModelCmdKong{Config: []string{invalid}}).handler(); err == nil {
		t.Fatal("accepted invalid config")
	}
	_, ids, err := (&VModelCmdKong{}).handler()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("no builtin models")
	}
}

func TestVModelLifecycle(t *testing.T) {
	path := vmodelConfigFile(t, `{"id":"lifecycle","kind":"mock","mock":{"content":"hello"}}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	cmd := VModelCmdKong{Config: []string{path}, Host: "127.0.0.1", Port: 0, JSON: true}
	done := make(chan error, 1)
	go func() { err := cmd.run(ctx, writer); writer.CloseWithError(err); done <- err }()
	readiness := make(chan vmodelReady, 1)
	go func() {
		var ready vmodelReady
		if json.NewDecoder(reader).Decode(&ready) == nil {
			readiness <- ready
		}
	}()
	var ready vmodelReady
	select {
	case ready = <-readiness:
	case err := <-done:
		t.Fatalf("startup failed: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("readiness timeout")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(ready.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(ready.OpenAIBaseURL, ready.URL) || len(ready.Models) != 1 {
		t.Fatalf("readiness: %+v", ready)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shutdown timeout")
	}
	if response, err := client.Get(ready.URL + "/healthz"); err == nil {
		response.Body.Close()
		t.Fatal("server remained reachable after shutdown")
	}
}

func TestVModelInvalidPort(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		if err := (&VModelCmdKong{Port: port}).run(context.Background(), &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted port %d", port)
		}
	}
}

func TestVModelCaptureFlagsParse(t *testing.T) {
	parse := func(args ...string) VModelCmdKong {
		var cli struct {
			VModel VModelCmdKong `kong:"cmd,name='vmodel'"`
		}
		parser, err := kong.New(&cli)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(append([]string{"vmodel"}, args...)); err != nil {
			t.Fatal(err)
		}
		return cli.VModel
	}
	if c := parse(); c.Capture != "" || !c.CaptureOut || c.CaptureRequestID {
		t.Fatalf("defaults: %+v", c)
	}
	if c := parse("--capture", "d", "--capture-request-id", "--no-capture-out"); c.Capture != "d" || c.CaptureOut || !c.CaptureRequestID {
		t.Fatalf("flags: %+v", c)
	}
	if c := parse("--capture", "d", "--capture-out=false"); c.CaptureOut {
		t.Fatalf("--capture-out=false: %+v", c)
	}
}

func TestVModelCaptureRejectsMisuse(t *testing.T) {
	if err := (&VModelCmdKong{CaptureRequestID: true}).Run(); err == nil {
		t.Fatal("--capture-request-id without --capture accepted")
	}
	if err := (&VModelCmdKong{Capture: t.TempDir(), Export: filepath.Join(t.TempDir(), "x.yaml")}).Run(); err == nil {
		t.Fatal("--export with --capture accepted")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (&VModelCmdKong{Capture: dir, Host: "127.0.0.1"}).run(context.Background(), &bytes.Buffer{}); err == nil {
		t.Fatal("non-empty capture directory accepted")
	}
}

// End to end through the real command: an unknown model id still gets an
// answer (--capture defaults the fallback), the capture directory holds the
// client's exact bytes, and a user-defined model keeps priority.
func TestVModelCapture(t *testing.T) {
	path := vmodelConfigFile(t, `{"id":"configured","kind":"mock","mock":{"content":"configured answer"}}`)
	dir := filepath.Join(t.TempDir(), "cap")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	cmd := VModelCmdKong{Config: []string{path}, Host: "127.0.0.1", JSON: true, Capture: dir, CaptureOut: true}
	done := make(chan error, 1)
	go func() { err := cmd.run(ctx, writer); writer.CloseWithError(err); done <- err }()
	var ready vmodelReady
	readiness := make(chan vmodelReady, 1)
	go func() {
		var r vmodelReady
		if json.NewDecoder(reader).Decode(&r) == nil {
			readiness <- r
		}
	}()
	select {
	case ready = <-readiness:
	case err := <-done:
		t.Fatalf("startup failed: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("readiness timeout")
	}
	if ready.CaptureDir != dir {
		t.Fatalf("readiness: %+v", ready)
	}

	post := func(model string) (int, string, string) {
		body := `{"model":"` + model + `","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`
		req, _ := http.NewRequest("POST", ready.URL+"/v1/messages?beta=true", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-ant-oat01-fake")
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), body
	}
	code, got, sent := post("claude-sonnet-4-6")
	if code != 200 || !strings.Contains(got, "ok") {
		t.Fatalf("unknown model: %d %s", code, got)
	}
	if code, got, _ := post("configured"); code != 200 || !strings.Contains(got, "configured answer") {
		t.Fatalf("configured model: %d %s", code, got)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "raw", "conn-0001.in"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), sent) {
		t.Fatalf("raw input does not hold the client's body verbatim:\n%s", raw)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.jsonl"))
	if err != nil || strings.Count(string(index), "\n") < 2 || !strings.Contains(string(index), `"/v1/messages?beta=true"`) {
		t.Fatalf("index: %v %q", err, index)
	}
	for _, name := range []string{"meta.json", "raw/conn-0001.out"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVModelFallback(t *testing.T) {
	path := vmodelConfigFile(t, "- {id: known, kind: mock, mock: {content: known answer}}\n- {id: other, kind: mock, mock: {content: other answer}}\n")
	ask := func(t *testing.T, cmd VModelCmdKong, model string) (int, string) {
		t.Helper()
		handler, _, err := cmd.handler()
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json",
			strings.NewReader(`{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	if code, _ := ask(t, VModelCmdKong{Config: []string{path}}, "nobody"); code != http.StatusNotFound {
		t.Fatalf("unknown model without --fallback: %d", code)
	}
	if code, body := ask(t, VModelCmdKong{Config: []string{path}, Fallback: "other"}, "nobody"); code != 200 || !strings.Contains(body, "other answer") {
		t.Fatalf("--fallback other: %d %s", code, body)
	}
	if code, body := ask(t, VModelCmdKong{Config: []string{path}, Fallback: "other"}, "known"); code != 200 || !strings.Contains(body, "known answer") {
		t.Fatalf("defined model must win: %d %s", code, body)
	}
	// An explicit --fallback beats the one --capture would pick.
	if code, body := ask(t, VModelCmdKong{Config: []string{path}, Fallback: "other", Capture: t.TempDir()}, "nobody"); code != 200 || !strings.Contains(body, "other answer") {
		t.Fatalf("--fallback with --capture: %d %s", code, body)
	}
	if _, _, err := (&VModelCmdKong{Config: []string{path}, Fallback: "missing"}).handler(); err == nil {
		t.Fatal("undefined --fallback accepted")
	}
}
