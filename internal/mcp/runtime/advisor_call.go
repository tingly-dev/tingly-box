package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AdvisorFormat indicates the API format of the advisor endpoint.
type AdvisorFormat int

const (
	FormatOpenAI AdvisorFormat = iota
	FormatAnthropic
)

func detectAdvisorFormat(cfg typ.AdvisorConfig) AdvisorFormat {
	url := strings.ToLower(cfg.BaseURL)
	model := strings.ToLower(cfg.Model)
	if strings.Contains(url, "anthropic") || strings.HasPrefix(model, "claude-") {
		return FormatAnthropic
	}
	return FormatOpenAI
}

func buildProvider(cfg typ.AdvisorConfig, style protocol.APIStyle) *typ.Provider {
	return &typ.Provider{
		Name:     "advisor",
		APIBase:  cfg.BaseURL,
		Token:    cfg.APIKey,
		APIStyle: style,
		Enabled:  true,
	}
}

func callOpenAI(ctx context.Context, cfg typ.AdvisorConfig, cp *client.ClientPool, actx *AdvisorContext) (string, error) {
	if cp == nil {
		return "", fmt.Errorf("advisor: client pool not available")
	}

	provider := buildProvider(cfg, protocol.APIStyleOpenAI)

	wrapper := cp.GetOpenAIClient(ctx, provider, cfg.Model)
	if wrapper == nil {
		return "", fmt.Errorf("advisor: failed to create OpenAI client")
	}
	if sink, ok := GetAdvisorRecordSink(ctx); ok {
		wrapper.SetRecordSink(sink)
	}
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(advisorSystemPrompt),
	}
	for _, m := range actx.Messages {
		role, _ := m["role"].(string)
		content := extractMessageText(m["content"])
		reasoning := extractMessageThinking(m["content"])
		if content == "" && reasoning == "" {
			continue
		}
		switch role {
		case "user":
			if content == "" {
				continue
			}
			messages = append(messages, openai.UserMessage(content))
		case "assistant":
			msg := openai.AssistantMessage(content)
			if reasoning != "" && msg.OfAssistant != nil {
				extra := msg.OfAssistant.ExtraFields()
				if extra == nil {
					extra = map[string]any{}
				}
				extra["reasoning_content"] = reasoning
				msg.OfAssistant.SetExtraFields(extra)
			}
			messages = append(messages, msg)
		case "system":
			if content == "" {
				continue
			}
			messages = append(messages, openai.SystemMessage(content))
		case "tool":
			if content == "" {
				continue
			}
			messages = append(messages, openai.UserMessage("[tool result]: "+content))
		default:
			logrus.WithField("role", role).Warn("advisor: dropping unknown message role")
		}
	}

	req := openai.ChatCompletionNewParams{
		Model:    cfg.Model,
		Messages: messages,
	}

	if logrus.IsLevelEnabled(logrus.DebugLevel) {
		summary := summarizeAdvisorMessages(messages)
		logrus.WithField("count", len(messages)).WithField("summary", summary).Debug("[MCP-DEBUG] ADVISOR: outgoing OpenAI messages")
	}

	resp, err := wrapper.ChatCompletionsNew(ctx, req)
	if err != nil {
		return "", fmt.Errorf("advisor: OpenAI request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("advisor: empty response from OpenAI")
	}

	content := resp.Choices[0].Message.Content
	return normalizeAdvisorResponse(content), nil
}

