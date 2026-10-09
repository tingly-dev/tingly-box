package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// ScriptCmd drives a vmodel script (.design/vmodel-script.md) one step at a
// time through the in-process gateway and checks each response against what
// the step declares.
//
// It answers "does my script do what I wrote?" on the real data-plane path —
// built-in rule → vmodel provider → the agent's wire format — hermetically,
// without spawning an agent CLI. The same file can then be dropped into
// <config-dir>/vmodels/ to serve a real tb or agent.
type ScriptCmd struct {
	Agent    string   `kong:"name='agent',default='claude',help='Wire format to drive: claude | codex | opencode'"`
	NoStream bool     `kong:"name='no-stream',help='Use non-streaming requests (mid-stream steps cannot be checked and are skipped)'"`
	Files    []string `kong:"arg,name='file',type='existingfile',help='Script file(s) (YAML)'"`
}

// Help returns extended help for `harness script --help`.
func (*ScriptCmd) Help() string {
	return `Run a vmodel script through the in-process gateway, one request per step.

Each step is checked against its declaration: error steps by HTTP status, tool
steps by tool name, text steps by content, mid-stream steps by the stream being
cut short. A script that loops or clamps is exercised for one pass.

Examples:
  harness script flow.yaml
  harness script flow.yaml --agent codex
  harness script flow.yaml --no-stream`
}

// scriptRow is the outcome of one step.
type scriptRow struct {
	Step     int
	Expect   string
	Status   int
	Skipped  string
	Failures []string
	Err      string
	Body     []byte
}

func (r scriptRow) passed() bool { return r.Err == "" && len(r.Failures) == 0 }

