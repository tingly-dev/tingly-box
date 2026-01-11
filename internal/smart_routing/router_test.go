package smartrouting

import (
	"strings"
	"testing"

	"tingly-box/internal/loadbalance"
)

func TestSmartOpPosition_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		pos   SmartOpPosition
		valid bool
	}{
		{"model valid", PositionModel, true},
		{"thinking valid", PositionThinking, true},
		{"system valid", PositionSystem, true},
		{"user valid", PositionUser, true},
		{"tool_use valid", PositionToolUse, true},
		{"token valid", PositionToken, true},
		{"invalid", SmartOpPosition("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pos.IsValid(); got != tt.valid {
				t.Errorf("SmartOpPosition.IsValid() = %v, want %v", got, tt.valid)
			}
		})
	}
}

func TestValidateSmartOp(t *testing.T) {
	tests := []struct {
		name    string
		op      SmartOp
		wantErr bool
	}{
		{
			name: "valid model contains with meta",
			op: SmartOp{
				Position:  PositionModel,
				Operation: "contains",
				Value:     "haiku",
				Meta: SmartOpMeta{
					Description: "Check if model name contains 'haiku'",
					Type:        ValueTypeString,
				},
			},
			wantErr: false,
		},
		{
			name: "valid model glob",
			op: SmartOp{
				Position:  PositionModel,
				Operation: "glob",
				Value:     "*haiku*",
				Meta: SmartOpMeta{
					Description: "Match model pattern",
					Type:        ValueTypeString,
				},
			},
			wantErr: false,
		},
		{
			name: "valid thinking enabled",
			op: SmartOp{
				Position:  PositionThinking,
				Operation: "enabled",
				Value:     "true",
				Meta: SmartOpMeta{
					Description: "Check if thinking mode is enabled",
					Type:        ValueTypeBool,
				},
			},
			wantErr: false,
		},
		{
			name: "valid token ge with meta",
			op: SmartOp{
				Position:  PositionToken,
				Operation: "ge",
				Value:     "6000",
				Meta: SmartOpMeta{
					Description: "Check if estimated tokens >= 6000",
					Type:        ValueTypeInt,
				},
			},
			wantErr: false,
		},
		{
			name: "valid model contains without meta",
			op: SmartOp{
				Position:  PositionModel,
				Operation: "contains",
				Value:     "haiku",
			},
			wantErr: false,
		},
		{
			name: "invalid position",
			op: SmartOp{
				Position:  SmartOpPosition("invalid"),
				Operation: "contains",
				Value:     "test",
			},
			wantErr: true,
		},
		{
			name: "invalid operation for position",
			op: SmartOp{
				Position:  PositionModel,
				Operation: "enabled", // Not valid for model
				Value:     "test",
			},
			wantErr: true,
		},
		{
			name: "empty operation",
			op: SmartOp{
				Position:  PositionModel,
				Operation: "",
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
			name: "valid rule with meta",
			rule: SmartRouting{
				Description: "Test rule",
				Ops: []SmartOp{
					{
						Position:  PositionModel,
						Operation: "contains",
						Value:     "haiku",
						Meta: SmartOpMeta{
							Description: "Check if model is haiku",
							Type:        ValueTypeString,
						},
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
			name: "valid rule without meta",
			rule: SmartRouting{
				Description: "Test rule",
				Ops: []SmartOp{
					{
						Position:  PositionModel,
						Operation: "contains",
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
						Operation: "contains",
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
						Operation: "contains",
						Value:     "haiku",
					},
				},
				Services: []loadbalance.Service{},
			},
			wantErr: true,
		},
		{
			name: "service with empty provider",
			rule: SmartRouting{
				Description: "Test rule",
				Ops: []SmartOp{
					{
						Position:  PositionModel,
						Operation: "contains",
						Value:     "haiku",
					},
				},
				Services: []loadbalance.Service{
					{
						Provider: "",
						Model:    "gpt-4",
						Weight:   1,
						Active:   true,
					},
				},
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
							Operation: "contains",
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
			name: "invalid rule",
			rules: []SmartRouting{
				{
					Description: "",
					Ops: []SmartOp{
						{
							Position:  PositionModel,
							Operation: "contains",
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
			wantErr: true,
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
	// Create test router with rules
	router, err := NewRouter([]SmartRouting{
		{
			Description: "Route haiku models",
			Ops: []SmartOp{
				{
					Position:  PositionModel,
					Operation: "contains",
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
		{
			Description: "Route thinking requests",
			Ops: []SmartOp{
				{
					Position:  PositionThinking,
					Operation: "enabled",
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
	})
	if err != nil {
		t.Fatalf("Failed to create router: %v", err)
	}

	tests := []struct {
		name         string
		ctx          *RequestContext
		wantMatch    bool
		wantProvider string
		wantModel    string
	}{
		{
			name: "matches haiku model",
			ctx: &RequestContext{
				Model:           "claude-3-5-haiku-20241022",
				ThinkingEnabled: false,
				SystemMessages:  []string{},
				UserMessages:    []string{},
				ToolUses:        []string{},
				EstimatedTokens: 1000,
			},
			wantMatch:    true,
			wantProvider: "haiku-provider",
			wantModel:    "claude-3-5-haiku-20241022",
		},
		{
			name: "matches thinking enabled",
			ctx: &RequestContext{
				Model:           "claude-3-5-sonnet-20241022",
				ThinkingEnabled: true,
				SystemMessages:  []string{},
				UserMessages:    []string{},
				ToolUses:        []string{},
				EstimatedTokens: 1000,
			},
			wantMatch:    true,
			wantProvider: "thinking-provider",
			wantModel:    "claude-3-5-sonnet-20241022",
		},
		{
			name: "no match",
			ctx: &RequestContext{
				Model:           "gpt-4",
				ThinkingEnabled: false,
				SystemMessages:  []string{},
				UserMessages:    []string{},
				ToolUses:        []string{},
				EstimatedTokens: 1000,
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
			if matched && len(services) > 0 {
				if services[0].Provider != tt.wantProvider {
					t.Errorf("Router.EvaluateRequest() provider = %v, want %v", services[0].Provider, tt.wantProvider)
				}
				if services[0].Model != tt.wantModel {
					t.Errorf("Router.EvaluateRequest() model = %v, want %v", services[0].Model, tt.wantModel)
				}
			}
		})
	}
}

func TestRouter_EvaluateModelOps(t *testing.T) {
	router, _ := NewRouter([]SmartRouting{})

	tests := []struct {
		name     string
		ctx      *RequestContext
		op       SmartOp
		expected bool
	}{
		{
			name: "contains - matches",
			ctx: &RequestContext{
				Model: "claude-3-5-haiku-20241022",
			},
			op: SmartOp{
				Position:  PositionModel,
				Operation: "contains",
				Value:     "haiku",
			},
			expected: true,
		},
		{
			name: "contains - no match",
			ctx: &RequestContext{
				Model: "gpt-4",
			},
			op: SmartOp{
				Position:  PositionModel,
				Operation: "contains",
				Value:     "haiku",
			},
			expected: false,
		},
		{
			name: "glob - matches",
			ctx: &RequestContext{
				Model: "claude-3-5-haiku-20241022",
			},
			op: SmartOp{
				Position:  PositionModel,
				Operation: "glob",
				Value:     "*haiku*",
			},
			expected: true,
		},
		{
			name: "glob - no match",
			ctx: &RequestContext{
				Model: "gpt-4",
			},
			op: SmartOp{
				Position:  PositionModel,
				Operation: "glob",
				Value:     "claude*",
			},
			expected: false,
		},
		{
			name: "equals - matches",
			ctx: &RequestContext{
				Model: "claude-3-5-haiku-20241022",
			},
			op: SmartOp{
				Position:  PositionModel,
				Operation: "equals",
				Value:     "claude-3-5-haiku-20241022",
			},
			expected: true,
		},
		{
			name: "equals - no match",
			ctx: &RequestContext{
				Model: "claude-3-5-haiku-20241022",
			},
			op: SmartOp{
				Position:  PositionModel,
				Operation: "equals",
				Value:     "gpt-4",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.evaluateModelOp(tt.ctx, &tt.op)
			if result != tt.expected {
				t.Errorf("evaluateModelOp() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRouter_EvaluateTokenOps(t *testing.T) {
	router, _ := NewRouter([]SmartRouting{})

	tests := []struct {
		name     string
		ctx      *RequestContext
		op       SmartOp
		expected bool
	}{
		{
			name: "ge - matches",
			ctx: &RequestContext{
				EstimatedTokens: 6000,
			},
			op: SmartOp{
				Position:  PositionToken,
				Operation: "ge",
				Value:     "6000",
			},
			expected: true,
		},
		{
			name: "ge - no match",
			ctx: &RequestContext{
				EstimatedTokens: 5000,
			},
			op: SmartOp{
				Position:  PositionToken,
				Operation: "ge",
				Value:     "6000",
			},
			expected: false,
		},
		{
			name: "gt - matches",
			ctx: &RequestContext{
				EstimatedTokens: 6001,
			},
			op: SmartOp{
				Position:  PositionToken,
				Operation: "gt",
				Value:     "6000",
			},
			expected: true,
		},
		{
			name: "le - matches",
			ctx: &RequestContext{
				EstimatedTokens: 5000,
			},
			op: SmartOp{
				Position:  PositionToken,
				Operation: "le",
				Value:     "6000",
			},
			expected: true,
		},
		{
			name: "lt - matches",
			ctx: &RequestContext{
				EstimatedTokens: 5999,
			},
			op: SmartOp{
				Position:  PositionToken,
				Operation: "lt",
				Value:     "6000",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := router.evaluateTokenOp(tt.ctx, &tt.op)
			if result != tt.expected {
				t.Errorf("evaluateTokenOp() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty string", "", 0},
		{"short text", "hello", 1},                         // 5 chars / 4 = 1
		{"medium text", "hello world this is a test", 6},   // 25 chars / 4 = 6
		{"longer text", strings.Repeat("word ", 100), 125}, // 500 chars / 4 = 125
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EstimateTokens(tt.text); got != tt.want {
				t.Errorf("EstimateTokens() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestContext_GetLatestUserMessage(t *testing.T) {
	tests := []struct {
		name string
		ctx  *RequestContext
		want string
	}{
		{
			name: "single message",
			ctx: &RequestContext{
				UserMessages: []string{"first message"},
			},
			want: "first message",
		},
		{
			name: "multiple messages",
			ctx: &RequestContext{
				UserMessages: []string{"first", "second", "third"},
			},
			want: "third",
		},
		{
			name: "empty messages",
			ctx: &RequestContext{
				UserMessages: []string{},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.GetLatestUserMessage(); got != tt.want {
				t.Errorf("RequestContext.GetLatestUserMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestContext_CombineMessages(t *testing.T) {
	ctx := &RequestContext{}
	tests := []struct {
		name     string
		messages []string
		want     string
	}{
		{
			name:     "empty messages",
			messages: []string{},
			want:     "",
		},
		{
			name:     "single message",
			messages: []string{"hello"},
			want:     "hello",
		},
		{
			name:     "multiple messages",
			messages: []string{"first", "second", "third"},
			want:     "first\nsecond\nthird",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ctx.CombineMessages(tt.messages); got != tt.want {
				t.Errorf("RequestContext.CombineMessages() = %v, want %v", got, tt.want)
			}
		})
	}
}
