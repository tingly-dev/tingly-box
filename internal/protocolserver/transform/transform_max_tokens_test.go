package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"google.golang.org/genai"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

func apply(t *testing.T, tr protocoltransform.Transform, req any) {
	t.Helper()
	if err := tr.Apply(&protocoltransform.TransformContext{Request: req}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

// TestMaxTokensDefaultTransform pins the source-side fill: only an Anthropic
// request's missing max_tokens (a required field of that protocol) is filled.
func TestMaxTokensDefaultTransform(t *testing.T) {
	tr := NewMaxTokensDefaultTransform(4096)

	v1 := &anthropic.MessageNewParams{}
	beta := &anthropic.BetaMessageNewParams{}
	apply(t, tr, v1)
	apply(t, tr, beta)
	if v1.MaxTokens != 4096 || beta.MaxTokens != 4096 {
		t.Errorf("Anthropic MaxTokens = %d / %d, want 4096", v1.MaxTokens, beta.MaxTokens)
	}

	set := &anthropic.BetaMessageNewParams{MaxTokens: 100000}
	apply(t, tr, set)
	if set.MaxTokens != 100000 {
		t.Errorf("client max_tokens = %d, want it untouched (capping is the target side's job)", set.MaxTokens)
	}

	// Chat and Responses have no required limit: a client that omits it keeps
	// the provider's own default.
	chat := &openai.ChatCompletionNewParams{}
	resp := &responses.ResponseNewParams{}
	apply(t, tr, chat)
	apply(t, tr, resp)
	if chat.MaxTokens.Valid() || chat.MaxCompletionTokens.Valid() || resp.MaxOutputTokens.Valid() {
		t.Errorf("OpenAI shapes must stay unset: chat=%v/%v responses=%v", chat.MaxTokens, chat.MaxCompletionTokens, resp.MaxOutputTokens)
	}

	if tr.Name() != "max_tokens_default" {
		t.Errorf("Name() = %q", tr.Name())
	}
}

func TestOutputLimitTransform_Anthropic(t *testing.T) {
	tests := []struct {
		name          string
		maxAllowed    int
		maxTokens     int64
		wantMaxTokens int64
	}{
		{"over limit capped", 8192, 10000, 8192},
		{"within limit unchanged", 8192, 5000, 5000},
		{"MaxAllowed=0 disables", 0, 50000, 50000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := NewOutputLimitTransform(tt.maxAllowed)
			v1 := &anthropic.MessageNewParams{MaxTokens: tt.maxTokens}
			beta := &anthropic.BetaMessageNewParams{MaxTokens: tt.maxTokens}
			apply(t, tr, v1)
			apply(t, tr, beta)
			if v1.MaxTokens != tt.wantMaxTokens || beta.MaxTokens != tt.wantMaxTokens {
				t.Errorf("MaxTokens = %d / %d, want %d", v1.MaxTokens, beta.MaxTokens, tt.wantMaxTokens)
			}
		})
	}
}

// TestOutputLimitTransform_ThinkingBudget pins the budget rules. They only
// exist on the Anthropic shape: the transform runs on the upstream-bound
// request, so a budget is only ever shrunk for an Anthropic provider (#1897).
func TestOutputLimitTransform_ThinkingBudget(t *testing.T) {
	tests := []struct {
		name       string
		maxTokens  int64
		budget     int64
		wantBudget int64 // 0 = thinking turned off
	}{
		{name: "budget over maxAllowed shrinks", maxTokens: 40000, budget: 10240, wantBudget: 1024},
		{name: "budget within limits unchanged", maxTokens: 40000, budget: 4096, wantBudget: 4096},
		{name: "budget over max_tokens capped below it", maxTokens: 2048, budget: 4096, wantBudget: 2047},
		{name: "budget equal to max_tokens capped below it", maxTokens: 4096, budget: 4096, wantBudget: 4095},
		{name: "max_tokens 1024 leaves no room after shrinking: thinking off", maxTokens: 1024, budget: 10240, wantBudget: 0},
		{name: "max_tokens leaves no room: thinking off", maxTokens: 512, budget: 4096, wantBudget: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := NewOutputLimitTransform(8192)
			v1 := &anthropic.MessageNewParams{MaxTokens: tt.maxTokens, Thinking: anthropic.ThinkingConfigParamOfEnabled(tt.budget)}
			beta := &anthropic.BetaMessageNewParams{MaxTokens: tt.maxTokens, Thinking: anthropic.BetaThinkingConfigParamOfEnabled(tt.budget)}
			beta.Thinking.OfEnabled.Display = anthropic.BetaThinkingConfigEnabledDisplaySummarized
			apply(t, tr, v1)
			apply(t, tr, beta)
			if tt.wantBudget == 0 {
				if v1.Thinking.OfDisabled == nil || beta.Thinking.OfDisabled == nil {
					t.Errorf("thinking not turned off: V1=%+v Beta=%+v", v1.Thinking, beta.Thinking)
				}
				return
			}
			if got := *v1.Thinking.GetBudgetTokens(); got != tt.wantBudget {
				t.Errorf("V1 budget = %d, want %d", got, tt.wantBudget)
			}
			if got := *beta.Thinking.GetBudgetTokens(); got != tt.wantBudget {
				t.Errorf("Beta budget = %d, want %d", got, tt.wantBudget)
			}
			if beta.Thinking.OfEnabled.Display != anthropic.BetaThinkingConfigEnabledDisplaySummarized {
				t.Errorf("Beta display = %q, want it kept", beta.Thinking.OfEnabled.Display)
			}
		})
	}
}

