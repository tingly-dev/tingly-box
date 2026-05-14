package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// fixtureFS holds captured agent request bodies, grouped by agent and scenario:
//
//	testdata/fixtures/<agent>/<scenario>.json
//
// Each fixture is the on-the-wire request body an agent CLI sends to the
// gateway. Replaying it exercises the gateway's built-in rule + dispatch
// pipeline without spawning the real CLI.
//
//go:embed testdata/fixtures
var fixtureFS embed.FS

// ReplayCmd replays a captured agent request fixture through the gateway's
// built-in rule, routed to an in-process virtual-model upstream.
//
// Unlike `agent --mock` (which spawns the real CLI against a VirtualServer
// mock), replay sends a fixed request body directly. This makes it hermetic
// and fast — no CLI install, no subprocess — and lets it cover the vmodel
// dispatch path (gateway → built-in rule → vmodel provider → in-process
// handler) that the spawn-the-CLI modes never touch.
type ReplayCmd struct {
	Upstream  string `kong:"name='upstream',default='vmodel',enum='vmodel',help='Upstream to route through (currently: vmodel)'"`
	Scenario  string `kong:"name='scenario',default='text',help='Fixture scenario name (testdata/fixtures/<agent>/<scenario>.json)'"`
	VModel    string `kong:"name='vmodel',default='virtual-claude-3',help='vmodel registry ID to route to when --upstream=vmodel'"`
	AgentType string `kong:"arg,name='agent',help='Agent type: claude'"`
}

// Help returns extended help for `harness replay --help`.
func (*ReplayCmd) Help() string {
	return `Replay a captured agent request fixture through the gateway.

The fixture is the on-the-wire request body an agent CLI sends. Replaying it
exercises the gateway's built-in rule and dispatch pipeline without spawning
the real CLI — hermetic, fast, and able to cover the in-process vmodel
dispatch path.

Examples:
  harness replay claude
  harness replay claude --scenario text --vmodel echo-model`
}

// Run executes the replay subcommand.
func (r *ReplayCmd) Run() error {
	if r.AgentType == "" {
		return fmt.Errorf("agent type is required (claude)")
	}
	agentType := parseAgentType(r.AgentType)
	if agentType == "" {
		return fmt.Errorf("unknown agent: %q", r.AgentType)
	}
	if agentType != protocol_validate.AgentTypeClaudeCode {
		return fmt.Errorf("replay currently supports only the claude agent")
	}

	body, err := loadFixture(r.AgentType, r.Scenario)
	if err != nil {
		return err
	}

	fmt.Printf("🧪 Replay: agent=%s scenario=%s upstream=%s vmodel=%s\n",
		r.AgentType, r.Scenario, r.Upstream, r.VModel)

	env, err := protocol_validate.NewAgentTestEnv(agentType)
	if err != nil {
		return fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupVModelAgent(agentType, r.VModel); err != nil {
		return fmt.Errorf("setup vmodel agent: %w", err)
	}

	// Normalize the fixture's model field to the built-in rule's RequestModel
	// so fixtures stay upstream-agnostic — the rule (not the fixture) decides
	// which provider+model the request resolves to.
	requestModel := builtinRequestModel(agentType)
	body, err = rewriteModel(body, requestModel)
	if err != nil {
		return fmt.Errorf("rewrite fixture model: %w", err)
	}

	url := env.BaseURL() + "/tingly/claude_code/v1/messages"
	fmt.Printf("🔧 Gateway: %s\n", url)

	start := time.Now()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", env.ModelToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	duration := time.Since(start)

	if err := assertAnthropicMessage(resp.StatusCode, respBody); err != nil {
		fmt.Printf("❌ FAIL  duration=%s\n  %v\n  body: %s\n",
			duration.Round(time.Millisecond), err, truncateBody(respBody))
		return fmt.Errorf("replay assertion failed")
	}

	fmt.Printf("✅ PASS  duration=%s\n  %s\n",
		duration.Round(time.Millisecond), truncateBody(respBody))
	return nil
}

// loadFixture reads testdata/fixtures/<agent>/<scenario>.json from the embedded FS.
func loadFixture(agent, scenario string) ([]byte, error) {
	path := fmt.Sprintf("testdata/fixtures/%s/%s.json", agent, scenario)
	body, err := fixtureFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load fixture %s: %w", path, err)
	}
	return body, nil
}

// rewriteModel sets the top-level "model" field of a JSON request body.
func rewriteModel(body []byte, model string) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	m["model"] = model
	return json.Marshal(m)
}

// assertAnthropicMessage verifies the gateway returned a well-formed
// Anthropic message response with non-empty content.
func assertAnthropicMessage(status int, body []byte) error {
	if status != http.StatusOK {
		return fmt.Errorf("expected HTTP 200, got %d", status)
	}
	var parsed struct {
		Type    string            `json:"type"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("response is not valid JSON: %w", err)
	}
	if parsed.Type != "message" {
		return fmt.Errorf(`expected response type "message", got %q`, parsed.Type)
	}
	if len(parsed.Content) == 0 {
		return fmt.Errorf("response has no content blocks")
	}
	return nil
}

// truncateBody returns a single-line, length-capped view of a response body.
func truncateBody(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