func callAnthropic(ctx context.Context, cfg typ.AdvisorConfig, cp *client.ClientPool, actx *AdvisorContext) (string, error) {
	if cp == nil {
		return "", fmt.Errorf("advisor: client pool not available")
	}

	provider := buildProvider(cfg, protocol.APIStyleAnthropic)

	wrapper := cp.GetAnthropicClient(ctx, provider, cfg.Model)
	if wrapper == nil {
		return "", fmt.Errorf("advisor: failed to create Anthropic client")
	}
	if sink, ok := GetAdvisorRecordSink(ctx); ok {
		wrapper.SetRecordSink(sink)
	}

	var messages []anthropic.MessageParam
	var systemParts []string
	for _, m := range actx.Messages {
		role, _ := m["role"].(string)
		content := extractMessageText(m["content"])
		if content == "" {
			continue
		}
		switch role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(content)))
		case "system":
			systemParts = append(systemParts, content)
		case "tool":
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock("[tool result]: "+content)))
		default:
			logrus.WithField("role", role).Warn("advisor: dropping unknown message role")
		}
	}

	systemPrompt := advisorSystemPrompt
	if len(systemParts) > 0 {
		systemPrompt = strings.Join(append(systemParts, advisorSystemPrompt), "\n\n")
	}

	req := anthropic.MessageNewParams{
		Model:     anthropic.Model(cfg.Model),
		MaxTokens: int64(cfg.MaxTokens),
		Messages:  messages,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
	}

	resp, err := wrapper.MessagesNew(ctx, &req)
	if err != nil {
		return "", fmt.Errorf("advisor: Anthropic request failed: %w", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("advisor: empty response from Anthropic")
	}

	var content strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}

	return normalizeAdvisorResponse(content.String()), nil
}

type AdvisorResponse struct {
	Assessment          string `json:"assessment"`
	Recommendation      string `json:"recommendation"`
	UnsolicitedFindings string `json:"unsolicited_findings,omitempty"`
}

func normalizeAdvisorResponse(raw string) string {
	var r AdvisorResponse
	if err := json.Unmarshal([]byte(raw), &r); err == nil {
		return raw
	}
	fallback := AdvisorResponse{
		Assessment:     "Advisor returned non-JSON response.",
		Recommendation: raw,
	}
	b, err := json.Marshal(fallback)
	if err != nil {
		return fmt.Sprintf(`{"assessment":"Advisor returned non-JSON response.","recommendation":%q}`, raw)
	}
	return string(b)
}

// AdvisorToolDescription is the short, concise tool description shown alongside
// other tools. The full behavioral contract lives in AdvisorBehaviorPrompt and
// is appended to the worker's system prompt by the MCP injection transform.
const AdvisorToolDescription = "Consult a stronger reviewer model for strategic guidance. Your full conversation context is forwarded automatically. Use this when facing significant decisions, when stuck, or before declaring a task complete."

// AdvisorBehaviorPrompt is the behavioral contract that tingly-box appends to
// the worker's system prompt whenever the advisor tool is injected. Behavior
// instructions are weighted more heavily in system prompts than in tool
// descriptions, so models follow them more reliably.
const AdvisorBehaviorPrompt = `You have access to an advisor tool backed by a stronger reviewer model. When you call advisor(), your entire conversation history is automatically forwarded. They see the task, every tool call you've made, every result you've seen.

Call advisor BEFORE substantive work -- before writing, before committing to an interpretation, before building on an assumption. If the task requires orientation first (finding files, fetching a source, seeing what's there), do that, then call advisor. Orientation is not substantive work. Writing, editing, and declaring an answer are.

Also call advisor:

- When you believe the task is complete. BEFORE this call, make your deliverable durable: write the file, save the result, commit the change. The advisor call takes time; if the session ends during it, a durable result persists and an unwritten one doesn't.
- When stuck -- errors recurring, approach not converging, results that don't fit.
- When considering a change of approach.

On tasks longer than a few steps, call advisor at least once before committing to an approach and once before declaring done. On short reactive tasks where the next action is dictated by tool output you just read, you don't need to keep calling -- the advisor adds most of its value on the first call, before the approach crystallizes.

Give the advice serious weight. If you follow a step and it fails empirically, or you have primary-source evidence that contradicts a specific claim (the file says X, the paper states Y), adapt. A passing self-test is not evidence the advice is wrong -- it's evidence your test doesn't check what the advice is checking.

If you've already retrieved data pointing one way and the advisor points another: don't silently switch. Surface the conflict in one more advisor call -- "I found X, you suggest Y, which constraint breaks the tie?" The advisor saw your evidence but may have underweighted it; a reconcile call is cheaper than committing to the wrong branch.`

