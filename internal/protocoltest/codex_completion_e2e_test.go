package protocoltest_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/ai"
	pt "github.com/tingly-dev/tingly-box/internal/protocoltest"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

// Opt-in: TINGLY_CODEX_BIN selects a real Codex executable. All model traffic
// goes through a production gateway to vmodel's scenario server over HTTP.
// TINGLY_CODEX_EXPECT=before verifies the same matrix against the old handler.
func TestCodexResponsesCompletionE2E(t *testing.T) {
	bin := os.Getenv("TINGLY_CODEX_BIN")
	if bin == "" {
		t.Skip("set TINGLY_CODEX_BIN to run the real Codex/vmodel experiment")
	}
	stage := os.Getenv("TINGLY_CODEX_EXPECT")
	if stage == "" {
		stage = "after"
	}
	require.Contains(t, []string{"before", "after"}, stage)
	root := os.Getenv("TINGLY_CODEX_EVIDENCE_DIR")
	if root == "" {
		root = t.TempDir()
	}
	root, err := filepath.Abs(root)
	require.NoError(t, err)
	retries := 1
	if s := os.Getenv("TINGLY_CODEX_RETRIES"); s != "" {
		retries, err = strconv.Atoi(s)
		require.NoError(t, err)
		require.GreaterOrEqual(t, retries, 0)
	}
	version, err := exec.Command(bin, "--version").Output()
	require.NoError(t, err)
	binary, err := os.ReadFile(bin)
	require.NoError(t, err)
	digest := sha256.Sum256(binary)
	for _, tc := range []struct {
		name, usage string
		broken      bool
	}{
		{"valid-usage", `{"input_tokens":7,"output_tokens":3,"total_tokens":10,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1}}`, false},
		{"absent-usage", "", false},
		{"null-usage", "null", true},
		{"null-counters", `{"input_tokens":7,"output_tokens":3,"total_tokens":10,"input_tokens_details":{"cached_tokens":null},"output_tokens_details":{"reasoning_tokens":null}}`, true},
		{"truncated", "", true},
	} {
		if filter := os.Getenv("TINGLY_CODEX_CASE"); filter != "" && filter != tc.name {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, stage, tc.name)
			require.NoError(t, os.MkdirAll(dir, 0755))
			write := func(name string, data []byte) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0644))
			}
			frames := codexCompletionFrames(tc.usage, tc.name == "truncated")
			write("upstream.sse", []byte(strings.Join(frames, "\n\n")+"\n\n"))
			upstream := benchmark.NewScenarioServer()
			upstream.RegisterScenario(scenario.Scenario{
				Name: "codex-completion",
				MockResponses: map[scenario.ResponseFormat]scenario.MockResponseBuilder{
					scenario.FormatOpenAIResponses: {Stream: func() []string { return frames }},
				},
			})
			upstream.InProcess()
			defer upstream.Close()
			env, err := pt.NewAgentTestEnv(pt.AgentTypeCodex)
			require.NoError(t, err)
			defer env.Close(false)
			require.NoError(t, env.SetupRealAgent(pt.AgentTypeCodex, "completion-vmodel", "virtual-model-codex-completion", upstream.URL()+"/v1", "synthetic-vmodel-key", "openai"))
			provider, err := env.AppConfig().GetProviderByUUID("completion-vmodel")
			require.NoError(t, err)
			provider.OpenAIEndpointMode = ai.EndpointModeResponses
			require.NoError(t, env.AppConfig().UpdateProvider(provider.UUID, provider))

			// Transparent capture at the client boundary; it never changes SSE.
			target, err := url.Parse(env.BaseURL())
			require.NoError(t, err)
			proxy := httputil.NewSingleHostReverseProxy(target)
			var calls atomic.Int64
			type requestIndex struct{}
			proxy.ModifyResponse = func(resp *http.Response) error {
				i := resp.Request.Context().Value(requestIndex{}).(int64)
				f, err := os.Create(filepath.Join(dir, fmt.Sprintf("gateway-%d.sse", i)))
				if err != nil {
					return err
				}
				resp.Body = &codexCaptureBody{Reader: io.TeeReader(resp.Body, f), body: resp.Body, file: f}
				return nil
			}
			capture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				hash := sha256.Sum256(body)
				meta, _ := json.Marshal(map[string]any{"method": r.Method, "path": r.URL.Path, "body_sha256": hex.EncodeToString(hash[:])})
				_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("request-%d.json", i)), meta, 0644)
				proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIndex{}, i)))
			}))
			defer capture.Close()

			// Use CODEX_HOME for its intended purpose, as the agent harness does:
			// a fresh configuration directory, without the user's credentials.
			codexDir := t.TempDir()
			workspace := t.TempDir()
			cfg := fmt.Sprintf("model = %q\nmodel_provider = \"completion\"\n[model_providers.completion]\nname = \"Local completion experiment\"\nbase_url = %q\nwire_api = \"responses\"\nrequires_openai_auth = false\nstream_max_retries = %d\nrequest_max_retries = 0\nsupports_websockets = false\nhttp_headers = { Authorization = %q }\n", pt.BuiltinRequestModel(pt.AgentTypeCodex), capture.URL+"/tingly/codex/v1", retries, "Bearer "+env.ModelToken())
			require.NoError(t, os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(cfg), 0600))
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			args := []string{"exec", "--ephemeral", "--skip-git-repo-check", "--json", "-C", workspace, "Reply with the provided completion marker. Do not use tools."}
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "CODEX_HOME=" + codexDir, "RUST_LOG=codex_api::sse::responses=debug"}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			runErr := cmd.Run()
			write("codex.stdout.jsonl", stdout.Bytes())
			write("codex.stderr.log", stderr.Bytes())
			completed, failed, reconnects, answers := 0, 0, 0, 0
			for _, line := range strings.Split(stdout.String(), "\n") {
				var event struct {
					Type    string                      `json:"type"`
					Message string                      `json:"message"`
					Item    struct{ Type, Text string } `json:"item"`
				}
				if json.Unmarshal([]byte(line), &event) != nil {
					continue
				}
				switch event.Type {
				case "turn.completed":
					completed++
				case "turn.failed":
					failed++
				case "error":
					if strings.Contains(event.Message, "Reconnecting") {
						reconnects++
					}
				case "item.completed":
					if event.Item.Type == "agent_message" && strings.Contains(event.Item.Text, "COMPLETION_PROOF_OK") {
						answers++
					}
				}
			}
			wantSuccess := tc.name != "truncated" && !(stage == "before" && tc.broken)
			result := map[string]any{"stage": stage, "case": tc.name, "codex_version": strings.TrimSpace(string(version)), "codex_binary_sha256": hex.EncodeToString(digest[:]), "exit_code": cmd.ProcessState.ExitCode(), "gateway_requests": calls.Load(), "upstream_responses_requests": upstream.EndpointHits(benchmark.EndpointResponses), "upstream_chat_requests": upstream.EndpointHits(benchmark.EndpointChat), "turn_completed": completed, "turn_failed": failed, "reconnects": reconnects, "answer_items": answers, "expected_success": wantSuccess, "retry_limit": retries, "timed_out": ctx.Err() != nil}
			data, err := json.MarshalIndent(result, "", "  ")
			require.NoError(t, err)
			write("result.json", append(data, '\n'))
			t.Log(string(data))
			require.NoError(t, ctx.Err(), "Codex timed out")
			require.Equal(t, 0, upstream.EndpointHits(benchmark.EndpointChat), "must exercise native Responses, not a Chat fallback")
			require.EqualValues(t, calls.Load(), upstream.EndpointHits(benchmark.EndpointResponses))
			if wantSuccess {
				require.NoError(t, runErr, stderr.String())
				require.Equal(t, 1, completed)
				require.Zero(t, failed)
				require.Zero(t, reconnects)
				require.EqualValues(t, 1, calls.Load())
				require.Equal(t, 1, answers)
			} else {
				require.Error(t, runErr)
				require.Zero(t, completed)
				require.Equal(t, 1, failed)
				require.Equal(t, retries, reconnects)
				require.EqualValues(t, retries+1, calls.Load())
				if tc.name == "null-usage" {
					require.Contains(t, stdout.String(), "missing field `input_tokens`")
				}
			}
		})
	}
}

type codexCaptureBody struct {
	io.Reader
	body io.Closer
	file *os.File
}

func (b *codexCaptureBody) Close() error {
	defer b.file.Close()
	return b.body.Close()
}

func codexCompletionFrames(usage string, truncated bool) []string {
	item := `{"id":"msg_completion_proof","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"COMPLETION_PROOF_OK","annotations":[]}]}`
	events := []string{
		`{"type":"response.created","sequence_number":0,"response":{"id":"resp_completion_proof","status":"in_progress"}}`,
		`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_completion_proof","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_completion_proof","output_index":0,"content_index":0,"delta":"COMPLETION_PROOF_OK"}`,
	}
	if !truncated {
		events = append(events, `{"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":`+item+`}`)
		response := `{"id":"resp_completion_proof","status":"completed","output":[` + item + `]`
		if usage != "" {
			response += `,"usage":` + usage
		}
		events = append(events, `{"type":"response.completed","sequence_number":4,"response":`+response+`}}`)
	}
	var frames []string
	for _, event := range events {
		frames = append(frames, "data: "+event)
	}
	return frames
}
