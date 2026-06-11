package protocoltest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// subprocessClient drives requests through an external driver process
// (e.g. the real Python or Node SDKs), so the gateway is exercised by
// genuinely foreign client stacks — pydantic strict validation, the JS SDK's
// SSE consumer — not just Go code.
//
// Contract: one JSON request object on stdin, one JSON response object on
// stdout. Gateway/API errors (4xx/5xx) are reported in-band via the "error"
// field; a non-zero exit means the driver itself is broken. One process is
// spawned per request, which is acceptable at nightly cadence.
type subprocessClient struct {
	name    string
	argv    []string
	timeout time.Duration
}

// NewSubprocessClient returns a client driver that shells out to argv for
// each request, speaking the JSON-over-stdin/stdout driver contract.
func NewSubprocessClient(name string, argv ...string) Client {
	return &subprocessClient{name: name, argv: argv, timeout: 60 * time.Second}
}

// NewPythonClient returns a driver backed by tests/clients/python/driver.py
// (real anthropic + openai Python SDKs). driverDir is the tests/clients root.
func NewPythonClient(driverDir string) Client {
	return NewSubprocessClient("python", "python3", filepath.Join(driverDir, "python", "driver.py"))
}

// NewNodeClient returns a driver backed by tests/clients/node/driver.mjs
// (real @anthropic-ai/sdk + openai Node SDKs). driverDir is the tests/clients root.
func NewNodeClient(driverDir string) Client {
	return NewSubprocessClient("node", "node", filepath.Join(driverDir, "node", "driver.mjs"))
}

// NewAISDKClient returns a driver backed by tests/clients/aisdk/driver.mjs
// (AI SDK by Vercel: ai + @ai-sdk/anthropic + @ai-sdk/openai). driverDir is
// the tests/clients root.
func NewAISDKClient(driverDir string) Client {
	return NewSubprocessClient("aisdk", "node", filepath.Join(driverDir, "aisdk", "driver.mjs"))
}

// driverRequest is the JSON object written to the driver's stdin.
type driverRequest struct {
	Version   int    `json:"version"`
	Source    string `json:"source"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Stream    bool   `json:"stream"`
	Scenario  string `json:"scenario"`
	Prompt    string `json:"prompt"`
	TimeoutMs int    `json:"timeout_ms"`
}

// driverResponse is the JSON object the driver writes to stdout.
type driverResponse struct {
	HTTPStatus       int                  `json:"http_status"`
	Role             string               `json:"role"`
	Content          string               `json:"content"`
	Model            string               `json:"model"`
	FinishReason     string               `json:"finish_reason"`
	Thinking         string               `json:"thinking"`
	ToolCalls        []driverToolCall     `json:"tool_calls"`
	Usage            *driverUsage         `json:"usage"`
	StreamEventCount int                  `json:"stream_event_count"`
	RawBody          string               `json:"raw_body"`
	Error            *driverErrorEnvelope `json:"error"`
}

type driverToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type driverUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// driverErrorEnvelope carries an in-band gateway/API error.
type driverErrorEnvelope struct {
	Status  int    `json:"status"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (c *subprocessClient) Name() string { return c.name }

func (c *subprocessClient) Supports(source protocol.APIType) bool {
	switch source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta,
		protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses:
		return true
	}
	return false
}

func (c *subprocessClient) Send(env *TestEnv, spec SendSpec) (*RoundTripResult, error) {
	reqJSON, err := json.Marshal(driverRequest{
		Version:   1,
		Source:    string(spec.Source),
		BaseURL:   spec.GatewayURL,
		APIKey:    spec.APIKey,
		Model:     spec.RequestModel,
		Stream:    spec.Streaming,
		Scenario:  spec.ScenarioName,
		Prompt:    harnessPrompt,
		TimeoutMs: 30000,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal driver request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.argv[0], c.argv[1:]...)
	cmd.Stdin = bytes.NewReader(reqJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s driver failed: %w\nstderr: %s", c.name, err, truncate(stderr.String(), 2000))
	}

	var resp driverResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("%s driver returned invalid JSON: %w\nstdout: %s\nstderr: %s",
			c.name, err, truncate(stdout.String(), 1000), truncate(stderr.String(), 1000))
	}

	result := newRoundTripResult(spec)
	result.HTTPStatus = resp.HTTPStatus
	result.Role = resp.Role
	result.Content = resp.Content
	result.Model = resp.Model
	result.FinishReason = resp.FinishReason
	result.ThinkingContent = resp.Thinking
	result.RawBody = []byte(resp.RawBody)
	for _, tc := range resp.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCallResult{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	if resp.Usage != nil {
		result.Usage = &TokenUsage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	}
	// The harness only asserts on event counts, so the driver reports a count
	// rather than shipping every raw event across the process boundary.
	if resp.StreamEventCount > 0 {
		result.StreamEvents = make([]string, resp.StreamEventCount)
	}
	if resp.Error != nil {
		if resp.Error.Status != 0 {
			result.HTTPStatus = resp.Error.Status
		}
		if len(result.RawBody) == 0 {
			result.RawBody = []byte(resp.Error.Message)
		}
	}
	return result, nil
}
