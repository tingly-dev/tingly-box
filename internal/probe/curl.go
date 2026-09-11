package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Secret placeholder env-var names used in generated cURL commands. Secrets
// are never embedded — the user substitutes their own key.
const (
	curlKeyEnvThroughTB = "$TB_API_KEY"
	curlKeyEnvUpstream  = "$UPSTREAM_API_KEY"
)

// CurlData is the constructed (never executed) equivalent of a probe request,
// returned by POST /api/v2/probe/curl. Request carries the structured pieces;
// Command is the copy-pasteable rendering.
type CurlData struct {
	Command string `json:"command" example:"curl -N http://localhost:9999/tingly/openai/v1/chat/completions …"`
	Method  string `json:"method" example:"POST"`
	URL     string `json:"url" example:"http://localhost:9999/tingly/openai/v1/chat/completions"`
	// Headers maps header name → value with the secret as a placeholder env
	// var ($TB_API_KEY through TB, $UPSTREAM_API_KEY direct upstream).
	Headers map[string]string `json:"headers"`
	// Body is the exact JSON body the probe would send (same param builders
	// the SDK helpers use).
	Body string `json:"body"`
	// KeyEnvVar names the placeholder used in Headers/Command, so callers can
	// render a substitution hint.
	KeyEnvVar string `json:"key_env_var" example:"$TB_API_KEY"`
}

// BuildCurl constructs the curl equivalent of a probe request without
// executing it. It resolves the target exactly like Probe (loopback or direct
// upstream) and reuses the same param builders, so the generated body cannot
// drift from what a real probe sends.
func (e *E2EProber) BuildCurl(ctx context.Context, req *E2ERequest) (*CurlData, error) {
	provider, model, probeHeaders, err := e.resolveTargetToProviderModel(ctx, req)
	if err != nil {
		return nil, err
	}

	stream, _ := req.ResolveAxes()
	params := req.probeParams(model)

	// resolveTargetToProviderModel has already applied the protocol override
	// to the provider (loopback synthetic carries the client style; direct
	// targets went through ResolveStyle), so the style read here is final.
	clientStyle := provider.APIStyle

	// Google has its own SDK shape and no curl mapping — refuse explicitly
	// rather than emitting a misleading command.
	if clientStyle == protocol.APIStyleGoogle {
		return nil, fmt.Errorf("curl generation is not supported for Google-style providers")
	}

	// Loopback probes carry probe headers (provider/rule pins); those are the
	// through-TB curls and authenticate with the user's gateway key. Direct
	// probes carry none and use the upstream key.
	throughTB := len(probeHeaders) > 0
	keyEnv := curlKeyEnvUpstream
	if throughTB {
		keyEnv = curlKeyEnvThroughTB
	}

	var (
		url     string
		bodyObj any
	)
	switch clientStyle {
	case protocol.APIStyleOpenAI:
		if resolveOpenAIProbeEndpoint(req.ResolveOpenAIEndpointOverride(), provider) == "responses" {
			url = strings.TrimSuffix(provider.APIBase, "/") + "/responses"
			bodyObj = buildOpenAIResponsesParams(params)
		} else {
			url = strings.TrimSuffix(provider.APIBase, "/") + "/chat/completions"
			bodyObj = buildOpenAIChatParams(params)
		}
	case protocol.APIStyleAnthropic:
		url = strings.TrimSuffix(provider.APIBase, "/") + "/v1/messages"
		bodyObj = buildAnthropicMessageParams(params, provider.IsClaudeCodeProvider())
	default:
		return nil, fmt.Errorf("unsupported API style for curl generation: %s", clientStyle)
	}

	body, err := marshalStreamAware(bodyObj, stream)
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	headers := map[string]string{"Content-Type": "application/json"}
	switch clientStyle {
	case protocol.APIStyleAnthropic:
		headers["x-api-key"] = keyEnv
		headers["anthropic-version"] = "2023-06-01"
	default:
		headers["Authorization"] = "Bearer " + keyEnv
	}

	return &CurlData{
		Command:   renderCurl(url, headers, body, stream),
		Method:    "POST",
		URL:       url,
		Headers:   headers,
		Body:      body,
		KeyEnvVar: keyEnv,
	}, nil
}

// marshalStreamAware marshals an SDK params struct and, for streaming probes,
// adds the "stream": true member the SDKs inject at request time via
// WithJSONSet (it is not a field on the params structs).
func marshalStreamAware(bodyObj any, stream bool) (string, error) {
	b, err := json.Marshal(bodyObj)
	if err != nil {
		return "", err
	}
	if !stream {
		return string(b), nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	m["stream"] = true
	out, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// shellQuote wraps s in single quotes, escaping embedded quotes as '\” —
// single-quoted shell strings need no backslash escaping, so a JSON payload
// reads verbatim instead of drowning in \" escapes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// renderCurl composes the copy-pasteable multi-line curl command. The URL
// leads — it is the address the user scans first — followed by headers and
// the pretty-printed body.
func renderCurl(url string, headers map[string]string, body string, stream bool) string {
	var sb strings.Builder
	sb.WriteString("curl")
	if stream {
		// -N disables buffering so SSE chunks arrive as they are emitted.
		sb.WriteString(" -N")
	}
	sb.WriteString(" \\\n  " + url)
	// Deterministic order: Content-Type first, then the rest alphabetically.
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&sb, " \\\n  -H %s", shellQuote(name+": "+headers[name]))
	}
	// Pretty-print the payload and align continuation lines under -d.
	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte(body), "", "  "); err != nil {
		fmt.Fprintf(&sb, " \\\n  -d %s", shellQuote(body))
		return sb.String()
	}
	fmt.Fprintf(&sb, " \\\n  -d %s", shellQuote(strings.ReplaceAll(indented.String(), "\n", "\n  ")))
	return sb.String()
}
