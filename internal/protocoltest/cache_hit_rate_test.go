package protocoltest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel"
	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
)

// TestCacheHitRate measures the prompt-cache hit rate a Claude Code session
// gets through the gateway, end to end: requests go through the claude_code
// scenario to the virtual-prompt-cache model (vmodel/promptcache), whose usage
// is what an ideal prefix cache would serve for the request it received. The
// usage then travels back through the gateway's response conversion to the
// client and the usage tracker.
//
// cache_prefix proves the dispatched requests share a prefix; this proves the
// number the user actually reads — the cache_read in the client's usage and on
// the dashboard — comes out right, in every target protocol and stream mode,
// so a regression on either half (request rewriting or usage conversion)
// shows up as a hit-rate drop.
func TestCacheHitRate(t *testing.T) {
	targets := []protocol.APIType{protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses}
	for _, target := range targets {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s", target, streamMode(streaming)), func(t *testing.T) {
				t.Parallel()
				env := NewTestEnv(t)
				defer env.Close()
				model := setupPromptCacheRoute(t, env, target)

				// One session growing by one exchange: turn N+1 must read all of
				// turn N from the cache.
				turn3 := sendClaudeCodeTurn(t, env, model, streaming, 3, cachePrefixSessionID)
				turn4 := sendClaudeCodeTurn(t, env, model, streaming, 4, cachePrefixSessionID)
				if turn4.cacheRead != turn3.prompt() {
					t.Errorf("turn 4 read %d cached tokens, want all %d of turn 3's prompt (hit rate %.1f%%)\n  reply: %s",
						turn4.cacheRead, turn3.prompt(), turn4.hitRate()*100, turn4.text)
				}
				assertTrackedUsage(t, env, turn4)

				// Another session with the same history differs only in Claude
				// Code's billing header (system[0], its fingerprint is per
				// session). clean_header must strip it, so the request is the
				// same prompt and is served entirely from the cache; a leaked
				// header leaves only the tool definitions shared.
				other := sendClaudeCodeTurn(t, env, model, streaming, 3, cachePrefixOtherSessionID)
				if other.cacheRead != other.prompt() {
					t.Errorf("another session with the same history read %d of %d prompt tokens from the cache, want all — a per-session billing header reached the upstream?\n  reply: %s",
						other.cacheRead, other.prompt(), other.text)
				}
			})
		}
	}
}

// setupPromptCacheRoute wires a claude_code rule (with the flags the built-in
// Claude Code rule seeds) to an in-process vmodel provider serving
// virtual-prompt-cache in target's protocol.
func setupPromptCacheRoute(t *testing.T, env *TestEnv, target protocol.APIType) string {
	t.Helper()
	style := targetToAPIStyle(target)
	uuid := "prompt-cache-" + string(target)
	if err := env.appConfig.AddProvider(&typ.Provider{
		UUID: uuid, Name: uuid, APIBase: vmodelclient.APIBase(style), APIStyle: style,
		OpenAIEndpointMode: targetToOpenAIEndpointMode(target),
		AuthType:           typ.AuthTypeVirtual, Enabled: true, Timeout: int64(constant.DefaultRequestTimeout),
		VModelDetail: &typ.VModelDetail{Models: []string{vmodel.PromptCacheModelID}},
	}); err != nil {
		t.Fatalf("add vmodel provider: %v", err)
	}
	requestModel := "cc-prompt-cache-" + string(target)
	rule := newHarnessRule(requestModel, typ.ScenarioClaudeCode, requestModel, vmodel.PromptCacheModelID,
		harnessService(uuid, vmodel.PromptCacheModelID))
	rule.Flags = typ.RuleFlags{ClaudeCodeCompat: true, CleanHeader: true, SessionAffinity: 1800}
	if err := env.appConfig.GetGlobalConfig().AddRequestConfig(rule); err != nil {
		t.Fatalf("add rule: %v", err)
	}
	return requestModel
}

