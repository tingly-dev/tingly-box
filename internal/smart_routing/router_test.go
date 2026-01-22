package smartrouting

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"

	"tingly-box/internal/loadbalance"
)

func TestSmartRouting_Integration_OpenAI(t *testing.T) {
	// Create router with a rule that routes models containing "gpt" to a specific provider
	router, err := NewRouter([]SmartRouting{
		{
			Description: "Route gpt models to openai-provider",
			Ops: []SmartOp{
				{
					Position:  PositionModel,
					Operation: OpModelContains,
					Value:     "gpt",
				},
			},
			Services: []loadbalance.Service{
				{
					Provider: "openai-provider",
					Model:    "gpt-4",
					Weight:   1,
					Active:   true,
				},
			},
		},
	})
	require.NoError(t, err)

	// Create a realistic OpenAI request
	req := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("gpt-4o"),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You are a helpful assistant."),
			openai.UserMessage("What is the capital of France?"),
		},
		MaxTokens: openai.Opt(int64(100)),
	}

	// Extract context and evaluate
	ctx := ExtractContextFromOpenAIRequest(req)
	services, matched := router.EvaluateRequest(ctx)

	require.True(t, matched, "should match the gpt routing rule")
	require.Len(t, services, 1)
	require.Equal(t, "openai-provider", services[0].Provider)
	require.Equal(t, "gpt-4", services[0].Model)
	require.Equal(t, "gpt-4o", ctx.Model)
	require.Equal(t, "You are a helpful assistant.", ctx.SystemMessages[0])
	require.Equal(t, "What is the capital of France?", ctx.UserMessages[0])
}

func TestSmartRouting_Integration_Anthropic(t *testing.T) {
	// Create router with rules for thinking mode and haiku model
	router, err := NewRouter([]SmartRouting{
		{
			Description: "Route thinking enabled requests to thinking-provider",
			Ops: []SmartOp{
				{
					Position:  PositionThinking,
					Operation: OpThinkingEnabled,
					Value:     "true",
				},
			},
			Services: []loadbalance.Service{
				{
					Provider: "thinking-provider",
					Model:    "claude-3-5-sonnet-20241022",
					Weight:   1,
					Active:   true,
				},
			},
		},
		{
			Description: "Route haiku models to haiku-provider",
			Ops: []SmartOp{
				{
					Position:  PositionModel,
					Operation: OpModelContains,
					Value:     "haiku",
				},
			},
			Services: []loadbalance.Service{
				{
					Provider: "haiku-provider",
					Model:    "claude-3-5-haiku-20241022",
					Weight:   1,
					Active:   true,
				},
			},
		},
	})
	require.NoError(t, err)

	// Test with thinking enabled
	reqThinking := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 100,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: 10000,
			},
		},
		System: []anthropic.TextBlockParam{
			{Text: "You are a helpful assistant."},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Explain quantum computing")),
		},
	}

	ctx := ExtractContextFromAnthropicRequest(reqThinking)
	services, matched := router.EvaluateRequest(ctx)

	require.True(t, matched, "should match the thinking routing rule")
	require.Len(t, services, 1)
	require.Equal(t, "thinking-provider", services[0].Provider)
	require.True(t, ctx.ThinkingEnabled)

	// Test with haiku model
	reqHaiku := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-haiku-20241022"),
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Hello!")),
		},
	}

	ctx = ExtractContextFromAnthropicRequest(reqHaiku)
	services, matched = router.EvaluateRequest(ctx)

	require.True(t, matched, "should match the haiku routing rule")
	require.Len(t, services, 1)
	require.Equal(t, "haiku-provider", services[0].Provider)
}

func TestSmartRouting_Integration_Anthropic_ImageContent(t *testing.T) {
	// Test with image content
	req := &anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-3-5-sonnet-20241022"),
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock("What's in this image?"),
				anthropic.NewImageBlockBase64("image/jpeg", "/9j/4AAQ..."),
			),
		},
	}

	ctx := ExtractContextFromAnthropicRequest(req)
	require.Equal(t, "image", ctx.LatestContentType)
}

