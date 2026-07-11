package protocoltest

// Functional phase of the duo environment: protocol correctness of one
// conversion route, driven end-to-end over real HTTP through both processes.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
)

// DuoCheck is one functional verification result.
type DuoCheck struct {
	Route  string `json:"route"`
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// RunFunctionalChecks verifies protocol correctness of one conversion route
// with a bodyBytes-sized conversation: streaming SSE shape, assembled
// content, usage propagation, and the non-streaming response body.
func (env *DuoEnv) RunFunctionalChecks(route DuoRoute, bodyBytes int) []DuoCheck {
	var checks []DuoCheck
	add := func(name string, pass bool, detail string) {
		checks = append(checks, DuoCheck{Route: route.Name, Name: name, Pass: pass, Detail: detail})
	}
	env.streamingChecks(route, bodyBytes, add)
	env.nonStreamingChecks(route, bodyBytes, add)
	return checks
}

// streamingChecks verifies SSE event shape and the assembled streaming result.
func (env *DuoEnv) streamingChecks(route DuoRoute, bodyBytes int, add func(name string, pass bool, detail string)) {
	resp, err := env.post(route, BuildConversationBody(route, bodyBytes, true))
	if err != nil {
		add("stream/http", false, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		add("stream/http", false, fmt.Sprintf("status %d: %s", resp.StatusCode, b))
		return
	}
	add("stream/http", true, "200")

	events, _ := sse.ReadSSELines(resp.Body)
	joined := strings.Join(events, "\n")
	for _, evt := range []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"} {
		add("stream/event/"+evt, strings.Contains(joined, evt), "")
	}

	parsed := sse.AssembleAnthropicStream(events)
	if parsed == nil {
		add("stream/assemble", false, "assembler returned nil")
		return
	}
	add("stream/assemble", parsed.Content != "", fmt.Sprintf("content=%dB", len(parsed.Content)))
	add("stream/finish_reason", parsed.FinishReason != "", parsed.FinishReason)
	if parsed.Usage == nil {
		add("stream/usage", false, "no usage in stream")
	} else {
		add("stream/usage", parsed.Usage.InputTokens > 0 && parsed.Usage.OutputTokens > 0,
			fmt.Sprintf("in=%d out=%d", parsed.Usage.InputTokens, parsed.Usage.OutputTokens))
	}
}

// nonStreamingChecks verifies the non-streaming response body shape.
func (env *DuoEnv) nonStreamingChecks(route DuoRoute, bodyBytes int, add func(name string, pass bool, detail string)) {
	resp, err := env.post(route, BuildConversationBody(route, bodyBytes, false))
	if err != nil {
		add("nonstream/http", false, err.Error())
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		add("nonstream/http", false, fmt.Sprintf("status %d: %s", resp.StatusCode, raw[:min(len(raw), 2048)]))
		return
	}
	add("nonstream/http", true, "200")
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		add("nonstream/body", false, "invalid JSON: "+err.Error())
		return
	}
	parsed := sse.ParseAnthropicResult(m)
	if parsed == nil {
		add("nonstream/body", false, "unparseable anthropic body")
		return
	}
	add("nonstream/content", parsed.Content != "", fmt.Sprintf("content=%dB", len(parsed.Content)))
	add("nonstream/usage", parsed.Usage != nil && parsed.Usage.InputTokens > 0, "")
}
