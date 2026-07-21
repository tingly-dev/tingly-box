package client

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func hasBeta(betas []anthropic.AnthropicBeta, want anthropic.AnthropicBeta) int {
	n := 0
	for _, b := range betas {
		if b == want {
			n++
		}
	}
	return n
}

func TestWithContext1MBeta(t *testing.T) {
	const c1m = anthropic.AnthropicBetaContext1m2025_08_07

	t.Run("no hint is a no-op", func(t *testing.T) {
		got := withContext1MBeta(context.Background(), nil)
		if got != nil {
			t.Errorf("got %v, want nil (no hint)", got)
		}
	})

	t.Run("hint appends to empty", func(t *testing.T) {
		ctx := typ.WithContext1M(context.Background())
		got := withContext1MBeta(ctx, nil)
		if hasBeta(got, c1m) != 1 {
			t.Errorf("context-1m not appended once: %v", got)
		}
	})

	t.Run("hint dedupes when already present", func(t *testing.T) {
		ctx := typ.WithContext1M(context.Background())
		got := withContext1MBeta(ctx, []anthropic.AnthropicBeta{c1m})
		if hasBeta(got, c1m) != 1 {
			t.Errorf("context-1m duplicated: %v", got)
		}
	})

	t.Run("hint preserves other betas and appends", func(t *testing.T) {
		other := anthropic.AnthropicBetaPromptCaching2024_07_31
		ctx := typ.WithContext1M(context.Background())
		got := withContext1MBeta(ctx, []anthropic.AnthropicBeta{other})
		if hasBeta(got, other) != 1 || hasBeta(got, c1m) != 1 {
			t.Errorf("expected both betas once: %v", got)
		}
	})
}

func TestContext1MHeaderOpts(t *testing.T) {
	if opts := context1MHeaderOpts(context.Background()); opts != nil {
		t.Errorf("no hint should yield nil opts, got %d", len(opts))
	}
	ctx := typ.WithContext1M(context.Background())
	if opts := context1MHeaderOpts(ctx); len(opts) != 1 {
		t.Errorf("hint should yield exactly one header opt, got %d", len(opts))
	}
}