func TestValidateSmartOp(t *testing.T) {
	tests := []struct {
		name    string
		op      SmartOp
		wantErr bool
	}{
		{
			name: "valid model contains",
			op: SmartOp{
				Position:  PositionModel,
				Operation: OpModelContains,
				Value:     "haiku",
			},
			wantErr: false,
		},
		{
			name: "valid thinking enabled",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: OpThinkingEnabled,
				Value:     "true",
			},
			wantErr: false,
		},
		{
			name: "valid token ge",
			op: SmartOp{
				Position:  PositionToken,
				Operation: OpTokenGe,
				Value:     "6000",
			},
			wantErr: false,
		},
		{
			name: "invalid position",
			op: SmartOp{
				Position:  SmartOpPosition("invalid"),
				Operation: "any_contains",
				Value:     "test",
			},
			wantErr: true,
		},
		{
			name: "invalid operation for position",
			op: SmartOp{
				Position:  PositionModel,
				Operation: "any_contains",
				Value:     "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSmartOp(&tt.op)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSmartOp() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSmartRouting(t *testing.T) {
	tests := []struct {
		name    string
		rule    SmartRouting
		wantErr bool
	}{
		{
			name: "valid rule",
			rule: SmartRouting{
				Description: "Test rule",
				Ops: []SmartOp{
					{
						Position:  PositionModel,
						Operation: OpModelContains,
						Value:     "haiku",
					},
				},
				Services: []loadbalance.Service{
					{
						Provider: "provider-1",
						Model:    "gpt-4",
						Weight:   1,
						Active:   true,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty description",
			rule: SmartRouting{
				Description: "",
				Ops: []SmartOp{
					{
						Position:  PositionModel,
						Operation: OpModelContains,
						Value:     "haiku",
					},
				},
				Services: []loadbalance.Service{
					{
						Provider: "provider-1",
						Model:    "gpt-4",
						Weight:   1,
						Active:   true,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty ops",
			rule: SmartRouting{
				Description: "Test rule",
				Ops:         []SmartOp{},
				Services: []loadbalance.Service{
					{
						Provider: "provider-1",
						Model:    "gpt-4",
						Weight:   1,
						Active:   true,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty services",
			rule: SmartRouting{
				Description: "Test rule",
				Ops: []SmartOp{
					{
						Position:  PositionModel,
						Operation: OpModelContains,
						Value:     "haiku",
					},
				},
				Services: []loadbalance.Service{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSmartRouting(&tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSmartRouting() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewRouter(t *testing.T) {
	tests := []struct {
		name    string
		rules   []SmartRouting
		wantErr bool
	}{
		{
			name: "valid rules",
			rules: []SmartRouting{
				{
					Description: "Rule 1",
					Ops: []SmartOp{
						{
							Position:  PositionModel,
							Operation: OpModelContains,
							Value:     "haiku",
						},
					},
					Services: []loadbalance.Service{
						{
							Provider: "provider-1",
							Model:    "gpt-4",
							Weight:   1,
							Active:   true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty rules",
			rules:   []SmartRouting{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, err := NewRouter(tt.rules)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRouter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && router == nil {
				t.Errorf("NewRouter() returned nil router")
			}
		})
	}
}

func TestRouter_EvaluateRequest(t *testing.T) {
	router, err := NewRouter([]SmartRouting{
		{
			Description: "Route haiku models",
			Ops: []SmartOp{
				{
					Position:  PositionModel,
					Operation: OpModelContains,
					Value:     "haiku",
				},
			},
			Services: []loadbalance.Service{
				{
					Provider: "haiku-provider",
					Model:    "claude-3-5-haiku-20241022",
					Weight:   1,
					Active:   true,
				},
			},
		},
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		ctx          *RequestContext
		wantMatch    bool
		wantProvider string
	}{
		{
			name: "matches haiku model",
			ctx: &RequestContext{
				Model: "claude-3-5-haiku-20241022",
			},
			wantMatch:    true,
			wantProvider: "haiku-provider",
		},
		{
			name: "no match",
			ctx: &RequestContext{
				Model: "gpt-4",
			},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			services, matched := router.EvaluateRequest(tt.ctx)
			if matched != tt.wantMatch {
				t.Errorf("Router.EvaluateRequest() matched = %v, want %v", matched, tt.wantMatch)
			}
			if matched && len(services) > 0 && services[0].Provider != tt.wantProvider {
				t.Errorf("Router.EvaluateRequest() provider = %v, want %v", services[0].Provider, tt.wantProvider)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"", 0},
		{"hello", 1},       // 5 chars / 4 = 1
		{"hello world", 2}, // 11 chars / 4 = 2
		{"This is a longer text with more words to estimate tokens properly", 16},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := EstimateTokens(tt.text); got != tt.want {
				t.Errorf("EstimateTokens() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSmartOpTypeSafeGetters(t *testing.T) {
	tests := []struct {
		name    string
		op      SmartOp
		wantErr bool
	}{
		{
			name: "valid string op",
			op: SmartOp{
				Position:  PositionModel,
				Operation: OpModelContains,
				Value:     "gpt",
				Meta: SmartOpMeta{
					Type: ValueTypeString,
				},
			},
			wantErr: false,
		},
		{
			name: "valid int op",
			op: SmartOp{
				Position:  PositionToken,
				Operation: OpTokenGe,
				Value:     "6000",
				Meta: SmartOpMeta{
					Type: ValueTypeInt,
				},
			},
			wantErr: false,
		},
		{
			name: "valid bool op with true",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: OpThinkingEnabled,
				Value:     "true",
				Meta: SmartOpMeta{
					Type: ValueTypeBool,
				},
			},
			wantErr: false,
		},
		{
			name: "valid bool op with false",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: OpThinkingEnabled,
				Value:     "false",
				Meta: SmartOpMeta{
					Type: ValueTypeBool,
				},
			},
			wantErr: false,
		},
		{
			name: "valid bool op empty",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: OpThinkingEnabled,
				Value:     "",
				Meta: SmartOpMeta{
					Type: ValueTypeBool,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid int value",
			op: SmartOp{
				Position:  PositionToken,
				Operation: OpTokenGe,
				Value:     "not_a_number",
				Meta: SmartOpMeta{
					Type: ValueTypeInt,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid bool value",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: OpThinkingEnabled,
				Value:     "invalid",
				Meta: SmartOpMeta{
					Type: ValueTypeBool,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test appropriate getter based on type
			var err error
			{
				switch tt.op.Meta.Type {
				case ValueTypeString:
					_, err = tt.op.String()
				case ValueTypeInt:
					_, err = tt.op.Int()
				case ValueTypeBool:
					_, err = tt.op.Bool()
				default:
					// No type specified, use String()
					_, err = tt.op.String()
				}
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("getter error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateOpValueType(t *testing.T) {
	tests := []struct {
		name    string
		op      SmartOp
		wantErr bool
	}{
		{
			name: "valid int token op",
			op: SmartOp{
				Position:  PositionToken,
				Operation: OpTokenGe,
				Value:     "6000",
				Meta:      SmartOpMeta{Type: ValueTypeInt},
			},
			wantErr: false,
		},
		{
			name: "invalid int token op",
			op: SmartOp{
				Position:  PositionToken,
				Operation: OpTokenGe,
				Value:     "abc",
				Meta:      SmartOpMeta{Type: ValueTypeInt},
			},
			wantErr: true,
		},
		{
			name: "valid bool thinking op",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: OpThinkingEnabled,
				Value:     "true",
				Meta:      SmartOpMeta{Type: ValueTypeBool},
			},
			wantErr: false,
		},
		{
			name: "invalid bool thinking op",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: OpThinkingEnabled,
				Value:     "not_bool",
				Meta:      SmartOpMeta{Type: ValueTypeBool},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOpValueType(&tt.op)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOpValueType() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