// Run executes the script subcommand.
func (c *ScriptCmd) Run() error {
	if parseAgentType(c.Agent) == "" {
		return fmt.Errorf("unknown agent: %q (available: claude, codex, opencode)", c.Agent)
	}
	failed := 0
	for _, file := range c.Files {
		if err := c.runFile(file); err != nil {
			fmt.Printf("❌ %v\n\n", err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d script(s) failed", failed, len(c.Files))
	}
	return nil
}

// runFile drives one script file through a fresh in-process gateway.
func (c *ScriptCmd) runFile(file string) error {
	agentType := parseAgentType(c.Agent)
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	cfg, err := vmodel.ParseScript(raw, stem)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}

	env, err := protocoltest.NewAgentTestEnv(agentType)
	if err != nil {
		return fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	// Drop the script where a real tb would find it: <config-dir>/vmodels.
	dir := filepath.Join(env.ConfigDir(), virtualserver.ScriptDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, stem+".yaml"), raw, 0o644); err != nil {
		return err
	}
	if err := env.SetupVModelAgent(agentType, cfg.ID); err != nil {
		return fmt.Errorf("route agent to script model %q: %w", cfg.ID, err)
	}

	scenario := "streaming_text"
	if c.NoStream {
		scenario = "text"
	}
	body, err := loadFixture(agentType, scenario)
	if err != nil {
		return err
	}
	if body, err = rewriteModel(body, builtinRequestModel(agentType)); err != nil {
		return err
	}

	// Resolve the expected outcome of every request with the same engine the
	// server uses, so defaults (default text, tool ids, repeat) match exactly.
	seq := vmodel.NewSequence(cfg)
	n := seq.Len()
	fmt.Printf("🎬 Script %q: %d request(s) via %s (%s)\n\n", cfg.ID, n, c.Agent, scenario)

	var rows []scriptRow
	for i := 1; i <= n; i++ {
		want := seq.Next()
		row := scriptRow{Step: i, Expect: describeStep(want)}
		result, err := env.ReplayFixture(agentType, body, !c.NoStream)
		if err != nil {
			row.Err = err.Error()
		} else {
			row.Status, row.Body = result.HTTPStatus, result.RawBody
			if want.Error != nil && want.Error.Stage == vmodel.ErrorStageMidStream && c.NoStream {
				row.Skipped = "mid-stream cut needs streaming"
			} else {
				row.Failures = checkScriptStep(agentType, want, result)
			}
		}
		printScriptRow(row)
		rows = append(rows, row)
	}
	fmt.Println()
	return summarizeScript(rows)
}

func describeStep(s vmodel.ResolvedStep) string {
	switch {
	case s.Error != nil && s.Error.Stage == vmodel.ErrorStagePreContent:
		return fmt.Sprintf("error %d", s.Error.Status)
	case s.Tool != nil:
		return "tool " + s.Tool.Name
	case s.Error != nil:
		return "cut-off stream"
	default:
		return fmt.Sprintf("say %q", truncateBody([]byte(s.Content)))
	}
}

// checkScriptStep compares one response with its resolved step.
func checkScriptStep(agentType protocoltest.AgentType, want vmodel.ResolvedStep, got *protocoltest.RoundTripResult) []string {
	var f []string
	// A stream is complete when it reached its protocol's terminal event. Not
	// the full replay shape: a tool-only step legitimately has no text deltas.
	terminal := "message_stop"
	if agentType == protocoltest.AgentTypeCodex {
		terminal = "response.completed"
	}
	streamComplete := func() bool {
		return !got.IsStreaming || protocoltest.AssertStreamEventsContain(terminal).Check(got) == nil
	}
	switch {
	case want.Error != nil && want.Error.Stage == vmodel.ErrorStagePreContent:
		if got.HTTPStatus != want.Error.Status {
			f = append(f, fmt.Sprintf("status: want %d, got %d", want.Error.Status, got.HTTPStatus))
		}
	case want.Error != nil: // mid-stream cut
		if streamComplete() {
			f = append(f, "stream completed, but the step cuts it short")
		}
	default:
		if got.HTTPStatus != 200 {
			f = append(f, fmt.Sprintf("status: want 200, got %d", got.HTTPStatus))
			break
		}
		if want.Content != "" && got.Content != want.Content {
			f = append(f, fmt.Sprintf("text: want %q, got %q", want.Content, got.Content))
		}
		if want.Tool != nil {
			if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != want.Tool.Name {
				f = append(f, fmt.Sprintf("tool: want %s, got %v", want.Tool.Name, got.ToolCalls))
			}
		} else if len(got.ToolCalls) != 0 {
			f = append(f, fmt.Sprintf("tool: want none, got %v", got.ToolCalls))
		}
		if !streamComplete() {
			f = append(f, "stream did not complete")
		}
	}
	return f
}

func printScriptRow(r scriptRow) {
	switch {
	case r.Err != "":
		fmt.Printf("❌ ERROR step %d (%s)\n  %s\n", r.Step, r.Expect, r.Err)
	case r.Skipped != "":
		fmt.Printf("⏭  SKIP  step %d (%s)  %s\n", r.Step, r.Expect, r.Skipped)
	case r.passed():
		fmt.Printf("✅ PASS  step %d (%s)  status=%d\n", r.Step, r.Expect, r.Status)
	default:
		fmt.Printf("❌ FAIL  step %d (%s)  status=%d\n", r.Step, r.Expect, r.Status)
		for _, m := range r.Failures {
			fmt.Printf("  %s\n", m)
		}
		if len(r.Body) > 0 {
			fmt.Printf("  body: %s\n", truncateBody(r.Body))
		}
	}
}

func summarizeScript(rows []scriptRow) error {
	pass, fail, skip := 0, 0, 0
	for _, r := range rows {
		switch {
		case r.Skipped != "":
			skip++
		case r.passed():
			pass++
		default:
			fail++
		}
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "📊 Script Summary\nTotal: %d | ✓ Pass: %d | ✗ Fail: %d | ⏭ Skip: %d\n", len(rows), pass, fail, skip)
	w.Flush()
	if fail > 0 {
		return fmt.Errorf("script: %d of %d steps failed", fail, len(rows))
	}
	return nil
}
