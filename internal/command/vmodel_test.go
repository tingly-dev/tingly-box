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
