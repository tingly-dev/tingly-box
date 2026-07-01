package transform

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

func TestMaxTokensTransform_ApplyAnthropicV1(t *testing.T) {
	tests := []struct {
		name             string
		transform        *MaxTokensTransform
		initialMaxTokens int64
		wantMaxTokens    int64
	}{
		{
			name:             "Zero max_tokens filled with default",
			transform:        NewMaxTokensTransform(4096, 8192),
			initialMaxTokens: 0,
			wantMaxTokens:    4096,
		},
		{
			name:             "Max tokens capped at maxAllowed",
			transform:        NewMaxTokensTransform(4096, 8192),
			initialMaxTokens: 10000,
			wantMaxTokens:    8192,
		},
		{
			name:             "Valid max tokens unchanged",
			transform:        NewMaxTokensTransform(4096, 8192),
			initialMaxTokens: 5000,
			wantMaxTokens:    5000,
		},
		{
			name:             "No default set - zero stays zero",
			transform:        NewMaxTokensTransform(0, 8192),
			initialMaxTokens: 0,
			wantMaxTokens:    0,
		},
		{
			name:             "MaxAllowed=0 means no cap - value preserved",
			transform:        NewMaxTokensTransform(4096, 0),
			initialMaxTokens: 8000,
			wantMaxTokens:    8000,
		},
		{
			name:             "Both zero - zero stays zero",
			transform:        NewMaxTokensTransform(0, 0),
			initialMaxTokens: 0,
			wantMaxTokens:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &anthropic.MessageNewParams{
				MaxTokens: tt.initialMaxTokens,
			}

			ctx := &protocoltransform.TransformContext{Request: req}
			if err := tt.transform.Apply(ctx); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			if req.MaxTokens != tt.wantMaxTokens {
				t.Errorf("MaxTokens = %v, want %v", req.MaxTokens, tt.wantMaxTokens)
			}
		})
	}
}

func TestMaxTokensTransform_ApplyAnthropicBeta(t *testing.T) {
	tests := []struct {
		name             string
		transform        *MaxTokensTransform
		initialMaxTokens int64
		wantMaxTokens    int64
	}{
		{
			name:             "Zero max_tokens filled with default",
			transform:        NewMaxTokensTransform(4096, 8192),
			initialMaxTokens: 0,
			wantMaxTokens:    4096,
		},
		{
			name:             "Max tokens capped at maxAllowed",
			transform:        NewMaxTokensTransform(4096, 8192),
			initialMaxTokens: 10000,
			wantMaxTokens:    8192,
		},
		{
			name:             "No default set - zero stays zero",
			transform:        NewMaxTokensTransform(0, 8192),
			initialMaxTokens: 0,
			wantMaxTokens:    0,
		},
		{
			name:             "MaxAllowed=0 means no cap - value preserved",
			transform:        NewMaxTokensTransform(4096, 0),
			initialMaxTokens: 8000,
			wantMaxTokens:    8000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &anthropic.BetaMessageNewParams{
				MaxTokens: tt.initialMaxTokens,
			}

			ctx := &protocoltransform.TransformContext{Request: req}
			if err := tt.transform.Apply(ctx); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			if req.MaxTokens != tt.wantMaxTokens {
				t.Errorf("MaxTokens = %v, want %v", req.MaxTokens, tt.wantMaxTokens)
			}
		})
	}
}