// cacheTurn is the usage one turn reported to the client.
type cacheTurn struct {
	input, cacheRead, cacheWrite int64
	text                         string
}

// prompt is the whole prompt in Anthropic wire terms: input_tokens excludes
// both cache reads and cache writes.
func (r cacheTurn) prompt() int64 { return r.input + r.cacheRead + r.cacheWrite }

func (r cacheTurn) hitRate() float64 {
	if r.prompt() == 0 {
		return 0
	}
	return float64(r.cacheRead) / float64(r.prompt())
}

// sendClaudeCodeTurn sends turn `turns` of a Claude Code conversation — the
// cache_prefix claude_code fixture plus the billing header Claude Code puts in
// system[0], whose fingerprint differs per session — and reads the usage the
// client received.
func sendClaudeCodeTurn(t *testing.T, env *TestEnv, model string, streaming bool, turns int, sessionID string) cacheTurn {
	t.Helper()
	body := anthropicCachePrefixBody(model, streaming, turns, cachePrefixCacheableBlocks(turns)-1, sessionID, true)
	system := body["system"].([]map[string]any)
	billing := map[string]any{"type": "text", "text": fmt.Sprintf("x-anthropic-billing-header: cc_version=2.1.293.%s; cc_entrypoint=cli;", sessionID[:3])}
	body["system"] = append([]map[string]any{billing}, system...)

	req, err := http.NewRequest(http.MethodPost, env.GatewayURL()+"/tingly/claude_code/v1/messages?beta=true", bytes.NewReader(mustMarshal(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.ModelToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("turn %d: %v", turns, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("turn %d: status %d: %s", turns, resp.StatusCode, truncate(string(raw), 600))
	}

	var out cacheTurn
	apply := func(usage map[string]any) {
		num := func(k string) int64 { f, _ := usage[k].(float64); return int64(f) }
		// The latest report wins, as in the client SDKs' stream accumulators.
		if v := num("input_tokens"); v > 0 || usage["input_tokens"] != nil {
			out.input = v
		}
		if _, ok := usage["cache_read_input_tokens"]; ok {
			out.cacheRead = num("cache_read_input_tokens")
		}
		if _, ok := usage["cache_creation_input_tokens"]; ok {
			out.cacheWrite = num("cache_creation_input_tokens")
		}
	}
	if !streaming {
		var msg struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Usage map[string]any `json:"usage"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("turn %d: decode: %v", turns, err)
		}
		apply(msg.Usage)
		for _, c := range msg.Content {
			out.text += c.Text
		}
		return out
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Usage map[string]any `json:"usage"`
			} `json:"message"`
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
			Usage map[string]any `json:"usage"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "message_start":
			apply(ev.Message.Usage)
		case "message_delta":
			apply(ev.Usage)
		case "content_block_delta":
			out.text += ev.Delta.Text
		}
	}
	return out
}

// assertTrackedUsage checks the usage tracker recorded what the client saw:
// normalized input = uncached input + cache writes, cache read as is.
func assertTrackedUsage(t *testing.T, env *TestEnv, want cacheTurn) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		recs, _, err := env.gateway.UsageStore().GetRecords(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), nil, 1, 0)
		if err != nil {
			t.Fatalf("read usage records: %v", err)
		}
		if len(recs) > 0 && recs[0].CacheReadTokens == int(want.cacheRead) {
			if got, wantIn := recs[0].InputTokens, int(want.input+want.cacheWrite); got != wantIn {
				t.Errorf("tracked input %d, want %d (client input %d + cache write %d)", got, wantIn, want.input, want.cacheWrite)
			}
			return
		}
		if time.Now().After(deadline) {
			got := "none"
			if len(recs) > 0 {
				got = fmt.Sprintf("cache_read=%d input=%d", recs[0].CacheReadTokens, recs[0].InputTokens)
			}
			t.Errorf("tracked usage never matched the client's cache_read=%d (latest record: %s)", want.cacheRead, got)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