func TestOutputLimitTransform_OpenAIChat(t *testing.T) {
	tests := []struct {
		name                    string
		maxAllowed              int
		initMaxTokens           param.Opt[int64] // absent = zero value
		initMaxCompletionTokens param.Opt[int64]
		wantMaxTokens           param.Opt[int64]
		wantMaxCompletionTokens param.Opt[int64]
	}{
		{
			name:       "both absent: stay absent",
			maxAllowed: 8192,
		},
		{
			name:          "max_tokens over limit: capped",
			maxAllowed:    8192,
			initMaxTokens: param.NewOpt[int64](10000),
			wantMaxTokens: param.NewOpt[int64](8192),
		},
		{
			name:          "max_tokens within limit: unchanged",
			maxAllowed:    8192,
			initMaxTokens: param.NewOpt[int64](5000),
			wantMaxTokens: param.NewOpt[int64](5000),
		},
		{
			name:                    "max_completion_tokens over limit: capped",
			maxAllowed:              8192,
			initMaxCompletionTokens: param.NewOpt[int64](20000),
			wantMaxCompletionTokens: param.NewOpt[int64](8192),
		},
		{
			name:          "MaxAllowed=0: no cap applied",
			maxAllowed:    0,
			initMaxTokens: param.NewOpt[int64](50000),
			wantMaxTokens: param.NewOpt[int64](50000),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &openai.ChatCompletionNewParams{
				MaxTokens:           tt.initMaxTokens,
				MaxCompletionTokens: tt.initMaxCompletionTokens,
				Model:               openai.ChatModelGPT4o,
			}
			apply(t, NewOutputLimitTransform(tt.maxAllowed), req)
			if req.MaxTokens != tt.wantMaxTokens {
				t.Errorf("MaxTokens = %v, want %v", req.MaxTokens, tt.wantMaxTokens)
			}
			if req.MaxCompletionTokens != tt.wantMaxCompletionTokens {
				t.Errorf("MaxCompletionTokens = %v, want %v", req.MaxCompletionTokens, tt.wantMaxCompletionTokens)
			}
		})
	}
}

func TestOutputLimitTransform_OpenAIResponses(t *testing.T) {
	tests := []struct {
		name                string
		maxAllowed          int
		initMaxOutputTokens param.Opt[int64]
		wantMaxOutputTokens param.Opt[int64]
	}{
		{name: "absent: stays absent", maxAllowed: 8192},
		{name: "over limit: capped", maxAllowed: 8192, initMaxOutputTokens: param.NewOpt[int64](10000), wantMaxOutputTokens: param.NewOpt[int64](8192)},
		{name: "within limit: unchanged", maxAllowed: 8192, initMaxOutputTokens: param.NewOpt[int64](5000), wantMaxOutputTokens: param.NewOpt[int64](5000)},
		{name: "MaxAllowed=0: no cap applied", maxAllowed: 0, initMaxOutputTokens: param.NewOpt[int64](50000), wantMaxOutputTokens: param.NewOpt[int64](50000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &responses.ResponseNewParams{MaxOutputTokens: tt.initMaxOutputTokens}
			apply(t, NewOutputLimitTransform(tt.maxAllowed), req)
			if req.MaxOutputTokens != tt.wantMaxOutputTokens {
				t.Errorf("MaxOutputTokens = %v, want %v", req.MaxOutputTokens, tt.wantMaxOutputTokens)
			}
		})
	}
}

func TestOutputLimitTransform_Google(t *testing.T) {
	req := &protocol.GoogleRequest{Config: &genai.GenerateContentConfig{MaxOutputTokens: 10000}}
	apply(t, NewOutputLimitTransform(8192), req)
	if req.Config.MaxOutputTokens != 8192 {
		t.Errorf("MaxOutputTokens = %d, want 8192", req.Config.MaxOutputTokens)
	}
	apply(t, NewOutputLimitTransform(8192), &protocol.GoogleRequest{}) // nil config must not panic
}

func TestOutputLimitTransform_UnsupportedShape(t *testing.T) {
	apply(t, NewOutputLimitTransform(8192), "some unsupported type")
	if name := NewOutputLimitTransform(8192).Name(); name != "output_limit" {
		t.Errorf("Name() = %q", name)
	}
}