// summarizeAdvisorMessages produces a short, log-friendly summary of the
// OpenAI messages the advisor is about to send (role + content length).
func summarizeAdvisorMessages(messages []openai.ChatCompletionMessageParamUnion) string {
	var b strings.Builder
	for i, m := range messages {
		if i > 0 {
			b.WriteString(", ")
		}
		role := "?"
		contentLen := 0
		switch {
		case m.OfSystem != nil:
			role = "system"
			contentLen = len(m.OfSystem.Content.OfString.Value)
		case m.OfUser != nil:
			role = "user"
			contentLen = len(m.OfUser.Content.OfString.Value)
		case m.OfAssistant != nil:
			role = "assistant"
			contentLen = len(m.OfAssistant.Content.OfString.Value)
		}
		fmt.Fprintf(&b, "%s(%d)", role, contentLen)
	}
	return b.String()
}

func description() string {
	return AdvisorToolDescription
}

// extractMessageText normalizes a message "content" field to a plain string.
// OpenAI/Anthropic clients may serialize content as either a bare string or as
// an array of content parts (e.g. [{"type":"text","text":"..."},
// {"type":"tool_use",...}]). The advisor handler only consumes plain text, so
// we extract and concatenate the text parts and ignore the rest. Returning
// "" lets callers skip the message entirely instead of forwarding an empty
// content block (which some upstreams reject as "messages 参数非法").
func extractMessageText(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, part := range v {
			switch p := part.(type) {
			case string:
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(p)
			case map[string]any:
				// Standard text part: {"type":"text","text":"..."}
				if t, _ := p["type"].(string); t == "text" {
					if txt, _ := p["text"].(string); txt != "" {
						if b.Len() > 0 {
							b.WriteString("\n")
						}
						b.WriteString(txt)
					}
				}
				// Tool-result part (Anthropic style): {"type":"tool_result","content":...}
				if t, _ := p["type"].(string); t == "tool_result" {
					if inner := extractMessageText(p["content"]); inner != "" {
						if b.Len() > 0 {
							b.WriteString("\n")
						}
						b.WriteString("[tool result]: ")
						b.WriteString(inner)
					}
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}

// extractMessageThinking extracts Anthropic-style thinking text from content parts.
// This is mapped to OpenAI-compatible reasoning_content for providers like DeepSeek.
func extractMessageThinking(content any) string {
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		p, ok := part.(map[string]any)
		if !ok {
			continue
		}
		t, _ := p["type"].(string)
		if t != "thinking" {
			continue
		}
		txt, _ := p["thinking"].(string)
		if txt == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(txt)
	}
	return b.String()
}

const advisorCallTimeout = 60 * time.Second

const advisorSystemPrompt = `You are an advisor to a coding agent. You share the agent's full conversation context and provide strategic guidance.

Your role:
- Provide plans, corrections, or stop signals
- Be concise and actionable — the executor will act on your advice immediately
- Focus on the "why" and the "what", not the "how" (the executor handles execution)
- Flag risks, edge cases, or better approaches the executor may have missed
- IMPORTANT: If you notice issues the executor did NOT ask about — bugs, security flaws, design problems, missed edge cases — proactively report them. The executor may have blind spots; your job is to catch what they miss.

CRITICAL constraints:
- NEVER ask follow-up questions. You have all the context you need. Always give your best guidance immediately.
- Do NOT call tools or execute commands
- Do NOT produce user-facing output
- Do NOT repeat information already in the conversation

You MUST respond with valid JSON only — no markdown, no prose outside the JSON object:
{
  "assessment": "What's the situation? (1-2 sentences)",
  "recommendation": "What should the executor do? (actionable steps)",
  "unsolicited_findings": "Anything else you noticed that the executor should know, even if they didn't ask."
}
Only include "unsolicited_findings" if you actually have something to add; omit the field entirely otherwise.`
