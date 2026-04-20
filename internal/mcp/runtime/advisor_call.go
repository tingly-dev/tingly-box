package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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

func callOpenAI(ctx context.Context, cfg typ.AdvisorConfig, cp *client.ClientPool, reason string, actx *AdvisorContext) (string, error) {
	if cp == nil {
		return "", fmt.Errorf("advisor: client pool not available")
	}

	provider := buildProvider(cfg, protocol.APIStyleOpenAI)

	wrapper := cp.GetOpenAIClient(ctx, provider, cfg.Model)
	if wrapper == nil {
		return "", fmt.Errorf("advisor: failed to create OpenAI client")
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(advisorSystemPrompt),
	}
	for _, m := range actx.Messages {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		switch role {
		case "user":
			messages = append(messages, openai.UserMessage(content))
		case "assistant":
			messages = append(messages, openai.AssistantMessage(content))
		case "system":
			messages = append(messages, openai.SystemMessage(content))
		case "tool":
			messages = append(messages, openai.UserMessage("[tool result]: "+content))
		default:
			if content != "" {
				logrus.WithField("role", role).Warn("advisor: dropping unknown message role")
			}
		}
	}
	if reason == "" {
		reason = "The executor has requested strategic guidance."
	}
	messages = append(messages, openai.UserMessage(reason))

	req := openai.ChatCompletionNewParams{
		Model:    cfg.Model,
		Messages: messages,
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

func callAnthropic(ctx context.Context, cfg typ.AdvisorConfig, cp *client.ClientPool, reason string, actx *AdvisorContext) (string, error) {
	if cp == nil {
		return "", fmt.Errorf("advisor: client pool not available")
	}

	provider := buildProvider(cfg, protocol.APIStyleAnthropic)

	wrapper := cp.GetAnthropicClient(ctx, provider, cfg.Model)
	if wrapper == nil {
		return "", fmt.Errorf("advisor: failed to create Anthropic client")
	}

	var messages []anthropic.MessageParam
	var systemParts []string
	for _, m := range actx.Messages {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
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
			if content != "" {
				logrus.WithField("role", role).Warn("advisor: dropping unknown message role")
			}
		}
	}
	if reason == "" {
		reason = "The executor has requested strategic guidance."
	}
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(reason)))

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

func description(remainingUses int) string {
	return "Consult a more powerful advisor model for strategic guidance. " +
		"Use this when facing architectural decisions, complex debugging, unclear trade-offs, or when stuck. " +
		"You have " + strconv.Itoa(remainingUses) + " advisor consultation(s) remaining this request."
}

const advisorCallTimeout = 60 * time.Second

const advisorSystemPrompt = `You are an advisor to a coding agent. You share the agent's full conversation context and provide strategic guidance.

Your role:
- Provide plans, corrections, or stop signals
- Be concise and actionable — the executor will act on your advice immediately
- Focus on the "why" and the "what", not the "how" (the executor handles execution)
- Flag risks, edge cases, or better approaches the executor may have missed
- IMPORTANT: If you notice issues the executor did NOT ask about — bugs, security flaws, design problems, missed edge cases — proactively report them. The executor may have blind spots; your job is to catch what they miss.

You do NOT:
- Call tools or execute commands
- Produce user-facing output
- Repeat information already in the conversation
- Ask follow-up questions (give your best guidance with available context)

Structure your response as valid JSON:
{
  "assessment": "What's the situation? (1-2 sentences)",
  "recommendation": "What should the executor do? (actionable steps)",
  "unsolicited_findings": "Anything else you noticed that the executor should know, even if they didn't ask. Skip this field if there's nothing to add."
}`