func TestMaxTokensTransform_ApplyOpenAIChat(t *testing.T) {
	tests := []struct {
		name                    string
		transform               *MaxTokensTransform
		initMaxTokens           param.Opt[int64] // absent = zero value
		initMaxCompletionTokens param.Opt[int64]
		wantMaxTokens           param.Opt[int64]
		wantMaxCompletionTokens param.Opt[int64]
	}{
		{
			name:          "Both absent: max_tokens filled with default",
			transform:     NewMaxTokensTransform(4096, 8192),
			wantMaxTokens: param.NewOpt[int64](4096),
		},
		{
			name:          "max_tokens present: capped at maxAllowed",
			transform:     NewMaxTokensTransform(4096, 8192),
			initMaxTokens: param.NewOpt[int64](10000),
			wantMaxTokens: param.NewOpt[int64](8192),
		},
		{
			name:          "max_tokens present and within limit: unchanged",
			transform:     NewMaxTokensTransform(4096, 8192),
			initMaxTokens: param.NewOpt[int64](5000),
			wantMaxTokens: param.NewOpt[int64](5000),
		},
		{
			name:                    "max_completion_tokens present: max_tokens NOT auto-filled",
			transform:               NewMaxTokensTransform(4096, 8192),
			initMaxCompletionTokens: param.NewOpt[int64](6000),
			wantMaxTokens:           param.Opt[int64]{}, // absent
			wantMaxCompletionTokens: param.NewOpt[int64](6000),
		},
		{
			name:                    "max_completion_tokens over limit: capped",
			transform:               NewMaxTokensTransform(4096, 8192),
			initMaxCompletionTokens: param.NewOpt[int64](20000),
			wantMaxTokens:           param.Opt[int64]{}, // absent
			wantMaxCompletionTokens: param.NewOpt[int64](8192),
		},
		{
			name:          "MaxAllowed=0: no cap applied",
			transform:     NewMaxTokensTransform(4096, 0),
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
			ctx := &protocoltransform.TransformContext{Request: req}
			if err := tt.transform.Apply(ctx); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if req.MaxTokens != tt.wantMaxTokens {
				t.Errorf("MaxTokens = %v, want %v", req.MaxTokens, tt.wantMaxTokens)
			}
			if req.MaxCompletionTokens != tt.wantMaxCompletionTokens {
				t.Errorf("MaxCompletionTokens = %v, want %v", req.MaxCompletionTokens, tt.wantMaxCompletionTokens)
			}
		})
	}
}

func TestMaxTokensTransform_ApplyOpenAIResponses(t *testing.T) {
	tests := []struct {
		name                string
		transform           *MaxTokensTransform
		initMaxOutputTokens param.Opt[int64]
		wantMaxOutputTokens param.Opt[int64]
	}{
		{
			name:                "Absent: filled with default",
			transform:           NewMaxTokensTransform(4096, 8192),
			wantMaxOutputTokens: param.NewOpt[int64](4096),
		},
		{
			name:                "Over limit: capped",
			transform:           NewMaxTokensTransform(4096, 8192),
			initMaxOutputTokens: param.NewOpt[int64](10000),
			wantMaxOutputTokens: param.NewOpt[int64](8192),
		},
		{
			name:                "Within limit: unchanged",
			transform:           NewMaxTokensTransform(4096, 8192),
			initMaxOutputTokens: param.NewOpt[int64](5000),
			wantMaxOutputTokens: param.NewOpt[int64](5000),
		},
		{
			name:                "MaxAllowed=0: no cap applied",
			transform:           NewMaxTokensTransform(4096, 0),
			initMaxOutputTokens: param.NewOpt[int64](50000),
			wantMaxOutputTokens: param.NewOpt[int64](50000),
		},
		{
			name:                "No default, absent: stays absent",
			transform:           NewMaxTokensTransform(0, 8192),
			wantMaxOutputTokens: param.Opt[int64]{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &responses.ResponseNewParams{
				MaxOutputTokens: tt.initMaxOutputTokens,
			}
			ctx := &protocoltransform.TransformContext{Request: req}
			if err := tt.transform.Apply(ctx); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if req.MaxOutputTokens != tt.wantMaxOutputTokens {
				t.Errorf("MaxOutputTokens = %v, want %v", req.MaxOutputTokens, tt.wantMaxOutputTokens)
			}
		})
	}
}

func TestMaxTokensTransform_ApplyUnsupportedProtocol(t *testing.T) { // Test that unsupported protocols don't crash
	transform := NewMaxTokensTransform(4096, 8192)
	ctx := &protocoltransform.TransformContext{Request: "some unsupported type"}

	if err := transform.Apply(ctx); err != nil {
		t.Fatalf("Apply() should not error for unsupported protocols, got: %v", err)
	}
}

func TestMaxTokensTransform_Name(t *testing.T) {
	transform := NewMaxTokensTransform(4096, 8192)
	if transform.Name() != "max_tokens" {
		t.Errorf("Name() = %v, want %v", transform.Name(), "max_tokens")
	}
}

func TestMaxTokensTransform_NewMaxTokensTransform(t *testing.T) {
	transform := NewMaxTokensTransform(4096, 8192)

	if transform.DefaultMaxTokens != 4096 {
		t.Errorf("DefaultMaxTokens = %v, want %v", transform.DefaultMaxTokens, 4096)
	}

	if transform.MaxAllowed != 8192 {
		t.Errorf("MaxAllowed = %v, want %v", transform.MaxAllowed, 8192)
	}
}
